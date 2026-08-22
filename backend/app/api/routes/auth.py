from __future__ import annotations

import hashlib
import secrets
from datetime import datetime, timedelta, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Request
from sqlalchemy import delete, select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db
from app.core.config import settings
from app.core.exceptions import AppError
from app.core.logging import get_logger
from app.core.rate_limit import enforce_limit
from app.core.security import (
    create_access_token,
    create_refresh_token_value,
    hash_password,
    verify_password,
)
from app.models import PasswordResetToken, RefreshToken, User
from app.schemas import (
    ForgotPasswordIn,
    GoogleIn,
    LoginIn,
    RefreshIn,
    RegisterIn,
    ResetPasswordIn,
    TokenPair,
    UserOut,
    UserUpdateIn,
    VerifyResetIn,
)
from app.services.email import send_password_reset_email

router = APIRouter(prefix="/auth", tags=["auth"])
logger = get_logger(__name__)


def _aware(dt: datetime) -> datetime:
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt


def _hash_token(raw: str) -> str:
    return hashlib.sha256(raw.encode()).hexdigest()


async def _issue_tokens(session: AsyncSession, user: User) -> TokenPair:
    access = create_access_token(str(user.id))
    raw_refresh = create_refresh_token_value()
    session.add(
        RefreshToken(
            user_id=user.id,
            token_hash=_hash_token(raw_refresh),
            expires_at=datetime.now(timezone.utc)
            + timedelta(days=settings.refresh_token_expire_days),
            revoked=False,
        )
    )
    await session.commit()
    return TokenPair(access_token=access, refresh_token=raw_refresh)


@router.post("/register", response_model=TokenPair)
async def register(request: Request, body: RegisterIn, session: AsyncSession = Depends(get_db)):
    enforce_limit(request, "register", 8, 15 * 60)
    existing = await session.execute(select(User).where(User.email == body.email.lower()))
    if existing.scalar_one_or_none():
        raise AppError(409, "email_taken", "An account with this email already exists.")
    user = User(
        email=body.email.lower(),
        password_hash=hash_password(body.password),
        name=body.name or body.email.split("@")[0],
        timezone=body.timezone or "UTC",
    )
    session.add(user)
    await session.flush()
    tokens = await _issue_tokens(session, user)
    return tokens


@router.post("/login", response_model=TokenPair)
async def login(request: Request, body: LoginIn, session: AsyncSession = Depends(get_db)):
    enforce_limit(request, "login", 5, 15 * 60)
    result = await session.execute(select(User).where(User.email == body.email.lower()))
    user = result.scalar_one_or_none()
    if user is None or not user.password_hash or not verify_password(body.password, user.password_hash):
        logger.info("Auth failure: invalid login for email=%s", body.email.lower())
        raise AppError(401, "invalid_credentials", "Invalid email or password.")
    if not user.is_active:
        raise AppError(401, "inactive", "Account is inactive.")
    return await _issue_tokens(session, user)


@router.post("/refresh", response_model=TokenPair)
async def refresh(body: RefreshIn, session: AsyncSession = Depends(get_db)):
    token_hash = _hash_token(body.refresh_token)
    result = await session.execute(select(RefreshToken).where(RefreshToken.token_hash == token_hash))
    row = result.scalar_one_or_none()
    now = datetime.now(timezone.utc)
    if row is None:
        raise AppError(401, "invalid_refresh", "Refresh token is invalid or expired.")
    user = await session.get(User, row.user_id)
    if user is None or not user.is_active:
        raise AppError(401, "unauthenticated", "User not found or inactive.")
    if _aware(row.expires_at) < now:
        raise AppError(401, "invalid_refresh", "Refresh token is invalid or expired.")
    if row.revoked:
        # Concurrent 401 retries often reuse the previous refresh token after rotation.
        latest = (
            await session.execute(
                select(RefreshToken)
                .where(RefreshToken.user_id == user.id, RefreshToken.revoked.is_(False))
                .order_by(RefreshToken.created_at.desc())
            )
        ).scalars().first()
        if latest is None or (now - _aware(latest.created_at)).total_seconds() > 20:
            raise AppError(401, "invalid_refresh", "Refresh token is invalid or expired.")
        await session.execute(
            update(RefreshToken).where(RefreshToken.user_id == user.id).values(revoked=True)
        )
        return await _issue_tokens(session, user)
    row.revoked = True
    return await _issue_tokens(session, user)


@router.post("/logout")
async def logout(body: RefreshIn, session: AsyncSession = Depends(get_db)):
    token_hash = _hash_token(body.refresh_token)
    result = await session.execute(select(RefreshToken).where(RefreshToken.token_hash == token_hash))
    row = result.scalar_one_or_none()
    if row:
        await session.execute(
            update(RefreshToken).where(RefreshToken.user_id == row.user_id).values(revoked=True)
        )
        await session.commit()
    return {"ok": True}


@router.post("/google", response_model=TokenPair)
async def google_login(request: Request, body: GoogleIn, session: AsyncSession = Depends(get_db)):
    enforce_limit(request, "google", 8, 15 * 60)
    email, google_id, name = _verify_google(body.id_token)
    result = await session.execute(select(User).where(User.email == email.lower()))
    user = result.scalar_one_or_none()
    if user:
        if not user.google_id:
            user.google_id = google_id
        elif user.google_id != google_id:
            raise AppError(409, "google_mismatch", "This email is linked to a different Google account.")
    else:
        user = User(
            email=email.lower(),
            password_hash=None,
            google_id=google_id,
            name=name or email.split("@")[0],
            timezone=body.timezone or "UTC",
        )
        session.add(user)
        await session.flush()
    return await _issue_tokens(session, user)


def _verify_google(id_token: str) -> tuple[str, str, str]:
    if settings.environment == "test":
        # Tests use a deterministic fake token: test.<email>
        if not id_token.startswith("test.") or "." not in id_token[5:]:
            raise AppError(401, "invalid_google_token", "Google token could not be verified.")
        email = id_token.split(".", 1)[1]
        return email, f"google-{email}", email.split("@")[0]
    if not settings.google_client_id:
        raise AppError(503, "google_unconfigured", "Google Sign-In is not configured.")
    from google.oauth2 import id_token as google_id_token
    from google.auth.transport import requests as google_requests

    try:
        info = google_id_token.verify_oauth2_token(
            id_token, google_requests.Request(), settings.google_client_id
        )
    except Exception:
        logger.info("Auth failure: invalid Google ID token")
        raise AppError(401, "invalid_google_token", "Google token could not be verified.")
    return info["email"], info["sub"], info.get("name") or ""


async def _unused_reset_row(session: AsyncSession, user: User, code: str) -> PasswordResetToken:
    now = datetime.now(timezone.utc)
    tokens = await session.execute(
        select(PasswordResetToken)
        .where(
            PasswordResetToken.user_id == user.id,
            PasswordResetToken.code_hash == _hash_token(code),
            PasswordResetToken.used.is_(False),
        )
        .order_by(PasswordResetToken.created_at.desc())
    )
    row = tokens.scalars().first()
    if row is None or _aware(row.expires_at) < now:
        raise AppError(400, "invalid_reset", "Invalid or expired reset code.")
    return row


@router.post("/forgot-password")
async def forgot_password(request: Request, body: ForgotPasswordIn, session: AsyncSession = Depends(get_db)):
    enforce_limit(request, "forgot", 5, 15 * 60)
    result = await session.execute(select(User).where(User.email == body.email.lower()))
    user = result.scalar_one_or_none()
    if user is None:
        raise AppError(404, "email_not_registered", "This email is not registered. Please register an account first.")
    if not user.password_hash:
        raise AppError(400, "password_unavailable", "This account does not have a password. Sign in with Google instead.")
    code = f"{secrets.randbelow(1000000):06d}"
    session.add(
        PasswordResetToken(
            user_id=user.id,
            code_hash=_hash_token(code),
            expires_at=datetime.now(timezone.utc) + timedelta(minutes=15),
            used=False,
        )
    )
    await session.commit()
    send_password_reset_email(user.email, code)
    payload: dict = {"ok": True, "message": "Reset code sent to your email."}
    if settings.environment == "test":
        payload["dev_code"] = code
    return payload


@router.post("/verify-reset-code")
async def verify_reset_code(request: Request, body: VerifyResetIn, session: AsyncSession = Depends(get_db)):
    enforce_limit(request, "verify-reset", 8, 15 * 60)
    result = await session.execute(select(User).where(User.email == body.email.lower()))
    user = result.scalar_one_or_none()
    if user is None:
        raise AppError(400, "invalid_reset", "Invalid or expired reset code.")
    await _unused_reset_row(session, user, body.code)
    return {"ok": True}


@router.post("/reset-password")
async def reset_password(request: Request, body: ResetPasswordIn, session: AsyncSession = Depends(get_db)):
    enforce_limit(request, "reset", 8, 15 * 60)
    result = await session.execute(select(User).where(User.email == body.email.lower()))
    user = result.scalar_one_or_none()
    if user is None:
        raise AppError(400, "invalid_reset", "Invalid or expired reset code.")
    row = await _unused_reset_row(session, user, body.code)
    row.used = True
    user.password_hash = hash_password(body.new_password)
    await session.execute(update(RefreshToken).where(RefreshToken.user_id == user.id).values(revoked=True))
    await session.commit()
    return {"ok": True}


@router.get("/me", response_model=UserOut)
async def me(user: User = Depends(get_current_user)):
    return user


@router.patch("/me", response_model=UserOut)
async def update_me(
    body: UserUpdateIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    if body.name is not None:
        user.name = body.name
    if body.timezone is not None:
        user.timezone = body.timezone
    await session.commit()
    await session.refresh(user)
    return user


@router.delete("/me")
async def delete_account(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    uid: UUID = user.id
    # SQL DELETE so SQLite/Postgres ON DELETE CASCADE removes skills, notes, tokens, etc.
    # ORM session.delete() would try to NULL skills.user_id and hit NOT NULL.
    await session.execute(delete(User).where(User.id == uid))
    await session.commit()
    return {"ok": True}

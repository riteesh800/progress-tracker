from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import Depends, Header, Request
from sqlalchemy import delete, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.admin import is_admin_email
from app.core.exceptions import AppError
from app.core.security import decode_access_token
from app.db.session import get_session
from app.models import Skill, Topic, User
from app.schemas import UserOut
import jwt


async def get_db(session: AsyncSession = Depends(get_session)) -> AsyncSession:
    return session


_PASSWORD_CHANGE_ALLOWED = {("GET", "/auth/me"), ("POST", "/auth/change-password")}

# Paths that should NOT count as meaningful user activity
# (background polling, automated refetches, health checks)
_NON_ACTIVITY_PATHS = {
    "/health",
    "/auth/refresh",
    "/auth/logout",
}


def _is_meaningful_activity(request: Request) -> bool:
    """Determine if this request represents meaningful user activity.

    Only explicit user-initiated actions count. Background polling,
    automatic React Query refetches, and token refresh do not count.
    """
    path = (request.url.path or "").rstrip("/")

    # Never count certain endpoints
    if path in _NON_ACTIVITY_PATHS:
        return False

    # GET requests to data endpoints are typically background refetches
    # Only non-GET requests (POST, PATCH, DELETE, PUT) count as meaningful
    if request.method == "GET":
        return False

    return True


async def get_current_user(
    request: Request,
    authorization: str | None = Header(default=None),
    session: AsyncSession = Depends(get_db),
) -> User:
    if not authorization or not authorization.lower().startswith("bearer "):
        raise AppError(401, "unauthenticated", "Missing or invalid authorization header.")
    token = authorization.split(" ", 1)[1]
    try:
        payload = decode_access_token(token)
    except jwt.ExpiredSignatureError:
        raise AppError(401, "token_expired", "Access token expired.")
    except jwt.InvalidTokenError:
        raise AppError(401, "invalid_token", "Invalid access token.")
    if payload.get("type") != "access":
        raise AppError(401, "invalid_token", "Invalid access token.")
    user_id = payload.get("sub")
    try:
        uid = UUID(str(user_id))
    except (TypeError, ValueError):
        raise AppError(401, "invalid_token", "Invalid access token.")
    result = await session.execute(select(User).where(User.id == uid))
    user = result.scalar_one_or_none()
    if user is None or not user.is_active:
        raise AppError(401, "unauthenticated", "User not found or inactive.")

    # Check if deletion deadline has expired
    if user.deletion_requested_at is not None and user.deletion_deadline is not None:
        now = datetime.now(timezone.utc)
        deadline = user.deletion_deadline
        if deadline.tzinfo is None:
            deadline = deadline.replace(tzinfo=timezone.utc)
        if now >= deadline:
            # Permanently delete the expired account
            uid = user.id
            await session.execute(delete(User).where(User.id == uid))
            await session.commit()
            raise AppError(401, "unauthenticated", "User not found or inactive.")

    # Track meaningful user activity and cancel pending deletion if applicable
    if _is_meaningful_activity(request):
        if user.deletion_requested_at is not None:
            # Cancel pending deletion on meaningful authenticated activity
            user.deletion_requested_at = None
            user.deletion_deadline = None
        user.last_active_at = datetime.now(timezone.utc)

    if user.must_change_password and not _password_change_allowed(request):
        raise AppError(403, "password_change_required", "You must set a new password before continuing.")
    return user


def _password_change_allowed(request: Request) -> bool:
    path = (request.url.path or "").rstrip("/")
    return (request.method, path) in _PASSWORD_CHANGE_ALLOWED or (
        (request.method == "GET" and path.endswith("/auth/me"))
        or (request.method == "POST" and path.endswith("/auth/change-password"))
    )


def serialize_user(user: User) -> UserOut:
    return UserOut(
        id=user.id,
        email=user.email,
        name=user.name,
        timezone=user.timezone,
        is_active=user.is_active,
        can_open_admin=is_admin_email(user.email),
        must_change_password=user.must_change_password,
    )


async def require_admin_account(user: User = Depends(get_current_user)) -> User:
    if not is_admin_email(user.email):
        raise AppError(403, "forbidden", "Admin is not available for this account.")
    return user


async def get_verified_admin(
    x_admin_session: str | None = Header(default=None, alias="X-Admin-Session"),
    user: User = Depends(require_admin_account),
) -> User:
    if not x_admin_session:
        raise AppError(401, "admin_locked", "Database Access Code verification is required.")
    try:
        payload = decode_access_token(x_admin_session)
    except jwt.ExpiredSignatureError:
        raise AppError(401, "admin_locked", "Admin session expired. Enter the Database Access Code again.")
    except jwt.InvalidTokenError:
        raise AppError(401, "admin_locked", "Admin session is invalid.")
    if payload.get("type") != "admin" or str(payload.get("sub")) != str(user.id):
        raise AppError(401, "admin_locked", "Admin session is invalid.")
    return user


async def owned_skill(skill_id: UUID, user: User, session: AsyncSession, allow_deleted: bool = False) -> Skill:
    stmt = select(Skill).where(Skill.id == skill_id, Skill.user_id == user.id)
    if not allow_deleted:
        stmt = stmt.where(Skill.deleted_at.is_(None))
    result = await session.execute(stmt)
    skill = result.scalar_one_or_none()
    if skill is None:
        raise AppError(404, "not_found", "Skill not found.")
    return skill


async def owned_topic(topic_id: UUID, user: User, session: AsyncSession) -> Topic:
    result = await session.execute(
        select(Topic)
        .join(Skill, Skill.id == Topic.skill_id)
        .where(Topic.id == topic_id, Skill.user_id == user.id)
    )
    topic = result.scalar_one_or_none()
    if topic is None:
        raise AppError(404, "not_found", "Topic not found.")
    return topic

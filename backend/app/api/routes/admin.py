from __future__ import annotations

import hmac
from datetime import datetime, timedelta, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Header, Request
from sqlalchemy import select, update, delete
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_db, get_verified_admin, require_admin_account
from app.core.admin import admin_access_configured, verify_admin_access_code
from app.core.config import settings
from app.core.exceptions import AppError
from app.core.logging import get_logger
from app.core.rate_limit import enforce_limit
from app.core.security import create_access_token, hash_password
from app.models import RefreshToken, Skill, User
from app.schemas import AdminPasswordIn, AdminUnlockIn, AdminUnlockOut, AdminUserOut

router = APIRouter(prefix="/admin", tags=["admin"])
logger = get_logger(__name__)

ACTIVE_THRESHOLD_DAYS = 7


def _compute_status(user: User) -> str:
    """Compute the display status for the admin user list.

    Returns one of: 'Active', 'Not active', 'Deleted'.
    """
    # Pending deletion overrides normal status
    if user.deletion_requested_at is not None:
        return "Deleted"

    now = datetime.now(timezone.utc)
    threshold = now - timedelta(days=ACTIVE_THRESHOLD_DAYS)

    # Check last_active_at (which is updated on login and meaningful activity)
    if user.last_active_at is not None:
        last_active = user.last_active_at
        if last_active.tzinfo is None:
            last_active = last_active.replace(tzinfo=timezone.utc)
        if last_active >= threshold:
            return "Active"

    # Check created_at as fallback for newly registered users
    created = user.created_at
    if created.tzinfo is None:
        created = created.replace(tzinfo=timezone.utc)
    if created >= threshold:
        return "Active"

    return "Not active"


@router.post("/unlock", response_model=AdminUnlockOut)
async def unlock_admin(
    request: Request,
    body: AdminUnlockIn,
    user: User = Depends(require_admin_account),
):
    enforce_limit(request, f"admin_unlock:{user.id}", 5, 15 * 60)
    if not admin_access_configured():
        raise AppError(503, "admin_unconfigured", "Admin access is not configured on the server.")
    if not verify_admin_access_code(body.access_code):
        raise AppError(403, "invalid_access_code", "Invalid Database Access Code.")
    token = create_access_token(
        str(user.id),
        extra={"type": "admin"},
        expire_minutes=settings.admin_session_expire_minutes,
    )
    return AdminUnlockOut(
        admin_token=token,
        expires_in=settings.admin_session_expire_minutes * 60,
    )


@router.get("/users", response_model=list[AdminUserOut])
async def list_users(
    _admin: User = Depends(get_verified_admin),
    session: AsyncSession = Depends(get_db),
):
    result = await session.execute(select(User).order_by(User.created_at.asc(), User.email.asc()))
    return [
        AdminUserOut(id=u.id, name=u.name, email=u.email, status=_compute_status(u))
        for u in result.scalars().all()
    ]


@router.post("/users/{user_id}/password")
async def admin_set_password(
    user_id: UUID,
    body: AdminPasswordIn,
    _admin: User = Depends(get_verified_admin),
    session: AsyncSession = Depends(get_db),
):
    target = await session.get(User, user_id)
    if target is None:
        raise AppError(404, "not_found", "User not found.")
    target.password_hash = hash_password(body.new_password)
    target.must_change_password = True
    # Intentionally do NOT cancel pending deletion or reset deadline
    await session.execute(update(RefreshToken).where(RefreshToken.user_id == target.id).values(revoked=True))
    await session.commit()
    return {"ok": True}


@router.post("/cleanup/expired-accounts")
async def cleanup_expired_accounts(
    _admin: User = Depends(get_verified_admin),
    session: AsyncSession = Depends(get_db),
):
    """Admin-triggered cleanup of expired pending-deletion accounts.
    Also called automatically by the background task.
    """
    count = await _run_cleanup(session)
    return {"ok": True, "deleted_count": count}


async def _run_cleanup(session: AsyncSession) -> int:
    """Find and permanently delete all accounts past their deletion deadline.

    Returns the number of accounts deleted.
    """
    now = datetime.now(timezone.utc)
    result = await session.execute(
        select(User).where(
            User.deletion_requested_at.is_not(None),
            User.deletion_deadline.is_not(None),
            User.deletion_deadline <= now,
        )
    )
    expired_users = result.scalars().all()
    count = 0
    for user in expired_users:
        uid = user.id
        email = user.email
        await session.execute(delete(User).where(User.id == uid))
        count += 1
        logger.info("Permanently deleted expired account: %s (id=%s)", email, uid)
    if count > 0:
        await session.commit()
    return count


async def _run_skill_cleanup(session: AsyncSession) -> int:
    """Permanently delete skills whose purge_at deadline has passed."""
    now = datetime.now(timezone.utc)
    result = await session.execute(
        select(Skill).where(Skill.purge_at <= now, Skill.deleted_at.is_not(None))
    )
    expired = result.scalars().all()
    for s in expired:
        await session.delete(s)
    if expired:
        await session.commit()
        logger.info("Purged %d expired skills from trash", len(expired))
    return len(expired)


def _verify_cleanup_secret(authorization: str | None) -> None:
    """Validate the Bearer token matches the CLEANUP_SECRET env var."""
    secret = settings.cleanup_secret or ""
    if len(secret) < 16:
        raise AppError(503, "cleanup_unconfigured", "Cleanup secret is not configured on the server.")
    if not authorization or not authorization.lower().startswith("bearer "):
        raise AppError(401, "unauthenticated", "Missing or invalid authorization header.")
    provided = authorization.split(" ", 1)[1]
    if not hmac.compare_digest(provided.encode("utf-8"), secret.encode("utf-8")):
        raise AppError(403, "forbidden", "Invalid cleanup secret.")


@router.post("/cron/cleanup")
async def cron_cleanup(
    authorization: str | None = Header(default=None),
    session: AsyncSession = Depends(get_db),
):
    """Endpoint for Render Cron Job to trigger all periodic cleanup tasks.

    Secured by CLEANUP_SECRET Bearer token (not admin JWT).
    Idempotent: safe to call multiple times or concurrently.
    """
    _verify_cleanup_secret(authorization)
    accounts_deleted = await _run_cleanup(session)
    skills_purged = await _run_skill_cleanup(session)
    return {
        "ok": True,
        "accounts_deleted": accounts_deleted,
        "skills_purged": skills_purged,
    }

from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, Request
from sqlalchemy import select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_db, get_verified_admin, require_admin_account
from app.core.admin import admin_access_configured, verify_admin_access_code
from app.core.config import settings
from app.core.exceptions import AppError
from app.core.rate_limit import enforce_limit
from app.core.security import create_access_token, hash_password
from app.models import RefreshToken, User
from app.schemas import AdminPasswordIn, AdminUnlockIn, AdminUnlockOut, AdminUserOut

router = APIRouter(prefix="/admin", tags=["admin"])


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
    return [AdminUserOut(id=u.id, name=u.name, email=u.email) for u in result.scalars().all()]


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
    await session.execute(update(RefreshToken).where(RefreshToken.user_id == target.id).values(revoked=True))
    await session.commit()
    return {"ok": True}

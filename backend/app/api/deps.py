from __future__ import annotations

from uuid import UUID

from fastapi import Depends, Header, Request
from sqlalchemy import select
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
    result = await session.execute(select(User).where(User.id == UUID(user_id)))
    user = result.scalar_one_or_none()
    if user is None or not user.is_active:
        raise AppError(401, "unauthenticated", "User not found or inactive.")
    if user.must_change_password and (request.method, request.url.path) not in _PASSWORD_CHANGE_ALLOWED:
        raise AppError(403, "password_change_required", "You must set a new password before continuing.")
    return user


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


async def owned_skill(skill_id: UUID, user: User, session: AsyncSession) -> Skill:
    result = await session.execute(
        select(Skill).where(Skill.id == skill_id, Skill.user_id == user.id)
    )
    skill = result.scalar_one_or_none()
    if skill is None:
        raise AppError(404, "not_found", "Skill not found.")
    return skill


async def owned_topic(topic_id: UUID, user: User, session: AsyncSession) -> Topic:
    result = await session.execute(select(Topic).where(Topic.id == topic_id))
    topic = result.scalar_one_or_none()
    if topic is None:
        raise AppError(404, "not_found", "Topic not found.")
    skill = await owned_skill(topic.skill_id, user, session)
    _ = skill
    return topic

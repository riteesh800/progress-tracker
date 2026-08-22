from __future__ import annotations

from uuid import UUID

from fastapi import Depends, Header
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.exceptions import AppError
from app.core.security import decode_access_token
from app.db.session import get_session
from app.models import Skill, Topic, User
import jwt


async def get_db(session: AsyncSession = Depends(get_session)) -> AsyncSession:
    return session


async def get_current_user(
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

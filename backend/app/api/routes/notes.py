from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db, owned_topic
from app.core.exceptions import AppError
from app.models import Note, Skill, User
from app.models.enums import ActionType
from app.schemas import NoteIn, NoteOut, NoteUpdateIn
from app.services.activity import make_activity

router = APIRouter(prefix="/notes", tags=["notes"])


@router.post("", response_model=NoteOut, status_code=201)
async def create_note(
    body: NoteIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    topic = await owned_topic(body.topic_id, user, session)
    if not body.content.strip():
        raise AppError(422, "empty_note", "Note content cannot be empty.")
    note = Note(topic_id=topic.id, user_id=user.id, content=body.content.strip())
    session.add(note)
    skill = await session.get(Skill, topic.skill_id)
    session.add(
        make_activity(
            user_id=user.id,
            action_type=ActionType.note_added,
            occurred_at=datetime.now(timezone.utc),
            timezone_name=user.timezone,
            topic_id=topic.id,
            skill_id=topic.skill_id,
            topic_name_snapshot=topic.name,
            skill_name_snapshot=skill.name if skill else None,
        )
    )
    await session.commit()
    await session.refresh(note)
    return note


@router.get("", response_model=list[NoteOut])
async def list_notes(
    topic_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    await owned_topic(topic_id, user, session)
    result = await session.execute(
        select(Note)
        .where(Note.topic_id == topic_id, Note.user_id == user.id)
        .order_by(Note.created_at.desc())
    )
    return list(result.scalars().all())


@router.get("/{note_id}", response_model=NoteOut)
async def get_note(
    note_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    note = await session.get(Note, note_id)
    if note is None or note.user_id != user.id:
        raise AppError(404, "not_found", "Note not found.")
    return note


@router.patch("/{note_id}", response_model=NoteOut)
async def patch_note(
    note_id: UUID,
    body: NoteUpdateIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    note = await session.get(Note, note_id)
    if note is None or note.user_id != user.id:
        raise AppError(404, "not_found", "Note not found.")
    note.content = body.content.strip()
    topic = await owned_topic(note.topic_id, user, session)
    skill = await session.get(Skill, topic.skill_id)
    session.add(
        make_activity(
            user_id=user.id,
            action_type=ActionType.note_updated,
            occurred_at=datetime.now(timezone.utc),
            timezone_name=user.timezone,
            topic_id=topic.id,
            skill_id=topic.skill_id,
            topic_name_snapshot=topic.name,
            skill_name_snapshot=skill.name if skill else None,
        )
    )
    await session.commit()
    await session.refresh(note)
    return note


@router.delete("/{note_id}")
async def delete_note(
    note_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    note = await session.get(Note, note_id)
    if note is None or note.user_id != user.id:
        raise AppError(404, "not_found", "Note not found.")
    await session.delete(note)
    await session.commit()
    return {"ok": True}

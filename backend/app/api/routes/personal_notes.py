from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends
from sqlalchemy import select
from sqlalchemy.orm import selectinload
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db
from app.core.exceptions import AppError
from app.models import PersonalNote, PersonalNoteTask, User
from app.schemas import PersonalNoteIn, PersonalNoteOut, PersonalNoteUpdateIn

router = APIRouter(prefix="/personal-notes", tags=["personal-notes"])


async def _owned_note(note_id: UUID, user: User, session: AsyncSession) -> PersonalNote:
    result = await session.execute(
        select(PersonalNote)
        .options(selectinload(PersonalNote.tasks))
        .where(PersonalNote.id == note_id, PersonalNote.user_id == user.id)
    )
    note = result.scalar_one_or_none()
    if note is None:
        raise AppError(404, "not_found", "Note not found.")
    return note


@router.get("", response_model=list[PersonalNoteOut])
async def list_personal_notes(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    result = await session.execute(
        select(PersonalNote)
        .options(selectinload(PersonalNote.tasks))
        .where(PersonalNote.user_id == user.id)
        .order_by(PersonalNote.updated_at.desc())
    )
    return list(result.scalars().all())


@router.post("", response_model=PersonalNoteOut, status_code=201)
async def create_personal_note(
    body: PersonalNoteIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    title = (body.title or "Untitled note").strip() or "Untitled note"
    note = PersonalNote(user_id=user.id, title=title)
    session.add(note)
    await session.flush()
    session.add(PersonalNoteTask(note_id=note.id, content="", is_completed=False, order_index=0))
    await session.commit()
    return await _owned_note(note.id, user, session)


@router.get("/{note_id}", response_model=PersonalNoteOut)
async def get_personal_note(
    note_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    return await _owned_note(note_id, user, session)


@router.patch("/{note_id}", response_model=PersonalNoteOut)
async def patch_personal_note(
    note_id: UUID,
    body: PersonalNoteUpdateIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    note = await _owned_note(note_id, user, session)
    if body.title is not None:
        note.title = body.title.strip() or "Untitled note"
    if body.tasks is not None:
        for task in list(note.tasks):
            await session.delete(task)
        await session.flush()
        for index, item in enumerate(body.tasks):
            session.add(
                PersonalNoteTask(
                    note_id=note.id,
                    content=item.content,
                    is_completed=item.is_completed,
                    order_index=index,
                )
            )
    note.updated_at = datetime.now(timezone.utc)
    await session.commit()
    return await _owned_note(note.id, user, session)


@router.delete("/{note_id}")
async def delete_personal_note(
    note_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    note = await _owned_note(note_id, user, session)
    await session.delete(note)
    await session.commit()
    return {"ok": True}

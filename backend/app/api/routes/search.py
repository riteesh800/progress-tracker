from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, Query
from sqlalchemy import or_, select, text
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db
from app.models import Note, Skill, Topic, User
from app.schemas import SearchHit

router = APIRouter(tags=["search"])


@router.get("/search", response_model=list[SearchHit])
async def search(
    q: str = Query(min_length=1),
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    bind = session.get_bind()
    dialect = bind.dialect.name if bind is not None else "sqlite"
    if dialect == "postgresql":
        return await _postgres_search(session, user.id, q)
    return await _sqlite_search(session, user.id, q)


async def _postgres_search(session: AsyncSession, user_id: UUID, q: str) -> list[SearchHit]:
    hits: list[SearchHit] = []
    skill_rows = await session.execute(
        text(
            """
            SELECT s.id, s.name, s.description
            FROM skills s
            WHERE s.user_id = :uid AND s.search_tsv @@ plainto_tsquery('english', :q)
            """
        ),
        {"uid": str(user_id), "q": q},
    )
    for row in skill_rows:
        hits.append(
            SearchHit(
                kind="skill",
                id=row.id,
                title=row.name,
                snippet=(row.description or "")[:180],
                skill_id=row.id,
            )
        )
    topic_rows = await session.execute(
        text(
            """
            SELECT t.id, t.name, t.description, t.skill_id
            FROM topics t
            JOIN skills s ON s.id = t.skill_id
            WHERE s.user_id = :uid AND t.search_tsv @@ plainto_tsquery('english', :q)
            """
        ),
        {"uid": str(user_id), "q": q},
    )
    for row in topic_rows:
        hits.append(
            SearchHit(
                kind="topic",
                id=row.id,
                title=row.name,
                snippet=(row.description or "")[:180],
                skill_id=row.skill_id,
                topic_id=row.id,
            )
        )
    note_rows = await session.execute(
        text(
            """
            SELECT n.id, n.content, n.topic_id, t.skill_id, t.name AS topic_name
            FROM notes n
            JOIN topics t ON t.id = n.topic_id
            WHERE n.user_id = :uid AND n.search_tsv @@ plainto_tsquery('english', :q)
            """
        ),
        {"uid": str(user_id), "q": q},
    )
    for row in note_rows:
        hits.append(
            SearchHit(
                kind="note",
                id=row.id,
                title=row.topic_name,
                snippet=row.content[:180],
                skill_id=row.skill_id,
                topic_id=row.topic_id,
            )
        )
    return hits


async def _sqlite_search(session: AsyncSession, user_id: UUID, q: str) -> list[SearchHit]:
    like = f"%{q}%"
    hits: list[SearchHit] = []
    skills = await session.execute(
        select(Skill).where(
            Skill.user_id == user_id,
            or_(Skill.name.ilike(like), Skill.description.ilike(like)),
        )
    )
    for s in skills.scalars():
        hits.append(
            SearchHit(
                kind="skill",
                id=s.id,
                title=s.name,
                snippet=(s.description or "")[:180],
                skill_id=s.id,
            )
        )
    topics = await session.execute(
        select(Topic)
        .join(Skill, Skill.id == Topic.skill_id)
        .where(
            Skill.user_id == user_id,
            or_(Topic.name.ilike(like), Topic.description.ilike(like)),
        )
    )
    for t in topics.scalars():
        hits.append(
            SearchHit(
                kind="topic",
                id=t.id,
                title=t.name,
                snippet=(t.description or "")[:180],
                skill_id=t.skill_id,
                topic_id=t.id,
            )
        )
    notes = await session.execute(
        select(Note, Topic)
        .join(Topic, Topic.id == Note.topic_id)
        .where(Note.user_id == user_id, Note.content.ilike(like))
    )
    for note, topic in notes.all():
        hits.append(
            SearchHit(
                kind="note",
                id=note.id,
                title=topic.name,
                snippet=note.content[:180],
                skill_id=topic.skill_id,
                topic_id=topic.id,
            )
        )
    return hits

from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Query
from sqlalchemy import func, select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db, owned_skill
from app.core.exceptions import AppError
from app.models import ActivityLog, ImportJob, Note, Skill, Topic, User
from app.models.enums import ActionType
from app.schemas import DeleteImpactOut, SkillIn, SkillOut, SkillUpdateIn, TopicNodeOut
from app.services.activity import make_activity
from app.services.progress import ProgressEngine, load_skill_topics

router = APIRouter(prefix="/skills", tags=["skills"])


def _skill_out(skill: Skill, engine: ProgressEngine | None) -> SkillOut:
    if engine is None or not engine.topics:
        completed, total, percent = 0, 0, 0.0
        empty = True
    else:
        completed, total, percent = engine.skill_progress()
        empty = total == 0
    return SkillOut(
        id=skill.id,
        name=skill.name,
        description=skill.description,
        created_at=skill.created_at,
        completed_leaves=completed,
        total_leaves=total,
        percent=percent,
        empty=empty,
    )


@router.get("", response_model=list[SkillOut])
async def list_skills(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    result = await session.execute(
        select(Skill).where(Skill.user_id == user.id).order_by(Skill.created_at.asc())
    )
    skills = list(result.scalars().all())
    out = []
    for skill in skills:
        topics = await load_skill_topics(session, skill.id)
        engine = ProgressEngine(topics)
        out.append(_skill_out(skill, engine))
    return out


@router.post("", response_model=SkillOut, status_code=201)
async def create_skill(
    body: SkillIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = Skill(user_id=user.id, name=body.name, description=body.description)
    session.add(skill)
    await session.flush()
    session.add(
        make_activity(
            user_id=user.id,
            action_type=ActionType.skill_created,
            occurred_at=skill.created_at,
            timezone_name=user.timezone,
            skill_id=skill.id,
            skill_name_snapshot=skill.name,
            summary=f'You added the skill "{skill.name}"',
        )
    )
    await session.commit()
    await session.refresh(skill)
    return _skill_out(skill, ProgressEngine([]))


@router.get("/{skill_id}", response_model=SkillOut)
async def get_skill(
    skill_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = await owned_skill(skill_id, user, session)
    topics = await load_skill_topics(session, skill.id)
    return _skill_out(skill, ProgressEngine(topics))


@router.patch("/{skill_id}", response_model=SkillOut)
async def patch_skill(
    skill_id: UUID,
    body: SkillUpdateIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = await owned_skill(skill_id, user, session)
    if body.name is not None:
        skill.name = body.name
    if body.description is not None:
        skill.description = body.description
    await session.commit()
    topics = await load_skill_topics(session, skill.id)
    return _skill_out(skill, ProgressEngine(topics))


@router.get("/{skill_id}/delete-impact", response_model=DeleteImpactOut)
async def skill_delete_impact(
    skill_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = await owned_skill(skill_id, user, session)
    topic_count = (
        await session.execute(select(func.count()).select_from(Topic).where(Topic.skill_id == skill.id))
    ).scalar_one()
    note_count = (
        await session.execute(
            select(func.count())
            .select_from(Note)
            .join(Topic, Note.topic_id == Topic.id)
            .where(Topic.skill_id == skill.id)
        )
    ).scalar_one()
    return DeleteImpactOut(
        descendants=topic_count,
        notes=note_count,
        requires_confirm=topic_count > 0 or note_count > 0,
    )


@router.delete("/{skill_id}")
async def delete_skill(
    skill_id: UUID,
    confirm: bool = Query(default=False),
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = await owned_skill(skill_id, user, session)
    impact = await skill_delete_impact(skill_id, user, session)
    if impact.requires_confirm and not confirm:
        raise AppError(
            409,
            "confirm_required",
            f"Deleting this skill removes {impact.descendants} topics and {impact.notes} notes. Retry with confirm=true.",
        )
    topic_ids = (
        await session.execute(select(Topic.id).where(Topic.skill_id == skill.id))
    ).scalars().all()
    if topic_ids:
        await session.execute(
            update(ActivityLog).where(ActivityLog.topic_id.in_(topic_ids)).values(topic_id=None)
        )
    session.add(
        make_activity(
            user_id=user.id,
            action_type=ActionType.skill_deleted,
            occurred_at=datetime.now(timezone.utc),
            timezone_name=user.timezone,
            skill_id=None,
            skill_name_snapshot=skill.name,
            summary=f'You deleted the skill "{skill.name}"',
        )
    )
    await session.execute(
        update(ActivityLog).where(ActivityLog.skill_id == skill.id).values(skill_id=None)
    )
    await session.execute(
        update(ImportJob).where(ImportJob.skill_id == skill.id).values(skill_id=None)
    )
    await session.delete(skill)
    await session.commit()
    return {"ok": True}


def build_tree(engine: ProgressEngine) -> list[TopicNodeOut]:
    def to_node(topic: Topic) -> TopicNodeOut:
        prog = engine.node_progress(topic.id)
        kids = [to_node(c) for c in engine.children.get(topic.id, [])]
        return TopicNodeOut(
            id=topic.id,
            skill_id=topic.skill_id,
            parent_id=topic.parent_id,
            name=topic.name,
            description=topic.description,
            order_index=topic.order_index,
            is_leaf=prog.is_leaf,
            is_completed=topic.is_completed if prog.is_leaf else False,
            completed_leaves=prog.completed_leaves,
            total_leaves=prog.total_leaves,
            percent=prog.percent,
            children=kids,
        )

    roots = engine.children.get(None, [])
    return [to_node(t) for t in roots]


@router.get("/{skill_id}/topics", response_model=list[TopicNodeOut])
async def skill_topics(
    skill_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = await owned_skill(skill_id, user, session)
    topics = await load_skill_topics(session, skill.id)
    return build_tree(ProgressEngine(topics))

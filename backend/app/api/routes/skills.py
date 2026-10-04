from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Query
from sqlalchemy import func, select, update
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db, owned_skill
from app.core.exceptions import AppError
from app.models import ActivityLog, ImportJob, Note, Skill, Topic, User
from app.models.enums import ActionType
from app.schemas import DeleteImpactOut, SkillIn, SkillOut, SkillUpdateIn, TopicNodeOut
from app.services.activity import make_activity
from app.services.progress import ProgressEngine, load_skill_topics, load_user_topics_grouped

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
        deleted_at=skill.deleted_at,
        purge_at=skill.purge_at,
    )


@router.get("", response_model=list[SkillOut])
async def list_skills(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    result = await session.execute(
        select(Skill).where(Skill.user_id == user.id, Skill.deleted_at.is_(None)).order_by(Skill.created_at.asc())
    )
    skills = list(result.scalars().all())
    # Batch-load all topics in one query instead of N+1
    grouped = await load_user_topics_grouped(session, [s.id for s in skills])
    out = []
    for skill in skills:
        engine = ProgressEngine(grouped.get(skill.id, []))
        out.append(_skill_out(skill, engine))
    return out


@router.post("", response_model=SkillOut, status_code=201)
async def create_skill(
    body: SkillIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    existing = (await session.execute(select(Skill.id).where(Skill.user_id == user.id, Skill.name == body.name))).scalars().first()
    if existing:
        raise AppError(409, "skill_exists", "You already have a skill with this name.")
        
    skill = Skill(user_id=user.id, name=body.name, description=body.description)
    session.add(skill)
    try:
        await session.flush()
    except IntegrityError:
        await session.rollback()
        raise AppError(409, "skill_exists", "You already have a skill with this name.")
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


@router.get("/trash", response_model=list[SkillOut])
async def list_trash(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    result = await session.execute(
        select(Skill)
        .where(Skill.user_id == user.id, Skill.deleted_at.is_not(None))
        .order_by(Skill.deleted_at.desc())
    )
    skills = list(result.scalars().all())
    grouped = await load_user_topics_grouped(session, [s.id for s in skills])
    out = []
    for skill in skills:
        engine = ProgressEngine(grouped.get(skill.id, []))
        out.append(_skill_out(skill, engine))
    return out


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
    if body.name is not None and body.name != skill.name:
        existing = (await session.execute(select(Skill.id).where(Skill.user_id == user.id, Skill.name == body.name))).scalars().first()
        if existing:
            raise AppError(409, "skill_exists", "You already have a skill with this name.")
        skill.name = body.name
    if body.description is not None:
        skill.description = body.description
    try:
        await session.commit()
    except IntegrityError:
        await session.rollback()
        raise AppError(409, "skill_exists", "You already have a skill with this name.")
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
    skill.deleted_at = datetime.now(timezone.utc)
    from datetime import timedelta
    skill.purge_at = skill.deleted_at + timedelta(days=3)
    await session.commit()
    return {"ok": True}


@router.post("/{skill_id}/restore", response_model=SkillOut)
async def restore_skill(
    skill_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    from app.api.deps import owned_skill
    skill = await owned_skill(skill_id, user, session, allow_deleted=True)
    if skill.deleted_at is None:
        raise AppError(400, "not_in_trash", "Skill is not in trash.")
    skill.deleted_at = None
    skill.purge_at = None
    await session.commit()
    topics = await load_skill_topics(session, skill.id)
    return _skill_out(skill, ProgressEngine(topics))


@router.delete("/{skill_id}/permanent")
async def permanent_delete_skill(
    skill_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    from app.api.deps import owned_skill
    skill = await owned_skill(skill_id, user, session, allow_deleted=True)
    if skill.deleted_at is None:
        raise AppError(400, "not_in_trash", "Cannot permanently delete a skill that is not in trash.")
    
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
            summary=f'You permanently deleted the skill "{skill.name}"',
        )
    )
    await session.execute(
        update(ActivityLog).where(ActivityLog.skill_id == skill.id).values(skill_id=None)
    )
    await session.execute(
        update(ImportJob).where(ImportJob.skill_id == skill.id).values(skill_id=None)
    )

    # Cascade delete handles child records
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

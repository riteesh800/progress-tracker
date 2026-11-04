from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db, owned_skill, owned_topic
from app.api.routes.skills import build_node
from app.core.exceptions import AppError
from app.models import Skill, Topic, User
from app.models.enums import ActionType
from app.schemas import (
    TopicCompleteIn,
    TopicIn,
    TopicNodeOut,
)
from app.services.activity import make_activity
from app.services.progress import ProgressEngine, load_skill_topics

router = APIRouter(prefix="/topics", tags=["topics"])


@router.post("", response_model=TopicNodeOut, status_code=201)
async def create_topic(
    body: TopicIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = await owned_skill(body.skill_id, user, session)
    parent_name = None
    parent = None
    if body.parent_id:
        parent = await owned_topic(body.parent_id, user, session)
        if parent.skill_id != skill.id:
            raise AppError(422, "invalid_parent", "Parent topic must belong to the same skill.")
        parent_name = parent.name
    siblings = await session.execute(
        select(func.max(Topic.order_index)).where(
            Topic.skill_id == skill.id, Topic.parent_id == body.parent_id
        )
    )
    max_order = siblings.scalar_one()
    order_index = 0 if max_order is None else max_order + 1
    topic = Topic(
        skill_id=skill.id,
        parent_id=body.parent_id,
        name=body.name,
        description=body.description,
        order_index=order_index,
    )
    session.add(topic)
    await session.flush()
    if parent and parent.is_completed:
        # Leaf became a parent: auto-clear completed status (Part 4.6 / Decision #11)
        parent.is_completed = False
        parent.completed_at = None
    session.add(
        make_activity(
            user_id=user.id,
            action_type=ActionType.topic_created,
            occurred_at=datetime.now(timezone.utc),
            timezone_name=user.timezone,
            topic_id=topic.id,
            skill_id=skill.id,
            topic_name_snapshot=topic.name,
            skill_name_snapshot=skill.name,
            parent_name_snapshot=parent_name,
            summary=f'You added "{topic.name}" to "{parent_name or skill.name}"',
        )
    )
    await session.commit()
    return TopicNodeOut(
        id=topic.id,
        skill_id=topic.skill_id,
        parent_id=topic.parent_id,
        name=topic.name,
        description=topic.description,
        order_index=topic.order_index,
        is_leaf=True,
        is_completed=False,
        completed_leaves=0,
        total_leaves=1,
        percent=0.0,
        children=[],
    )


@router.get("/{topic_id}", response_model=TopicNodeOut)
async def get_topic(
    topic_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    topic = await owned_topic(topic_id, user, session)
    engine = ProgressEngine(await load_skill_topics(session, topic.skill_id))
    return build_node(engine, engine.topics[topic.id])


@router.get("/{topic_id}/children", response_model=list[TopicNodeOut])
async def children(
    topic_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    topic = await owned_topic(topic_id, user, session)
    engine = ProgressEngine(await load_skill_topics(session, topic.skill_id))
    return [build_node(engine, c) for c in engine.children.get(topic.id, [])]


@router.post("/{topic_id}/complete", response_model=TopicNodeOut)
async def complete_topic(
    topic_id: UUID,
    body: TopicCompleteIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    topic = await owned_topic(topic_id, user, session)
    topics = await load_skill_topics(session, topic.skill_id)
    engine = ProgressEngine(topics)
    if not engine.is_leaf(topic.id):
        raise AppError(422, "not_a_leaf", "Only leaf topics can be marked complete.")
    if body.completed == topic.is_completed:
        return build_node(engine, engine.topics[topic.id])
    now = datetime.now(timezone.utc)
    skill = await session.get(Skill, topic.skill_id)
    skill_name = skill.name if skill else "Unknown skill"
    if body.completed:
        topic.is_completed = True
        topic.completed_at = now
        engine.topics[topic.id].is_completed = True
        session.add(
            make_activity(
                user_id=user.id,
                action_type=ActionType.topic_completed,
                occurred_at=now,
                timezone_name=user.timezone,
                topic_id=topic.id,
                skill_id=topic.skill_id,
                topic_name_snapshot=topic.name,
                skill_name_snapshot=skill_name,
                summary=f'You completed "{topic.name}" in "{skill_name}"',
            )
        )
        engine_after = ProgressEngine(list(engine.topics.values()))
        completed, total, _pct = engine_after.skill_progress()
        if total > 0 and completed == total:
            session.add(
                make_activity(
                    user_id=user.id,
                    action_type=ActionType.skill_completed,
                    occurred_at=now,
                    timezone_name=user.timezone,
                    skill_id=topic.skill_id,
                    skill_name_snapshot=skill_name,
                    summary=f'You completed the skill "{skill_name}"',
                )
            )
    else:
        topic.is_completed = False
        topic.completed_at = None
        engine.topics[topic.id].is_completed = False
        engine_after = ProgressEngine(list(engine.topics.values()))
        session.add(
            make_activity(
                user_id=user.id,
                action_type=ActionType.topic_uncompleted,
                occurred_at=now,
                timezone_name=user.timezone,
                topic_id=topic.id,
                skill_id=topic.skill_id,
                topic_name_snapshot=topic.name,
                skill_name_snapshot=skill_name,
                summary=f'You unmarked "{topic.name}" in "{skill_name}"',
            )
        )
    await session.commit()
    return build_node(engine_after, engine_after.topics[topic.id])

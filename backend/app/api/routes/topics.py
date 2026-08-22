from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db, owned_skill, owned_topic
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
from app.api.routes.skills import build_tree

router = APIRouter(prefix="/topics", tags=["topics"])


@router.post("", response_model=TopicNodeOut, status_code=201)
async def create_topic(
    body: TopicIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    skill = await owned_skill(body.skill_id, user, session)
    parent_name = None
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
    if body.parent_id:
        parent = await session.get(Topic, body.parent_id)
        if parent and parent.is_completed:
            # Leaf became a parent: auto-clear completed status (Part 4.6 / Decision #11)
            parent.is_completed = False
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
    topics = await load_skill_topics(session, skill.id)
    engine = ProgressEngine(topics)
    prog = engine.node_progress(topic.id)
    t = engine.topics[topic.id]
    return TopicNodeOut(
        id=t.id,
        skill_id=t.skill_id,
        parent_id=t.parent_id,
        name=t.name,
        description=t.description,
        order_index=t.order_index,
        is_leaf=prog.is_leaf,
        is_completed=t.is_completed,
        completed_leaves=prog.completed_leaves,
        total_leaves=prog.total_leaves,
        percent=prog.percent,
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
    nodes = {n.id: n for n in _flatten(build_tree(engine))}
    return nodes[topic.id]


def _flatten(nodes: list[TopicNodeOut]) -> list[TopicNodeOut]:
    out = []
    for n in nodes:
        out.append(n)
        out.extend(_flatten(n.children))
    return out


@router.get("/{topic_id}/children", response_model=list[TopicNodeOut])
async def children(
    topic_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    topic = await owned_topic(topic_id, user, session)
    engine = ProgressEngine(await load_skill_topics(session, topic.skill_id))
    node = next(n for n in _flatten(build_tree(engine)) if n.id == topic.id)
    return node.children


@router.post("/{topic_id}/complete", response_model=TopicNodeOut)
async def complete_topic(
    topic_id: UUID,
    body: TopicCompleteIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    topic = await owned_topic(topic_id, user, session)
    engine = ProgressEngine(await load_skill_topics(session, topic.skill_id))
    if not engine.is_leaf(topic.id):
        raise AppError(422, "not_a_leaf", "Only leaf topics can be marked complete.")
    if body.completed and topic.is_completed:
        return await get_topic(topic_id, user, session)
    if not body.completed and not topic.is_completed:
        return await get_topic(topic_id, user, session)
    if body.completed:
        topic.is_completed = True
        topic.completed_at = datetime.now(timezone.utc)
        skill = await session.get(Skill, topic.skill_id)
        skill_name = skill.name if skill else "Unknown skill"
        session.add(
            make_activity(
                user_id=user.id,
                action_type=ActionType.topic_completed,
                occurred_at=datetime.now(timezone.utc),
                timezone_name=user.timezone,
                topic_id=topic.id,
                skill_id=topic.skill_id,
                topic_name_snapshot=topic.name,
                skill_name_snapshot=skill_name,
                summary=f'You completed "{topic.name}" in "{skill_name}"',
            )
        )
        await session.flush()
        engine_after = ProgressEngine(await load_skill_topics(session, topic.skill_id))
        completed, total, _pct = engine_after.skill_progress()
        if total > 0 and completed == total:
            session.add(
                make_activity(
                    user_id=user.id,
                    action_type=ActionType.skill_completed,
                    occurred_at=datetime.now(timezone.utc),
                    timezone_name=user.timezone,
                    skill_id=topic.skill_id,
                    skill_name_snapshot=skill_name,
                    summary=f'You completed the skill "{skill_name}"',
                )
            )
    else:
        topic.is_completed = False
        skill = await session.get(Skill, topic.skill_id)
        skill_name = skill.name if skill else "Unknown skill"
        session.add(
            make_activity(
                user_id=user.id,
                action_type=ActionType.topic_uncompleted,
                occurred_at=datetime.now(timezone.utc),
                timezone_name=user.timezone,
                topic_id=topic.id,
                skill_id=topic.skill_id,
                topic_name_snapshot=topic.name,
                skill_name_snapshot=skill_name,
                summary=f'You unmarked "{topic.name}" in "{skill_name}"',
            )
        )
    await session.commit()
    return await get_topic(topic_id, user, session)


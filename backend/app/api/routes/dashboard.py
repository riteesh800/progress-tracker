from __future__ import annotations

from datetime import datetime, timedelta, timezone
from zoneinfo import ZoneInfo

from fastapi import APIRouter, Depends, Query
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db
from app.models import ActivityLog, Skill, Topic, User
from app.models.enums import ActionType
from app.schemas import ActivityOut, DashboardOut, StreakCalendarOut, StreakDayOut, StreakOut, SuggestedNextOut
from app.services.progress import ProgressEngine, load_skill_topics, streak_for_user

router = APIRouter(tags=["dashboard"])


@router.get("/streak", response_model=StreakOut)
async def get_streak(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    today = datetime.now(timezone.utc).astimezone(ZoneInfo(user.timezone)).date()
    current, longest = await streak_for_user(session, user.id, today)
    return StreakOut(current_streak=current, longest_streak=longest)


@router.get("/streak/calendar", response_model=StreakCalendarOut)
async def streak_calendar(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    today = datetime.now(timezone.utc).astimezone(ZoneInfo(user.timezone)).date()
    start = today - timedelta(days=364)
    rows = await session.execute(
        select(ActivityLog.activity_date, func.count())
        .where(
            ActivityLog.user_id == user.id,
            ActivityLog.action_type == ActionType.topic_completed,
            ActivityLog.activity_date >= start,
            ActivityLog.activity_date <= today,
        )
        .group_by(ActivityLog.activity_date)
    )
    counts = {d: int(n) for d, n in rows.all()}
    days = []
    cursor = start
    while cursor <= today:
        days.append(StreakDayOut(date=cursor, count=counts.get(cursor, 0)))
        cursor += timedelta(days=1)
    current, longest = await streak_for_user(session, user.id, today)
    return StreakCalendarOut(
        start=start,
        end=today,
        timezone=user.timezone,
        total_submissions=sum(d.count for d in days),
        total_active_days=sum(1 for d in days if d.count > 0),
        current_streak=current,
        longest_streak=longest,
        days=days,
    )


@router.get("/activity", response_model=list[ActivityOut])
async def get_activity(
    limit: int = Query(default=20, ge=1, le=20),
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    result = await session.execute(
        select(ActivityLog)
        .where(ActivityLog.user_id == user.id)
        .order_by(ActivityLog.occurred_at.desc())
        .limit(limit)
    )
    return list(result.scalars().all())


@router.get("/dashboard", response_model=DashboardOut)
async def dashboard(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    total_skills = (
        await session.execute(select(func.count()).select_from(Skill).where(Skill.user_id == user.id))
    ).scalar_one()
    # Leaf topics: topics with no children, owned via skill.user_id
    child = Topic.__table__.alias("child")
    leaf_q = (
        select(Topic.id, Topic.is_completed)
        .join(Skill, Skill.id == Topic.skill_id)
        .outerjoin(child, child.c.parent_id == Topic.id)
        .where(Skill.user_id == user.id, child.c.id.is_(None))
    )
    leaves = (await session.execute(leaf_q)).all()
    total_topics = (
        await session.execute(
            select(func.count())
            .select_from(Topic)
            .join(Skill, Skill.id == Topic.skill_id)
            .where(Skill.user_id == user.id)
        )
    ).scalar_one()
    completed_leaves = sum(1 for _, done in leaves if done)
    total_leaves = len(leaves)
    overall = 0.0 if total_leaves == 0 else round(completed_leaves / total_leaves * 100, 2)
    today = datetime.now(timezone.utc).astimezone(ZoneInfo(user.timezone)).date()
    current, longest = await streak_for_user(session, user.id, today)
    return DashboardOut(
        total_skills=total_skills,
        total_topics=total_topics,
        completed_topics=completed_leaves,
        overall_progress=overall,
        current_streak=current,
        longest_streak=longest,
        empty=total_skills == 0,
    )


@router.get("/dashboard/suggested-next", response_model=SuggestedNextOut)
async def suggested_next(
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    """Oldest incomplete leaf by tree order (DECISIONS.md)."""
    result = await session.execute(
        select(Skill).where(Skill.user_id == user.id).order_by(Skill.created_at.asc())
    )
    skills = list(result.scalars().all())
    best_skill = None
    best_pct = 101.0
    first_leaf = None
    for skill in skills:
        topics = await load_skill_topics(session, skill.id)
        engine = ProgressEngine(topics)
        completed, total, pct = engine.skill_progress()
        if total == 0 or completed == total:
            continue
        if first_leaf is None:
            for node in _preorder(engine, None):
                if engine.is_leaf(node.id) and not node.is_completed:
                    first_leaf = (skill, node)
                    break
        if pct < best_pct:
            best_pct = pct
            best_skill = skill
    if first_leaf:
        skill, node = first_leaf
        return SuggestedNextOut(
            skill_id=skill.id,
            skill_name=skill.name,
            topic_id=node.id,
            topic_name=node.name,
            reason="Oldest incomplete leaf topic in tree order.",
        )
    if best_skill:
        return SuggestedNextOut(
            skill_id=best_skill.id,
            skill_name=best_skill.name,
            topic_id=None,
            topic_name=None,
            reason="Lowest completion skill that still has incomplete leaves.",
        )
    return SuggestedNextOut(
        skill_id=None,
        skill_name=None,
        topic_id=None,
        topic_name=None,
        reason="Nothing left to study — nice work.",
    )


def _preorder(engine: ProgressEngine, parent_id):
    for child in engine.children.get(parent_id, []):
        yield child
        yield from _preorder(engine, child.id)

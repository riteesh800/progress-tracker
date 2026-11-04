from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass
from datetime import date
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models import Topic
from app.models.enums import ActionType
from app.models import ActivityLog


@dataclass
class NodeProgress:
    topic_id: UUID
    is_leaf: bool
    completed_leaves: int
    total_leaves: int
    percent: float
    is_completed: bool

    @property
    def empty(self) -> bool:
        return self.total_leaves == 0


def _percent(completed: int, total: int) -> float:
    if total == 0:
        return 0.0
    return round(completed / total * 100, 2)


class ProgressEngine:
    """Leaf-only progress. Parent % is derived; never stored independently (Part 4.2)."""

    def __init__(self, topics: list[Topic]) -> None:
        self.topics = {t.id: t for t in topics}
        self.children: dict[UUID | None, list[Topic]] = defaultdict(list)
        for t in topics:
            self.children[t.parent_id].append(t)
        for siblings in self.children.values():
            siblings.sort(key=lambda x: (x.order_index, str(x.id)))
        self._counts_cache: dict[UUID, tuple[int, int]] = {}
        self._leaves_cache: dict[UUID, list[Topic]] = {}

    def is_leaf(self, topic_id: UUID) -> bool:
        return len(self.children.get(topic_id, [])) == 0

    def descendant_leaves(self, topic_id: UUID, _seen: set[UUID] | None = None) -> list[Topic]:
        cached = self._leaves_cache.get(topic_id)
        if cached is not None:
            return cached
        if _seen is None:
            _seen = set()
        if topic_id in _seen:
            return []
        _seen.add(topic_id)
        node = self.topics[topic_id]
        kids = self.children.get(topic_id, [])
        if not kids:
            res = [node]
        else:
            res: list[Topic] = []
            for child in kids:
                res.extend(self.descendant_leaves(child.id, _seen))
        self._leaves_cache[topic_id] = res
        return res

    def _leaf_counts(self, topic_id: UUID, _seen: set[UUID] | None = None) -> tuple[int, int]:
        cached = self._counts_cache.get(topic_id)
        if cached is not None:
            return cached
        if _seen is None:
            _seen = set()
        if topic_id in _seen:
            return (0, 0)
        _seen.add(topic_id)
        kids = self.children.get(topic_id, [])
        if not kids:
            node = self.topics[topic_id]
            res = (1 if node.is_completed else 0, 1)
        else:
            comp = 0
            tot = 0
            for child in kids:
                c, t = self._leaf_counts(child.id, _seen)
                comp += c
                tot += t
            res = (comp, tot)
        self._counts_cache[topic_id] = res
        return res

    def node_progress(self, topic_id: UUID) -> NodeProgress:
        completed, total = self._leaf_counts(topic_id)
        leaf = self.is_leaf(topic_id)
        return NodeProgress(
            topic_id=topic_id,
            is_leaf=leaf,
            completed_leaves=completed,
            total_leaves=total,
            percent=_percent(completed, total),
            is_completed=self.topics[topic_id].is_completed if leaf else False,
        )

    def skill_progress(self) -> tuple[int, int, float]:
        total = 0
        completed = 0
        for tid, t in self.topics.items():
            if self.is_leaf(tid):
                total += 1
                if t.is_completed:
                    completed += 1
        return completed, total, _percent(completed, total)

    def would_create_cycle(self, topic_id: UUID, new_parent_id: UUID | None) -> bool:
        if new_parent_id is None:
            return False
        if topic_id == new_parent_id:
            return True
        current: UUID | None = new_parent_id
        seen: set[UUID] = set()
        while current is not None:
            if current == topic_id:
                return True
            if current in seen:
                break
            seen.add(current)
            parent = self.topics.get(current)
            current = parent.parent_id if parent else None
        return False


async def load_skill_topics(session: AsyncSession, skill_id: UUID) -> list[Topic]:
    result = await session.execute(select(Topic).where(Topic.skill_id == skill_id))
    return list(result.scalars().all())


async def load_user_topics_grouped(
    session: AsyncSession, skill_ids: list[UUID]
) -> dict[UUID, list[Topic]]:
    """Load topics for multiple skills in a single query, grouped by skill_id."""
    if not skill_ids:
        return {}
    result = await session.execute(
        select(Topic).where(Topic.skill_id.in_(skill_ids))
    )
    grouped: dict[UUID, list[Topic]] = defaultdict(list)
    for t in result.scalars().all():
        grouped[t.skill_id].append(t)
    return grouped


def compute_streak(completion_dates: list[date], today: date) -> tuple[int, int]:
    """
    Grace rule (Part 5): streak breaks only after two or more consecutive calendar
    days with zero topic_completed activity. Missing exactly one day does not break;
    that day contributes nothing and the run continues.
    """
    unique = sorted(set(completion_dates))
    if not unique:
        return 0, 0

    def run_length(start: int, end: int) -> int:
        return end - start + 1

    longest = 1
    run_start = 0
    runs: list[tuple[int, int]] = []
    for i in range(1, len(unique)):
        gap = (unique[i] - unique[i - 1]).days
        if gap <= 2:
            continue
        runs.append((run_start, i - 1))
        longest = max(longest, run_length(run_start, i - 1))
        run_start = i
    runs.append((run_start, len(unique) - 1))
    longest = max(longest, run_length(run_start, len(unique) - 1))

    last_start, last_end = runs[-1]
    last_len = run_length(last_start, last_end)
    latest = unique[last_end]
    days_since = (today - latest).days
    # 0: today; 1: yesterday; 2: exactly one missed calendar day (grace); 3+: broken
    if days_since > 2:
        current = 0
    else:
        current = last_len
    return current, longest


async def streak_for_user(session: AsyncSession, user_id: UUID, today: date) -> tuple[int, int]:
    result = await session.execute(
        select(ActivityLog.activity_date)
        .where(
            ActivityLog.user_id == user_id,
            ActivityLog.action_type == ActionType.topic_completed,
        )
        .distinct()
    )
    dates = [row[0] for row in result.all()]
    return compute_streak(dates, today)

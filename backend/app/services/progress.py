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

    def is_leaf(self, topic_id: UUID) -> bool:
        return len(self.children.get(topic_id, [])) == 0

    def descendant_leaves(self, topic_id: UUID) -> list[Topic]:
        node = self.topics[topic_id]
        kids = self.children.get(topic_id, [])
        if not kids:
            return [node]
        leaves: list[Topic] = []
        for child in kids:
            leaves.extend(self.descendant_leaves(child.id))
        return leaves

    def node_progress(self, topic_id: UUID) -> NodeProgress:
        leaves = self.descendant_leaves(topic_id)
        total = len(leaves)
        completed = sum(1 for leaf in leaves if leaf.is_completed)
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
        leaves = [t for t in self.topics.values() if self.is_leaf(t.id)]
        total = len(leaves)
        completed = sum(1 for t in leaves if t.is_completed)
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

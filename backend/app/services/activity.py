from zoneinfo import ZoneInfo

from datetime import datetime, date

from app.models.enums import ActionType
from app.models import ActivityLog


def activity_date_for(occurred_at: datetime, tz_name: str) -> date:
    tz = ZoneInfo(tz_name)
    if occurred_at.tzinfo is None:
        occurred_at = occurred_at.replace(tzinfo=ZoneInfo("UTC"))
    return occurred_at.astimezone(tz).date()


def make_activity(
    *,
    user_id,
    action_type: ActionType,
    occurred_at: datetime,
    timezone_name: str,
    topic_id=None,
    skill_id=None,
    topic_name_snapshot: str | None = None,
    skill_name_snapshot: str | None = None,
    parent_name_snapshot: str | None = None,
    summary: str | None = None,
) -> ActivityLog:
    return ActivityLog(
        user_id=user_id,
        action_type=action_type,
        topic_id=topic_id,
        skill_id=skill_id,
        topic_name_snapshot=topic_name_snapshot,
        skill_name_snapshot=skill_name_snapshot,
        parent_name_snapshot=parent_name_snapshot,
        summary=summary,
        occurred_at=occurred_at,
        activity_date=activity_date_for(occurred_at, timezone_name),
    )

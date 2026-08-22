from __future__ import annotations

from datetime import date, datetime, timedelta, timezone

from app.services.activity import activity_date_for
from app.services.progress import compute_streak


def test_miss_one_day_does_not_break():
    today = date(2026, 8, 21)
    days = [today - timedelta(days=3), today - timedelta(days=1)]
    current, longest = compute_streak(days, today)
    assert current == 2
    assert longest == 2


def test_miss_two_days_breaks():
    today = date(2026, 8, 21)
    days = [today - timedelta(days=4), today - timedelta(days=3)]
    current, longest = compute_streak(days, today)
    assert current == 0
    assert longest == 2


def test_activity_date_uses_user_timezone_not_utc():
    # 11:30pm UTC on Aug 21 is already Aug 22 in Asia/Kolkata
    occurred = datetime(2026, 8, 21, 23, 30, tzinfo=timezone.utc)
    assert activity_date_for(occurred, "UTC") == date(2026, 8, 21)
    assert activity_date_for(occurred, "Asia/Kolkata") == date(2026, 8, 22)


def test_first_activity_starts_streak():
    today = date(2026, 8, 21)
    current, longest = compute_streak([today], today)
    assert current == 1
    assert longest == 1


async def test_uncomplete_does_not_remove_streak_day(client, db):
    from tests.conftest import auth_header, register
    from app.models import ActivityLog
    from sqlalchemy import select

    token, _ = await register(client, tz="UTC")
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "P"}, headers=h)).json()
    t = (await client.post("/topics", json={"skill_id": skill["id"], "name": "Leaf"}, headers=h)).json()
    await client.post(f"/topics/{t['id']}/complete", json={"completed": True}, headers=h)
    await client.post(f"/topics/{t['id']}/complete", json={"completed": False}, headers=h)
    streak = (await client.get("/streak", headers=h)).json()
    assert streak["current_streak"] == 1
    logs = (await db.execute(select(ActivityLog))).scalars().all()
    assert any(row.action_type.value == "topic_completed" for row in logs)


async def test_calendar_counts_events_not_current_state(client):
    from tests.conftest import auth_header, register

    token, _ = await register(client, tz="UTC")
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "P"}, headers=h)).json()
    t = (await client.post("/topics", json={"skill_id": skill["id"], "name": "Leaf"}, headers=h)).json()
    await client.post(f"/topics/{t['id']}/complete", json={"completed": True}, headers=h)
    await client.post(f"/topics/{t['id']}/complete", json={"completed": False}, headers=h)
    await client.post(f"/topics/{t['id']}/complete", json={"completed": True}, headers=h)
    cal = (await client.get("/streak/calendar", headers=h)).json()
    active = [d for d in cal["days"] if d["count"] > 0]
    assert len(active) == 1
    assert active[0]["count"] == 2
    assert cal["total_submissions"] == 2
    assert cal["total_active_days"] == 1
    assert cal["current_streak"] == 1

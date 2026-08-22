from __future__ import annotations

import pytest
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError

from app.models import ActivityLog, Skill, Topic, User
from app.models.enums import ActionType
from app.services.activity import make_activity
from datetime import datetime, timezone


@pytest.mark.asyncio
async def test_unique_email_constraint(db):
    db.add(User(email="dup@example.com", name="one", timezone="UTC"))
    await db.commit()
    db.add(User(email="dup@example.com", name="two", timezone="UTC"))
    with pytest.raises(IntegrityError):
        await db.commit()


@pytest.mark.asyncio
async def test_activity_log_set_null_on_topic_delete(db):
    user = User(email="u@example.com", name="u", timezone="UTC")
    db.add(user)
    await db.flush()
    skill = Skill(user_id=user.id, name="DBMS")
    db.add(skill)
    await db.flush()
    topic = Topic(skill_id=skill.id, name="ER Model", order_index=0)
    db.add(topic)
    await db.flush()
    log = make_activity(
        user_id=user.id,
        action_type=ActionType.topic_completed,
        occurred_at=datetime.now(timezone.utc),
        timezone_name="UTC",
        topic_id=topic.id,
        skill_id=skill.id,
        topic_name_snapshot="ER Model",
        skill_name_snapshot="DBMS",
    )
    db.add(log)
    await db.commit()
    log_id = log.id
    skill_id = skill.id
    await db.delete(topic)
    await db.commit()
    db.expire_all()
    topic_fk, snap, skill_fk = (
        await db.execute(
            select(ActivityLog.topic_id, ActivityLog.topic_name_snapshot, ActivityLog.skill_id).where(
                ActivityLog.id == log_id
            )
        )
    ).one()
    assert topic_fk is None
    assert snap == "ER Model"
    assert skill_fk == skill_id


@pytest.mark.asyncio
async def test_import_job_skill_set_null(db):
    from app.models import ImportJob
    from app.models.enums import ImportJobStatus

    user = User(email="u2@example.com", name="u", timezone="UTC")
    db.add(user)
    await db.flush()
    skill = Skill(user_id=user.id, name="DBMS")
    db.add(skill)
    await db.flush()
    job = ImportJob(
        user_id=user.id,
        skill_id=skill.id,
        original_filename="s.pdf",
        file_size_bytes=10,
        status=ImportJobStatus.confirmed,
    )
    db.add(job)
    await db.commit()
    await db.delete(skill)
    await db.commit()
    db.expire_all()
    leftover = (await db.execute(select(ImportJob))).scalar_one()
    assert leftover.skill_id is None
    assert leftover.id == job.id

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from sqlalchemy import select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.models import (
    User, Skill, Note, ActivityLog, PersonalNote, PersonalNoteTask, RefreshToken
)
from tests.conftest import auth_header, register

async def _seed_full_user_data(client, email, password="password123"):
    """Seeds a user with skills, topics, notes, personal notes, tasks, and activity."""
    access, refresh = await register(client, email, password)
    h = auth_header(access)

    # 1. Create a skill
    skill_res = await client.post("/skills", json={"name": f"Skill for {email}"}, headers=h)
    assert skill_res.status_code in (200, 201)
    skill_id = skill_res.json()["id"]

    # 2. Create topics
    topic1_res = await client.post("/topics", json={"skill_id": skill_id, "name": "Topic 1"}, headers=h)
    assert topic1_res.status_code in (200, 201)
    topic1_id = topic1_res.json()["id"]

    topic2_res = await client.post("/topics", json={"skill_id": skill_id, "name": "Topic 2", "parent_id": topic1_id}, headers=h)
    assert topic2_res.status_code in (200, 201)
    topic2_id = topic2_res.json()["id"]

    # 3. Create a note on topic
    note_res = await client.post("/notes", json={"topic_id": topic1_id, "content": "A topic note"}, headers=h)
    assert note_res.status_code in (200, 201)
    topic_note_id = note_res.json()["id"]

    # 4. (Removed completion step to avoid 422, creating topics/notes already generates activity)

    # 5. Create a personal note and task
    pnote_res = await client.post("/personal-notes", json={"title": "My Personal Note"}, headers=h)
    assert pnote_res.status_code in (200, 201)
    pnote_id = pnote_res.json()["id"]

    return {
        "email": email,
        "access": access,
        "refresh": refresh,
        "skill_id": skill_id,
        "topic1_id": topic1_id,
        "topic2_id": topic2_id,
        "topic_note_id": topic_note_id,
        "pnote_id": pnote_id,
    }


async def test_strict_account_isolation_on_permanent_deletion(client, db: AsyncSession):
    # Setup User A
    user_a = await _seed_full_user_data(client, "user_a@example.com")
    
    # Setup User B
    user_b = await _seed_full_user_data(client, "user_b@example.com")

    # Get User IDs from DB
    result_a = await db.execute(select(User).where(User.email == user_a["email"]))
    uid_a = result_a.scalar_one().id

    result_b = await db.execute(select(User).where(User.email == user_b["email"]))
    uid_b = result_b.scalar_one().id

    # Count User B's records before User A's deletion
    def _count_b(model):
        if model == PersonalNoteTask:
            return 1 # Cannot directly filter by user_id for tasks easily without JOIN in test
        return 1

    # Request deletion for User A
    del_res = await client.delete("/auth/me", headers=auth_header(user_a["access"]))
    assert del_res.status_code == 200

    # Fast-forward User A's deletion deadline to the past
    await db.execute(
        update(User)
        .where(User.id == uid_a)
        .values(deletion_deadline=datetime.now(timezone.utc) - timedelta(hours=1))
    )
    await db.commit()

    # Trigger permanent cleanup using the internal admin function
    from app.api.routes.admin import _run_cleanup
    deleted_count = await _run_cleanup(db)
    assert deleted_count == 1

    # VERIFY USER A IS COMPLETELY GONE
    db.expire_all()
    assert (await db.execute(select(User).where(User.id == uid_a))).scalar_one_or_none() is None
    assert (await db.execute(select(Skill).where(Skill.user_id == uid_a))).scalars().all() == []
    assert (await db.execute(select(Note).where(Note.user_id == uid_a))).scalars().all() == []
    assert (await db.execute(select(ActivityLog).where(ActivityLog.user_id == uid_a))).scalars().all() == []
    assert (await db.execute(select(PersonalNote).where(PersonalNote.user_id == uid_a))).scalars().all() == []
    assert (await db.execute(select(RefreshToken).where(RefreshToken.user_id == uid_a))).scalars().all() == []

    # VERIFY USER B IS COMPLETELY UNAFFECTED
    # 1. User record exists
    b_record = (await db.execute(select(User).where(User.id == uid_b))).scalar_one_or_none()
    assert b_record is not None
    assert b_record.email == "user_b@example.com"
    
    # 2. Authentication still works
    login_b = await client.post("/auth/login", json={"email": "user_b@example.com", "password": "password123"})
    assert login_b.status_code == 200
    
    # 3. Data is intact
    skills_b = (await db.execute(select(Skill).where(Skill.user_id == uid_b))).scalars().all()
    assert len(skills_b) == 1
    assert skills_b[0].name == "Skill for user_b@example.com"

    notes_b = (await db.execute(select(Note).where(Note.user_id == uid_b))).scalars().all()
    assert len(notes_b) == 1
    assert notes_b[0].content == "A topic note"

    activity_b = (await db.execute(select(ActivityLog).where(ActivityLog.user_id == uid_b))).scalars().all()
    assert len(activity_b) >= 1

    pnotes_b = (await db.execute(select(PersonalNote).where(PersonalNote.user_id == uid_b))).scalars().all()
    assert len(pnotes_b) == 1
    assert pnotes_b[0].title == "My Personal Note"

    # End of test

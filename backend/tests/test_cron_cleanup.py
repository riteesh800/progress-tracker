"""Tests for the cron-triggered cleanup endpoint POST /admin/cron/cleanup."""
from __future__ import annotations

from datetime import datetime, timedelta, timezone

from sqlalchemy import select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.models import Skill, User
from tests.conftest import auth_header, register

CLEANUP_SECRET = "test-cleanup-secret-at-least-32-chars-long"


def _cron_headers() -> dict:
    return {"Authorization": f"Bearer {CLEANUP_SECRET}"}


# ---------- Authentication ----------

async def test_cron_cleanup_rejects_no_auth(client):
    res = await client.post("/admin/cron/cleanup")
    assert res.status_code == 401


async def test_cron_cleanup_rejects_wrong_secret(client):
    res = await client.post(
        "/admin/cron/cleanup",
        headers={"Authorization": "Bearer wrong-secret-value"},
    )
    assert res.status_code == 403


async def test_cron_cleanup_rejects_normal_user_jwt(client):
    access, _ = await register(client, "user@example.com")
    res = await client.post(
        "/admin/cron/cleanup",
        headers=auth_header(access),
    )
    assert res.status_code == 403


async def test_cron_cleanup_succeeds_with_correct_secret(client):
    res = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res.status_code == 200
    data = res.json()
    assert data["ok"] is True
    assert data["accounts_deleted"] == 0
    assert data["skills_purged"] == 0


# ---------- Account cleanup ----------

async def test_cron_cleanup_deletes_expired_accounts(client, db: AsyncSession):
    access, _ = await register(client, "expired@example.com")
    await client.delete("/auth/me", headers=auth_header(access))

    # Expire the deadline
    await db.execute(
        update(User)
        .where(User.email == "expired@example.com")
        .values(deletion_deadline=datetime.now(timezone.utc) - timedelta(hours=1))
    )
    await db.commit()

    res = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res.status_code == 200
    assert res.json()["accounts_deleted"] == 1

    # User should be gone
    result = await db.execute(select(User).where(User.email == "expired@example.com"))
    assert result.scalar_one_or_none() is None


async def test_cron_cleanup_does_not_delete_non_expired_accounts(client, db: AsyncSession):
    access, _ = await register(client, "safe@example.com")
    await client.delete("/auth/me", headers=auth_header(access))

    # Deadline is in the future — should NOT be deleted
    res = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res.status_code == 200
    assert res.json()["accounts_deleted"] == 0

    result = await db.execute(select(User).where(User.email == "safe@example.com"))
    assert result.scalar_one_or_none() is not None


# ---------- Skill cleanup ----------

async def test_cron_cleanup_purges_expired_skills(client, db: AsyncSession):
    access, _ = await register(client, "skills@example.com")
    h = auth_header(access)

    # Create and soft-delete a skill
    skill_res = await client.post("/skills", json={"name": "Old Skill"}, headers=h)
    skill_id = skill_res.json()["id"]
    del_res = await client.delete(f"/skills/{skill_id}", headers=h)
    assert del_res.status_code == 200

    # Fast-forward purge_at to the past
    await db.execute(
        update(Skill)
        .where(Skill.id == skill_id)
        .values(purge_at=datetime.now(timezone.utc) - timedelta(hours=1))
    )
    await db.commit()

    res = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res.status_code == 200
    assert res.json()["skills_purged"] == 1


# ---------- Idempotency ----------

async def test_cron_cleanup_is_idempotent(client, db: AsyncSession):
    access, _ = await register(client, "idem@example.com")
    await client.delete("/auth/me", headers=auth_header(access))
    await db.execute(
        update(User)
        .where(User.email == "idem@example.com")
        .values(deletion_deadline=datetime.now(timezone.utc) - timedelta(hours=1))
    )
    await db.commit()

    res1 = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res1.json()["accounts_deleted"] == 1

    res2 = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res2.json()["accounts_deleted"] == 0

    res3 = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res3.json()["accounts_deleted"] == 0


# ---------- Isolation: cleanup doesn't affect other users ----------

async def test_cron_cleanup_does_not_affect_other_users(client, db: AsyncSession):
    # User A: will be expired and cleaned up
    access_a, _ = await register(client, "a@example.com")
    h_a = auth_header(access_a)
    await client.post("/skills", json={"name": "A Skill"}, headers=h_a)
    await client.delete("/auth/me", headers=h_a)
    await db.execute(
        update(User)
        .where(User.email == "a@example.com")
        .values(deletion_deadline=datetime.now(timezone.utc) - timedelta(hours=1))
    )
    await db.commit()

    # User B: normal active user
    access_b, _ = await register(client, "b@example.com")
    h_b = auth_header(access_b)
    await client.post("/skills", json={"name": "B Skill"}, headers=h_b)

    # Run cleanup
    res = await client.post("/admin/cron/cleanup", headers=_cron_headers())
    assert res.json()["accounts_deleted"] == 1

    # User B is completely unaffected
    login_b = await client.post("/auth/login", json={"email": "b@example.com", "password": "password1"})
    assert login_b.status_code == 200
    new_access = login_b.json()["access_token"]
    skills = (await client.get("/skills", headers=auth_header(new_access))).json()
    assert len(skills) == 1
    assert skills[0]["name"] == "B Skill"

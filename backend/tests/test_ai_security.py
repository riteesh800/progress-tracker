from __future__ import annotations

from unittest.mock import patch

import pytest
from httpx import AsyncClient
from sqlalchemy import select

from app.models import Skill, Topic
from tests.conftest import auth_header, register


@pytest.mark.asyncio
async def test_account_isolation_ai_skills(client: AsyncClient, db):
    """User B must never see or access skills created by User A via AI."""
    token_a, _ = await register(client, email="user_iso_a@example.com")
    token_b, _ = await register(client, email="user_iso_b@example.com")

    # User A confirms an AI syllabus
    res_a = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Private AI Skill",
            "skill_description": "User A secret syllabus",
            "topics": [
                {
                    "name": "Secret Root Topic",
                    "children": [
                        {"name": "Secret Subtopic 1", "children": []},
                        {"name": "Secret Subtopic 2", "children": []},
                    ],
                }
            ],
        },
        headers=auth_header(token_a),
    )
    assert res_a.status_code == 201
    skill_a_id = res_a.json()["id"]

    # User B lists skills -> must NOT contain User A's skill
    res_b_list = await client.get("/skills", headers=auth_header(token_b))
    assert res_b_list.status_code == 200
    skills_b = res_b_list.json()
    assert all(s["id"] != skill_a_id for s in skills_b)

    # User B attempts direct GET of User A's skill -> must return 404
    res_b_get = await client.get(f"/skills/{skill_a_id}", headers=auth_header(token_b))
    assert res_b_get.status_code == 404

    # User B attempts to access topics of User A's skill -> must return 404
    res_b_topics = await client.get(f"/skills/{skill_a_id}/topics", headers=auth_header(token_b))
    assert res_b_topics.status_code == 404

    # User B checks recent activity -> must NOT see User A's AI activity
    res_b_act = await client.get("/activity", headers=auth_header(token_b))
    assert res_b_act.status_code == 200
    acts_b = res_b_act.json()
    assert all(a.get("skill_id") != skill_a_id for a in acts_b)

    # User B can create a skill with the identical name without conflict
    res_b_create = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Private AI Skill",
            "topics": [{"name": "User B Root Topic", "children": []}],
        },
        headers=auth_header(token_b),
    )
    assert res_b_create.status_code == 201
    assert res_b_create.json()["id"] != skill_a_id


@pytest.mark.asyncio
async def test_admin_security_unaffected(client: AsyncClient):
    """Regular user cannot access admin routes, and AI feature introduces no admin bypass."""
    regular_token, _ = await register(client, email="regular_student@example.com")

    # Regular user attempting admin users list -> 403 Forbidden
    res_admin = await client.get("/admin/users", headers=auth_header(regular_token))
    assert res_admin.status_code == 403

    # Regular user attempting admin cleanup -> 403 Forbidden
    res_clean = await client.post(
        "/admin/cleanup/expired-accounts",
        headers=auth_header(regular_token),
    )
    assert res_clean.status_code == 403

    # AI endpoints do not accept admin headers or elevate privileges
    res_ai = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Standard Student Skill",
            "topics": [{"name": "Basics", "children": []}],
        },
        headers={**auth_header(regular_token), "X-Admin-Session": "fake_admin_token"},
    )
    assert res_ai.status_code == 201
    # User remains a standard user; cannot access admin routes
    res_admin2 = await client.get("/admin/users", headers=auth_header(regular_token))
    assert res_admin2.status_code == 403


@pytest.mark.asyncio
async def test_atomic_rollback_on_persistence_failure(client: AsyncClient, db):
    """If inserting topics fails, transaction is completely rolled back; 0 skills or topics remain."""
    token, _ = await register(client, email="tx_fail@example.com")

    # Inject an unexpected failure during _persist_ai_nodes
    with patch("app.api.routes.ai._persist_ai_nodes", side_effect=RuntimeError("Simulated DB Disk Error")):
        res = await client.post(
            "/ai/confirm-syllabus",
            json={
                "skill_name": "Doomed Skill",
                "topics": [
                    {
                        "name": "Topic 1",
                        "children": [{"name": "Subtopic 1", "children": []}],
                    }
                ],
            },
            headers=auth_header(token),
        )
        assert res.status_code == 500

    # Verify zero skills were created in the database
    skills_in_db = (
        await db.execute(select(Skill).where(Skill.name == "Doomed Skill"))
    ).scalars().all()
    assert len(skills_in_db) == 0

    # Verify zero topics were created in the database
    topics_in_db = (
        await db.execute(select(Topic).where(Topic.name.in_(["Topic 1", "Subtopic 1"])))
    ).scalars().all()
    assert len(topics_in_db) == 0


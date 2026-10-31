from __future__ import annotations

import json
from unittest.mock import AsyncMock, patch

import pytest
from httpx import AsyncClient

from tests.conftest import auth_header, register


MOCK_VALID_SYLLABUS = {
    "skill_name": "Operating Systems",
    "topics": [
        {
            "name": "Introduction",
            "children": [
                {"name": "Definition and Purpose", "children": []},
                {
                    "name": "Operating System Types",
                    "children": [
                        {"name": "Batch OS", "children": []},
                        {"name": "Time-Sharing OS", "children": []},
                    ],
                },
            ],
        },
        {
            "name": "Process Management",
            "children": [
                {"name": "Process Concept and States", "children": []},
                {"name": "CPU Scheduling Algorithms", "children": []},
            ],
        },
    ],
}

MOCK_REFINED_SYLLABUS = {
    "skill_name": "Operating Systems",
    "topics": [
        {
            "name": "Introduction",
            "children": [
                {"name": "Definition and Purpose", "children": []},
            ],
        },
        {
            "name": "Process Management",
            "children": [
                {"name": "Process Concept and States", "children": []},
                {"name": "CPU Scheduling Algorithms", "children": []},
                {"name": "Deadlocks and Synchronization", "children": []},
            ],
        },
    ],
}


@pytest.mark.asyncio
async def test_generate_requires_auth(client: AsyncClient):
    res = await client.post(
        "/ai/generate-syllabus",
        json={"subject": "OS", "depth": "medium", "purpose": "interview"},
    )
    assert res.status_code == 401


@pytest.mark.asyncio
async def test_generate_validation_empty_subject(client: AsyncClient):
    token, _ = await register(client, email="val1@example.com")
    res = await client.post(
        "/ai/generate-syllabus",
        json={"subject": "   ", "depth": "medium", "purpose": "interview"},
        headers=auth_header(token),
    )
    assert res.status_code == 422


@pytest.mark.asyncio
async def test_generate_validation_invalid_depth(client: AsyncClient):
    token, _ = await register(client, email="val2@example.com")
    res = await client.post(
        "/ai/generate-syllabus",
        json={"subject": "OS", "depth": "super_hard", "purpose": "interview"},
        headers=auth_header(token),
    )
    assert res.status_code == 422


@pytest.mark.asyncio
async def test_generate_validation_invalid_purpose(client: AsyncClient):
    token, _ = await register(client, email="val3@example.com")
    res = await client.post(
        "/ai/generate-syllabus",
        json={"subject": "OS", "depth": "easy", "purpose": "just_for_fun"},
        headers=auth_header(token),
    )
    assert res.status_code == 422


@pytest.mark.asyncio
async def test_generate_missing_api_key(client: AsyncClient):
    token, _ = await register(client, email="nokey@example.com")
    with patch("app.services.gemini.settings.gemini_api_key", ""):
        res = await client.post(
            "/ai/generate-syllabus",
            json={"subject": "OS", "depth": "medium", "purpose": "interview"},
            headers=auth_header(token),
        )
        assert res.status_code == 503
        data = res.json()
        assert data["error"]["code"] == "ai_unavailable"


@pytest.mark.asyncio
async def test_generate_syllabus_success(client: AsyncClient):
    token, _ = await register(client, email="gen_success@example.com")
    with patch("app.services.gemini._call_gemini", new=AsyncMock(return_value=json.dumps(MOCK_VALID_SYLLABUS))):
        res = await client.post(
            "/ai/generate-syllabus",
            json={"subject": "Operating Systems", "depth": "medium", "purpose": "interview"},
            headers=auth_header(token),
        )
        assert res.status_code == 200
        data = res.json()
        assert data["skill_name"] == "Operating Systems"
        assert len(data["topics"]) == 2
        assert data["topics"][0]["name"] == "Introduction"
        assert len(data["topics"][0]["children"]) == 2


@pytest.mark.asyncio
async def test_generate_strips_numbering(client: AsyncClient):
    token, _ = await register(client, email="strip_num@example.com")
    numbered_syllabus = {
        "skill_name": "Python",
        "topics": [
            {
                "name": "1. Introduction to Python",
                "children": [
                    {"name": "1.1 Syntax & Variables", "children": []},
                    {"name": "1.2 Data Types", "children": []},
                ],
            }
        ],
    }
    with patch("app.services.gemini._call_gemini", new=AsyncMock(return_value=json.dumps(numbered_syllabus))):
        res = await client.post(
            "/ai/generate-syllabus",
            json={"subject": "Python", "depth": "easy", "purpose": "a_to_z"},
            headers=auth_header(token),
        )
        assert res.status_code == 200
        data = res.json()
        assert data["topics"][0]["name"] == "Introduction to Python"
        assert data["topics"][0]["children"][0]["name"] == "Syntax & Variables"
        assert data["topics"][0]["children"][1]["name"] == "Data Types"


@pytest.mark.asyncio
async def test_refine_syllabus_success(client: AsyncClient):
    token, _ = await register(client, email="refine_success@example.com")
    with patch("app.services.gemini._call_gemini", new=AsyncMock(return_value=json.dumps(MOCK_REFINED_SYLLABUS))):
        res = await client.post(
            "/ai/refine-syllabus",
            json={
                "subject": "Operating Systems",
                "depth": "medium",
                "purpose": "interview",
                "current_tree": MOCK_VALID_SYLLABUS,
                "conversation_history": [{"role": "user", "content": "Add deadlocks"}],
                "instruction": "Add deadlocks to process management",
            },
            headers=auth_header(token),
        )
        assert res.status_code == 200
        data = res.json()
        assert len(data["topics"][1]["children"]) == 3
        assert data["topics"][1]["children"][2]["name"] == "Deadlocks and Synchronization"


@pytest.mark.asyncio
async def test_confirm_syllabus_creates_skill_and_topics(client: AsyncClient):
    token, _ = await register(client, email="confirm_ok@example.com")
    res = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Operating Systems",
            "skill_description": "AI generated OS syllabus",
            "topics": MOCK_VALID_SYLLABUS["topics"],
        },
        headers=auth_header(token),
    )
    assert res.status_code == 201
    skill_data = res.json()
    assert skill_data["name"] == "Operating Systems"
    skill_id = skill_data["id"]

    # Verify topics were created under the skill
    topics_res = await client.get(f"/skills/{skill_id}/topics", headers=auth_header(token))
    assert topics_res.status_code == 200
    roots = topics_res.json()
    assert len(roots) == 2
    assert roots[0]["name"] == "Introduction"
    assert len(roots[0]["children"]) == 2

    # Verify activity was logged
    act_res = await client.get("/activity", headers=auth_header(token))
    assert act_res.status_code == 200
    acts = act_res.json()
    assert any(a["action_type"] == "ai_generated" for a in acts)


@pytest.mark.asyncio
async def test_confirm_duplicate_skill_name_rejected(client: AsyncClient):
    token, _ = await register(client, email="dupe_test@example.com")
    # First confirmation
    res1 = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "DBMS",
            "topics": [{"name": "SQL Basics", "children": []}],
        },
        headers=auth_header(token),
    )
    assert res1.status_code == 201

    # Second confirmation with same name
    res2 = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "DBMS",
            "topics": [{"name": "NoSQL Basics", "children": []}],
        },
        headers=auth_header(token),
    )
    assert res2.status_code == 409
    assert res2.json()["error"]["code"] == "skill_exists"


@pytest.mark.asyncio
async def test_confirm_empty_topics_rejected(client: AsyncClient):
    token, _ = await register(client, email="empty_tree@example.com")
    res = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Empty Skill",
            "topics": [],
        },
        headers=auth_header(token),
    )
    assert res.status_code == 400
    assert res.json()["error"]["code"] == "empty_tree"


@pytest.mark.asyncio
async def test_user_isolation(client: AsyncClient):
    token1, _ = await register(client, email="user1@example.com")
    token2, _ = await register(client, email="user2@example.com")

    # User 1 confirms skill
    res1 = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Same Name",
            "topics": [{"name": "Topic A", "children": []}],
        },
        headers=auth_header(token1),
    )
    assert res1.status_code == 201

    # User 2 can create skill with same name (isolated namespaces)
    res2 = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Same Name",
            "topics": [{"name": "Topic B", "children": []}],
        },
        headers=auth_header(token2),
    )
    assert res2.status_code == 201
    assert res1.json()["id"] != res2.json()["id"]


@pytest.mark.asyncio
async def test_generate_invalid_json_fails_gracefully(client: AsyncClient):
    token, _ = await register(client, email="invalid_json@example.com")
    with patch("app.services.gemini._call_gemini", new=AsyncMock(return_value="Sorry, I cannot help with that.")):
        res = await client.post(
            "/ai/generate-syllabus",
            json={"subject": "Math", "depth": "easy", "purpose": "quiz"},
            headers=auth_header(token),
        )
        assert res.status_code == 502
        data = res.json()
        assert data["error"]["code"] == "ai_invalid_response"


@pytest.mark.asyncio
async def test_generate_extracts_markdown_code_fence(client: AsyncClient):
    token, _ = await register(client, email="fence_test@example.com")
    wrapped_json = f"```json\n{json.dumps(MOCK_VALID_SYLLABUS)}\n```"
    with patch("app.services.gemini._call_gemini", new=AsyncMock(return_value=wrapped_json)):
        res = await client.post(
            "/ai/generate-syllabus",
            json={"subject": "Operating Systems", "depth": "medium", "purpose": "interview"},
            headers=auth_header(token),
        )
        assert res.status_code == 200
        data = res.json()
        assert data["skill_name"] == "Operating Systems"


@pytest.mark.asyncio
async def test_generate_deduplicates_duplicate_siblings(client: AsyncClient):
    token, _ = await register(client, email="dedup_test@example.com")
    dupe_syllabus = {
        "skill_name": "Networking",
        "topics": [
            {
                "name": "OSI Model",
                "children": [
                    {"name": "Physical Layer", "children": []},
                    {"name": "Physical Layer", "children": []},
                    {"name": "Data Link Layer", "children": []},
                ],
            }
        ],
    }
    with patch("app.services.gemini._call_gemini", new=AsyncMock(return_value=json.dumps(dupe_syllabus))):
        res = await client.post(
            "/ai/generate-syllabus",
            json={"subject": "Networking", "depth": "easy", "purpose": "quiz"},
            headers=auth_header(token),
        )
        assert res.status_code == 200
        data = res.json()
        children = data["topics"][0]["children"]
        assert len(children) == 2
        assert [c["name"] for c in children] == ["Physical Layer", "Data Link Layer"]


@pytest.mark.asyncio
async def test_gemini_quota_exceeded_error_handling(client: AsyncClient):
    token, _ = await register(client, email="quota_err@example.com")
    with patch("app.services.gemini._call_gemini", side_effect=Exception("429 Resource Exhausted: Quota exceeded")):
        res = await client.post(
            "/ai/generate-syllabus",
            json={"subject": "OS", "depth": "medium", "purpose": "interview"},
            headers=auth_header(token),
        )
        # Note: Exception in _call_gemini raises directly or via _handle_gemini_error
        # Here side_effect simulates what calling generate raises
        assert res.status_code in (429, 502)


@pytest.mark.asyncio
async def test_confirm_empty_topic_name_rejected(client: AsyncClient):
    token, _ = await register(client, email="empty_name@example.com")
    res = await client.post(
        "/ai/confirm-syllabus",
        json={
            "skill_name": "Testing",
            "topics": [{"name": "   ", "children": []}],
        },
        headers=auth_header(token),
    )
    assert res.status_code == 400
    assert res.json()["error"]["code"] == "empty_topic_name"

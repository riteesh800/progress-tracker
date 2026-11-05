from __future__ import annotations

import json
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

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


@pytest.mark.asyncio
async def test_gemini_fallback_on_model_not_found():
    from app.services.gemini import _call_gemini

    attempts = []

    def mock_generative_model(model_name, **kwargs):
        attempts.append(model_name)
        mock_instance = MagicMock()
        if "gemini-3.8-flash" in model_name:
            async def fail(*args, **kw):
                raise Exception("404 This model models/gemini-3.8-flash is no longer available.")
            mock_instance.generate_content_async = fail
        else:
            async def succeed(*args, **kw):
                res = MagicMock()
                res.text = json.dumps(MOCK_VALID_SYLLABUS)
                return res
            mock_instance.generate_content_async = succeed
        return mock_instance

    import google

    fake_genai = types.ModuleType("google.generativeai")
    fake_genai.configure = MagicMock()
    fake_genai.GenerationConfig = MagicMock()
    fake_genai.GenerativeModel = mock_generative_model

    with patch.dict(sys.modules, {"google.generativeai": fake_genai}), \
         patch.object(google, "generativeai", fake_genai, create=True), \
         patch("app.core.config.settings.gemini_api_key", "test-key"), \
         patch("app.core.config.settings.gemini_model", "gemini-3.8-flash"):
        raw = await _call_gemini("test prompt")
        assert raw is not None
        assert len(attempts) >= 2
        assert "gemini-3.8-flash" in attempts[0]
        assert "gemini-2.5-flash" in attempts[1]


@pytest.mark.asyncio
async def test_soft_deleted_skill_isolated_from_dashboard_search_topics_and_timer(client: AsyncClient):
    token, _ = await register(client, email="trash_isolation@example.com")
    # Create skill, topic, note, and timer
    s_res = await client.post(
        "/skills",
        json={"name": "TrashedRust", "description": "Rust programming"},
        headers=auth_header(token),
    )
    assert s_res.status_code == 201
    skill_id = s_res.json()["id"]

    t_res = await client.post(
        "/topics",
        json={"skill_id": skill_id, "name": "BorrowCheckerUnique"},
        headers=auth_header(token),
    )
    assert t_res.status_code == 201
    topic_id = t_res.json()["id"]

    n_res = await client.post(
        "/notes",
        json={"topic_id": topic_id, "content": "LifetimeAnnotationNoteUnique"},
        headers=auth_header(token),
    )
    assert n_res.status_code == 201

    tm_res = await client.post(
        f"/skills/{skill_id}/timer",
        json={"from_date": "2026-01-01", "to_date": "2026-02-01"},
        headers=auth_header(token),
    )
    assert tm_res.status_code == 200

    # Move skill to trash
    del_res = await client.delete(f"/skills/{skill_id}?confirm=true", headers=auth_header(token))
    assert del_res.status_code == 200

    # Dashboard must not count trashed skills or their leaf topics
    dash = (await client.get("/dashboard", headers=auth_header(token))).json()
    assert dash["total_skills"] == 0
    assert dash["total_topics"] == 0

    # Suggested-next must not suggest trashed skill/topic
    sug = (await client.get("/dashboard/suggested-next", headers=auth_header(token))).json()
    assert sug["skill_id"] is None
    assert sug["topic_id"] is None

    # Search must not return trashed skill, topic, or note
    for query in ("TrashedRust", "BorrowCheckerUnique", "LifetimeAnnotationNoteUnique"):
        s_hits = (await client.get(f"/search?q={query}", headers=auth_header(token))).json()
        assert s_hits == []

    # Topic, note, and timer endpoints must return 404 while skill is in trash
    assert (await client.get(f"/topics/{topic_id}", headers=auth_header(token))).status_code == 404
    assert (await client.post(f"/topics/{topic_id}/complete", json={"completed": True}, headers=auth_header(token))).status_code == 404
    assert (await client.get(f"/notes?topic_id={topic_id}", headers=auth_header(token))).status_code == 404
    assert (await client.get(f"/skills/{skill_id}/timer", headers=auth_header(token))).status_code == 404

    # Restoring skill makes everything accessible again
    rest_res = await client.post(f"/skills/{skill_id}/restore", headers=auth_header(token))
    assert rest_res.status_code == 200
    dash2 = (await client.get("/dashboard", headers=auth_header(token))).json()
    assert dash2["total_skills"] == 1
    assert dash2["total_topics"] == 1


@pytest.mark.asyncio
async def test_vercel_api_prefix_and_supabase_url_normalization(client: AsyncClient):
    from app.core.config import Settings

    # Verify /api/* prefix middleware works seamlessly for Vercel monorepo routing
    health_res = await client.get("/api/health")
    assert health_res.status_code == 200
    assert health_res.json() == {"status": "ok"}

    token, _ = await register(client, email="vercel_api_prefix@example.com")
    dash_res = await client.get("/api/dashboard", headers=auth_header(token))
    assert dash_res.status_code == 200
    assert dash_res.json()["total_skills"] == 0

    # Verify Supabase pooler URLs with ?pgbouncer=true&sslmode=require are normalized for asyncpg
    s = Settings(
        database_url="postgres://postgres.ref:secret@aws-0-ap-south-1.pooler.supabase.com:6543/postgres?pgbouncer=true&supa=base-pooler.x&sslmode=require"
    )
    assert s.async_database_url == (
        "postgresql+asyncpg://postgres.ref:secret@aws-0-ap-south-1.pooler.supabase.com:6543/postgres?ssl=require"
    )


@pytest.mark.asyncio
async def test_gemini_rest_thinking_level_and_fallback():
    from app.services.gemini import _call_gemini, _handle_gemini_error
    from app.core.exceptions import AppError

    calls: list[tuple[str, dict]] = []

    async def fake_post(self, url: str, *, headers=None, json=None, timeout=None):
        calls.append((url, json or {}))
        resp = MagicMock()
        if "gemini-3.8-flash:" in url:
            resp.status_code = 503
            resp.text = '{"error": {"code": 503, "message": "This model is currently experiencing high demand.", "status": "UNAVAILABLE"}}'
            resp.json.return_value = {
                "error": {"code": 503, "message": "This model is currently experiencing high demand.", "status": "UNAVAILABLE"}
            }
            return resp
        resp.status_code = 200
        resp.json.return_value = {
            "candidates": [
                {
                    "content": {
                        "parts": [
                            {"thought": True, "text": "internal reasoning"},
                            {"text": '{"skill_name":"OS","topics":[{"name":"Kernel"}]}'},
                        ]
                    },
                    "finishReason": "STOP",
                }
            ]
        }
        return resp

    with patch("httpx.AsyncClient.post", new=fake_post), \
         patch("app.core.config.settings.gemini_api_key", "test-rest-key"), \
         patch("app.core.config.settings.gemini_model", "gemini-2.5-flash"):
        raw = await _call_gemini("Generate OS syllabus")
        assert '"skill_name":"OS"' in raw
        assert len(calls) == 2
        # Retired gemini-2.5-flash is mapped directly to gemini-3.8-flash with thinkingLevel="LOW"
        assert "gemini-3.8-flash:generateContent" in calls[0][0]
        assert calls[0][1]["generationConfig"]["thinkingConfig"] == {"thinkingLevel": "LOW"}
        # On 503 from gemini-3.8-flash, falls back to gemini-3.5-flash-lite
        assert "gemini-3.5-flash-lite:generateContent" in calls[1][0]

    # Verify "generateContent" in 404 message does NOT falsely trigger 429 rate-limit due to "rate" substring
    with pytest.raises(AppError) as exc_info:
        _handle_gemini_error(
            RuntimeError(
                "404 NOT_FOUND: models/gemini-1.5-flash is not found for API version v1beta, or is not supported for generateContent."
            )
        )
    assert exc_info.value.status_code == 503
    assert exc_info.value.code == "ai_model_unavailable"


def test_truncated_json_auto_repair():
    from app.services.gemini import _extract_json, _validate_tree

    truncated = (
        '{"skill_name":"Distributed Systems","topics":['
        '{"name":"Consensus","children":[{"name":"Raft"},{"name":"Paxos"}]},'
        '{"name":"Replication","children":[{"name":"Leader-Follower"},{"name":"Multi-Leader'
    )
    extracted = _extract_json(truncated)
    validated = _validate_tree(extracted)
    assert validated["skill_name"] == "Distributed Systems"
    assert len(validated["topics"]) == 2
    assert validated["topics"][0]["name"] == "Consensus"
    assert [c["name"] for c in validated["topics"][0]["children"]] == ["Raft", "Paxos"]
    assert validated["topics"][1]["name"] == "Replication"
    assert [c["name"] for c in validated["topics"][1]["children"]] == ["Leader-Follower"]


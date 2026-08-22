from __future__ import annotations

from tests.conftest import auth_header, register


async def test_dashboard_aggregation_and_suggested_next(client):
    token, _ = await register(client)
    h = auth_header(token)
    empty = (await client.get("/dashboard", headers=h)).json()
    assert empty["empty"] is True
    assert empty["overall_progress"] == 0
    skill = (await client.post("/skills", json={"name": "DBMS"}, headers=h)).json()
    t1 = (await client.post("/topics", json={"skill_id": skill["id"], "name": "ER"}, headers=h)).json()
    t2 = (await client.post("/topics", json={"skill_id": skill["id"], "name": "SQL"}, headers=h)).json()
    await client.post(f"/topics/{t1['id']}/complete", json={"completed": True}, headers=h)
    dash = (await client.get("/dashboard", headers=h)).json()
    assert dash["total_skills"] == 1
    assert dash["completed_topics"] == 1
    assert dash["overall_progress"] == 50.0
    nxt = (await client.get("/dashboard/suggested-next", headers=h)).json()
    assert nxt["topic_id"] == t2["id"]
    feed = (await client.get("/activity", headers=h)).json()
    assert any(a["action_type"] == "skill_created" for a in feed)
    assert any(a["action_type"] == "topic_completed" for a in feed)

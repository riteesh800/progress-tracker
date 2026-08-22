from __future__ import annotations

from tests.conftest import auth_header, register


async def test_skill_list_preserves_creation_order(client):
    token, _ = await register(client)
    h = auth_header(token)
    await client.post("/skills", json={"name": "DSA"}, headers=h)
    await client.post("/skills", json={"name": "Operating Systems"}, headers=h)
    await client.post("/skills", json={"name": "DBMS"}, headers=h)
    await client.post("/skills", json={"name": "Computer Networks"}, headers=h)
    names = [s["name"] for s in (await client.get("/skills", headers=h)).json()]
    assert names == ["DSA", "Operating Systems", "DBMS", "Computer Networks"]


async def test_activity_summaries_and_limit(client):
    token, _ = await register(client)
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "DSA"}, headers=h)).json()
    parent = (await client.post("/topics", json={"skill_id": skill["id"], "name": "Search"}, headers=h)).json()
    leaf = (
        await client.post(
            "/topics",
            json={"skill_id": skill["id"], "parent_id": parent["id"], "name": "Binary Search"},
            headers=h,
        )
    ).json()
    await client.post(f"/topics/{leaf['id']}/complete", json={"completed": True}, headers=h)
    await client.post(f"/topics/{leaf['id']}/complete", json={"completed": False}, headers=h)
    await client.delete(f"/skills/{skill['id']}?confirm=true", headers=h)
    feed = (await client.get("/activity", headers=h)).json()
    summaries = [row["summary"] for row in feed]
    assert 'You added the skill "DSA"' in summaries
    assert 'You added "Binary Search" to "Search"' in summaries
    assert 'You completed "Binary Search" in "DSA"' in summaries
    assert 'You unmarked "Binary Search" in "DSA"' in summaries
    assert 'You deleted the skill "DSA"' in summaries
    assert all(row.get("summary") for row in feed)


async def test_skill_completed_only_on_transition(client):
    token, _ = await register(client)
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "Algo"}, headers=h)).json()
    a = (await client.post("/topics", json={"skill_id": skill["id"], "name": "A"}, headers=h)).json()
    b = (await client.post("/topics", json={"skill_id": skill["id"], "name": "B"}, headers=h)).json()
    await client.post(f"/topics/{a['id']}/complete", json={"completed": True}, headers=h)
    feed = (await client.get("/activity", headers=h)).json()
    assert not any(row["action_type"] == "skill_completed" for row in feed)
    await client.post(f"/topics/{b['id']}/complete", json={"completed": True}, headers=h)
    feed = (await client.get("/activity", headers=h)).json()
    assert sum(1 for row in feed if row["action_type"] == "skill_completed") == 1
    await client.post("/topics", json={"skill_id": skill["id"], "name": "C"}, headers=h)
    # Adding a child does not emit another skill_completed
    feed = (await client.get("/activity", headers=h)).json()
    assert sum(1 for row in feed if row["action_type"] == "skill_completed") == 1


async def test_activity_window_is_latest_twenty(client):
    token, _ = await register(client)
    h = auth_header(token)
    for i in range(25):
        await client.post("/skills", json={"name": f"S{i}"}, headers=h)
    feed = (await client.get("/activity", headers=h)).json()
    assert len(feed) == 20
    assert feed[0]["summary"] == 'You added the skill "S24"'
    assert feed[-1]["summary"] == 'You added the skill "S5"'


async def test_personal_notes_do_not_affect_streak(client):
    token, _ = await register(client)
    h = auth_header(token)
    created = (await client.post("/personal-notes", json={"title": "Plan"}, headers=h)).json()
    await client.patch(
        f"/personal-notes/{created['id']}",
        json={"title": "Plan", "tasks": [{"content": "Learn arrays", "is_completed": True}]},
        headers=h,
    )
    streak = (await client.get("/streak", headers=h)).json()
    assert streak["current_streak"] == 0
    feed = (await client.get("/activity", headers=h)).json()
    assert feed == []
    note = (await client.get(f"/personal-notes/{created['id']}", headers=h)).json()
    assert note["tasks"][0]["is_completed"] is True
    assert note["tasks"][0]["content"] == "Learn arrays"

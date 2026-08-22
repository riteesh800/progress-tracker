from __future__ import annotations

from uuid import UUID

from tests.conftest import auth_header, register
from app.models import Topic


async def test_worked_examples_part4(client, db):
    token, _ = await register(client)
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "P"}, headers=h)).json()
    sid = skill["id"]
    a = (await client.post("/topics", json={"skill_id": sid, "name": "A"}, headers=h)).json()
    b = (await client.post("/topics", json={"skill_id": sid, "parent_id": a["id"], "name": "B"}, headers=h)).json()
    c = (await client.post("/topics", json={"skill_id": sid, "parent_id": a["id"], "name": "C"}, headers=h)).json()
    d = (await client.post("/topics", json={"skill_id": sid, "parent_id": a["id"], "name": "D"}, headers=h)).json()
    await client.post(f"/topics/{b['id']}/complete", json={"completed": True}, headers=h)
    await client.post(f"/topics/{c['id']}/complete", json={"completed": True}, headers=h)
    tree = (await client.get(f"/skills/{sid}/topics", headers=h)).json()
    assert tree[0]["completed_leaves"] == 2
    assert tree[0]["total_leaves"] == 3
    assert tree[0]["percent"] == 66.67

    topic_d = await db.get(Topic, UUID(d["id"]))
    await db.delete(topic_d)
    await db.commit()
    tree = (await client.get(f"/skills/{sid}/topics", headers=h)).json()
    assert tree[0]["percent"] == 100.0
    assert tree[0]["completed_leaves"] == 2
    assert tree[0]["total_leaves"] == 2

    await client.post("/topics", json={"skill_id": sid, "parent_id": a["id"], "name": "E"}, headers=h)
    tree = (await client.get(f"/skills/{sid}/topics", headers=h)).json()
    assert tree[0]["percent"] == 66.67 or tree[0]["percent"] == 66.67
    assert tree[0]["total_leaves"] == 3
    assert tree[0]["completed_leaves"] == 2

    await client.post(f"/topics/{b['id']}/complete", json={"completed": True}, headers=h)
    await client.post(f"/topics/{c['id']}/complete", json={"completed": True}, headers=h)
    extra = (
        await client.post("/topics", json={"skill_id": sid, "parent_id": b["id"], "name": "B-child"}, headers=h)
    ).json()
    tree = (await client.get(f"/skills/{sid}/topics", headers=h)).json()
    b_in_tree = next(n for n in tree[0]["children"] if n["name"] == "B")
    assert b_in_tree["is_leaf"] is False
    assert len(b_in_tree["children"]) == 1
    assert b_in_tree["children"][0]["name"] == "B-child"
    assert b_in_tree["children"][0]["is_leaf"] is True
    assert b_in_tree["total_leaves"] == 1
    b_node = (await client.get(f"/topics/{b['id']}", headers=h)).json()
    assert b_node["is_leaf"] is False
    assert b_node["is_completed"] is False
    assert extra["is_leaf"] is True

    grandchild = (
        await client.post(
            "/topics",
            json={"skill_id": sid, "parent_id": extra["id"], "name": "B-grandchild"},
            headers=h,
        )
    ).json()
    tree = (await client.get(f"/skills/{sid}/topics", headers=h)).json()
    b_in_tree = next(n for n in tree[0]["children"] if n["name"] == "B")
    mid = b_in_tree["children"][0]
    assert mid["is_leaf"] is False
    assert mid["children"][0]["name"] == "B-grandchild"
    assert mid["children"][0]["is_leaf"] is True
    assert b_in_tree["total_leaves"] == 1
    assert grandchild["is_leaf"] is True

    empty = (await client.post("/skills", json={"name": "Empty"}, headers=h)).json()
    assert empty["empty"] is True
    assert empty["percent"] == 0


async def test_complete_idempotent(client):
    token, _ = await register(client)
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "P"}, headers=h)).json()
    t = (await client.post("/topics", json={"skill_id": skill["id"], "name": "Leaf"}, headers=h)).json()
    await client.post(f"/topics/{t['id']}/complete", json={"completed": True}, headers=h)
    await client.post(f"/topics/{t['id']}/complete", json={"completed": True}, headers=h)
    feed = (await client.get("/activity", headers=h)).json()
    completions = [a for a in feed if a["action_type"] == "topic_completed"]
    assert len(completions) == 1


async def test_uncomplete_lowers_progress_keeps_log(client):
    token, _ = await register(client)
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "P"}, headers=h)).json()
    t = (await client.post("/topics", json={"skill_id": skill["id"], "name": "Leaf"}, headers=h)).json()
    await client.post(f"/topics/{t['id']}/complete", json={"completed": True}, headers=h)
    await client.post(f"/topics/{t['id']}/complete", json={"completed": False}, headers=h)
    node = (await client.get(f"/topics/{t['id']}", headers=h)).json()
    assert node["is_completed"] is False
    assert node["percent"] == 0
    feed = (await client.get("/activity", headers=h)).json()
    assert any(a["action_type"] == "topic_completed" for a in feed)

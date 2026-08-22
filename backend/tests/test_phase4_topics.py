from __future__ import annotations

from tests.conftest import auth_header, register


async def _tree_setup(client):
    token, _ = await register(client)
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "Algo"}, headers=h)).json()
    sid = skill["id"]
    a = (await client.post("/topics", json={"skill_id": sid, "name": "A"}, headers=h)).json()
    b = (await client.post("/topics", json={"skill_id": sid, "parent_id": a["id"], "name": "B"}, headers=h)).json()
    c = (await client.post("/topics", json={"skill_id": sid, "parent_id": a["id"], "name": "C"}, headers=h)).json()
    return h, sid, a, b, c


async def test_leaf_detection_not_by_depth(client):
    h, sid, a, b, c = await _tree_setup(client)
    d = (
        await client.post(
            "/topics",
            json={"skill_id": sid, "parent_id": b["id"], "name": "B.1"},
            headers=h,
        )
    ).json()
    tree = (await client.get(f"/skills/{sid}/topics", headers=h)).json()
    a_node = tree[0]
    b_node = next(n for n in a_node["children"] if n["name"] == "B")
    c_node = next(n for n in a_node["children"] if n["name"] == "C")
    assert b_node["is_leaf"] is False
    assert c_node["is_leaf"] is True
    assert d["is_leaf"] is True


async def test_removed_topic_edit_endpoints_are_gone(client):
    h, sid, a, b, _c = await _tree_setup(client)
    assert (await client.post(f"/topics/{a['id']}/move", json={"new_parent_id": b["id"]}, headers=h)).status_code in (
        404,
        405,
    )
    assert (await client.patch(f"/topics/{a['id']}", json={"name": "Renamed"}, headers=h)).status_code in (404, 405)
    assert (await client.delete(f"/topics/{a['id']}", headers=h)).status_code in (404, 405)

from __future__ import annotations

from tests.conftest import auth_header, register


async def test_blank_note_rejected(client):
    token, _ = await register(client)
    h = auth_header(token)
    skill = (await client.post("/skills", json={"name": "P"}, headers=h)).json()
    t = (await client.post("/topics", json={"skill_id": skill["id"], "name": "Leaf"}, headers=h)).json()
    res = await client.post("/notes", json={"topic_id": t["id"], "content": "   "}, headers=h)
    assert res.status_code == 422
    ok = await client.post("/notes", json={"topic_id": t["id"], "content": "hello"}, headers=h)
    assert ok.status_code == 201

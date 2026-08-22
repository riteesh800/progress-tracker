from __future__ import annotations

from tests.conftest import auth_header, register


async def test_search_is_user_scoped(client):
    a, _ = await register(client, "a@example.com")
    b, _ = await register(client, "b@example.com")
    ha, hb = auth_header(a), auth_header(b)
    sa = (await client.post("/skills", json={"name": "Secret Graphs"}, headers=ha)).json()
    ta = (await client.post("/topics", json={"skill_id": sa["id"], "name": "Dijkstra"}, headers=ha)).json()
    await client.post("/notes", json={"topic_id": ta["id"], "content": "priority queue relaxation"}, headers=ha)
    await client.post("/skills", json={"name": "Public Math"}, headers=hb)
    hits_b = (await client.get("/search", params={"q": "Graphs"}, headers=hb)).json()
    assert hits_b == []
    hits_a = (await client.get("/search", params={"q": "Dijkstra"}, headers=ha)).json()
    assert any(h["title"] == "Dijkstra" for h in hits_a)
    notes = (await client.get("/search", params={"q": "relaxation"}, headers=ha)).json()
    assert any(h["kind"] == "note" for h in notes)
    notes_b = (await client.get("/search", params={"q": "relaxation"}, headers=hb)).json()
    assert notes_b == []

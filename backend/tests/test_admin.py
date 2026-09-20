from __future__ import annotations

from tests.conftest import auth_header, register

ADMIN_EMAIL = "riteeritee251@gmail.com"
ADMIN_CODE = "test-admin-access-ok"


def _admin_headers(access: str, admin_token: str) -> dict:
    return {**auth_header(access), "X-Admin-Session": admin_token}


async def test_normal_user_cannot_see_or_use_admin(client):
    access, _ = await register(client, "normal@example.com")
    me = (await client.get("/auth/me", headers=auth_header(access))).json()
    assert me["can_open_admin"] is False

    unlock = await client.post(
        "/admin/unlock",
        json={"access_code": ADMIN_CODE},
        headers=auth_header(access),
    )
    assert unlock.status_code == 403

    listed = await client.get("/admin/users", headers=auth_header(access))
    assert listed.status_code == 403


async def test_admin_unlock_and_list_users(client):
    admin_access, _ = await register(client, ADMIN_EMAIL)
    me = (await client.get("/auth/me", headers=auth_header(admin_access))).json()
    assert me["can_open_admin"] is True

    denied = await client.get("/admin/users", headers=auth_header(admin_access))
    assert denied.status_code == 401

    wrong = await client.post(
        "/admin/unlock",
        json={"access_code": "definitely-wrong-code"},
        headers=auth_header(admin_access),
    )
    assert wrong.status_code == 403

    unlocked = await client.post(
        "/admin/unlock",
        json={"access_code": ADMIN_CODE},
        headers=auth_header(admin_access),
    )
    assert unlocked.status_code == 200
    admin_token = unlocked.json()["admin_token"]
    assert admin_token
    assert "password" not in unlocked.json()
    assert "hash" not in str(unlocked.json()).lower()

    await register(client, "other@example.com")
    users = (
        await client.get("/admin/users", headers=_admin_headers(admin_access, admin_token))
    ).json()
    emails = {u["email"] for u in users}
    assert ADMIN_EMAIL in emails
    assert "other@example.com" in emails
    assert all("password" not in u for u in users)
    assert all("password_hash" not in u for u in users)


async def test_admin_change_password_only_affects_target(client):
    admin_access, _ = await register(client, ADMIN_EMAIL)
    user_a, _ = await register(client, "usera@example.com", password="alpha-pass")
    user_b, _ = await register(client, "userb@example.com", password="bravo-pass")
    unlocked = await client.post(
        "/admin/unlock",
        json={"access_code": ADMIN_CODE},
        headers=auth_header(admin_access),
    )
    admin_token = unlocked.json()["admin_token"]
    users = (
        await client.get("/admin/users", headers=_admin_headers(admin_access, admin_token))
    ).json()
    a_id = next(u["id"] for u in users if u["email"] == "usera@example.com")
    b_id = next(u["id"] for u in users if u["email"] == "userb@example.com")

    short = await client.post(
        f"/admin/users/{a_id}/password",
        json={"new_password": "short"},
        headers=_admin_headers(admin_access, admin_token),
    )
    assert short.status_code == 422

    changed = await client.post(
        f"/admin/users/{a_id}/password",
        json={"new_password": "newpass12"},
        headers=_admin_headers(admin_access, admin_token),
    )
    assert changed.status_code == 200

    old_a = await client.post("/auth/login", json={"email": "usera@example.com", "password": "alpha-pass"})
    assert old_a.status_code == 401
    new_a = await client.post("/auth/login", json={"email": "usera@example.com", "password": "newpass12"})
    assert new_a.status_code == 200
    still_b = await client.post("/auth/login", json={"email": "userb@example.com", "password": "bravo-pass"})
    assert still_b.status_code == 200

    # IDOR-style: normal user cannot change either account even with known ids
    normal, _ = await register(client, "intruder@example.com")
    probe = await client.post(
        f"/admin/users/{b_id}/password",
        json={"new_password": "hackedpass1"},
        headers=auth_header(normal),
    )
    assert probe.status_code == 403
    still_b2 = await client.post("/auth/login", json={"email": "userb@example.com", "password": "bravo-pass"})
    assert still_b2.status_code == 200


async def test_admin_password_forces_user_reset(client):
    admin_access, _ = await register(client, ADMIN_EMAIL)
    await register(client, "usera@example.com", password="alpha-pass")
    unlocked = await client.post(
        "/admin/unlock",
        json={"access_code": ADMIN_CODE},
        headers=auth_header(admin_access),
    )
    admin_token = unlocked.json()["admin_token"]
    users = (
        await client.get("/admin/users", headers=_admin_headers(admin_access, admin_token))
    ).json()
    a_id = next(u["id"] for u in users if u["email"] == "usera@example.com")
    await client.post(
        f"/admin/users/{a_id}/password",
        json={"new_password": "temp-pass1"},
        headers=_admin_headers(admin_access, admin_token),
    )
    login = await client.post("/auth/login", json={"email": "usera@example.com", "password": "temp-pass1"})
    assert login.status_code == 200
    token = login.json()["access_token"]
    me = (await client.get("/auth/me", headers=auth_header(token))).json()
    assert me["must_change_password"] is True
    blocked = await client.get("/skills", headers=auth_header(token))
    assert blocked.status_code == 403
    reused = await client.post(
        "/auth/change-password",
        json={"new_password": "temp-pass1"},
        headers=auth_header(token),
    )
    assert reused.status_code == 400
    changed = await client.post(
        "/auth/change-password",
        json={"new_password": "own-pass12"},
        headers=auth_header(token),
    )
    assert changed.status_code == 200
    assert changed.json()["must_change_password"] is False
    skills = await client.get("/skills", headers=auth_header(token))
    assert skills.status_code == 200
    old_temp = await client.post("/auth/login", json={"email": "usera@example.com", "password": "temp-pass1"})
    assert old_temp.status_code == 401
    own = await client.post("/auth/login", json={"email": "usera@example.com", "password": "own-pass12"})
    assert own.status_code == 200
    assert (await client.get("/auth/me", headers=auth_header(own.json()["access_token"]))).json()[
        "must_change_password"
    ] is False


async def test_change_password_rejected_without_admin_flag(client):
    access, _ = await register(client, "normalpw@example.com", password="keep-pass1")
    blocked = await client.post(
        "/auth/change-password",
        json={"new_password": "other-pass1"},
        headers=auth_header(access),
    )
    assert blocked.status_code == 403
    still = await client.post(
        "/auth/login",
        json={"email": "normalpw@example.com", "password": "keep-pass1"},
    )
    assert still.status_code == 200


async def test_deleted_account_disappears_from_admin_list(client):
    admin_access, _ = await register(client, ADMIN_EMAIL)
    gone_access, _ = await register(client, "gone@example.com")
    unlocked = await client.post(
        "/admin/unlock",
        json={"access_code": ADMIN_CODE},
        headers=auth_header(admin_access),
    )
    admin_token = unlocked.json()["admin_token"]
    before = (
        await client.get("/admin/users", headers=_admin_headers(admin_access, admin_token))
    ).json()
    assert "gone@example.com" in {u["email"] for u in before}
    deleted = await client.delete("/auth/me", headers=auth_header(gone_access))
    assert deleted.status_code == 200
    after = (
        await client.get("/admin/users", headers=_admin_headers(admin_access, admin_token))
    ).json()
    assert "gone@example.com" not in {u["email"] for u in after}

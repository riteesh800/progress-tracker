from __future__ import annotations

from tests.conftest import auth_header, register


async def test_register_login_refresh_logout(client):
    access, refresh = await register(client)
    me = await client.get("/auth/me", headers=auth_header(access))
    assert me.status_code == 200
    assert me.json()["email"] == "a@example.com"

    login = await client.post("/auth/login", json={"email": "a@example.com", "password": "password1"})
    assert login.status_code == 200

    rotated = await client.post("/auth/refresh", json={"refresh_token": refresh})
    assert rotated.status_code == 200
    new_refresh = rotated.json()["refresh_token"]
    # Immediate reuse is allowed so parallel 401 retries don't wipe the session.
    reuse = await client.post("/auth/refresh", json={"refresh_token": refresh})
    assert reuse.status_code == 200

    await client.post("/auth/logout", json={"refresh_token": new_refresh})
    after = await client.post("/auth/refresh", json={"refresh_token": new_refresh})
    assert after.status_code == 401


async def test_unauthenticated_rejected(client):
    res = await client.get("/skills")
    assert res.status_code == 401


async def test_owner_mismatch_is_404(client):
    a, _ = await register(client, "a@example.com")
    b, _ = await register(client, "b@example.com")
    created = await client.post("/skills", json={"name": "Mine"}, headers=auth_header(a))
    skill_id = created.json()["id"]
    other = await client.get(f"/skills/{skill_id}", headers=auth_header(b))
    assert other.status_code == 404


async def test_google_links_existing_email(client):
    await register(client, "linked@example.com")
    res = await client.post("/auth/google", json={"id_token": "test.linked@example.com", "timezone": "UTC"})
    assert res.status_code == 200
    me = await client.get("/auth/me", headers=auth_header(res.json()["access_token"]))
    assert me.json()["email"] == "linked@example.com"


async def test_forgot_reset_password(client):
    await register(client, "reset@example.com")
    unknown = await client.post("/auth/forgot-password", json={"email": "missing@example.com"})
    assert unknown.status_code == 404
    assert "not registered" in unknown.json()["error"]["message"]

    forgot = await client.post("/auth/forgot-password", json={"email": "reset@example.com"})
    assert forgot.status_code == 200
    code = forgot.json()["dev_code"]
    verify = await client.post(
        "/auth/verify-reset-code",
        json={"email": "reset@example.com", "code": code},
    )
    assert verify.status_code == 200
    bad = await client.post(
        "/auth/verify-reset-code",
        json={"email": "reset@example.com", "code": "000000"},
    )
    assert bad.status_code == 400
    reset = await client.post(
        "/auth/reset-password",
        json={"email": "reset@example.com", "code": code, "new_password": "newpass12"},
    )
    assert reset.status_code == 200
    reused = await client.post(
        "/auth/reset-password",
        json={"email": "reset@example.com", "code": code, "new_password": "another12"},
    )
    assert reused.status_code == 400
    login = await client.post("/auth/login", json={"email": "reset@example.com", "password": "newpass12"})
    assert login.status_code == 200


async def test_password_reset_revokes_refresh_tokens(client):
    access, refresh = await register(client, "session@example.com")
    forgot = await client.post("/auth/forgot-password", json={"email": "session@example.com"})
    code = forgot.json()["dev_code"]
    reset = await client.post(
        "/auth/reset-password",
        json={"email": "session@example.com", "code": code, "new_password": "newpass12"},
    )
    assert reset.status_code == 200
    reused = await client.post("/auth/refresh", json={"refresh_token": refresh})
    assert reused.status_code == 401


def test_google_test_tokens_rejected_outside_test_env():
    from app.api.routes.auth import _verify_google
    from app.core.config import settings
    from app.core.exceptions import AppError

    old = settings.environment
    settings.environment = "production"
    try:
        try:
            _verify_google("test.victim@example.com")
            raise AssertionError("test. tokens must not work in production")
        except AppError as exc:
            assert exc.status_code in (401, 503)
    finally:
        settings.environment = old


def test_production_rejects_default_jwt_secret():
    from app.core.config import Settings

    cfg = Settings.model_construct(environment="production", jwt_secret="dev-access-secret-change-me")
    try:
        cfg.assert_production_secrets()
        raise AssertionError("weak JWT secret must be rejected")
    except RuntimeError:
        pass


async def test_delete_account_with_skills(client):
    token, _ = await register(client, "gone@example.com")
    h = auth_header(token)
    await client.post("/skills", json={"name": "DSA"}, headers=h)
    res = await client.delete("/auth/me", headers=h)
    assert res.status_code == 200
    login = await client.post("/auth/login", json={"email": "gone@example.com", "password": "password1"})
    assert login.status_code == 401

from __future__ import annotations

import hmac

from app.core.config import settings


def is_admin_email(email: str | None) -> bool:
    if not email:
        return False
    return email.strip().lower() == settings.admin_email.strip().lower()


def admin_access_configured() -> bool:
    return len(settings.admin_access_code or "") >= 12


def verify_admin_access_code(provided: str) -> bool:
    expected = settings.admin_access_code or ""
    if len(expected) < 12:
        return False
    try:
        return hmac.compare_digest(provided.encode("utf-8"), expected.encode("utf-8"))
    except (TypeError, ValueError):
        return False

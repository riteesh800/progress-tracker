from __future__ import annotations

import time
from collections import defaultdict, deque

from fastapi import Request
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.util import get_remote_address

from app.core.config import settings
from app.core.exceptions import AppError


def get_client_ip(request: Request) -> str:
    """Extract client IP safely behind Vercel / reverse proxies."""
    forwarded_for = request.headers.get("x-forwarded-for")
    if forwarded_for:
        first_ip = forwarded_for.split(",")[0].strip()
        if first_ip:
            return first_ip
    real_ip = request.headers.get("x-real-ip")
    if real_ip:
        return real_ip.strip()
    return get_remote_address(request) or "127.0.0.1"


limiter = Limiter(
    key_func=get_client_ip,
    default_limits=["120/minute"],
    enabled=settings.environment != "test",
)
rate_limit_exceeded_handler = _rate_limit_exceeded_handler

_hits: dict[str, deque[float]] = defaultdict(deque)


def enforce_limit(request: Request, key: str, max_hits: int, window_seconds: int) -> None:
    if settings.environment == "test" or not limiter.enabled:
        return
    ident = f"{key}:{get_client_ip(request)}"
    now = time.time()
    if len(_hits) > 5000:
        stale_keys = [k for k, dq in _hits.items() if not dq or now - dq[-1] > window_seconds]
        for k in stale_keys:
            _hits.pop(k, None)
    q = _hits[ident]
    while q and now - q[0] > window_seconds:
        q.popleft()
    if len(q) >= max_hits:
        raise AppError(429, "rate_limited", "Too many requests. Please wait and try again.")
    q.append(now)

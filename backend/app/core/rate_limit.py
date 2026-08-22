from __future__ import annotations

import time
from collections import defaultdict, deque

from fastapi import Request
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.util import get_remote_address

from app.core.config import settings
from app.core.exceptions import AppError

limiter = Limiter(
    key_func=get_remote_address,
    default_limits=["120/minute"],
    enabled=settings.environment != "test",
)
rate_limit_exceeded_handler = _rate_limit_exceeded_handler

_hits: dict[str, deque[float]] = defaultdict(deque)


def enforce_limit(request: Request, key: str, max_hits: int, window_seconds: int) -> None:
    if settings.environment == "test" or not limiter.enabled:
        return
    ident = f"{key}:{get_remote_address(request)}"
    now = time.time()
    q = _hits[ident]
    while q and now - q[0] > window_seconds:
        q.popleft()
    if len(q) >= max_hits:
        raise AppError(429, "rate_limited", "Too many requests. Please wait and try again.")
    q.append(now)

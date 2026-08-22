from __future__ import annotations

import logging
import sys

SENSITIVE_KEYS = {"password", "token", "secret", "authorization", "api_key", "refresh_token"}


def get_logger(name: str) -> logging.Logger:
    logger = logging.getLogger(name)
    if not logger.handlers:
        handler = logging.StreamHandler(sys.stdout)
        handler.setFormatter(
            logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s")
        )
        logger.addHandler(handler)
        logger.setLevel(logging.INFO)
    return logger


def redact(data: dict) -> dict:
    out = {}
    for k, v in data.items():
        if any(s in k.lower() for s in SENSITIVE_KEYS):
            out[k] = "[redacted]"
        else:
            out[k] = v
    return out

#!/usr/bin/env python3
"""
Secure internal bridge between the Go backend and the Python PDF parser.
Uses the self-contained `pdf_parser.py` in `backend-go/pybridge/` (which is an exact
copy of `backend/app/services/pdf_parser.py`) so Vercel deployments do not depend on
a sibling `backend/` directory.
"""

from __future__ import annotations

import hashlib
import hmac
import io
import json
import os
import sys
from pathlib import Path
from typing import Any

_THIS_DIR = Path(__file__).resolve().parent
if str(_THIS_DIR) not in sys.path:
    sys.path.insert(0, str(_THIS_DIR))

from pypdf import PdfReader  # noqa: E402
from pdf_parser import parse_syllabus_text  # noqa: E402


def extract_and_parse_pdf(
    raw_bytes: bytes,
    max_pdf_pages: int = 200,
    anthropic_api_key: str = "",
) -> dict[str, Any]:
    if not raw_bytes or not raw_bytes.startswith(b"%PDF"):
        return {
            "ok": False,
            "error_code": "invalid_pdf",
            "error_message": "File is not a valid PDF (content-type/magic mismatch).",
        }

    try:
        reader = PdfReader(io.BytesIO(raw_bytes))
        pages = len(reader.pages)
    except Exception:
        return {
            "ok": False,
            "error_message": "Parsing failed. The file was not stored as a skill.",
        }

    if pages > max_pdf_pages:
        return {
            "ok": False,
            "error_message": f"PDF exceeds the {max_pdf_pages}-page limit.",
        }

    chunks: list[str] = []
    for page in reader.pages:
        try:
            chunks.append(page.extract_text() or "")
        except Exception:
            chunks.append("")
    text = "\n".join(chunks)

    if not text.strip():
        return {
            "ok": False,
            "error_message": (
                "No extractable text found. This looks like a scanned or image-only PDF. "
                "Export or print to a text-based PDF and try again. OCR is not supported."
            ),
        }

    try:
        tree = parse_syllabus_text(text)
    except Exception:
        return {
            "ok": False,
            "error_message": "Parsing failed. The file was not stored as a skill.",
        }

    sections = tree.get("low_confidence_sections") or []
    if sections:
        if not anthropic_api_key:
            tree.setdefault("confidence_notes", {})
            tree["confidence_notes"]["ai_skipped"] = (
                "ANTHROPIC_API_KEY unset; low-confidence sections flagged for manual review."
            )
        else:
            try:
                import anthropic

                client = anthropic.Anthropic(api_key=anthropic_api_key)
                prompt = (
                    'Return ONLY JSON: {"children": [{"name": str, "children": [...]}]}. '
                    "Build a topic hierarchy from this syllabus excerpt:\n\n"
                    + "\n\n---\n\n".join(sections[:8])
                )
                msg = client.messages.create(
                    model="claude-3-5-haiku-latest",
                    max_tokens=2000,
                    messages=[{"role": "user", "content": prompt}],
                )
                raw = msg.content[0].text
                start = raw.find("{")
                end = raw.rfind("}") + 1
                parsed = json.loads(raw[start:end])
                extra = parsed.get("children") or []
                if isinstance(extra, list):
                    tree.setdefault("children", []).extend(extra)
                tree["confidence_notes"] = {"ai_merged": True}
            except Exception:
                tree.setdefault("confidence_notes", {})
                tree["confidence_notes"]["ai_error"] = "AI fallback failed; review low-confidence nodes."

    return {
        "ok": True,
        "tree": tree,
    }


def main() -> int:
    raw_bytes = sys.stdin.buffer.read()

    secret = (
        os.environ.get("INTERNAL_PARSER_SECRET")
        or os.environ.get("BRIDGE_HMAC_SECRET")
        or ""
    ).strip()
    expected_sig = (os.environ.get("BRIDGE_HMAC_SIGNATURE") or "").strip()
    if not secret or not expected_sig:
        sys.stdout.write(
            json.dumps(
                {
                    "ok": False,
                    "error_message": "Unauthorized internal parser invocation.",
                }
            )
        )
        return 0

    computed = hmac.new(secret.encode("utf-8"), raw_bytes, hashlib.sha256).hexdigest()
    if not hmac.compare_digest(computed, expected_sig):
        sys.stdout.write(
            json.dumps(
                {
                    "ok": False,
                    "error_message": "Unauthorized internal parser invocation.",
                }
            )
        )
        return 0

    max_pdf_pages = int(os.environ.get("BRIDGE_MAX_PDF_PAGES", "200"))
    anthropic_api_key = os.environ.get("ANTHROPIC_API_KEY", "")

    result = extract_and_parse_pdf(
        raw_bytes=raw_bytes,
        max_pdf_pages=max_pdf_pages,
        anthropic_api_key=anthropic_api_key,
    )
    sys.stdout.write(json.dumps(result))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

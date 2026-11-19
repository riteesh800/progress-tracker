"""
Vercel internal Python serverless endpoint for PDF text extraction and syllabus parsing.
Protected by mandatory HMAC-SHA256 signature (using dedicated INTERNAL_PARSER_SECRET)
and a 60-second replay window. Supports both storage-based signed URL fetching (for PDFs
up to 10 MB without passing PDF bytes through Vercel function request bodies) and direct
binary PDF payloads.
"""

from __future__ import annotations

import hashlib
import hmac
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler
from pathlib import Path

_THIS_DIR = Path(__file__).resolve().parent
_BACKEND_GO_DIR = _THIS_DIR.parent
_PYBRIDGE_DIR = _BACKEND_GO_DIR / "pybridge"
for _p in (
    str(_BACKEND_GO_DIR),
    str(_PYBRIDGE_DIR),
    str(_BACKEND_GO_DIR / "backend-go"),
    str(_BACKEND_GO_DIR / "backend-go" / "pybridge"),
):
    if _p not in sys.path:
        sys.path.insert(0, _p)

try:
    from pybridge.pdf_bridge import extract_and_parse_pdf  # noqa: E402
except ImportError:
    from pdf_bridge import extract_and_parse_pdf  # noqa: E402


class _NoRedirectHandler(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # noqa: ANN001, D401
        raise urllib.error.HTTPError(req.full_url, code, "Redirects are not permitted for storage downloads.", headers, fp)


def _validate_storage_download_url(download_url: str) -> bool:
    """Ensure download_url points strictly to the configured private storage origin (anti-SSRF)."""
    try:
        parsed = urllib.parse.urlparse(download_url)
    except Exception:
        return False

    if parsed.scheme not in ("https", "http"):
        return False
    if not parsed.netloc or "@" in parsed.netloc:
        return False
    if ".." in parsed.path or "\\" in parsed.path:
        return False

    supabase_url = (os.environ.get("SUPABASE_URL") or "").strip().rstrip("/")
    if supabase_url:
        try:
            supa_parsed = urllib.parse.urlparse(supabase_url)
        except Exception:
            return False
        if parsed.scheme != supa_parsed.scheme or parsed.netloc.lower() != supa_parsed.netloc.lower():
            return False
        if not parsed.path.startswith("/storage/v1/object/sign/"):
            return False
        return True

    # In local/test mode without SUPABASE_URL, only allow loopback test storage server if ENVIRONMENT=test
    env = (os.environ.get("ENVIRONMENT") or "").strip().lower()
    host = (parsed.hostname or "").lower()
    if env == "test" and host in ("127.0.0.1", "localhost") and parsed.path.startswith("/storage/v1/object/sign/"):
        return True

    return False


def _fetch_pdf_from_storage(download_url: str, max_bytes: int) -> tuple[bytes | None, dict | None]:
    if not _validate_storage_download_url(download_url):
        return None, {
            "ok": False,
            "error_code": "forbidden_storage_url",
            "error_message": "Storage download URL is not permitted.",
        }

    opener = urllib.request.build_opener(_NoRedirectHandler())
    req = urllib.request.Request(download_url, method="GET")
    try:
        with opener.open(req, timeout=25) as resp:
            data = resp.read(max_bytes + 1)
    except Exception:
        return None, {
            "ok": False,
            "error_code": "storage_fetch_failed",
            "error_message": "Could not retrieve uploaded PDF from private storage.",
        }

    if not data:
        return None, {
            "ok": False,
            "error_code": "invalid_pdf",
            "error_message": "Uploaded PDF is empty.",
        }
    if len(data) > max_bytes:
        return None, {
            "ok": False,
            "error_code": "pdf_too_large",
            "error_message": "PDF exceeds the 10 MB limit.",
        }
    if not data.startswith(b"%PDF"):
        return None, {
            "ok": False,
            "error_code": "invalid_pdf",
            "error_message": "File is not a valid PDF (content-type/magic mismatch).",
        }
    return data, None


class handler(BaseHTTPRequestHandler):
    def _send_json(self, status_code: int, payload: dict) -> None:
        raw = json.dumps(payload).encode("utf-8")
        self.send_response(status_code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_POST(self) -> None:  # noqa: N802
        # Strictly require dedicated INTERNAL_PARSER_SECRET; never fall back to JWT/CLEANUP/CRON secrets.
        secret = (os.environ.get("INTERNAL_PARSER_SECRET") or "").strip()
        if not secret:
            self._send_json(503, {"ok": False, "error_message": "Internal parser secret is not configured."})
            return

        ts_str = (self.headers.get("X-Internal-Bridge-Timestamp") or "").strip()
        sig = (self.headers.get("X-Internal-Bridge-Signature") or "").strip()
        if not ts_str or not sig:
            self._send_json(403, {"ok": False, "error_message": "Missing internal bridge authentication headers."})
            return

        try:
            ts = int(ts_str)
        except ValueError:
            self._send_json(403, {"ok": False, "error_message": "Invalid internal bridge timestamp."})
            return

        if abs(int(time.time()) - ts) > 60:
            self._send_json(403, {"ok": False, "error_message": "Internal bridge request timestamp expired."})
            return

        max_bytes = int(os.environ.get("MAX_PDF_BYTES", str(10 * 1024 * 1024)))
        content_length = int(self.headers.get("Content-Length") or "0")
        if content_length <= 0 or content_length > max_bytes:
            self._send_json(400, {"ok": False, "error_code": "pdf_too_large", "error_message": "PDF exceeds the 10 MB limit."})
            return

        raw_body = self.rfile.read(content_length)
        mac_input = ts_str.encode("utf-8") + b"." + raw_body
        expected_sig = hmac.new(secret.encode("utf-8"), mac_input, hashlib.sha256).hexdigest()
        if not hmac.compare_digest(expected_sig, sig):
            self._send_json(403, {"ok": False, "error_message": "Invalid internal bridge signature."})
            return

        content_type = (self.headers.get("Content-Type") or "").lower()
        if "application/json" in content_type:
            try:
                payload = json.loads(raw_body.decode("utf-8"))
            except Exception:
                self._send_json(400, {"ok": False, "error_message": "Invalid JSON payload."})
                return

            download_url = str(payload.get("download_url") or "").strip()
            req_max = int(payload.get("max_bytes") or max_bytes)
            effective_max = min(max_bytes, req_max) if req_max > 0 else max_bytes

            raw_pdf, err_resp = _fetch_pdf_from_storage(download_url, effective_max)
            if err_resp is not None or raw_pdf is None:
                self._send_json(400, err_resp or {"ok": False, "error_message": "Failed to fetch PDF from storage."})
                return
        else:
            raw_pdf = raw_body
            if len(raw_pdf) > max_bytes:
                self._send_json(400, {"ok": False, "error_code": "pdf_too_large", "error_message": "PDF exceeds the 10 MB limit."})
                return
            if not raw_pdf.startswith(b"%PDF"):
                self._send_json(400, {"ok": False, "error_code": "invalid_pdf", "error_message": "File is not a valid PDF (content-type/magic mismatch)."})
                return

        result = extract_and_parse_pdf(
            raw_bytes=raw_pdf,
            max_pdf_pages=int(os.environ.get("MAX_PDF_PAGES", "200")),
            anthropic_api_key=os.environ.get("ANTHROPIC_API_KEY", ""),
        )
        self._send_json(200, result)

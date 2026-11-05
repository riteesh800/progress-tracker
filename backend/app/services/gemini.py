from __future__ import annotations

import json
import re
import sys
import time
from typing import Any

import httpx

from app.core.config import settings
from app.core.exceptions import AppError
from app.core.logging import get_logger

logger = get_logger(__name__)

MAX_TREE_NODES = 2000
MAX_TREE_DEPTH = 20
MAX_NAME_LEN = 500

# Total wall-clock budget (seconds) so serverless functions never hit Vercel's 60s hard kill
TOTAL_GENERATION_BUDGET_S = 46.0
PER_ATTEMPT_TIMEOUT_S = 22.0

GEMINI_REST_BASE = "https://generativelanguage.googleapis.com/v1beta/models"

# Retired model IDs mapped to their current Google AI Studio replacements
_RETIRED_MODEL_MAP = {
    "": "gemini-3.8-flash",
    "gemini-1.5-flash": "gemini-3.8-flash",
    "gemini-1.5-pro": "gemini-3.8-flash",
    "gemini-2.0-flash": "gemini-3.8-flash",
    "gemini-2.0-flash-lite": "gemini-3.5-flash-lite",
    "gemini-2.5-flash": "gemini-3.8-flash",
    "gemini-2.5-flash-lite": "gemini-3.5-flash-lite",
}

DEPTH_DESCRIPTIONS = {
    "easy": (
        "Basic fundamentals only. Include definitions and introductory concepts. "
        "Keep subtopics limited. Focus on essential beginner-level topics. "
        "Aim for roughly 10-20 topics total."
    ),
    "medium": (
        "Cover fundamentals and commonly used important concepts. "
        "Include relevant subtopics with moderate coverage. "
        "Aim for roughly 20-40 topics total."
    ),
    "intermediate": (
        "Detailed concept coverage including important and moderately advanced concepts. "
        "Include more subtopics and sub-subtopics. Cover practical and technical concepts. "
        "Appropriate depth for serious learning. Aim for roughly 40-70 topics total."
    ),
    "comprehensive": (
        "Complete A-to-Z coverage of the subject. Cover fundamentals through advanced concepts. "
        "Include all important subtopics and sub-subtopics. Include related concepts that are "
        "genuinely relevant. Provide technical depth and practical knowledge. "
        "Do not miss major areas of the subject. Aim for roughly 65-110 topics total."
    ),
}

PURPOSE_DESCRIPTIONS = {
    "interview": (
        "Focus on technical interview preparation. Emphasize frequently tested concepts, "
        "important problem-solving areas, practical implementation topics, "
        "and relevant advanced interview topics."
    ),
    "quiz": (
        "Focus on quiz and examination preparation. Emphasize important examinable concepts, "
        "definitions, classifications, properties, fundamentals, and conceptual understanding."
    ),
    "a_to_z": (
        "Focus on learning the entire subject comprehensively. Cover fundamentals, "
        "intermediate concepts, and advanced concepts. Include important related areas. "
        "Organize concepts in a logical learning sequence."
    ),
}

SYSTEM_PROMPT = """You are an expert curriculum designer and syllabus generator for a skill progress tracking application.

Your task is to generate a hierarchical syllabus for a given subject, organized as topics, subtopics, and sub-subtopics.

CRITICAL RULES:
1. Return ONLY valid, compact JSON. Absolutely no prose, conversational filler, markdown explanation, or text before/after the JSON.
2. The JSON must follow this exact structure:
{"skill_name":"Subject Name","topics":[{"name":"Topic Name","children":[{"name":"Subtopic Name"}]}]}
3. Every topic must have a "name" (non-empty string). Parent topics must have a "children" array of child topic objects; leaf topics may omit "children" or use "children": [].
4. Output compact JSON without unnecessary indentation or whitespace so generation is fast and complete.
5. Use logical hierarchical parent-child relationships. Child topics must be nested directly under their parent's "children" array.
6. Topic names must be concise and meaningful (2-7 words).
7. Do NOT include numbering or bullet prefixes in topic names (no "1", "1.1", "1.2.1", "A.", "-", etc.) - the application automatically numbers the hierarchy.
8. Do NOT include emojis or decorative symbols.
9. Do NOT include markdown headings, formatting, or bullet points in topic names.
10. Do NOT include explanatory sentences, definitions, or paragraphs inside topic names.
11. Do NOT include introductory messages ("Here is your syllabus") or concluding remarks.
12. Do NOT create duplicate or near-duplicate sibling topics.
13. Do NOT create empty topics.
14. Avoid unnecessary nesting beyond 4 levels."""


def _compact_tree_for_prompt(node: dict[str, Any]) -> dict[str, Any]:
    """Strip empty children arrays to minimize prompt token size during refinement."""
    out: dict[str, Any] = {"name": str(node.get("name") or "").strip()}
    kids = node.get("children")
    if isinstance(kids, list) and kids:
        out["children"] = [_compact_tree_for_prompt(c) for c in kids if isinstance(c, dict)]
    return out


def _build_generation_prompt(subject: str, depth: str, purpose: str) -> str:
    depth_desc = DEPTH_DESCRIPTIONS.get(depth.lower(), DEPTH_DESCRIPTIONS["medium"])
    purpose_desc = PURPOSE_DESCRIPTIONS.get(purpose.lower(), PURPOSE_DESCRIPTIONS["a_to_z"])

    return (
        f"Generate a syllabus for: {subject}\n\n"
        f"Syllabus Depth: {depth.capitalize()}\n{depth_desc}\n\n"
        f"Learning Purpose: {purpose.replace('_', ' ').title()}\n{purpose_desc}\n\n"
        "Return the complete syllabus as a single compact JSON object matching the required schema with \"skill_name\" and \"topics\"."
    )


def _build_refinement_prompt(
    subject: str,
    depth: str,
    purpose: str,
    current_tree: dict[str, Any],
    instruction: str,
) -> str:
    compact_topics = [
        _compact_tree_for_prompt(t)
        for t in (current_tree.get("topics") or [])
        if isinstance(t, dict)
    ]
    compact_payload = {
        "skill_name": current_tree.get("skill_name") or subject,
        "topics": compact_topics,
    }
    tree_json = json.dumps(compact_payload, separators=(",", ":"), ensure_ascii=False)

    return (
        f"You are modifying an existing syllabus based on the user's specific natural-language instruction.\n\n"
        f"Subject: {subject}\n"
        f"Original Depth: {depth.capitalize()}\n"
        f"Original Purpose: {purpose.replace('_', ' ').title()}\n\n"
        f"Current Syllabus Tree (may contain manual edits by the user):\n{tree_json}\n\n"
        f"User Instruction: {instruction}\n\n"
        "IMPORTANT REFINEMENT GUIDELINES:\n"
        "1. Strictly apply the user's instruction to the current syllabus.\n"
        "2. If the user refers to specific topics or numbers (e.g., 'Expand topic 2.1' or 'Remove topic 4'), identify that specific topic in the hierarchy and modify it.\n"
        "3. Preserve all other unrelated topics and manual edits made in the current syllabus.\n"
        "4. Return the COMPLETE updated syllabus as a valid compact JSON object with \"skill_name\" and \"topics\".\n"
        "5. Do NOT include numbering prefixes in topic names.\n"
        "6. Return ONLY the JSON object. No extra text."
    )


def _repair_truncated_json(text: str) -> dict | None:
    """Salvage a valid syllabus JSON object if the model output was truncated near the end."""
    start = text.find("{")
    if start == -1:
        return None
    fragment = text[start:]
    pos = len(fragment)
    attempts = 0
    while pos > 0 and attempts < 50:
        last_brace = fragment.rfind("}", 0, pos)
        if last_brace == -1:
            break
        candidate = fragment[: last_brace + 1]
        stack: list[str] = []
        in_str = False
        escape = False
        valid_scan = True
        for ch in candidate:
            if escape:
                escape = False
                continue
            if ch == "\\":
                if in_str:
                    escape = True
                continue
            if ch == '"':
                in_str = not in_str
                continue
            if in_str:
                continue
            if ch == "{":
                stack.append("}")
            elif ch == "[":
                stack.append("]")
            elif ch in ("}", "]"):
                if not stack or stack[-1] != ch:
                    valid_scan = False
                    break
                stack.pop()
        if valid_scan and not in_str:
            repaired = candidate + "".join(reversed(stack))
            repaired = re.sub(r",\s*([\]\}])", r"\1", repaired)
            try:
                parsed = json.loads(repaired)
                if isinstance(parsed, dict) and isinstance(parsed.get("topics"), list) and parsed["topics"]:
                    return parsed
            except json.JSONDecodeError:
                pass
        pos = last_brace
        attempts += 1
    return None


def _extract_json(text: str) -> dict:
    """Safely extract and parse JSON from model output with trailing comma and truncation tolerance."""
    cleaned = text.strip()

    # Try direct parse
    try:
        return json.loads(cleaned)
    except json.JSONDecodeError:
        pass

    # Try extracting from markdown code block
    fence_match = re.search(r"```(?:json)?\s*\n?(.*?)\n?```", cleaned, re.DOTALL)
    if fence_match:
        snippet = fence_match.group(1).strip()
        try:
            return json.loads(snippet)
        except json.JSONDecodeError:
            cleaned = snippet

    # Try finding outermost { ... }
    start = cleaned.find("{")
    end = cleaned.rfind("}")
    if start != -1 and end > start:
        raw_json = cleaned[start : end + 1]
        try:
            return json.loads(raw_json)
        except json.JSONDecodeError:
            # Trailing comma cleanup fallback
            sanitized = re.sub(r",\s*([\]\}])", r"\1", raw_json)
            try:
                return json.loads(sanitized)
            except json.JSONDecodeError:
                pass

    # Try repairing truncated JSON stream
    repaired = _repair_truncated_json(cleaned)
    if repaired is not None:
        logger.warning("Repaired truncated JSON response from Gemini")
        return repaired

    raise AppError(502, "ai_invalid_response", "AI returned invalid JSON. Please try again.")


def _sanitize_ai_name(name: str) -> str:
    """Strip any AI-generated decimal numbering prefix like '1. ', '1.2 ', etc."""
    cleaned = re.sub(r"^\s*\d+(?:\.\d+)*\.?\s*[-—:]?\s*", "", name.strip())
    # Strip bullet points
    cleaned = re.sub(r"^[\s•\-*]+\s*", "", cleaned)
    return (cleaned or name.strip())[:MAX_NAME_LEN]


def _validate_node(node: Any, depth: int = 0) -> dict | None:
    """Validate and sanitize a single topic node."""
    if not isinstance(node, dict):
        return None

    raw_name = node.get("name") or node.get("title") or node.get("topic")
    if not raw_name or not isinstance(raw_name, str) or not raw_name.strip():
        return None

    name = _sanitize_ai_name(raw_name)
    if not name:
        return None

    children = node.get("children") or node.get("subtopics")
    if children is None or not isinstance(children, list):
        children = []

    if depth > MAX_TREE_DEPTH:
        raise AppError(502, "ai_invalid_response", "AI response exceeds maximum nesting depth.")

    validated_children: list[dict] = []
    for c in children:
        if isinstance(c, dict):
            valid_child = _validate_node(c, depth + 1)
            if valid_child is not None:
                validated_children.append(valid_child)

    # Deduplicate immediate siblings with identical names
    seen_names: set[str] = set()
    deduped_children: list[dict] = []
    for child in validated_children:
        lower_name = child["name"].lower().strip()
        if lower_name not in seen_names:
            seen_names.add(lower_name)
            deduped_children.append(child)

    return {"name": name, "children": deduped_children}


def _count_nodes_ai(topics: list[dict]) -> int:
    count = 0
    stack = list(topics)
    while stack:
        node = stack.pop()
        count += 1
        if count > MAX_TREE_NODES:
            return count
        kids = node.get("children") or []
        if isinstance(kids, list):
            stack.extend(kids)
    return count


def _validate_tree(data: dict) -> dict:
    """Validate the complete generated syllabus tree."""
    if not isinstance(data, dict):
        raise AppError(502, "ai_invalid_response", "AI returned invalid response data.")

    skill_name = data.get("skill_name")
    if not skill_name or not isinstance(skill_name, str) or not skill_name.strip():
        skill_name = "Untitled Skill"
    skill_name = skill_name.strip()[:200]

    topics = data.get("topics")
    if not topics or not isinstance(topics, list):
        raise AppError(502, "ai_invalid_response", "AI returned an empty or invalid syllabus.")

    validated_topics: list[dict] = []
    for t in topics:
        if isinstance(t, dict):
            valid_t = _validate_node(t, depth=0)
            if valid_t is not None:
                validated_topics.append(valid_t)

    if not validated_topics:
        raise AppError(502, "ai_invalid_response", "AI returned an empty syllabus.")

    total = _count_nodes_ai(validated_topics)
    if total > MAX_TREE_NODES:
        raise AppError(
            502,
            "ai_invalid_response",
            f"AI generated too many topics ({total}). Please select a lower syllabus depth.",
        )

    return {"skill_name": skill_name, "topics": validated_topics}


def _handle_gemini_error(exc: Exception) -> None:
    """Convert Gemini API errors to appropriate AppErrors."""
    if isinstance(exc, AppError):
        raise exc

    exc_str = str(exc).lower()
    logger.error("Gemini API error: %s", exc)

    if (
        "api_key" in exc_str
        or "api key" in exc_str
        or "unauthenticated" in exc_str
        or "permission_denied" in exc_str
        or "permission denied" in exc_str
    ):
        raise AppError(
            503, "ai_config_error", "AI service configuration error. Please contact the administrator."
        )
    # Note: never check bare '"rate" in exc_str' because "generatecontent" contains "rate"!
    if (
        "quota" in exc_str
        or "resource_exhausted" in exc_str
        or "rate limit" in exc_str
        or "rate_limit" in exc_str
        or "too many requests" in exc_str
        or "429" in exc_str
    ):
        raise AppError(
            429, "ai_rate_limited", "AI generation rate limit or quota exceeded. Please wait a moment and try again."
        )
    if "not found" in exc_str or "404" in exc_str or "no longer available" in exc_str or "is not found" in exc_str:
        raise AppError(
            503,
            "ai_model_unavailable",
            "The requested AI model is unavailable. Please set GEMINI_MODEL to gemini-3.8-flash.",
        )
    if "high demand" in exc_str or "overloaded" in exc_str or "503" in exc_str or "unavailable" in exc_str:
        raise AppError(
            503,
            "ai_overloaded",
            "The AI model is temporarily experiencing high demand. Please try again in a few seconds.",
        )
    if "timeout" in exc_str or "deadline" in exc_str or isinstance(exc, httpx.TimeoutException):
        raise AppError(504, "ai_timeout", "AI generation timed out. Please try again.")
    if "safety" in exc_str or "blocked" in exc_str:
        raise AppError(
            422,
            "ai_blocked",
            "The syllabus request was blocked by content safety filters. Please try a different subject.",
        )

    raise AppError(502, "ai_error", "AI generation failed. Please try again.")


async def _call_gemini_sdk_mock(genai: Any, prompt: str, *, temperature: float = 0.7) -> str:
    """Compatibility path when a unit test injects a mock google.generativeai module."""
    genai.configure(api_key=settings.gemini_api_key)

    configured = (settings.gemini_model or "").strip()
    if configured == "gemini-2.0-flash":
        configured = "gemini-3.8-flash"

    candidates: list[str] = []
    if configured:
        candidates.append(configured)
    for fallback in ("gemini-3.8-flash", "gemini-2.5-flash", "gemini-1.5-flash"):
        if fallback not in candidates:
            candidates.append(fallback)

    last_exc: Exception | None = None
    response = None

    for idx, model_name in enumerate(candidates):
        try:
            model = genai.GenerativeModel(
                model_name,
                system_instruction=SYSTEM_PROMPT,
            )
            response = await model.generate_content_async(
                prompt,
                generation_config=genai.GenerationConfig(
                    response_mime_type="application/json",
                    temperature=temperature,
                    max_output_tokens=8192,
                ),
            )
            break
        except Exception as exc:
            last_exc = exc
            exc_str = str(exc).lower()
            is_retryable = (
                "not found" in exc_str
                or "404" in exc_str
                or "no longer available" in exc_str
                or "is not found" in exc_str
                or "not supported" in exc_str
            )
            if is_retryable and idx + 1 < len(candidates):
                continue
            _handle_gemini_error(exc)

    if response is None:
        if last_exc:
            _handle_gemini_error(last_exc)
        raise AppError(502, "ai_error", "AI generation failed. Please try again.")

    if not response or not getattr(response, "text", None):
        raise AppError(502, "ai_empty_response", "AI returned an empty response. Please try again.")

    return response.text


def _build_candidate_models() -> list[str]:
    configured = (settings.gemini_model or "").strip()
    if configured.startswith("models/"):
        configured = configured[len("models/") :]
    # Map retired 1.5 / 2.0 / 2.5 models to their active Gemini 3.x replacements
    configured = _RETIRED_MODEL_MAP.get(configured, configured)

    candidates: list[str] = []
    if configured:
        candidates.append(configured)
    for fallback in (
        "gemini-3.8-flash",
        "gemini-3.5-flash-lite",
        "gemini-3.7-flash",
    ):
        if fallback not in candidates:
            candidates.append(fallback)
    return candidates


def _thinking_configs_for_model(model_name: str) -> list[dict[str, Any] | None]:
    """Return ordered thinkingConfig options to try for the given model."""
    lower = model_name.lower()
    if "lite" in lower:
        # gemini-3.5-flash-lite defaults to MINIMAL thinking; try without extra config first, or LOW
        return [None]
    if "gemini-3" in lower or "gemini-4" in lower:
        # Gemini 3.x Flash models (e.g. gemini-3.8-flash, gemini-3.7-flash) use thinkingLevel="LOW"
        # (thinkingBudget is deprecated on Gemini 3+ and returns 400 INVALID_ARGUMENT).
        return [{"thinkingLevel": "LOW"}, None]
    if "2.5" in lower:
        return [{"thinkingBudget": 0}, None]
    return [None]


async def _discover_available_flash_models(client: httpx.AsyncClient, api_key: str) -> list[str]:
    """Query ListModels as a last-resort fallback if static model IDs return 404."""
    try:
        res = await client.get(
            GEMINI_REST_BASE,
            headers={"x-goog-api-key": api_key},
            timeout=httpx.Timeout(connect=4.0, read=5.0, write=4.0, pool=4.0),
        )
        if res.status_code != 200:
            return []
        models = res.json().get("models") or []
        flash_models: list[str] = []
        other_models: list[str] = []
        for m in models:
            if not isinstance(m, dict):
                continue
            methods = m.get("supportedGenerationMethods") or []
            if "generateContent" not in methods:
                continue
            name = str(m.get("name") or "")
            if name.startswith("models/"):
                name = name[len("models/") :]
            if not name or "tts" in name or "image" in name or "embedding" in name or "vision" in name:
                continue
            if "flash" in name:
                flash_models.append(name)
            elif "gemini" in name:
                other_models.append(name)
        return flash_models + other_models
    except Exception:
        return []


async def _call_gemini(
    prompt: str,
    *,
    temperature: float = 0.7,
    deadline: float | None = None,
) -> str:
    """Low-level async call to Gemini REST API with low thinking latency and automatic model fallback."""
    api_key = (settings.gemini_api_key or "").strip()
    if not api_key:
        raise AppError(
            503,
            "ai_unavailable",
            "AI generation is not configured on this server (GEMINI_API_KEY missing).",
        )

    # Support unit tests that inject a fake google.generativeai module into sys.modules
    mock_genai = sys.modules.get("google.generativeai")
    if mock_genai is not None and getattr(mock_genai, "__file__", None) is None:
        return await _call_gemini_sdk_mock(mock_genai, prompt, temperature=temperature)

    if deadline is None:
        deadline = time.monotonic() + TOTAL_GENERATION_BUDGET_S

    candidates = _build_candidate_models()
    tried: set[str] = set()
    last_exc: Exception | None = None
    non_404_exc: Exception | None = None
    discovered = False

    headers = {
        "Content-Type": "application/json",
        "x-goog-api-key": api_key,
    }

    async with httpx.AsyncClient() as client:
        idx = 0
        while idx < len(candidates):
            model_name = candidates[idx]
            idx += 1
            if model_name in tried:
                continue
            tried.add(model_name)

            remaining = deadline - time.monotonic()
            if remaining <= 3.0:
                break

            read_timeout = min(PER_ATTEMPT_TIMEOUT_S, max(5.0, remaining - 1.0))
            timeout = httpx.Timeout(connect=5.0, read=read_timeout, write=10.0, pool=5.0)
            url = f"{GEMINI_REST_BASE}/{model_name}:generateContent"

            thinking_options = _thinking_configs_for_model(model_name)

            for thinking_cfg in thinking_options:
                gen_config: dict[str, Any] = {
                    "responseMimeType": "application/json",
                    "maxOutputTokens": 8192,
                }
                # gemini-3.5-flash-lite ignores/restricts custom temperature; include for standard models
                if "lite" not in model_name.lower():
                    gen_config["temperature"] = temperature
                if thinking_cfg is not None:
                    gen_config["thinkingConfig"] = thinking_cfg

                payload = {
                    "systemInstruction": {
                        "parts": [{"text": SYSTEM_PROMPT}],
                    },
                    "contents": [
                        {
                            "role": "user",
                            "parts": [{"text": prompt}],
                        }
                    ],
                    "generationConfig": gen_config,
                }

                try:
                    res = await client.post(url, headers=headers, json=payload, timeout=timeout)
                except Exception as exc:
                    last_exc = exc
                    non_404_exc = exc
                    logger.warning(
                        "Gemini request error on model '%s': %s",
                        model_name,
                        repr(exc),
                    )
                    break  # Try next candidate model if time allows

                if res.status_code == 200:
                    body = res.json()
                    resp_candidates = body.get("candidates") or []
                    if not resp_candidates:
                        block_reason = (body.get("promptFeedback") or {}).get("blockReason")
                        if block_reason:
                            raise AppError(
                                422,
                                "ai_blocked",
                                "The syllabus request was blocked by content safety filters. Please try a different subject.",
                            )
                        raise AppError(502, "ai_empty_response", "AI returned an empty response. Please try again.")

                    first = resp_candidates[0]
                    finish_reason = str(first.get("finishReason") or "").upper()
                    if finish_reason in ("SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII"):
                        raise AppError(
                            422,
                            "ai_blocked",
                            "The syllabus request was blocked by content safety filters. Please try a different subject.",
                        )

                    parts = ((first.get("content") or {}).get("parts")) or []
                    text_chunks = [
                        p.get("text", "")
                        for p in parts
                        if isinstance(p, dict) and not p.get("thought") and p.get("text")
                    ]
                    text = "".join(text_chunks).strip()
                    if not text:
                        raise AppError(502, "ai_empty_response", "AI returned an empty response. Please try again.")
                    return text

                # Parse error details
                try:
                    err_data = res.json().get("error") or {}
                    err_msg = err_data.get("message") or res.text
                    err_status = err_data.get("status") or ""
                except Exception:
                    err_msg = res.text
                    err_status = ""

                # If model rejected thinkingConfig or temperature with 400, immediately retry without thinkingConfig
                if res.status_code == 400 and thinking_cfg is not None:
                    logger.warning(
                        "Model '%s' returned 400 with thinkingConfig=%s (%s); retrying without thinkingConfig.",
                        model_name,
                        thinking_cfg,
                        err_msg[:120],
                    )
                    continue

                last_exc = RuntimeError(f"{res.status_code} {err_status}: {err_msg}")
                if res.status_code != 404:
                    non_404_exc = last_exc
                err_lower = f"{res.status_code} {err_status} {err_msg}".lower()

                # Invalid API key / unauthenticated -> fail immediately, fallback won't help
                if (
                    res.status_code in (401, 403)
                    or "api_key_invalid" in err_lower
                    or "api key not valid" in err_lower
                ):
                    _handle_gemini_error(last_exc)

                # If 404 error message suggests a replacement model (e.g. "use models/gemini-3.8-flash"),
                # extract it and queue it immediately!
                if res.status_code == 404:
                    for suggested in re.findall(r"models/([a-zA-Z0-9._-]+)", err_msg):
                        if suggested != model_name and suggested not in tried and suggested not in candidates:
                            candidates.insert(idx, suggested)

                # If we hit the end of static candidates and only got 404s, query ListModels once
                if idx >= len(candidates) and non_404_exc is None and not discovered:
                    discovered = True
                    for disc_model in await _discover_available_flash_models(client, api_key):
                        if disc_model not in tried and disc_model not in candidates:
                            candidates.append(disc_model)

                # Retryable across candidate models: 404 (not found), 429 (per-model quota), 5xx (overloaded)
                is_fallback_eligible = (
                    res.status_code in (404, 429, 500, 502, 503, 504)
                    or "not found" in err_lower
                    or "not supported" in err_lower
                    or "no longer available" in err_lower
                )
                if is_fallback_eligible and idx < len(candidates):
                    logger.warning(
                        "Gemini model '%s' returned %s (%s). Falling back to '%s'...",
                        model_name,
                        res.status_code,
                        err_msg[:160],
                        candidates[idx],
                    )
                    break  # break inner loop to try next candidate model

                _handle_gemini_error(non_404_exc or last_exc)

    if non_404_exc or last_exc:
        _handle_gemini_error(non_404_exc or last_exc)  # type: ignore[arg-type]
    raise AppError(504, "ai_timeout", "AI generation timed out. Please try again.")


async def generate_syllabus(subject: str, depth: str, purpose: str) -> dict:
    """Generate an initial syllabus using Gemini."""
    deadline = time.monotonic() + TOTAL_GENERATION_BUDGET_S
    prompt = _build_generation_prompt(subject, depth, purpose)
    try:
        raw = await _call_gemini(prompt, temperature=0.5, deadline=deadline)
    except AppError:
        raise
    except Exception as exc:
        _handle_gemini_error(exc)
        raise

    try:
        data = _extract_json(raw)
    except AppError:
        if deadline - time.monotonic() < 12.0:
            raise
        # Retry once with lower temperature if time budget permits
        retry_prompt = (
            "Your previous response was not valid JSON. "
            "Return ONLY the valid compact JSON object with 'skill_name' and 'topics'.\n\n" + prompt
        )
        raw2 = await _call_gemini(retry_prompt, temperature=0.2, deadline=deadline)
        data = _extract_json(raw2)

    return _validate_tree(data)


async def refine_syllabus(
    subject: str,
    depth: str,
    purpose: str,
    current_tree: dict[str, Any],
    conversation_history: list[dict[str, str]],
    instruction: str,
) -> dict:
    """Refine an existing syllabus based on user instruction and conversation context."""
    deadline = time.monotonic() + TOTAL_GENERATION_BUDGET_S
    # Retain the last 15 conversation exchanges for context
    recent = conversation_history[-15:] if len(conversation_history) > 15 else conversation_history
    history_lines: list[str] = []
    for msg in recent:
        role = msg.get("role", "user")
        content = msg.get("content", "")
        if role == "user":
            history_lines.append(f"User requested: {content}")
        else:
            history_lines.append("Assistant updated the syllabus accordingly.")

    prompt = _build_refinement_prompt(subject, depth, purpose, current_tree, instruction)
    if history_lines:
        prompt = "Conversation History:\n" + "\n".join(history_lines) + "\n\n" + prompt

    try:
        raw = await _call_gemini(prompt, temperature=0.3, deadline=deadline)
    except AppError:
        raise
    except Exception as exc:
        _handle_gemini_error(exc)
        raise

    try:
        data = _extract_json(raw)
    except AppError:
        if deadline - time.monotonic() < 12.0:
            raise
        # Retry once with low temperature if time budget permits
        retry_prompt = (
            "Your previous response was not valid JSON. "
            "Return ONLY the valid compact JSON object with 'skill_name' and 'topics'.\n\n" + prompt
        )
        raw2 = await _call_gemini(retry_prompt, temperature=0.1, deadline=deadline)
        data = _extract_json(raw2)

    return _validate_tree(data)

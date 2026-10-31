from __future__ import annotations

import json
import re
from typing import Any

from app.core.config import settings
from app.core.exceptions import AppError
from app.core.logging import get_logger

logger = get_logger(__name__)

MAX_TREE_NODES = 2000
MAX_TREE_DEPTH = 20
MAX_NAME_LEN = 500

DEPTH_DESCRIPTIONS = {
    "easy": (
        "Basic fundamentals only. Include definitions and introductory concepts. "
        "Keep subtopics limited. Focus on essential beginner-level topics. "
        "Aim for roughly 10-25 topics total."
    ),
    "medium": (
        "Cover fundamentals and commonly used important concepts. "
        "Include relevant subtopics with moderate coverage. "
        "Aim for roughly 25-50 topics total."
    ),
    "intermediate": (
        "Detailed concept coverage including important and moderately advanced concepts. "
        "Include more subtopics and sub-subtopics. Cover practical and technical concepts. "
        "Appropriate depth for serious learning. Aim for roughly 50-100 topics total."
    ),
    "comprehensive": (
        "Complete A-to-Z coverage of the subject. Cover fundamentals through advanced concepts. "
        "Include all important subtopics and sub-subtopics. Include related concepts that are "
        "genuinely relevant. Provide technical depth and practical knowledge. "
        "Do not miss major areas of the subject. Aim for roughly 80-200 topics total."
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
1. Return ONLY valid JSON. Absolutely no prose, conversational filler, markdown explanation, or text before/after the JSON.
2. The JSON must follow this exact structure:
{
  "skill_name": "Subject Name",
  "topics": [
    {
      "name": "Topic Name",
      "children": [
        {
          "name": "Subtopic Name",
          "children": []
        }
      ]
    }
  ]
}
3. Every topic must have a "name" (non-empty string) and a "children" list (empty list [] for leaf nodes).
4. Use logical hierarchical parent-child relationships. Child topics must be nested directly under their parent's "children" array.
5. Topic names must be concise and meaningful.
6. Do NOT include numbering or bullet prefixes in topic names (no "1", "1.1", "1.2.1", "A.", "-", etc.) - the application automatically numbers the hierarchy.
7. Do NOT include emojis or decorative symbols.
8. Do NOT include markdown headings, formatting, or bullet points in topic names.
9. Do NOT include explanatory sentences, definitions, or paragraphs inside topic names.
10. Do NOT include introductory messages ("Here is your syllabus") or concluding remarks.
11. Do NOT create duplicate or near-duplicate sibling topics.
12. Do NOT create empty topics.
13. Avoid unnecessary nesting beyond 4-5 levels unless truly essential."""


def _build_generation_prompt(subject: str, depth: str, purpose: str) -> str:
    depth_desc = DEPTH_DESCRIPTIONS.get(depth.lower(), DEPTH_DESCRIPTIONS["medium"])
    purpose_desc = PURPOSE_DESCRIPTIONS.get(purpose.lower(), PURPOSE_DESCRIPTIONS["a_to_z"])

    return (
        f"Generate a syllabus for: {subject}\n\n"
        f"Syllabus Depth: {depth.capitalize()}\n{depth_desc}\n\n"
        f"Learning Purpose: {purpose.replace('_', ' ').title()}\n{purpose_desc}\n\n"
        "Return the complete syllabus as a valid JSON object matching the required schema with \"skill_name\" and \"topics\"."
    )


def _build_refinement_prompt(
    subject: str,
    depth: str,
    purpose: str,
    current_tree: dict[str, Any],
    instruction: str,
) -> str:
    tree_json = json.dumps(current_tree, indent=2, ensure_ascii=False)

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
        "4. Return the COMPLETE updated syllabus as a valid JSON object with \"skill_name\" and \"topics\".\n"
        "5. Do NOT include numbering prefixes in topic names.\n"
        "6. Return ONLY the JSON object. No extra text."
    )


def _extract_json(text: str) -> dict:
    """Safely extract and parse JSON from model output with trailing comma tolerance."""
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

    raise AppError(502, "ai_invalid_response", "AI returned invalid JSON. Please try again.")


def _sanitize_ai_name(name: str) -> str:
    """Strip any AI-generated decimal numbering prefix like '1. ', '1.2 ', etc."""
    cleaned = re.sub(r"^\s*\d+(?:\.\d+)*\.?\s*[-—:]?\s*", "", name.strip())
    # Strip bullet points
    cleaned = re.sub(r"^[\s•\-*]+\s*", "", cleaned)
    return (cleaned or name.strip())[:MAX_NAME_LEN]


def _validate_node(node: Any, depth: int = 0) -> dict:
    """Validate and sanitize a single topic node."""
    if not isinstance(node, dict):
        raise AppError(502, "ai_invalid_response", "AI returned an invalid topic structure.")

    raw_name = node.get("name")
    if not raw_name or not isinstance(raw_name, str) or not raw_name.strip():
        raise AppError(502, "ai_invalid_response", "AI returned a topic with an empty name.")

    name = _sanitize_ai_name(raw_name)
    if not name:
        raise AppError(502, "ai_invalid_response", "AI returned a topic with an empty name.")

    children = node.get("children")
    if children is None or not isinstance(children, list):
        children = []

    if depth > MAX_TREE_DEPTH:
        raise AppError(502, "ai_invalid_response", "AI response exceeds maximum nesting depth.")

    validated_children = [_validate_node(c, depth + 1) for c in children if isinstance(c, dict)]

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

    validated_topics = [_validate_node(t, depth=0) for t in topics if isinstance(t, dict)]

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
    exc_str = str(exc).lower()
    logger.exception("Gemini API error occurred")

    if "api_key" in exc_str or "unauthenticated" in exc_str or "permission" in exc_str:
        raise AppError(
            503, "ai_config_error", "AI service configuration error. Please contact the administrator."
        )
    if "quota" in exc_str or "resource_exhausted" in exc_str or "rate" in exc_str or "429" in exc_str:
        raise AppError(
            429, "ai_rate_limited", "AI generation rate limit or quota exceeded. Please wait a moment and try again."
        )
    if "not found" in exc_str and "model" in exc_str:
        raise AppError(
            503, "ai_model_unavailable", "The requested AI model is unavailable. Please contact the administrator."
        )
    if "timeout" in exc_str or "deadline" in exc_str:
        raise AppError(504, "ai_timeout", "AI generation timed out. Please try again.")
    if "safety" in exc_str or "blocked" in exc_str:
        raise AppError(
            422, "ai_blocked", "The syllabus request was blocked by content safety filters. Please try a different subject."
        )

    raise AppError(502, "ai_error", "AI generation failed. Please try again.")


async def _call_gemini(prompt: str, *, temperature: float = 0.7) -> str:
    """Low-level async call to Gemini API."""
    if not settings.gemini_api_key:
        raise AppError(
            503,
            "ai_unavailable",
            "AI generation is not configured on this server (GEMINI_API_KEY missing).",
        )

    try:
        import google.generativeai as genai
    except ImportError:
        raise AppError(
            503,
            "ai_unavailable",
            "Google GenAI package is not installed on this server.",
        )

    genai.configure(api_key=settings.gemini_api_key)
    model_name = settings.gemini_model or "gemini-2.0-flash"

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
    except AppError:
        raise
    except Exception as exc:
        _handle_gemini_error(exc)

    if not response or not response.text:
        if response and hasattr(response, "prompt_feedback") and response.prompt_feedback:
            block = getattr(response.prompt_feedback, "block_reason", None)
            if block:
                raise AppError(
                    422,
                    "ai_blocked",
                    "The syllabus request was blocked by content safety filters. Please try a different subject.",
                )
        raise AppError(502, "ai_empty_response", "AI returned an empty response. Please try again.")

    return response.text


async def generate_syllabus(subject: str, depth: str, purpose: str) -> dict:
    """Generate an initial syllabus using Gemini."""
    prompt = _build_generation_prompt(subject, depth, purpose)
    try:
        raw = await _call_gemini(prompt, temperature=0.7)
    except AppError:
        raise
    except Exception as exc:
        _handle_gemini_error(exc)
        raise

    try:
        data = _extract_json(raw)
    except AppError:
        # Retry once with lower temperature
        retry_prompt = (
            "Your previous response was not valid JSON. "
            "Return ONLY the valid JSON object with 'skill_name' and 'topics'.\n\n" + prompt
        )
        raw2 = await _call_gemini(retry_prompt, temperature=0.2)
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
        raw = await _call_gemini(prompt, temperature=0.4)
    except AppError:
        raise
    except Exception as exc:
        _handle_gemini_error(exc)
        raise

    try:
        data = _extract_json(raw)
    except AppError:
        # Retry once with low temperature
        retry_prompt = (
            "Your previous response was not valid JSON. "
            "Return ONLY the valid JSON object with 'skill_name' and 'topics'.\n\n" + prompt
        )
        raw2 = await _call_gemini(retry_prompt, temperature=0.1)
        data = _extract_json(raw2)

    return _validate_tree(data)

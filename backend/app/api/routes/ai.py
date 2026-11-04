from __future__ import annotations

from datetime import datetime, timezone
from typing import Any
from uuid import uuid4

from fastapi import APIRouter, Depends, Request
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db
from app.core.exceptions import AppError
from app.core.logging import get_logger
from app.core.rate_limit import enforce_limit
from app.models import Skill, Topic, User
from app.models.enums import ActionType
from app.schemas import (
    ConfirmSyllabusIn,
    GenerateSyllabusIn,
    GenerateSyllabusOut,
    RefineSyllabusIn,
    SkillOut,
)
from app.services.activity import make_activity
from app.services.gemini import MAX_NAME_LEN, MAX_TREE_DEPTH, MAX_TREE_NODES, generate_syllabus, refine_syllabus

router = APIRouter(prefix="/ai", tags=["ai"])
logger = get_logger(__name__)


def _sanitize_ai_nodes(nodes: list, depth: int = 0) -> list[dict]:
    """Validate and sanitize topic hierarchy for persistence."""
    if depth > MAX_TREE_DEPTH:
        raise AppError(400, "invalid_tree", "Topic hierarchy exceeds the maximum depth limit.")
    if not isinstance(nodes, list):
        return []
    out: list[dict] = []
    for node in nodes:
        if not isinstance(node, dict):
            continue
        raw_name = node.get("name")
        name = str(raw_name or "").strip()
        if not name:
            raise AppError(400, "empty_topic_name", "All topics must have a non-empty name before confirming.")
        name = name[:MAX_NAME_LEN]
        kids = node.get("children") or []
        if not isinstance(kids, list):
            kids = []
        out.append(
            {
                "name": name,
                "description": (
                    str(node.get("description"))[:2000]
                    if node.get("description") is not None
                    else None
                ),
                "children": _sanitize_ai_nodes(kids, depth + 1),
            }
        )
    return out


def _count_ai_nodes(nodes: list) -> int:
    total = 0
    stack = list(nodes) if isinstance(nodes, list) else []
    while stack:
        node = stack.pop()
        if not isinstance(node, dict):
            continue
        total += 1
        if total > MAX_TREE_NODES:
            return total
        kids = node.get("children") or []
        if isinstance(kids, list):
            stack.extend(kids)
    return total


def _count_ai_leaves(nodes: list) -> int:
    leaves = 0
    stack = list(nodes) if isinstance(nodes, list) else []
    while stack:
        node = stack.pop()
        if not isinstance(node, dict):
            continue
        kids = node.get("children") or []
        if not kids:
            leaves += 1
        else:
            stack.extend(kids)
    return leaves


def _persist_ai_nodes(
    session: AsyncSession,
    skill: Skill,
    nodes: list[dict],
    parent_id: Any | None,
) -> int:
    created = 0
    for index, node in enumerate(nodes):
        topic = Topic(
            id=uuid4(),
            skill_id=skill.id,
            parent_id=parent_id,
            name=node.get("name") or "Untitled Topic",
            description=node.get("description"),
            order_index=index,
        )
        session.add(topic)
        created += 1
        children = node.get("children") or []
        if children:
            created += _persist_ai_nodes(session, skill, children, topic.id)
    return created


@router.post("/generate-syllabus", response_model=GenerateSyllabusOut)
async def api_generate_syllabus(
    request: Request,
    body: GenerateSyllabusIn,
    user: User = Depends(get_current_user),
    _session: AsyncSession = Depends(get_db),
):
    """Generate a structured syllabus using Gemini. Nothing is saved to the database."""
    enforce_limit(request, "ai_generate", 15, 3600)
    result = await generate_syllabus(body.subject, body.depth, body.purpose)
    return result


@router.post("/refine-syllabus", response_model=GenerateSyllabusOut)
async def api_refine_syllabus(
    request: Request,
    body: RefineSyllabusIn,
    user: User = Depends(get_current_user),
    _session: AsyncSession = Depends(get_db),
):
    """Refine an existing syllabus tree using Gemini. Nothing is saved to the database."""
    enforce_limit(request, "ai_refine", 30, 3600)
    result = await refine_syllabus(
        subject=body.subject,
        depth=body.depth,
        purpose=body.purpose,
        current_tree=body.current_tree,
        conversation_history=body.conversation_history,
        instruction=body.instruction,
    )
    return result


@router.post("/confirm-syllabus", response_model=SkillOut, status_code=201)
async def api_confirm_syllabus(
    request: Request,
    body: ConfirmSyllabusIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    """Confirm and save an AI-generated syllabus as a new Skill with Topics."""
    enforce_limit(request, "ai_confirm", 10, 3600)

    topics = _sanitize_ai_nodes(body.topics, depth=0)
    if not topics:
        raise AppError(400, "empty_tree", "Cannot create a skill without topics.")

    total_nodes = _count_ai_nodes(topics)
    if total_nodes > MAX_TREE_NODES:
        raise AppError(400, "invalid_tree", f"Topic tree exceeds the {MAX_TREE_NODES} topic limit.")

    skill_name = body.skill_name.strip()[:200]
    existing = (
        await session.execute(
            select(Skill.id).where(
                Skill.user_id == user.id,
                Skill.name == skill_name,
            )
        )
    ).scalars().first()
    if existing:
        raise AppError(409, "skill_exists", "You already have a skill with this name.")

    skill = Skill(
        user_id=user.id,
        name=skill_name,
        description=body.skill_description,
    )
    session.add(skill)

    try:
        await session.flush()
        _persist_ai_nodes(session, skill, topics, None)
        session.add(
            make_activity(
                user_id=user.id,
                action_type=ActionType.ai_generated,
                occurred_at=datetime.now(timezone.utc),
                timezone_name=user.timezone,
                skill_id=skill.id,
                skill_name_snapshot=skill.name,
                summary=f'You generated "{skill.name}" using AI',
            )
        )
        await session.commit()
    except IntegrityError:
        await session.rollback()
        raise AppError(409, "skill_exists", "You already have a skill with this name.")
    except Exception:
        await session.rollback()
        raise
    await session.refresh(skill)

    total = _count_ai_leaves(topics)
    return SkillOut(
        id=skill.id,
        name=skill.name,
        description=skill.description,
        created_at=skill.created_at,
        completed_leaves=0,
        total_leaves=total,
        percent=0.0,
        empty=total == 0,
        deleted_at=None,
        purge_at=None,
    )

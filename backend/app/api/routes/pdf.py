from __future__ import annotations

import io
from datetime import datetime, timezone
from pathlib import Path
from uuid import UUID, uuid4

from fastapi import APIRouter, BackgroundTasks, Depends, Request, UploadFile
from pypdf import PdfReader
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user, get_db
from app.core.config import settings
from app.core.exceptions import AppError
from app.core.logging import get_logger
from app.core.rate_limit import enforce_limit
from app.db.session import SessionLocal
from app.models import ImportJob, Skill, Topic, User
from app.models.enums import ActionType, ImportJobStatus
from app.schemas import ConfirmImportIn, ImportJobOut, TreePatchIn
from app.services.activity import make_activity
from app.services.pdf_parser import parse_syllabus_text

router = APIRouter(prefix="/pdf", tags=["pdf"])
logger = get_logger(__name__)

PDF_MAGIC = b"%PDF"
MAX_TREE_NODES = 2000
MAX_TREE_DEPTH = 20
MAX_NAME_LEN = 500
READ_CHUNK = 64 * 1024


async def _owned_job(job_id: UUID, user: User, session: AsyncSession) -> ImportJob:
    job = await session.get(ImportJob, job_id)
    if job is None or job.user_id != user.id:
        raise AppError(404, "not_found", "Import job not found.")
    return job


def _extract_text(data: bytes) -> tuple[str, int]:
    reader = PdfReader(io.BytesIO(data))
    pages = len(reader.pages)
    if pages > settings.max_pdf_pages:
        raise AppError(400, "pdf_too_long", f"PDF exceeds the {settings.max_pdf_pages}-page limit.")
    chunks = []
    for page in reader.pages:
        chunks.append(page.extract_text() or "")
    return "\n".join(chunks), pages


async def _ai_fill_low_confidence(tree: dict) -> dict:
    sections = tree.get("low_confidence_sections") or []
    if not sections:
        return tree
    if not settings.anthropic_api_key:
        tree.setdefault("confidence_notes", {})
        tree["confidence_notes"]["ai_skipped"] = (
            "ANTHROPIC_API_KEY unset; low-confidence sections flagged for manual review."
        )
        return tree
    try:
        import json
        import anthropic

        client = anthropic.Anthropic(api_key=settings.anthropic_api_key)
        prompt = (
            "Return ONLY JSON: {\"children\": [{\"name\": str, \"children\": [...]}]}. "
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
        if not isinstance(extra, list):
            extra = []
        sanitized = _sanitize_nodes(extra, depth=0)
        tree.setdefault("children", []).extend(sanitized)
        _assert_tree_budget(tree)
        tree["confidence_notes"] = {"ai_merged": True}
    except Exception:
        logger.exception("PDF AI fallback failed")
        tree.setdefault("confidence_notes", {})
        tree["confidence_notes"]["ai_error"] = "AI fallback failed; review low-confidence nodes."
    return tree


async def run_parse_job(job_id: UUID, raw: bytes) -> None:
    async with SessionLocal() as session:
        job = await session.get(ImportJob, job_id)
        if job is None:
            return
        try:
            job.status = ImportJobStatus.extracting
            await session.commit()
            text, _pages = _extract_text(raw)
            if not text.strip():
                job.status = ImportJobStatus.failed
                job.error_message = (
                    "No extractable text found. This looks like a scanned or image-only PDF. "
                    "Export or print to a text-based PDF and try again. OCR is not supported."
                )
                await session.commit()
                return
            job.status = ImportJobStatus.parsing
            await session.commit()
            tree = parse_syllabus_text(text)
            tree = await _ai_fill_low_confidence(tree)
            _assert_tree_budget(tree)
            tree["children"] = _sanitize_nodes(tree.get("children") or [], depth=0)
            job.generated_tree_json = tree
            job.confidence_notes = {
                "low_confidence_sections": tree.get("low_confidence_sections") or [],
                **(tree.get("confidence_notes") or {}),
            }
            job.status = ImportJobStatus.ready_for_preview
            await session.commit()
        except AppError as exc:
            job.status = ImportJobStatus.failed
            job.error_message = exc.message
            await session.commit()
        except Exception:
            logger.exception("PDF parsing failed for job %s", job_id)
            job.status = ImportJobStatus.failed
            job.error_message = "Parsing failed. The file was not stored as a skill."
            await session.commit()


@router.post("/upload", response_model=ImportJobOut)
async def upload_pdf(
    request: Request,
    background: BackgroundTasks,
    file: UploadFile,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    enforce_limit(request, "pdf_upload", 10, 3600)
    content_length = request.headers.get("content-length")
    if content_length:
        try:
            if int(content_length) > settings.max_pdf_bytes + 65536:
                raise AppError(400, "pdf_too_large", "PDF exceeds the 10MB size limit.")
        except ValueError:
            pass
    chunks: list[bytes] = []
    total = 0
    while True:
        piece = await file.read(READ_CHUNK)
        if not piece:
            break
        total += len(piece)
        if total > settings.max_pdf_bytes:
            raise AppError(400, "pdf_too_large", "PDF exceeds the 10MB size limit.")
        chunks.append(piece)
    data = b"".join(chunks)
    if not data.startswith(PDF_MAGIC):
        raise AppError(400, "invalid_pdf", "File is not a valid PDF (content-type/magic mismatch).")
    filename = Path(file.filename or "upload.pdf").name
    job = ImportJob(
        user_id=user.id,
        original_filename=filename[:500],
        file_size_bytes=len(data),
        status=ImportJobStatus.uploaded,
    )
    session.add(job)
    await session.commit()
    await session.refresh(job)
    background.add_task(run_parse_job, job.id, data)
    return job


@router.get("/jobs/{job_id}", response_model=ImportJobOut)
async def get_job(
    job_id: UUID,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    return await _owned_job(job_id, user, session)


@router.patch("/jobs/{job_id}/tree", response_model=ImportJobOut)
async def patch_tree(
    job_id: UUID,
    body: TreePatchIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    job = await _owned_job(job_id, user, session)
    if job.status not in (ImportJobStatus.ready_for_preview, ImportJobStatus.parsing):
        raise AppError(409, "not_editable", "This import job cannot be edited in its current state.")
    job.generated_tree_json = _sanitize_tree(body.generated_tree_json)
    await session.commit()
    await session.refresh(job)
    return job


def _sanitize_tree(tree: dict) -> dict:
    if not isinstance(tree, dict):
        raise AppError(400, "invalid_tree", "Import tree is invalid.")
    name = str(tree.get("name") or "Imported skill")[:MAX_NAME_LEN]
    children = tree.get("children") or []
    if not isinstance(children, list):
        children = []
    return {
        "name": name,
        "children": _sanitize_nodes(children, depth=0),
        "confidence_notes": tree.get("confidence_notes"),
        "low_confidence_sections": tree.get("low_confidence_sections") or [],
    }


def _sanitize_nodes(nodes: list, depth: int) -> list[dict]:
    if depth > MAX_TREE_DEPTH:
        raise AppError(400, "invalid_tree", "Import tree exceeds the maximum depth.")
    if not isinstance(nodes, list):
        return []
    out: list[dict] = []
    for node in nodes:
        if not isinstance(node, dict):
            continue
        kids = node.get("children") or []
        if not isinstance(kids, list):
            kids = []
        out.append(
            {
                "name": str(node.get("name") or "Untitled")[:MAX_NAME_LEN],
                "description": (
                    str(node.get("description"))[:2000] if node.get("description") is not None else None
                ),
                "confidence": node.get("confidence"),
                "source": node.get("source"),
                "children": _sanitize_nodes(kids, depth + 1),
            }
        )
    return out


def _assert_tree_budget(tree: dict, extra: list | None = None) -> None:
    count = _count_nodes(tree.get("children") or [])
    if extra:
        count += _count_nodes(extra)
    if count > MAX_TREE_NODES:
        raise AppError(400, "invalid_tree", "Import tree has too many topics.")


def _count_nodes(nodes: list) -> int:
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


def _persist_nodes(
    session: AsyncSession,
    skill: Skill,
    nodes: list[dict],
    parent_id: UUID | None,
) -> int:
    created = 0
    for index, node in enumerate(nodes):
        topic = Topic(
            id=uuid4(),
            skill_id=skill.id,
            parent_id=parent_id,
            name=node.get("name") or "Untitled",
            description=node.get("description"),
            order_index=index,
        )
        session.add(topic)
        created += 1
        children = node.get("children") or []
        if children:
            created += _persist_nodes(session, skill, children, topic.id)
    return created


@router.post("/jobs/{job_id}/confirm", response_model=ImportJobOut)
async def confirm_job(
    job_id: UUID,
    body: ConfirmImportIn,
    user: User = Depends(get_current_user),
    session: AsyncSession = Depends(get_db),
):
    job = await _owned_job(job_id, user, session)
    if job.status != ImportJobStatus.ready_for_preview:
        raise AppError(409, "not_ready", "Import is not ready to confirm.")
    tree = _sanitize_tree(job.generated_tree_json or {})
    _assert_tree_budget(tree)
    skill = Skill(
        user_id=user.id,
        name=(body.skill_name or tree.get("name") or Path(job.original_filename).stem)[:300],
        description=body.skill_description,
    )
    session.add(skill)
    await session.flush()
    _persist_nodes(session, skill, tree.get("children") or [], None)
    job.skill_id = skill.id
    job.status = ImportJobStatus.confirmed
    session.add(
        make_activity(
            user_id=user.id,
            action_type=ActionType.pdf_imported,
            occurred_at=datetime.now(timezone.utc),
            timezone_name=user.timezone,
            skill_id=skill.id,
            skill_name_snapshot=skill.name,
            topic_name_snapshot=job.original_filename,
            summary=f'You imported "{job.original_filename}" and created "{skill.name}"',
        )
    )
    await session.commit()
    await session.refresh(job)
    return job

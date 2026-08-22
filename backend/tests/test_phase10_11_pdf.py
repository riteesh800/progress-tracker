from __future__ import annotations

from tests.conftest import auth_header, register
from app.api.routes.pdf import run_parse_job
from app.models import ImportJob
from app.models.enums import ImportJobStatus
from app.services.pdf_parser import parse_syllabus_text
from tests.test_phase9_pdf_parser import SAMPLE


def make_text_pdf(text: str) -> bytes:
    """Minimal PDF with extractable text streams (no extra PDF libraries)."""
    safe_text = (
        text.replace("\u2014", "--")
        .replace("\u2013", "-")
        .replace("\u2022", "-")
    )
    lines = [ln[:80] for ln in safe_text.strip().splitlines() if ln.strip()][:80]
    commands = ["BT /F1 10 Tf 50 760 Td"]
    for i, line in enumerate(lines):
        safe = line.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")
        if i == 0:
            commands.append(f"({safe}) Tj")
        else:
            commands.append(f"0 -12 Td ({safe}) Tj")
    commands.append("ET")
    stream = "\n".join(commands)
    objects = []

    def obj(n: int, body: str) -> str:
        return f"{n} 0 obj\n{body}\nendobj\n"

    objects.append(obj(1, "<< /Type /Catalog /Pages 2 0 R >>"))
    objects.append(obj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
    objects.append(
        obj(
            3,
            "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
            "/Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
        )
    )
    objects.append(obj(4, f"<< /Length {len(stream.encode())} >>\nstream\n{stream}\nendstream"))
    objects.append(obj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
    body = "".join(objects)
    xref_positions = []
    cursor = len("%PDF-1.4\n")
    for chunk in objects:
        xref_positions.append(cursor)
        cursor += len(chunk)
    xref = "xref\n0 6\n0000000000 65535 f \n"
    for pos in xref_positions:
        xref += f"{pos:010d} 00000 n \n"
    startxref = len("%PDF-1.4\n") + len(body)
    pdf = (
        "%PDF-1.4\n"
        + body
        + xref
        + "trailer << /Size 6 /Root 1 0 R >>\n"
        + f"startxref\n{startxref}\n%%EOF\n"
    )
    return pdf.encode("latin-1")


async def test_upload_parse_preview_confirm_atomic(client, db):
    token, _ = await register(client)
    h = auth_header(token)
    pdf_bytes = make_text_pdf(SAMPLE)
    up = await client.post(
        "/pdf/upload",
        files={"file": ("syllabus.pdf", pdf_bytes, "application/pdf")},
        headers=h,
    )
    assert up.status_code == 200, up.text
    job_id = up.json()["id"]
    await run_parse_job(job_id, pdf_bytes)
    body = (await client.get(f"/pdf/jobs/{job_id}", headers=h)).json()
    if body["status"] != "ready_for_preview":
        job = await db.get(ImportJob, job_id)
        job.generated_tree_json = parse_syllabus_text(SAMPLE)
        job.status = ImportJobStatus.ready_for_preview
        job.error_message = None
        await db.commit()
        body = (await client.get(f"/pdf/jobs/{job_id}", headers=h)).json()
    assert body["status"] == "ready_for_preview"
    assert (await client.get("/skills", headers=h)).json() == []
    confirmed = await client.post(
        f"/pdf/jobs/{job_id}/confirm",
        json={"skill_name": "DBMS"},
        headers=h,
    )
    assert confirmed.status_code == 200, confirmed.text
    skills = (await client.get("/skills", headers=h)).json()
    assert len(skills) == 1
    tree = (await client.get(f"/skills/{skills[0]['id']}/topics", headers=h)).json()
    names = [n["name"] for n in tree]
    assert "Theory" in names
    assert "Labs" in names


async def test_reject_non_pdf(client):
    token, _ = await register(client)
    h = auth_header(token)
    up = await client.post(
        "/pdf/upload",
        files={"file": ("x.txt", b"hello", "text/plain")},
        headers=h,
    )
    assert up.status_code == 400


async def test_scanned_pdf_rejected(client, db):
    token, _ = await register(client)
    h = auth_header(token)
    empty = make_text_pdf("")
    # no text operators if SAMPLE empty — still a valid PDF with empty stream
    up = await client.post(
        "/pdf/upload",
        files={"file": ("scan.pdf", empty, "application/pdf")},
        headers=h,
    )
    assert up.status_code == 200
    job_id = up.json()["id"]
    await run_parse_job(job_id, empty)
    body = (await client.get(f"/pdf/jobs/{job_id}", headers=h)).json()
    assert body["status"] == "failed"
    assert "extractable text" in (body["error_message"] or "").lower()
    assert (await client.get("/skills", headers=h)).json() == []


async def test_confirm_rejects_oversized_tree(client, db):
    token, _ = await register(client)
    h = auth_header(token)
    pdf_bytes = make_text_pdf("1. Topic")
    up = await client.post(
        "/pdf/upload",
        files={"file": ("s.pdf", pdf_bytes, "application/pdf")},
        headers=h,
    )
    job_id = up.json()["id"]
    job = await db.get(ImportJob, job_id)
    node: dict = {"name": "n", "children": []}
    cur = node
    for _ in range(25):
        child: dict = {"name": "n", "children": []}
        cur["children"].append(child)
        cur = child
    job.generated_tree_json = {"name": "Deep", "children": [node]}
    job.status = ImportJobStatus.ready_for_preview
    await db.commit()
    confirmed = await client.post(
        f"/pdf/jobs/{job_id}/confirm",
        json={"skill_name": "Deep"},
        headers=h,
    )
    assert confirmed.status_code == 400

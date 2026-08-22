from __future__ import annotations

from enum import StrEnum


class ActionType(StrEnum):
    topic_completed = "topic_completed"
    topic_uncompleted = "topic_uncompleted"
    note_added = "note_added"
    note_updated = "note_updated"
    topic_created = "topic_created"
    skill_created = "skill_created"
    skill_completed = "skill_completed"
    skill_deleted = "skill_deleted"
    pdf_imported = "pdf_imported"


class ImportJobStatus(StrEnum):
    uploaded = "uploaded"
    extracting = "extracting"
    parsing = "parsing"
    ready_for_preview = "ready_for_preview"
    confirmed = "confirmed"
    failed = "failed"

from __future__ import annotations

from datetime import date, datetime
from typing import Any, Optional
from uuid import UUID

from pydantic import BaseModel, EmailStr, Field, field_validator
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError


class TokenPair(BaseModel):
    access_token: str
    refresh_token: str
    token_type: str = "bearer"


class RegisterIn(BaseModel):
    email: EmailStr
    password: str = Field(min_length=8, max_length=128)
    name: str = ""
    timezone: str = "UTC"

    @field_validator("timezone")
    @classmethod
    def valid_tz(cls, v: str) -> str:
        try:
            ZoneInfo(v)
        except ZoneInfoNotFoundError as exc:
            raise ValueError("Invalid IANA timezone") from exc
        return v


class LoginIn(BaseModel):
    email: EmailStr
    password: str


class RefreshIn(BaseModel):
    refresh_token: str


class GoogleIn(BaseModel):
    id_token: str
    timezone: str = "UTC"


class ForgotPasswordIn(BaseModel):
    email: EmailStr


class VerifyResetIn(BaseModel):
    email: EmailStr
    code: str


class ResetPasswordIn(BaseModel):
    email: EmailStr
    code: str
    new_password: str = Field(min_length=8, max_length=128)


class UserOut(BaseModel):
    id: UUID
    email: EmailStr
    name: str
    timezone: str
    is_active: bool

    model_config = {"from_attributes": True}


class UserUpdateIn(BaseModel):
    name: Optional[str] = None
    timezone: Optional[str] = None

    @field_validator("timezone")
    @classmethod
    def valid_tz(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        try:
            ZoneInfo(v)
        except ZoneInfoNotFoundError as exc:
            raise ValueError("Invalid IANA timezone") from exc
        return v


class SkillIn(BaseModel):
    name: str = Field(min_length=1, max_length=300)
    description: Optional[str] = None


class SkillUpdateIn(BaseModel):
    name: Optional[str] = Field(default=None, min_length=1, max_length=300)
    description: Optional[str] = None


class SkillOut(BaseModel):
    id: UUID
    name: str
    description: Optional[str]
    created_at: datetime
    completed_leaves: int = 0
    total_leaves: int = 0
    percent: float = 0
    empty: bool = True

    model_config = {"from_attributes": True}


class TopicIn(BaseModel):
    skill_id: UUID
    parent_id: Optional[UUID] = None
    name: str = Field(min_length=1, max_length=500)
    description: Optional[str] = None


class TopicCompleteIn(BaseModel):
    completed: bool


class TopicNodeOut(BaseModel):
    id: UUID
    skill_id: UUID
    parent_id: Optional[UUID]
    name: str
    description: Optional[str]
    order_index: int
    is_leaf: bool
    is_completed: bool
    completed_leaves: int
    total_leaves: int
    percent: float
    children: list["TopicNodeOut"] = []


class DeleteImpactOut(BaseModel):
    descendants: int
    notes: int
    requires_confirm: bool


class NoteIn(BaseModel):
    topic_id: UUID
    content: str

    @field_validator("content")
    @classmethod
    def not_blank(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("Note content cannot be empty")
        return v.strip()


class NoteUpdateIn(BaseModel):
    content: str

    @field_validator("content")
    @classmethod
    def not_blank(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("Note content cannot be empty")
        return v.strip()


class NoteOut(BaseModel):
    id: UUID
    topic_id: UUID
    content: str
    created_at: datetime
    updated_at: datetime

    model_config = {"from_attributes": True}


class StreakOut(BaseModel):
    current_streak: int
    longest_streak: int


class StreakDayOut(BaseModel):
    date: date
    count: int


class StreakCalendarOut(BaseModel):
    start: date
    end: date
    timezone: str
    total_submissions: int
    total_active_days: int
    current_streak: int
    longest_streak: int
    days: list[StreakDayOut]


class SearchHit(BaseModel):
    kind: str
    id: UUID
    title: str
    snippet: str
    skill_id: Optional[UUID] = None
    topic_id: Optional[UUID] = None


class ActivityOut(BaseModel):
    id: UUID
    action_type: str
    topic_id: Optional[UUID]
    skill_id: Optional[UUID]
    topic_name_snapshot: Optional[str]
    skill_name_snapshot: Optional[str]
    parent_name_snapshot: Optional[str] = None
    summary: Optional[str] = None
    occurred_at: datetime
    activity_date: date


class PersonalNoteTaskIn(BaseModel):
    content: str = ""
    is_completed: bool = False


class PersonalNoteTaskOut(BaseModel):
    id: UUID
    content: str
    is_completed: bool
    order_index: int

    model_config = {"from_attributes": True}


class PersonalNoteIn(BaseModel):
    title: str = Field(default="Untitled note", max_length=300)


class PersonalNoteUpdateIn(BaseModel):
    title: Optional[str] = Field(default=None, max_length=300)
    tasks: Optional[list[PersonalNoteTaskIn]] = None


class PersonalNoteOut(BaseModel):
    id: UUID
    title: str
    created_at: datetime
    updated_at: datetime
    tasks: list[PersonalNoteTaskOut] = []

    model_config = {"from_attributes": True}


class DashboardOut(BaseModel):
    total_skills: int
    total_topics: int
    completed_topics: int
    overall_progress: float
    current_streak: int
    longest_streak: int
    empty: bool


class SuggestedNextOut(BaseModel):
    skill_id: Optional[UUID]
    skill_name: Optional[str]
    topic_id: Optional[UUID]
    topic_name: Optional[str]
    reason: str


class ImportJobOut(BaseModel):
    id: UUID
    status: str
    original_filename: str
    file_size_bytes: int
    skill_id: Optional[UUID]
    generated_tree_json: Optional[dict[str, Any]]
    confidence_notes: Optional[Any]
    error_message: Optional[str]

    model_config = {"from_attributes": True}


class TreePatchIn(BaseModel):
    generated_tree_json: dict[str, Any]


class ConfirmImportIn(BaseModel):
    skill_name: Optional[str] = None
    skill_description: Optional[str] = None

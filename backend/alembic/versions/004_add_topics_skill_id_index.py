"""Add standalone index on topics.skill_id for batch queries.

Revision ID: 004_skill_idx
Revises: 003_must_change_password
Create Date: 2026-09-25
"""
from __future__ import annotations

from alembic import op

revision = "004_skill_idx"
down_revision = "003_must_change_password"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_index("ix_topics_skill_id", "topics", ["skill_id"], unique=False)


def downgrade() -> None:
    op.drop_index("ix_topics_skill_id", table_name="topics")

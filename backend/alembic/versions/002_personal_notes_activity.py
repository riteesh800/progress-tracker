"""Add personal notes tables and richer activity snapshots."""

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "002_personal_notes_activity"
down_revision: Union[str, None] = "001_initial"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column("activity_log", sa.Column("parent_name_snapshot", sa.Text(), nullable=True))
    op.add_column("activity_log", sa.Column("summary", sa.Text(), nullable=True))
    op.create_table(
        "personal_notes",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("user_id", sa.CHAR(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("title", sa.String(300), nullable=False, server_default="Untitled note"),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_personal_notes_user_id", "personal_notes", ["user_id"])
    op.create_table(
        "personal_note_tasks",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column(
            "note_id",
            sa.CHAR(36),
            sa.ForeignKey("personal_notes.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("content", sa.Text(), nullable=False, server_default=""),
        sa.Column("is_completed", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("order_index", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_personal_note_tasks_note_id", "personal_note_tasks", ["note_id"])


def downgrade() -> None:
    op.drop_index("ix_personal_note_tasks_note_id", table_name="personal_note_tasks")
    op.drop_table("personal_note_tasks")
    op.drop_index("ix_personal_notes_user_id", table_name="personal_notes")
    op.drop_table("personal_notes")
    op.drop_column("activity_log", "summary")
    op.drop_column("activity_log", "parent_name_snapshot")

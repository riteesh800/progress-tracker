"""Initial schema: users, skills, topics, notes, activity_log, import_jobs, tokens.

Postgres-only: GIN full-text indexes are created when the dialect is postgresql.
"""

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "001_initial"
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        "users",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("email", sa.String(320), nullable=False),
        sa.Column("password_hash", sa.String(255), nullable=True),
        sa.Column("google_id", sa.String(255), nullable=True, unique=True),
        sa.Column("name", sa.String(200), nullable=False, server_default=""),
        sa.Column("timezone", sa.String(64), nullable=False, server_default="UTC"),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.UniqueConstraint("email", name="uq_users_email"),
    )
    op.create_table(
        "skills",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("user_id", sa.CHAR(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.String(300), nullable=False),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_skills_user_id", "skills", ["user_id"])
    op.create_table(
        "topics",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("skill_id", sa.CHAR(36), sa.ForeignKey("skills.id", ondelete="CASCADE"), nullable=False),
        sa.Column("parent_id", sa.CHAR(36), sa.ForeignKey("topics.id", ondelete="CASCADE"), nullable=True),
        sa.Column("name", sa.String(500), nullable=False),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("order_index", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("is_completed", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("completed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_topics_skill_parent_order", "topics", ["skill_id", "parent_id", "order_index"])
    op.create_index("ix_topics_parent_id", "topics", ["parent_id"])
    op.create_table(
        "notes",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("topic_id", sa.CHAR(36), sa.ForeignKey("topics.id", ondelete="CASCADE"), nullable=False),
        sa.Column("user_id", sa.CHAR(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("content", sa.Text(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_notes_topic_id", "notes", ["topic_id"])
    op.create_index("ix_notes_user_id", "notes", ["user_id"])
    op.create_table(
        "activity_log",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("user_id", sa.CHAR(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("action_type", sa.String(32), nullable=False),
        sa.Column("topic_id", sa.CHAR(36), sa.ForeignKey("topics.id", ondelete="SET NULL"), nullable=True),
        sa.Column("skill_id", sa.CHAR(36), sa.ForeignKey("skills.id", ondelete="SET NULL"), nullable=True),
        sa.Column("topic_name_snapshot", sa.Text(), nullable=True),
        sa.Column("skill_name_snapshot", sa.Text(), nullable=True),
        sa.Column("occurred_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("activity_date", sa.Date(), nullable=False),
    )
    op.create_index("ix_activity_user_date", "activity_log", ["user_id", "activity_date"])
    op.create_index("ix_activity_user_type_date", "activity_log", ["user_id", "action_type", "activity_date"])
    op.create_table(
        "import_jobs",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("user_id", sa.CHAR(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("skill_id", sa.CHAR(36), sa.ForeignKey("skills.id", ondelete="SET NULL"), nullable=True),
        sa.Column("original_filename", sa.String(500), nullable=False),
        sa.Column("file_size_bytes", sa.Integer(), nullable=False),
        sa.Column("status", sa.String(32), nullable=False),
        sa.Column("confidence_notes", sa.JSON(), nullable=True),
        sa.Column("error_message", sa.Text(), nullable=True),
        sa.Column("generated_tree_json", sa.JSON(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_import_jobs_user_id", "import_jobs", ["user_id"])
    op.create_table(
        "refresh_tokens",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("user_id", sa.CHAR(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("token_hash", sa.String(255), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("revoked", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_refresh_tokens_user_id", "refresh_tokens", ["user_id"])
    op.create_index("ix_refresh_tokens_token_hash", "refresh_tokens", ["token_hash"])
    op.create_table(
        "password_reset_tokens",
        sa.Column("id", sa.CHAR(36), primary_key=True),
        sa.Column("user_id", sa.CHAR(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("code_hash", sa.String(255), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("used", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
    )
    op.create_index("ix_password_reset_tokens_user_id", "password_reset_tokens", ["user_id"])

    bind = op.get_bind()
    if bind.dialect.name == "postgresql":
        op.execute(
            """
            ALTER TABLE skills ADD COLUMN search_tsv tsvector
            GENERATED ALWAYS AS (
              setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
              setweight(to_tsvector('english', coalesce(description, '')), 'B')
            ) STORED
            """
        )
        op.execute(
            """
            ALTER TABLE topics ADD COLUMN search_tsv tsvector
            GENERATED ALWAYS AS (
              setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
              setweight(to_tsvector('english', coalesce(description, '')), 'B')
            ) STORED
            """
        )
        op.execute(
            """
            ALTER TABLE notes ADD COLUMN search_tsv tsvector
            GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED
            """
        )
        op.execute("CREATE INDEX ix_skills_search_tsv ON skills USING GIN (search_tsv)")
        op.execute("CREATE INDEX ix_topics_search_tsv ON topics USING GIN (search_tsv)")
        op.execute("CREATE INDEX ix_notes_search_tsv ON notes USING GIN (search_tsv)")


def downgrade() -> None:
    bind = op.get_bind()
    if bind.dialect.name == "postgresql":
        op.execute("DROP INDEX IF EXISTS ix_notes_search_tsv")
        op.execute("DROP INDEX IF EXISTS ix_topics_search_tsv")
        op.execute("DROP INDEX IF EXISTS ix_skills_search_tsv")
    op.drop_table("password_reset_tokens")
    op.drop_table("refresh_tokens")
    op.drop_table("import_jobs")
    op.drop_table("activity_log")
    op.drop_table("notes")
    op.drop_table("topics")
    op.drop_table("skills")
    op.drop_table("users")

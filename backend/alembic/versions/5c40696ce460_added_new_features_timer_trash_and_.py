"""added new features: timer, trash, and account updates

Revision ID: 5c40696ce460
Revises: 0e1e5d00eb37
Create Date: 2026-10-04 15:56:29.569020
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = '5c40696ce460'
down_revision: Union[str, None] = '0e1e5d00eb37'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    bind = op.get_bind()
    if bind.dialect.name == 'postgresql':
        op.execute(
            """
            ALTER TABLE skills ADD COLUMN IF NOT EXISTS search_tsv tsvector
            GENERATED ALWAYS AS (
                setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
                setweight(to_tsvector('english', coalesce(description, '')), 'B')
            ) STORED
            """
        )
        op.execute("CREATE INDEX IF NOT EXISTS ix_skills_search_tsv ON skills USING GIN (search_tsv)")
        op.execute(
            """
            ALTER TABLE topics ADD COLUMN IF NOT EXISTS search_tsv tsvector
            GENERATED ALWAYS AS (
                setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
                setweight(to_tsvector('english', coalesce(description, '')), 'B')
            ) STORED
            """
        )
        op.execute("CREATE INDEX IF NOT EXISTS ix_topics_search_tsv ON topics USING GIN (search_tsv)")
        op.execute(
            """
            ALTER TABLE notes ADD COLUMN IF NOT EXISTS search_tsv tsvector
            GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED
            """
        )
        op.execute("CREATE INDEX IF NOT EXISTS ix_notes_search_tsv ON notes USING GIN (search_tsv)")
        op.execute("CREATE INDEX IF NOT EXISTS ix_activity_user_occurred ON activity_log (user_id, occurred_at)")


def downgrade() -> None:
    bind = op.get_bind()
    if bind.dialect.name == 'postgresql':
        op.execute("DROP INDEX IF EXISTS ix_activity_user_occurred")

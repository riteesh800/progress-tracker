"""add new feature

Revision ID: 0e1e5d00eb37
Revises: 33f542905828
Create Date: 2026-10-04 14:56:51.916790
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision: str = '0e1e5d00eb37'
down_revision: Union[str, None] = '33f542905828'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    bind = op.get_bind()
    is_pg = bind.dialect.name == 'postgresql'
    if is_pg:
        op.alter_column('import_jobs', 'confidence_notes',
                   existing_type=postgresql.JSON(astext_type=sa.Text()),
                   type_=sa.JSON().with_variant(postgresql.JSONB(astext_type=sa.Text()), 'postgresql'),
                   existing_nullable=True)
        op.alter_column('import_jobs', 'generated_tree_json',
                   existing_type=postgresql.JSON(astext_type=sa.Text()),
                   type_=sa.JSON().with_variant(postgresql.JSONB(astext_type=sa.Text()), 'postgresql'),
                   existing_nullable=True)
        op.execute("DROP INDEX IF EXISTS ix_skills_search_tsv")
        op.execute("ALTER TABLE skills DROP COLUMN IF EXISTS search_tsv")
        op.alter_column('skills', 'name',
                   existing_type=sa.VARCHAR(length=300),
                   type_=sa.String(length=200),
                   existing_nullable=False)
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


def downgrade() -> None:
    bind = op.get_bind()
    is_pg = bind.dialect.name == 'postgresql'
    if is_pg:
        op.execute("DROP INDEX IF EXISTS ix_skills_search_tsv")
        op.execute("ALTER TABLE skills DROP COLUMN IF EXISTS search_tsv")
        op.alter_column('skills', 'name',
                   existing_type=sa.String(length=200),
                   type_=sa.VARCHAR(length=300),
                   existing_nullable=False)
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
        op.alter_column('import_jobs', 'generated_tree_json',
                   existing_type=sa.JSON().with_variant(postgresql.JSONB(astext_type=sa.Text()), 'postgresql'),
                   type_=postgresql.JSON(astext_type=sa.Text()),
                   existing_nullable=True)
        op.alter_column('import_jobs', 'confidence_notes',
                   existing_type=sa.JSON().with_variant(postgresql.JSONB(astext_type=sa.Text()), 'postgresql'),
                   type_=postgresql.JSON(astext_type=sa.Text()),
                   existing_nullable=True)

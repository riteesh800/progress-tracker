"""add unique constraint to skill name per user

Revision ID: c5d03379eae0
Revises: 004_skill_idx
Create Date: 2026-09-26 07:25:38.003083
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = 'c5d03379eae0'
down_revision: Union[str, None] = '004_skill_idx'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_unique_constraint("uq_skills_user_name", "skills", ["user_id", "name"])

def downgrade() -> None:
    op.drop_constraint("uq_skills_user_name", "skills", type_="unique")

"""update_skill_name_length

Revision ID: af9f3c5b725c
Revises: c5d03379eae0
Create Date: 2026-09-26 08:10:13.965619
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = 'af9f3c5b725c'
down_revision: Union[str, None] = 'c5d03379eae0'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    with op.batch_alter_table("skills") as batch_op:
        pass


def downgrade() -> None:
    with op.batch_alter_table("skills") as batch_op:
        pass

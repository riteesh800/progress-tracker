"""Merge multiple heads

Revision ID: 33f542905828
Revises: 005_deletion_recovery, c0a7eabeb78e
Create Date: 2026-10-04 10:40:14.906532
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = '33f542905828'
down_revision: Union[str, None] = ('005_deletion_recovery', 'c0a7eabeb78e')
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    pass


def downgrade() -> None:
    pass

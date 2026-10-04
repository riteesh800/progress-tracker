"""Add account deletion recovery fields to users

Revision ID: 005_deletion_recovery
Revises: ecff4c27b63b
Create Date: 2026-09-30 10:00:00.000000
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = '005_deletion_recovery'
down_revision: Union[str, None] = 'ecff4c27b63b'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column('users', sa.Column('deletion_requested_at', sa.DateTime(timezone=True), nullable=True))
    op.add_column('users', sa.Column('deletion_deadline', sa.DateTime(timezone=True), nullable=True))
    op.add_column('users', sa.Column('last_active_at', sa.DateTime(timezone=True), nullable=True))


def downgrade() -> None:
    op.drop_column('users', 'last_active_at')
    op.drop_column('users', 'deletion_deadline')
    op.drop_column('users', 'deletion_requested_at')

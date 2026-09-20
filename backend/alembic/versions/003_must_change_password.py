"""Add must_change_password for admin-issued temporary passwords."""

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "003_must_change_password"
down_revision: Union[str, None] = "002_personal_notes_activity"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column(
        "users",
        sa.Column("must_change_password", sa.Boolean(), nullable=False, server_default=sa.false()),
    )


def downgrade() -> None:
    op.drop_column("users", "must_change_password")

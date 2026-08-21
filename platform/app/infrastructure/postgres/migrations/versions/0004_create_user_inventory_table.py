"""create user_inventory table

Revision ID: 0004_create_user_inventory
Revises: 0003_create_agent_heartbeats
Create Date: 2026-08-14

"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0004_create_user_inventory"
down_revision: Union[str, None] = "0003_create_agent_heartbeats"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "user_inventory",
        sa.Column("sid", sa.String(length=256), nullable=False),
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("username", sa.String(length=256), nullable=False),
        sa.Column("display_name", sa.String(length=256), nullable=False),
        sa.Column("account_type", sa.String(length=64), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("builtin", sa.Boolean(), nullable=False),
        sa.Column("password_required", sa.Boolean(), nullable=False),
        sa.Column("last_logon", sa.DateTime(timezone=True), nullable=True),
        sa.Column("source", sa.String(length=64), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_user_inventory_agent_id_agents",
        ),
        sa.PrimaryKeyConstraint("sid", name="pk_user_inventory"),
        sa.CheckConstraint("length(sid) > 0", name="sid_not_blank"),
        sa.CheckConstraint(
            "length(username) > 0", name="username_not_blank"
        ),
    )
    op.create_index("ix_user_inventory_agent_id", "user_inventory", ["agent_id"])
    op.create_index("ix_user_inventory_username", "user_inventory", ["username"])
    op.create_index("ix_user_inventory_account_type", "user_inventory", ["account_type"])
    op.create_index("ix_user_inventory_enabled", "user_inventory", ["enabled"])


def downgrade() -> None:
    op.drop_index("ix_user_inventory_enabled", table_name="user_inventory")
    op.drop_index("ix_user_inventory_account_type", table_name="user_inventory")
    op.drop_index("ix_user_inventory_username", table_name="user_inventory")
    op.drop_index("ix_user_inventory_agent_id", table_name="user_inventory")
    op.drop_table("user_inventory")

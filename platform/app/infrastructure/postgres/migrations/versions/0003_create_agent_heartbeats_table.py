"""create agent_heartbeats table

Revision ID: 0003_create_agent_heartbeats
Revises: 0002_create_device_inventory
Create Date: 2026-08-14

"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0003_create_agent_heartbeats"
down_revision: Union[str, None] = "0002_create_device_inventory"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "agent_heartbeats",
        sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("heartbeat_time", sa.DateTime(timezone=True), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("status", sa.String(length=32), nullable=False),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_agent_heartbeats_agent_id_agents",
        ),
        sa.PrimaryKeyConstraint("id", name="pk_agent_heartbeats"),
    )
    op.create_index("ix_agent_heartbeats_agent_id", "agent_heartbeats", ["agent_id"])
    op.create_index(
        "ix_agent_heartbeats_heartbeat_time", "agent_heartbeats", ["heartbeat_time"]
    )
    op.create_index(
        "ix_agent_heartbeats_agent_id_heartbeat_time",
        "agent_heartbeats",
        ["agent_id", "heartbeat_time"],
    )


def downgrade() -> None:
    op.drop_index(
        "ix_agent_heartbeats_agent_id_heartbeat_time", table_name="agent_heartbeats"
    )
    op.drop_index("ix_agent_heartbeats_heartbeat_time", table_name="agent_heartbeats")
    op.drop_index("ix_agent_heartbeats_agent_id", table_name="agent_heartbeats")
    op.drop_table("agent_heartbeats")

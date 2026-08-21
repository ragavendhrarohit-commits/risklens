"""create agents table

Revision ID: 0001_create_agents
Revises:
Create Date: 2026-08-14

"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0001_create_agents"
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    """Native uuid on PostgreSQL, CHAR(36) elsewhere.

    Mirrors the GUID TypeDecorator in models.py so the migrated schema and the
    ORM metadata agree on both backends.
    """
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "agents",
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("install_id", sa.String(length=256), nullable=False),
        sa.Column("agent_version", sa.String(length=64), nullable=False),
        sa.Column("machine_name", sa.String(length=256), nullable=False),
        sa.Column("operating_system", sa.String(length=256), nullable=False),
        sa.Column("display_name", sa.String(length=256), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("registered_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("status", sa.String(length=32), nullable=False),
        # PRIMARY KEY on agent_id is what enforces "unique AgentID"; PostgreSQL
        # backs it with a unique index, so a duplicate INSERT raises 23505.
        sa.PrimaryKeyConstraint("agent_id", name="pk_agents"),
        # Bare names: the metadata naming convention expands these to
        # ck_agents_<name>. Pre-prefixing would double up.
        sa.CheckConstraint("length(install_id) > 0", name="install_id_not_blank"),
        sa.CheckConstraint("length(machine_name) > 0", name="machine_name_not_blank"),
    )
    op.create_index("ix_agents_install_id", "agents", ["install_id"])
    op.create_index("ix_agents_machine_name", "agents", ["machine_name"])
    op.create_index("ix_agents_status", "agents", ["status"])
    op.create_index("ix_agents_registered_at", "agents", ["registered_at"])


def downgrade() -> None:
    op.drop_index("ix_agents_registered_at", table_name="agents")
    op.drop_index("ix_agents_status", table_name="agents")
    op.drop_index("ix_agents_machine_name", table_name="agents")
    op.drop_index("ix_agents_install_id", table_name="agents")
    op.drop_table("agents")

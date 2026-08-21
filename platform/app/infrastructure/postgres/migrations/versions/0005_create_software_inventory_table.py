"""create software_inventory table

Revision ID: 0005_create_software_inventory
Revises: 0004_create_user_inventory
Create Date: 2026-08-14

"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0005_create_software_inventory"
down_revision: Union[str, None] = "0004_create_user_inventory"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "software_inventory",
        sa.Column("id", sa.String(length=64), nullable=False),
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("application_name", sa.String(length=512), nullable=False),
        sa.Column("version", sa.String(length=128), nullable=False),
        sa.Column("publisher", sa.String(length=256), nullable=False),
        sa.Column("install_date", sa.String(length=32), nullable=False),
        sa.Column("install_location", sa.String(length=512), nullable=False),
        sa.Column("estimated_size", sa.BigInteger(), nullable=False),
        sa.Column("architecture", sa.String(length=16), nullable=False),
        sa.Column("system_component", sa.Boolean(), nullable=False),
        sa.Column("windows_component", sa.Boolean(), nullable=False),
        sa.Column("registry_path", sa.String(length=512), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_software_inventory_agent_id_agents",
        ),
        sa.PrimaryKeyConstraint("id", name="pk_software_inventory"),
        sa.CheckConstraint("length(id) > 0", name="id_not_blank"),
        sa.CheckConstraint(
            "length(application_name) > 0", name="application_name_not_blank"
        ),
        sa.CheckConstraint(
            "estimated_size >= 0", name="estimated_size_non_negative"
        ),
    )
    op.create_index(
        "ix_software_inventory_agent_id", "software_inventory", ["agent_id"]
    )
    op.create_index(
        "ix_software_inventory_application_name",
        "software_inventory",
        ["application_name"],
    )
    op.create_index(
        "ix_software_inventory_publisher", "software_inventory", ["publisher"]
    )
    op.create_index(
        "ix_software_inventory_architecture",
        "software_inventory",
        ["architecture"],
    )


def downgrade() -> None:
    op.drop_index(
        "ix_software_inventory_architecture", table_name="software_inventory"
    )
    op.drop_index(
        "ix_software_inventory_publisher", table_name="software_inventory"
    )
    op.drop_index(
        "ix_software_inventory_application_name", table_name="software_inventory"
    )
    op.drop_index(
        "ix_software_inventory_agent_id", table_name="software_inventory"
    )
    op.drop_table("software_inventory")

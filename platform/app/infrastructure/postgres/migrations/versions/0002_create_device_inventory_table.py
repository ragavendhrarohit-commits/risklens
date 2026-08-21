"""create device_inventory table

Revision ID: 0002_create_device_inventory
Revises: 0001_create_agents
Create Date: 2026-08-14

"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0002_create_device_inventory"
down_revision: Union[str, None] = "0001_create_agents"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "device_inventory",
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("install_id", sa.String(length=256), nullable=False),
        sa.Column("machine_name", sa.String(length=256), nullable=False),
        sa.Column("hostname", sa.String(length=256), nullable=False),
        sa.Column("operating_system", sa.String(length=256), nullable=False),
        sa.Column("os_version", sa.String(length=128), nullable=False),
        sa.Column("windows_build", sa.String(length=64), nullable=False),
        sa.Column("architecture", sa.String(length=64), nullable=False),
        sa.Column("cpu_model", sa.String(length=256), nullable=False),
        sa.Column("cpu_cores", sa.Integer(), nullable=False),
        sa.Column("memory_bytes", sa.BigInteger(), nullable=False),
        sa.Column("disk_total_bytes", sa.BigInteger(), nullable=False),
        sa.Column("disk_free_bytes", sa.BigInteger(), nullable=False),
        sa.Column("manufacturer", sa.String(length=256), nullable=False),
        sa.Column("model", sa.String(length=256), nullable=False),
        sa.Column("serial_number", sa.String(length=256), nullable=False),
        sa.Column("boot_time", sa.DateTime(timezone=True), nullable=True),
        sa.Column("current_user", sa.String(length=256), nullable=False),
        sa.Column("agent_version", sa.String(length=64), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("agent_id", name="pk_device_inventory"),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_device_inventory_agent_id_agents",
        ),
        sa.CheckConstraint("cpu_cores >= 0", name="cpu_cores_non_negative"),
        sa.CheckConstraint("memory_bytes >= 0", name="memory_bytes_non_negative"),
        sa.CheckConstraint("disk_total_bytes >= 0", name="disk_total_bytes_non_negative"),
        sa.CheckConstraint("disk_free_bytes >= 0", name="disk_free_bytes_non_negative"),
        sa.CheckConstraint("length(install_id) > 0", name="install_id_not_blank"),
        sa.CheckConstraint("length(machine_name) > 0", name="machine_name_not_blank"),
        sa.CheckConstraint("length(hostname) > 0", name="hostname_not_blank"),
    )
    op.create_index("ix_device_inventory_machine_name", "device_inventory", ["machine_name"])
    op.create_index("ix_device_inventory_hostname", "device_inventory", ["hostname"])
    op.create_index("ix_device_inventory_received_at", "device_inventory", ["received_at"])


def downgrade() -> None:
    op.drop_index("ix_device_inventory_received_at", table_name="device_inventory")
    op.drop_index("ix_device_inventory_hostname", table_name="device_inventory")
    op.drop_index("ix_device_inventory_machine_name", table_name="device_inventory")
    op.drop_table("device_inventory")

"""fleet management tables: policies, agent_policy, agent_tags, agent_status

Revision ID: 0006_fleet_management
Revises: 0005_create_software_inventory
Create Date: 2026-08-15

Reuses agent_heartbeats as heartbeat history. Does not duplicate inventory.
"""
from __future__ import annotations

from datetime import datetime, timezone
from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0006_fleet_management"
down_revision: Union[str, None] = "0005_create_software_inventory"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None

DEFAULT_POLICY_ID = "11111111-1111-4111-8111-111111111111"


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "policies",
        sa.Column("id", _uuid_type(), nullable=False),
        sa.Column("name", sa.String(length=256), nullable=False),
        sa.Column("heartbeat_interval_seconds", sa.Integer(), nullable=False),
        sa.Column("collect_devices", sa.Boolean(), nullable=False),
        sa.Column("collect_users", sa.Boolean(), nullable=False),
        sa.Column("collect_software", sa.Boolean(), nullable=False),
        sa.Column("collect_processes", sa.Boolean(), nullable=False),
        sa.Column("collect_services", sa.Boolean(), nullable=False),
        sa.Column("collect_event_logs", sa.Boolean(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("id", name="pk_policies"),
        sa.UniqueConstraint("name", name="uq_policies_name"),
        sa.CheckConstraint("length(name) > 0", name="name_not_blank"),
        sa.CheckConstraint(
            "heartbeat_interval_seconds > 0", name="heartbeat_interval_positive"
        ),
    )

    policies = sa.table(
        "policies",
        sa.column("id", _uuid_type()),
        sa.column("name", sa.String),
        sa.column("heartbeat_interval_seconds", sa.Integer),
        sa.column("collect_devices", sa.Boolean),
        sa.column("collect_users", sa.Boolean),
        sa.column("collect_software", sa.Boolean),
        sa.column("collect_processes", sa.Boolean),
        sa.column("collect_services", sa.Boolean),
        sa.column("collect_event_logs", sa.Boolean),
        sa.column("created_at", sa.DateTime(timezone=True)),
    )
    op.bulk_insert(
        policies,
        [
            {
                "id": DEFAULT_POLICY_ID,
                "name": "Default Windows Policy",
                "heartbeat_interval_seconds": 30,
                "collect_devices": True,
                "collect_users": True,
                "collect_software": True,
                "collect_processes": False,
                "collect_services": False,
                "collect_event_logs": False,
                "created_at": datetime.now(timezone.utc),
            }
        ],
    )

    op.create_table(
        "agent_policy",
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("policy_id", _uuid_type(), nullable=False),
        sa.Column("assigned_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(
            ["agent_id"], ["agents.agent_id"], name="fk_agent_policy_agent_id_agents"
        ),
        sa.ForeignKeyConstraint(
            ["policy_id"], ["policies.id"], name="fk_agent_policy_policy_id_policies"
        ),
        sa.PrimaryKeyConstraint("agent_id", name="pk_agent_policy"),
    )
    op.create_index("ix_agent_policy_policy_id", "agent_policy", ["policy_id"])

    op.create_table(
        "agent_tags",
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("tag", sa.String(length=64), nullable=False),
        sa.ForeignKeyConstraint(
            ["agent_id"], ["agents.agent_id"], name="fk_agent_tags_agent_id_agents"
        ),
        sa.PrimaryKeyConstraint("agent_id", "tag", name="pk_agent_tags"),
        sa.CheckConstraint("length(tag) > 0", name="tag_not_blank"),
        sa.CheckConstraint(
            "tag IN ('Finance','HR','Production','Development','Servers','Lab')",
            name="allowed_tag",
        ),
    )
    op.create_index("ix_agent_tags_tag", "agent_tags", ["tag"])

    op.create_table(
        "agent_status",
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("last_seen", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_inventory_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_communication_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("ip_address", sa.String(length=64), nullable=True),
        sa.ForeignKeyConstraint(
            ["agent_id"], ["agents.agent_id"], name="fk_agent_status_agent_id_agents"
        ),
        sa.PrimaryKeyConstraint("agent_id", name="pk_agent_status"),
    )
    op.create_index("ix_agent_status_last_seen", "agent_status", ["last_seen"])

    bind = op.get_bind()
    bind.execute(
        sa.text(
            """
            INSERT INTO agent_policy (agent_id, policy_id, assigned_at)
            SELECT a.agent_id, CAST(:policy_id AS uuid), CURRENT_TIMESTAMP
            FROM agents a
            WHERE NOT EXISTS (
                SELECT 1 FROM agent_policy p WHERE p.agent_id = a.agent_id
            )
            """
        ),
        {"policy_id": DEFAULT_POLICY_ID},
    )
    bind.execute(
        sa.text(
            """
            INSERT INTO agent_status (
                agent_id, last_seen, last_inventory_at, last_communication_at, ip_address
            )
            SELECT
                a.agent_id,
                h.last_seen,
                d.received_at,
                CASE
                    WHEN h.last_received IS NULL THEN d.received_at
                    WHEN d.received_at IS NULL THEN h.last_received
                    WHEN h.last_received >= d.received_at THEN h.last_received
                    ELSE d.received_at
                END,
                NULL
            FROM agents a
            LEFT JOIN (
                SELECT agent_id,
                       MAX(heartbeat_time) AS last_seen,
                       MAX(received_at) AS last_received
                FROM agent_heartbeats
                GROUP BY agent_id
            ) h ON h.agent_id = a.agent_id
            LEFT JOIN device_inventory d ON d.agent_id = a.agent_id
            WHERE NOT EXISTS (
                SELECT 1 FROM agent_status s WHERE s.agent_id = a.agent_id
            )
            """
        )
    )


def downgrade() -> None:
    op.drop_index("ix_agent_status_last_seen", table_name="agent_status")
    op.drop_table("agent_status")
    op.drop_index("ix_agent_tags_tag", table_name="agent_tags")
    op.drop_table("agent_tags")
    op.drop_index("ix_agent_policy_policy_id", table_name="agent_policy")
    op.drop_table("agent_policy")
    op.drop_table("policies")

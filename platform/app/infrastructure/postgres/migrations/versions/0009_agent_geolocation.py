"""agent geolocation table

Revision ID: 0009_agent_geolocation
Revises: 0008_browser_activity
Create Date: 2026-08-22
"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0009_agent_geolocation"
down_revision: Union[str, None] = "0008_browser_activity"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "agent_geolocation",
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("public_ip", sa.String(length=64), nullable=True),
        sa.Column("private_ip", sa.String(length=64), nullable=True),
        sa.Column("country", sa.String(length=128), nullable=True),
        sa.Column("state", sa.String(length=128), nullable=True),
        sa.Column("city", sa.String(length=128), nullable=True),
        sa.Column("latitude", sa.Float(), nullable=True),
        sa.Column("longitude", sa.Float(), nullable=True),
        sa.Column("timezone", sa.String(length=128), nullable=True),
        sa.Column("isp", sa.String(length=256), nullable=True),
        sa.Column("asn", sa.String(length=256), nullable=True),
        sa.Column("last_updated", sa.DateTime(timezone=True), nullable=True),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_agent_geolocation_agent_id_agents",
        ),
        sa.PrimaryKeyConstraint("agent_id", name="pk_agent_geolocation"),
    )
    op.create_index("ix_agent_geolocation_public_ip", "agent_geolocation", ["public_ip"])
    op.create_index("ix_agent_geolocation_country", "agent_geolocation", ["country"])
    op.create_index("ix_agent_geolocation_city", "agent_geolocation", ["city"])


def downgrade() -> None:
    op.drop_index("ix_agent_geolocation_city", table_name="agent_geolocation")
    op.drop_index("ix_agent_geolocation_country", table_name="agent_geolocation")
    op.drop_index("ix_agent_geolocation_public_ip", table_name="agent_geolocation")
    op.drop_table("agent_geolocation")

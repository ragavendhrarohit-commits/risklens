"""heartbeat health metrics json

Revision ID: 0010_heartbeat_health_metrics
Revises: 0009_agent_geolocation
Create Date: 2026-08-23
"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "0010_heartbeat_health_metrics"
down_revision: Union[str, None] = "0009_agent_geolocation"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column(
        "agent_heartbeats",
        sa.Column("health_metrics", sa.JSON(), nullable=True),
    )


def downgrade() -> None:
    op.drop_column("agent_heartbeats", "health_metrics")

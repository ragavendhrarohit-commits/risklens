"""browser activity labels

Revision ID: 0008_browser_activity
Revises: 0007_session_app_activity
Create Date: 2026-08-21

Stores Browser Activity labels joined to computer sessions. Does not alter
application_activity or other inventory tables. Browser rows are labels, not
a second clock.
"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0008_browser_activity"
down_revision: Union[str, None] = "0007_session_app_activity"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "browser_activity",
        sa.Column("id", sa.String(length=64), nullable=False),
        sa.Column("session_id", _uuid_type(), nullable=False),
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("executable_name", sa.String(length=512), nullable=False),
        sa.Column("domain", sa.String(length=256), nullable=False),
        sa.Column("url", sa.String(length=512), nullable=False),
        sa.Column("title", sa.String(length=512), nullable=False),
        sa.Column("profile", sa.String(length=256), nullable=False),
        sa.Column("window_id", sa.BigInteger(), nullable=False),
        sa.Column("tab_id", sa.BigInteger(), nullable=False),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("stopped_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("duration_seconds", sa.Integer(), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("length(id) > 0", name="browser_activity_id_not_blank"),
        sa.CheckConstraint("length(domain) > 0", name="browser_domain_not_blank"),
        sa.CheckConstraint("duration_seconds >= 0", name="browser_duration_seconds_non_negative"),
        sa.ForeignKeyConstraint(
            ["session_id"],
            ["computer_sessions.session_id"],
            name="fk_browser_activity_session_id_computer_sessions",
        ),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_browser_activity_agent_id_agents",
        ),
        sa.PrimaryKeyConstraint("id", name="pk_browser_activity"),
    )
    op.create_index(
        "ix_browser_activity_session_id_started_at",
        "browser_activity",
        ["session_id", "started_at"],
    )
    op.create_index(
        "ix_browser_activity_agent_id_started_at",
        "browser_activity",
        ["agent_id", "started_at"],
    )


def downgrade() -> None:
    op.drop_index("ix_browser_activity_agent_id_started_at", table_name="browser_activity")
    op.drop_index("ix_browser_activity_session_id_started_at", table_name="browser_activity")
    op.drop_table("browser_activity")

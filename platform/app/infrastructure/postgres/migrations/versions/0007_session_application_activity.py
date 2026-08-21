"""computer sessions and application activity

Revision ID: 0007_session_app_activity
Revises: 0006_fleet_management
Create Date: 2026-08-15

Stores Computer → Session → Application Activity. Does not alter inventory tables.
"""
from __future__ import annotations

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0007_session_app_activity"
down_revision: Union[str, None] = "0006_fleet_management"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def _uuid_type() -> sa.types.TypeEngine:
    return postgresql.UUID(as_uuid=True).with_variant(sa.String(36), "sqlite")


def upgrade() -> None:
    op.create_table(
        "computer_sessions",
        sa.Column("session_id", _uuid_type(), nullable=False),
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("username", sa.String(length=256), nullable=False),
        sa.Column("account_sid", sa.String(length=256), nullable=False),
        sa.Column("logon_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("logoff_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("status", sa.String(length=32), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("length(username) > 0", name="username_not_blank"),
        sa.CheckConstraint("status IN ('active','closed')", name="session_status_allowed"),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_computer_sessions_agent_id_agents",
        ),
        sa.PrimaryKeyConstraint("session_id", name="pk_computer_sessions"),
    )
    op.create_index(
        "ix_computer_sessions_agent_id_logon_at",
        "computer_sessions",
        ["agent_id", "logon_at"],
    )
    op.create_index(
        "ix_computer_sessions_agent_id_status",
        "computer_sessions",
        ["agent_id", "status"],
    )

    op.create_table(
        "application_activity",
        sa.Column("id", sa.String(length=64), nullable=False),
        sa.Column("session_id", _uuid_type(), nullable=False),
        sa.Column("agent_id", _uuid_type(), nullable=False),
        sa.Column("application_name", sa.String(length=512), nullable=False),
        sa.Column("executable_name", sa.String(length=512), nullable=False),
        sa.Column("window_title", sa.String(length=512), nullable=False),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("stopped_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("duration_seconds", sa.Integer(), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("length(id) > 0", name="activity_id_not_blank"),
        sa.CheckConstraint("length(application_name) > 0", name="application_name_not_blank"),
        sa.CheckConstraint("duration_seconds >= 0", name="duration_seconds_non_negative"),
        sa.ForeignKeyConstraint(
            ["session_id"],
            ["computer_sessions.session_id"],
            name="fk_application_activity_session_id_computer_sessions",
        ),
        sa.ForeignKeyConstraint(
            ["agent_id"],
            ["agents.agent_id"],
            name="fk_application_activity_agent_id_agents",
        ),
        sa.PrimaryKeyConstraint("id", name="pk_application_activity"),
    )
    op.create_index(
        "ix_application_activity_session_id_started_at",
        "application_activity",
        ["session_id", "started_at"],
    )
    op.create_index(
        "ix_application_activity_agent_id_started_at",
        "application_activity",
        ["agent_id", "started_at"],
    )


def downgrade() -> None:
    op.drop_index("ix_application_activity_agent_id_started_at", table_name="application_activity")
    op.drop_index("ix_application_activity_session_id_started_at", table_name="application_activity")
    op.drop_table("application_activity")
    op.drop_index("ix_computer_sessions_agent_id_status", table_name="computer_sessions")
    op.drop_index("ix_computer_sessions_agent_id_logon_at", table_name="computer_sessions")
    op.drop_table("computer_sessions")

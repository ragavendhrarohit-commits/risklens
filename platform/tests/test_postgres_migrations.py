"""Migration tests: the Alembic revision must produce exactly the ORM schema."""

from __future__ import annotations

from pathlib import Path

import pytest
from alembic import command
from alembic.autogenerate import compare_metadata
from alembic.config import Config
from alembic.migration import MigrationContext
from alembic.script import ScriptDirectory
from sqlalchemy import create_engine, inspect

from app.infrastructure.postgres.models import Base

ROOT = Path(__file__).resolve().parents[1]
EXPECTED_HEAD = "0008_browser_activity"

EXPECTED_COLUMNS = {
    "agent_id",
    "install_id",
    "agent_version",
    "machine_name",
    "operating_system",
    "display_name",
    "created_at",
    "registered_at",
    "status",
}

EXPECTED_DEVICE_COLUMNS = {
    "agent_id",
    "install_id",
    "machine_name",
    "hostname",
    "operating_system",
    "os_version",
    "windows_build",
    "architecture",
    "cpu_model",
    "cpu_cores",
    "memory_bytes",
    "disk_total_bytes",
    "disk_free_bytes",
    "manufacturer",
    "model",
    "serial_number",
    "boot_time",
    "current_user",
    "agent_version",
    "collected_at",
    "received_at",
}


def _config(url: str) -> Config:
    config = Config(str(ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(ROOT / "app/infrastructure/postgres/migrations"))
    config.set_main_option("sqlalchemy.url", url)
    return config


@pytest.fixture
def engine(migrated_postgres: str):
    engine = create_engine(migrated_postgres, future=True)
    yield engine
    engine.dispose()


def test_alembic_has_a_single_head() -> None:
    """Multiple heads mean someone branched the history without merging."""
    script = ScriptDirectory.from_config(_config("postgresql+psycopg://unused/unused"))
    assert list(script.get_heads()) == [EXPECTED_HEAD]


def test_database_is_stamped_at_head(engine, migrated_postgres: str) -> None:
    with engine.connect() as connection:
        context = MigrationContext.configure(connection)
        assert context.get_current_revision() == EXPECTED_HEAD


def test_agents_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "agents" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("agents")} == EXPECTED_COLUMNS


def test_device_inventory_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "device_inventory" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("device_inventory")} == EXPECTED_DEVICE_COLUMNS
    pk = inspector.get_pk_constraint("device_inventory")
    assert pk["constrained_columns"] == ["agent_id"]
    fks = inspector.get_foreign_keys("device_inventory")
    assert any(fk["referred_table"] == "agents" for fk in fks)


EXPECTED_HEARTBEAT_COLUMNS = {
    "id",
    "agent_id",
    "heartbeat_time",
    "received_at",
    "status",
}


def test_agent_heartbeats_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "agent_heartbeats" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("agent_heartbeats")} == EXPECTED_HEARTBEAT_COLUMNS
    pk = inspector.get_pk_constraint("agent_heartbeats")
    assert pk["constrained_columns"] == ["id"]
    fks = inspector.get_foreign_keys("agent_heartbeats")
    assert any(fk["referred_table"] == "agents" for fk in fks)


EXPECTED_USER_COLUMNS = {
    "sid",
    "agent_id",
    "username",
    "display_name",
    "account_type",
    "enabled",
    "builtin",
    "password_required",
    "last_logon",
    "source",
    "collected_at",
    "received_at",
}


def test_user_inventory_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "user_inventory" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("user_inventory")} == EXPECTED_USER_COLUMNS
    pk = inspector.get_pk_constraint("user_inventory")
    assert pk["constrained_columns"] == ["sid"]


EXPECTED_SOFTWARE_COLUMNS = {
    "id",
    "agent_id",
    "application_name",
    "version",
    "publisher",
    "install_date",
    "install_location",
    "estimated_size",
    "architecture",
    "system_component",
    "windows_component",
    "registry_path",
    "collected_at",
    "received_at",
}


EXPECTED_SESSION_COLUMNS = {
    "session_id",
    "agent_id",
    "username",
    "account_sid",
    "logon_at",
    "logoff_at",
    "status",
    "collected_at",
    "received_at",
}


def test_computer_sessions_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "computer_sessions" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("computer_sessions")} == EXPECTED_SESSION_COLUMNS
    pk = inspector.get_pk_constraint("computer_sessions")
    assert pk["constrained_columns"] == ["session_id"]
    fks = inspector.get_foreign_keys("computer_sessions")
    assert any(fk["referred_table"] == "agents" for fk in fks)


EXPECTED_APPLICATION_ACTIVITY_COLUMNS = {
    "id",
    "session_id",
    "agent_id",
    "application_name",
    "executable_name",
    "window_title",
    "started_at",
    "stopped_at",
    "duration_seconds",
    "collected_at",
    "received_at",
}


def test_application_activity_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "application_activity" in inspector.get_table_names()
    assert {
        c["name"] for c in inspector.get_columns("application_activity")
    } == EXPECTED_APPLICATION_ACTIVITY_COLUMNS
    pk = inspector.get_pk_constraint("application_activity")
    assert pk["constrained_columns"] == ["id"]
    fks = inspector.get_foreign_keys("application_activity")
    referred = {fk["referred_table"] for fk in fks}
    assert "computer_sessions" in referred
    assert "agents" in referred


EXPECTED_BROWSER_ACTIVITY_COLUMNS = {
    "id",
    "session_id",
    "agent_id",
    "executable_name",
    "domain",
    "url",
    "title",
    "profile",
    "window_id",
    "tab_id",
    "started_at",
    "stopped_at",
    "duration_seconds",
    "collected_at",
    "received_at",
}


def test_browser_activity_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "browser_activity" in inspector.get_table_names()
    assert {
        c["name"] for c in inspector.get_columns("browser_activity")
    } == EXPECTED_BROWSER_ACTIVITY_COLUMNS
    pk = inspector.get_pk_constraint("browser_activity")
    assert pk["constrained_columns"] == ["id"]
    fks = inspector.get_foreign_keys("browser_activity")
    referred = {fk["referred_table"] for fk in fks}
    assert "computer_sessions" in referred
    assert "agents" in referred
    assert {
        c["name"] for c in inspector.get_columns("application_activity")
    } == EXPECTED_APPLICATION_ACTIVITY_COLUMNS


def test_software_inventory_table_exists_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "software_inventory" in inspector.get_table_names()
    assert {
        c["name"] for c in inspector.get_columns("software_inventory")
    } == EXPECTED_SOFTWARE_COLUMNS
    pk = inspector.get_pk_constraint("software_inventory")
    assert pk["constrained_columns"] == ["id"]
    fks = inspector.get_foreign_keys("software_inventory")
    assert any(fk["referred_table"] == "agents" for fk in fks)


EXPECTED_POLICY_COLUMNS = {
    "id",
    "name",
    "heartbeat_interval_seconds",
    "collect_devices",
    "collect_users",
    "collect_software",
    "collect_processes",
    "collect_services",
    "collect_event_logs",
    "created_at",
}

EXPECTED_AGENT_STATUS_COLUMNS = {
    "agent_id",
    "last_seen",
    "last_inventory_at",
    "last_communication_at",
    "ip_address",
}


def test_fleet_tables_exist_with_expected_columns(engine) -> None:
    inspector = inspect(engine)
    assert "policies" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("policies")} == EXPECTED_POLICY_COLUMNS
    assert "agent_policy" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("agent_policy")} == {
        "agent_id",
        "policy_id",
        "assigned_at",
    }
    assert "agent_tags" in inspector.get_table_names()
    assert {c["name"] for c in inspector.get_columns("agent_tags")} == {"agent_id", "tag"}
    assert "agent_status" in inspector.get_table_names()
    assert {
        c["name"] for c in inspector.get_columns("agent_status")
    } == EXPECTED_AGENT_STATUS_COLUMNS
    assert "heartbeat_history" not in inspector.get_table_names()
    assert "agent_heartbeats" in inspector.get_table_names()


def test_default_windows_policy_is_seeded(engine) -> None:
    from sqlalchemy import text

    with engine.connect() as connection:
        row = connection.execute(
            text("SELECT name, collect_devices, collect_users, collect_software FROM policies")
        ).first()
    assert row is not None
    assert row[0] == "Default Windows Policy"
    assert row[1] is True
    assert row[2] is True
    assert row[3] is True


def test_agent_id_is_the_primary_key(engine) -> None:
    primary_key = inspect(engine).get_pk_constraint("agents")
    assert primary_key["constrained_columns"] == ["agent_id"]
    assert primary_key["name"] == "pk_agents"


def test_agent_id_is_uniquely_indexed(engine) -> None:
    """The PK index is what rejects a duplicate AgentID at the database level."""
    with engine.connect() as connection:
        from sqlalchemy import text

        rows = connection.execute(
            text(
                "SELECT i.relname, ix.indisunique FROM pg_index ix"
                " JOIN pg_class i ON i.oid = ix.indexrelid"
                " JOIN pg_class t ON t.oid = ix.indrelid"
                " WHERE t.relname = 'agents' AND ix.indisunique"
            )
        ).all()
    unique_index_names = {name for name, _ in rows}
    assert "pk_agents" in unique_index_names


def test_expected_secondary_indexes_exist(engine) -> None:
    names = {index["name"] for index in inspect(engine).get_indexes("agents")}
    assert {
        "ix_agents_install_id",
        "ix_agents_machine_name",
        "ix_agents_status",
        "ix_agents_registered_at",
    } <= names


def test_nullability_matches_the_domain(engine) -> None:
    columns = {c["name"]: c for c in inspect(engine).get_columns("agents")}
    assert columns["display_name"]["nullable"] is True
    for required in EXPECTED_COLUMNS - {"display_name"}:
        assert columns[required]["nullable"] is False, required


def test_timestamps_are_timezone_aware(engine) -> None:
    columns = {c["name"]: c for c in inspect(engine).get_columns("agents")}
    assert columns["created_at"]["type"].timezone is True
    assert columns["registered_at"]["type"].timezone is True


def test_check_constraints_reject_blank_identifiers(engine) -> None:
    names = {c["name"] for c in inspect(engine).get_check_constraints("agents")}
    assert "ck_agents_install_id_not_blank" in names
    assert "ck_agents_machine_name_not_blank" in names


def test_migrated_schema_matches_orm_metadata(engine) -> None:
    """Autogenerate must find nothing: no drift between models.py and the DDL."""
    with engine.connect() as connection:
        context = MigrationContext.configure(
            connection,
            opts={"compare_type": True, "compare_server_default": True},
        )
        diff = compare_metadata(context, Base.metadata)
    assert diff == [], f"schema drift detected: {diff}"


def test_downgrade_then_upgrade_restores_the_schema(postgres_url: str) -> None:
    config = _config(postgres_url)
    engine = create_engine(postgres_url, future=True)
    try:
        command.downgrade(config, "base")
        assert "agents" not in inspect(engine).get_table_names()

        command.upgrade(config, "head")
        inspector = inspect(engine)
        assert "agents" in inspector.get_table_names()
        assert {c["name"] for c in inspector.get_columns("agents")} == EXPECTED_COLUMNS
    finally:
        command.upgrade(config, "head")
        engine.dispose()


def test_upgrade_is_idempotent(postgres_url: str) -> None:
    """Re-running upgrade on an up-to-date database must be a no-op."""
    config = _config(postgres_url)
    command.upgrade(config, "head")
    command.upgrade(config, "head")

    engine = create_engine(postgres_url, future=True)
    try:
        with engine.connect() as connection:
            context = MigrationContext.configure(connection)
            assert context.get_current_revision() == EXPECTED_HEAD
    finally:
        engine.dispose()

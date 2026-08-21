"""Repository tests: PostgresAgentRepository against a real PostgreSQL database.

These mirror the MemoryAgentRepository suite so both adapters are proven to
satisfy the same AgentRepository contract.
"""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta, timezone
from uuid import uuid4

import pytest
from sqlalchemy import text

from app.domain.agent import Agent, AgentStatus, DuplicateAgentError
from app.infrastructure.postgres import Database, PostgresAgentRepository


def _agent(**overrides: object) -> Agent:
    base = {
        "agent_id": uuid4(),
        "install_id": "install-1",
        "agent_version": "0.1.0",
        "machine_name": "DESKTOP-01",
        "operating_system": "Windows 11 Pro",
        "display_name": None,
        "created_at": datetime(2026, 8, 14, tzinfo=timezone.utc),
        "registered_at": datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
    }
    base.update(overrides)
    return Agent(**base)  # type: ignore[arg-type]


def test_add_then_get(pg_repository: PostgresAgentRepository) -> None:
    agent = _agent(display_name="Reception PC")
    pg_repository.add(agent)

    assert pg_repository.exists(agent.agent_id) is True
    assert pg_repository.get(agent.agent_id) == agent


def test_get_returns_none_for_missing(pg_repository: PostgresAgentRepository) -> None:
    assert pg_repository.get(uuid4()) is None
    assert pg_repository.exists(uuid4()) is False


def test_add_rejects_duplicate_id(pg_repository: PostgresAgentRepository) -> None:
    first = _agent()
    pg_repository.add(first)

    second = _agent(
        agent_id=first.agent_id,
        install_id="different",
        agent_version="9.9.9",
        machine_name="OTHER",
        operating_system="Linux",
    )
    with pytest.raises(DuplicateAgentError) as excinfo:
        pg_repository.add(second)

    assert excinfo.value.agent_id == first.agent_id
    assert excinfo.value.status_code == 409


def test_duplicate_add_does_not_overwrite_the_original_row(
    pg_repository: PostgresAgentRepository,
) -> None:
    original = _agent(install_id="original", machine_name="ORIGINAL")
    pg_repository.add(original)

    with pytest.raises(DuplicateAgentError):
        pg_repository.add(_agent(agent_id=original.agent_id, install_id="attacker"))

    stored = pg_repository.get(original.agent_id)
    assert stored is not None
    assert stored.install_id == "original"
    assert stored.machine_name == "ORIGINAL"


def test_duplicate_add_leaves_session_usable(pg_repository: PostgresAgentRepository) -> None:
    """A rejected insert must not poison the connection for later writes."""
    first = _agent()
    pg_repository.add(first)

    with pytest.raises(DuplicateAgentError):
        pg_repository.add(_agent(agent_id=first.agent_id))

    other = _agent()
    pg_repository.add(other)
    assert pg_repository.exists(other.agent_id) is True
    assert len(pg_repository.list()) == 2


def test_list_returns_all_registered_agents(pg_repository: PostgresAgentRepository) -> None:
    a, b = _agent(), _agent()
    pg_repository.add(a)
    pg_repository.add(b)

    assert {agent.agent_id for agent in pg_repository.list()} == {a.agent_id, b.agent_id}


def test_list_is_ordered_by_registration_time(
    pg_repository: PostgresAgentRepository,
) -> None:
    now = datetime(2026, 8, 14, 12, tzinfo=timezone.utc)
    late = _agent(registered_at=now + timedelta(minutes=5))
    early = _agent(registered_at=now)

    pg_repository.add(late)
    pg_repository.add(early)

    assert [a.agent_id for a in pg_repository.list()] == [early.agent_id, late.agent_id]


def test_list_is_empty_on_a_fresh_database(pg_repository: PostgresAgentRepository) -> None:
    assert pg_repository.list() == []


def test_roundtrip_preserves_every_field(pg_repository: PostgresAgentRepository) -> None:
    agent = _agent(
        install_id="install-abc-123",
        agent_version="1.2.3-rc4",
        machine_name="WS-FINANCE-07",
        operating_system="Windows Server 2022 Datacenter",
        display_name="Finance Workstation 07",
        created_at=datetime(2026, 1, 2, 3, 4, 5, tzinfo=timezone.utc),
        registered_at=datetime(2026, 6, 7, 8, 9, 10, tzinfo=timezone.utc),
    )
    pg_repository.add(agent)

    stored = pg_repository.get(agent.agent_id)
    assert stored == agent
    assert stored is not None
    assert stored.status is AgentStatus.registered


def test_timestamps_are_stored_as_utc_aware(pg_repository: PostgresAgentRepository) -> None:
    """A non-UTC input must come back as the same instant, tz-aware."""
    ist = timezone(timedelta(hours=5, minutes=30))
    agent = _agent(created_at=datetime(2026, 8, 14, 12, 0, tzinfo=ist))
    pg_repository.add(agent)

    stored = pg_repository.get(agent.agent_id)
    assert stored is not None
    assert stored.created_at.tzinfo is not None
    assert stored.created_at == datetime(2026, 8, 14, 6, 30, tzinfo=timezone.utc)


def test_unicode_values_survive_the_roundtrip(pg_repository: PostgresAgentRepository) -> None:
    agent = _agent(machine_name="PC-Müller-东京", display_name="Tokyo Büro")
    pg_repository.add(agent)

    stored = pg_repository.get(agent.agent_id)
    assert stored is not None
    assert stored.machine_name == "PC-Müller-东京"
    assert stored.display_name == "Tokyo Büro"


def test_null_display_name_is_persisted_as_null(
    pg_repository: PostgresAgentRepository, pg_database: Database
) -> None:
    agent = _agent(display_name=None)
    pg_repository.add(agent)

    with pg_database.engine.connect() as connection:
        value = connection.execute(
            text("SELECT display_name FROM agents WHERE agent_id = :id"),
            {"id": agent.agent_id},
        ).scalar_one()
    assert value is None
    stored = pg_repository.get(agent.agent_id)
    assert stored is not None and stored.display_name is None


def test_data_survives_a_new_repository_instance(pg_database: Database) -> None:
    """Proves persistence: a second repository object sees the same rows."""
    writer = PostgresAgentRepository(pg_database.session_factory)
    agent = _agent()
    writer.add(agent)

    reader = PostgresAgentRepository(pg_database.session_factory)
    assert reader.get(agent.agent_id) == agent


def test_concurrent_add_of_same_id_produces_single_winner(
    pg_repository: PostgresAgentRepository,
) -> None:
    """The database primary key, not application locking, resolves the race."""
    shared_id = uuid4()

    def worker(idx: int) -> bool:
        try:
            pg_repository.add(_agent(agent_id=shared_id, install_id=f"i-{idx}"))
            return True
        except DuplicateAgentError:
            return False

    with ThreadPoolExecutor(max_workers=8) as pool:
        results = list(pool.map(worker, range(16)))

    assert results.count(True) == 1
    assert results.count(False) == 15
    assert len(pg_repository.list()) == 1


def test_concurrent_distinct_adds_all_succeed(
    pg_repository: PostgresAgentRepository,
) -> None:
    def worker(idx: int) -> None:
        pg_repository.add(_agent(install_id=f"i-{idx}"))

    with ThreadPoolExecutor(max_workers=8) as pool:
        list(pool.map(worker, range(24)))

    assert len(pg_repository.list()) == 24


def test_integrity_errors_that_are_not_duplicates_are_not_swallowed(
    pg_database: Database,
) -> None:
    """A CHECK violation must surface, not masquerade as DuplicateAgentError."""
    from sqlalchemy.exc import IntegrityError

    with pytest.raises(IntegrityError):
        with pg_database.engine.begin() as connection:
            connection.execute(
                text(
                    "INSERT INTO agents (agent_id, install_id, agent_version,"
                    " machine_name, operating_system, display_name, created_at,"
                    " registered_at, status) VALUES (:id, '', '0.1.0', 'M', 'OS',"
                    " NULL, now(), now(), 'registered')"
                ),
                {"id": uuid4()},
            )

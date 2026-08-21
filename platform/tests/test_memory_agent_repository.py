from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from uuid import uuid4

import pytest

from app.domain.agent import Agent, DuplicateAgentError
from app.infrastructure.memory import MemoryAgentRepository


def _agent() -> Agent:
    return Agent(
        agent_id=uuid4(),
        install_id="i",
        agent_version="0.1.0",
        machine_name="M",
        operating_system="OS",
        display_name=None,
        created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
        registered_at=datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
    )


def test_add_then_get() -> None:
    repo = MemoryAgentRepository()
    agent = _agent()
    repo.add(agent)

    assert repo.exists(agent.agent_id) is True
    assert repo.get(agent.agent_id) == agent


def test_get_returns_none_for_missing() -> None:
    repo = MemoryAgentRepository()
    assert repo.get(uuid4()) is None


def test_add_rejects_duplicate_id() -> None:
    repo = MemoryAgentRepository()
    a = _agent()
    repo.add(a)

    b = Agent(
        agent_id=a.agent_id,
        install_id="different",
        agent_version="9.9.9",
        machine_name="OTHER",
        operating_system="Linux",
        display_name=None,
        created_at=a.created_at,
        registered_at=a.registered_at,
    )
    with pytest.raises(DuplicateAgentError) as excinfo:
        repo.add(b)
    assert excinfo.value.agent_id == a.agent_id
    assert excinfo.value.status_code == 409


def test_list_returns_all_registered_agents() -> None:
    repo = MemoryAgentRepository()
    a, b = _agent(), _agent()
    repo.add(a)
    repo.add(b)
    ids = {agent.agent_id for agent in repo.list()}
    assert ids == {a.agent_id, b.agent_id}


def test_clear_empties_repository() -> None:
    repo = MemoryAgentRepository()
    repo.add(_agent())
    repo.clear()
    assert repo.list() == []


def test_concurrent_add_of_same_id_produces_single_winner() -> None:
    repo = MemoryAgentRepository()
    shared_id = uuid4()

    def make(idx: int) -> Agent:
        return Agent(
            agent_id=shared_id,
            install_id=f"i-{idx}",
            agent_version="0.1.0",
            machine_name="M",
            operating_system="OS",
            display_name=None,
            created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
            registered_at=datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
        )

    def worker(idx: int) -> bool:
        try:
            repo.add(make(idx))
            return True
        except DuplicateAgentError:
            return False

    with ThreadPoolExecutor(max_workers=8) as pool:
        results = list(pool.map(worker, range(32)))

    assert results.count(True) == 1
    assert results.count(False) == 31
    assert len(repo.list()) == 1

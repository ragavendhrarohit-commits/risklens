from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

import pytest

from app.domain.agent import DuplicateAgentError, RegisterAgentCommand
from app.infrastructure.memory import MemoryAgentRepository
from app.services.agent_service import AgentRegistrationService


FIXED_NOW = datetime(2026, 8, 14, 1, 23, 45, tzinfo=timezone.utc)


def _command(**overrides) -> RegisterAgentCommand:
    defaults = dict(
        agent_id=uuid4(),
        install_id="install-1",
        agent_version="0.1.0",
        machine_name="ENDPOINT-01",
        operating_system="Windows 11",
        display_name="Lab machine",
        created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
    )
    defaults.update(overrides)
    return RegisterAgentCommand(**defaults)  # type: ignore[arg-type]


def _service() -> tuple[AgentRegistrationService, MemoryAgentRepository]:
    repo = MemoryAgentRepository()
    return AgentRegistrationService(repo, clock=lambda: FIXED_NOW), repo


def test_register_persists_agent_and_sets_registered_at() -> None:
    service, repo = _service()
    cmd = _command()

    agent = service.register(cmd)

    assert repo.exists(cmd.agent_id) is True
    assert agent.agent_id == cmd.agent_id
    assert agent.registered_at == FIXED_NOW
    assert agent.status.value == "registered"
    assert agent.install_id == "install-1"


def test_register_trims_whitespace_on_input() -> None:
    service, _ = _service()
    cmd = _command(install_id="  install-2  ", machine_name="  HOST  ")

    agent = service.register(cmd)
    assert agent.install_id == "install-2"
    assert agent.machine_name == "HOST"


def test_register_normalizes_blank_display_name_to_none() -> None:
    service, _ = _service()
    cmd = _command(display_name="   ")
    agent = service.register(cmd)
    assert agent.display_name is None


def test_register_rejects_duplicate_agent_id() -> None:
    service, _ = _service()
    cmd = _command()
    service.register(cmd)

    with pytest.raises(DuplicateAgentError) as excinfo:
        service.register(cmd)
    assert excinfo.value.agent_id == cmd.agent_id
    assert excinfo.value.status_code == 409
    assert excinfo.value.code == "duplicate_agent"


def test_register_two_distinct_agents_succeeds() -> None:
    service, repo = _service()
    a = service.register(_command())
    b = service.register(_command())
    assert a.agent_id != b.agent_id
    assert len(repo.list()) == 2

from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID, uuid4

import pytest

from app.domain.agent import Agent, AgentStatus


def _make_agent(**overrides) -> Agent:
    defaults = dict(
        agent_id=uuid4(),
        install_id="install-1",
        agent_version="0.1.0",
        machine_name="ENDPOINT-01",
        operating_system="Windows 11",
        display_name="Lab machine",
        created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
        registered_at=datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
    )
    defaults.update(overrides)
    return Agent(**defaults)  # type: ignore[arg-type]


def test_agent_constructs_with_valid_data() -> None:
    agent = _make_agent()
    assert isinstance(agent.agent_id, UUID)
    assert agent.status is AgentStatus.registered


def test_agent_is_frozen() -> None:
    agent = _make_agent()
    with pytest.raises(Exception):
        agent.install_id = "changed"  # type: ignore[misc]


def test_agent_rejects_non_uuid_id() -> None:
    with pytest.raises(TypeError):
        _make_agent(agent_id="not-a-uuid")


@pytest.mark.parametrize(
    "field",
    ["install_id", "agent_version", "machine_name", "operating_system"],
)
def test_agent_rejects_empty_strings(field: str) -> None:
    with pytest.raises(ValueError):
        _make_agent(**{field: "   "})


def test_agent_rejects_non_datetime_created_at() -> None:
    with pytest.raises(TypeError):
        _make_agent(created_at="2026-08-14")


def test_agent_allows_null_display_name() -> None:
    agent = _make_agent(display_name=None)
    assert agent.display_name is None

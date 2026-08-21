from __future__ import annotations

from datetime import datetime, timedelta, timezone
from uuid import uuid4

import pytest

from app.domain.agent import Agent, AgentStatus
from app.domain.device import DeviceInventory, ListDevicesQuery
from app.domain.heartbeat import (
    Heartbeat,
    HeartbeatRowStatus,
    PresenceStatus,
    RecordHeartbeatCommand,
    UnknownAgentError,
    compute_presence,
)
from app.infrastructure.memory import (
    MemoryAgentRepository,
    MemoryDeviceInventoryRepository,
    MemoryDeviceQueryRepository,
    MemoryHeartbeatRepository,
)
from app.services.device_query_service import DeviceQueryService
from app.services.heartbeat_service import HeartbeatService


NOW = datetime(2026, 8, 14, 12, 0, 0, tzinfo=timezone.utc)


def test_compute_presence_online_stale_offline() -> None:
    assert compute_presence(NOW, now=NOW)[0] is PresenceStatus.online
    assert compute_presence(NOW - timedelta(seconds=60), now=NOW)[0] is PresenceStatus.online
    assert compute_presence(NOW - timedelta(seconds=61), now=NOW)[0] is PresenceStatus.stale
    assert compute_presence(NOW - timedelta(seconds=300), now=NOW)[0] is PresenceStatus.stale
    assert compute_presence(NOW - timedelta(seconds=301), now=NOW)[0] is PresenceStatus.offline
    status, age = compute_presence(None, now=NOW)
    assert status is PresenceStatus.offline
    assert age is None


def _agent(agent_id=None) -> Agent:
    return Agent(
        agent_id=agent_id or uuid4(),
        install_id="install-1",
        agent_version="0.1.0",
        machine_name="HOST",
        operating_system="Windows 11",
        display_name=None,
        created_at=NOW,
        registered_at=NOW,
        status=AgentStatus.registered,
    )


def _inventory(agent_id) -> DeviceInventory:
    return DeviceInventory(
        agent_id=agent_id,
        install_id="install-1",
        machine_name="DESKTOP",
        hostname="DESKTOP",
        operating_system="Windows 11",
        os_version="10.0",
        windows_build="26200",
        architecture="x86_64",
        cpu_model="CPU",
        cpu_cores=8,
        memory_bytes=16,
        disk_total_bytes=100,
        disk_free_bytes=40,
        manufacturer="MSI",
        model="Model",
        serial_number="",
        boot_time=None,
        current_user="user",
        agent_version="0.1.0",
        collected_at=NOW,
        received_at=NOW,
    )


def test_heartbeat_repository_keeps_history() -> None:
    repo = MemoryHeartbeatRepository()
    agent_id = uuid4()
    first = Heartbeat(agent_id, NOW - timedelta(seconds=10), NOW - timedelta(seconds=10))
    second = Heartbeat(agent_id, NOW, NOW)
    repo.append(first)
    repo.append(second)
    assert len(repo.list_for_agent(agent_id)) == 2
    assert repo.get_latest(agent_id).heartbeat_time == NOW


def test_heartbeat_service_records_and_rejects_unknown() -> None:
    agents = MemoryAgentRepository()
    heartbeats = MemoryHeartbeatRepository()
    service = HeartbeatService(agents, heartbeats, clock=lambda: NOW)
    agent = _agent()
    agents.add(agent)

    stored = service.record(
        agent.agent_id,
        RecordHeartbeatCommand(agent.agent_id, "install-1", NOW - timedelta(seconds=1)),
    )
    assert stored.status is HeartbeatRowStatus.ok
    assert heartbeats.get_latest(agent.agent_id) == stored

    with pytest.raises(UnknownAgentError):
        unknown = uuid4()
        service.record(
            unknown,
            RecordHeartbeatCommand(unknown, "install-1", NOW),
        )


def test_device_query_includes_presence() -> None:
    agents = MemoryAgentRepository()
    inventory = MemoryDeviceInventoryRepository()
    heartbeats = MemoryHeartbeatRepository()
    query = MemoryDeviceQueryRepository(
        inventory, agents, heartbeats, clock=lambda: NOW
    )
    service = DeviceQueryService(query)

    agent = _agent()
    agents.add(agent)
    inventory.upsert(_inventory(agent.agent_id))

    offline = service.get(agent.agent_id)
    assert offline.status == "offline"
    assert offline.last_seen is None

    heartbeats.append(Heartbeat(agent.agent_id, NOW - timedelta(seconds=30), NOW))
    online = service.get(agent.agent_id)
    assert online.status == "online"
    assert online.heartbeat_age_seconds == 30

    page = service.list(ListDevicesQuery())
    assert page.items[0].status == "online"

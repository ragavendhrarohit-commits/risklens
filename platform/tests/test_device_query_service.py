from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

import pytest

from app.domain.agent import Agent, AgentStatus
from app.domain.device import (
    DeviceInventory,
    DeviceNotFoundError,
    ListDevicesQuery,
)
from app.infrastructure.memory import (
    MemoryAgentRepository,
    MemoryDeviceInventoryRepository,
    MemoryDeviceQueryRepository,
    MemoryHeartbeatRepository,
)
from app.services.device_query_service import DeviceQueryService


def _agent(agent_id=None, **overrides) -> Agent:
    base = dict(
        agent_id=agent_id or uuid4(),
        install_id="install-1",
        agent_version="0.1.0",
        machine_name="HOST",
        operating_system="Windows 11",
        display_name=None,
        created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
        registered_at=datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
        status=AgentStatus.registered,
    )
    base.update(overrides)
    return Agent(**base)


def _inventory(agent_id, **overrides) -> DeviceInventory:
    base = dict(
        agent_id=agent_id,
        install_id="install-1",
        machine_name="DESKTOP-01",
        hostname="DESKTOP-01",
        operating_system="Windows 11 Pro",
        os_version="10.0",
        windows_build="26200",
        architecture="x86_64",
        cpu_model="CPU",
        cpu_cores=8,
        memory_bytes=16,
        disk_total_bytes=100,
        disk_free_bytes=40,
        manufacturer="MSI",
        model="Crosshair",
        serial_number="",
        boot_time=None,
        current_user=r"MSI\alice",
        agent_version="0.1.0",
        collected_at=datetime(2026, 8, 14, 12, tzinfo=timezone.utc),
        received_at=datetime(2026, 8, 14, 13, tzinfo=timezone.utc),
    )
    base.update(overrides)
    return DeviceInventory(**base)


def _wired() -> tuple[
    DeviceQueryService,
    MemoryAgentRepository,
    MemoryDeviceInventoryRepository,
    MemoryDeviceQueryRepository,
]:
    agents = MemoryAgentRepository()
    inventory = MemoryDeviceInventoryRepository()
    heartbeats = MemoryHeartbeatRepository()
    query_repo = MemoryDeviceQueryRepository(inventory, agents, heartbeats)
    return DeviceQueryService(query_repo), agents, inventory, query_repo


def test_repository_list_and_get() -> None:
    _, agents, inventory, query_repo = _wired()
    agent = _agent()
    agents.add(agent)
    inventory.upsert(_inventory(agent.agent_id, hostname="ALPHA"))

    page = query_repo.list(ListDevicesQuery())
    assert page.total == 1
    assert page.items[0].hostname == "ALPHA"
    assert page.items[0].registration_status == "registered"

    got = query_repo.get(agent.agent_id)
    assert got is not None
    assert got.agent_id == agent.agent_id
    assert query_repo.get(uuid4()) is None


def test_repository_filtering() -> None:
    _, agents, inventory, query_repo = _wired()
    a1 = _agent()
    a2 = _agent()
    agents.add(a1)
    agents.add(a2)
    inventory.upsert(
        _inventory(
            a1.agent_id,
            hostname="LAPTOP-A",
            operating_system="Windows 11 Pro",
            manufacturer="Dell",
            current_user=r"CORP\bob",
            received_at=datetime(2026, 8, 14, 10, tzinfo=timezone.utc),
        )
    )
    inventory.upsert(
        _inventory(
            a2.agent_id,
            hostname="DESKTOP-B",
            operating_system="Windows 10 Home",
            manufacturer="MSI",
            current_user=r"MSI\alice",
            received_at=datetime(2026, 8, 14, 11, tzinfo=timezone.utc),
        )
    )

    by_host = query_repo.list(ListDevicesQuery(hostname="laptop"))
    assert by_host.total == 1
    assert by_host.items[0].hostname == "LAPTOP-A"

    by_os = query_repo.list(ListDevicesQuery(operating_system="Windows 10"))
    assert by_os.total == 1
    assert by_os.items[0].hostname == "DESKTOP-B"

    by_mfr = query_repo.list(ListDevicesQuery(manufacturer="msi"))
    assert by_mfr.total == 1

    by_user = query_repo.list(ListDevicesQuery(current_user=r"CORP\bob"))
    assert by_user.total == 1


def test_repository_sorting_and_pagination() -> None:
    _, agents, inventory, query_repo = _wired()
    for index, name in enumerate(("C", "A", "B")):
        agent = _agent()
        agents.add(agent)
        inventory.upsert(
            _inventory(
                agent.agent_id,
                hostname=name,
                received_at=datetime(2026, 8, 14, index, tzinfo=timezone.utc),
            )
        )

    asc = query_repo.list(ListDevicesQuery(sort="hostname", limit=2, offset=0))
    assert [item.hostname for item in asc.items] == ["A", "B"]
    assert asc.total == 3

    page2 = query_repo.list(ListDevicesQuery(sort="hostname", limit=2, offset=2))
    assert [item.hostname for item in page2.items] == ["C"]

    desc = query_repo.list(ListDevicesQuery(sort="-hostname"))
    assert [item.hostname for item in desc.items] == ["C", "B", "A"]


def test_service_get_and_list() -> None:
    service, agents, inventory, _ = _wired()
    agent = _agent()
    agents.add(agent)
    inventory.upsert(_inventory(agent.agent_id, machine_name="BOX-1"))

    record = service.get(agent.agent_id)
    assert record.machine_name == "BOX-1"

    page = service.list(ListDevicesQuery())
    assert page.total == 1
    assert page.items[0].agent_id == agent.agent_id


def test_service_unknown_agent_raises() -> None:
    service, _, _, _ = _wired()
    unknown = uuid4()
    with pytest.raises(DeviceNotFoundError) as excinfo:
        service.get(unknown)
    assert excinfo.value.agent_id == unknown
    assert excinfo.value.code == "device_not_found"
    assert excinfo.value.status_code == 404


def test_invalid_sort_rejected() -> None:
    with pytest.raises(ValueError):
        ListDevicesQuery(sort="not_a_field")

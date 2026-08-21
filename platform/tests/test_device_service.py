from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

import pytest

from app.domain.agent import Agent, AgentStatus
from app.domain.device import DeviceInventory, UnknownAgentError, UpsertDeviceInventoryCommand
from app.infrastructure.memory import MemoryAgentRepository, MemoryDeviceInventoryRepository
from app.services.device_service import DeviceInventoryService


def _agent(agent_id=None) -> Agent:
    return Agent(
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


def _command(agent_id, **overrides) -> UpsertDeviceInventoryCommand:
    base = dict(
        agent_id=agent_id,
        install_id="install-1",
        machine_name="DESKTOP-01",
        hostname="DESKTOP-01",
        operating_system="Windows 11 Pro",
        os_version="10.0 (25H2)",
        windows_build="26200",
        architecture="x86_64",
        cpu_model="Intel Core Ultra 9",
        cpu_cores=24,
        memory_bytes=32 * 1024**3,
        disk_total_bytes=1024**4,
        disk_free_bytes=512 * 1024**3,
        manufacturer="ExampleCorp",
        model="ExampleBook",
        serial_number="SN-1",
        boot_time=datetime(2026, 8, 11, tzinfo=timezone.utc),
        current_user=r"EXAMPLE\alice",
        agent_version="0.1.0",
        collected_at=datetime(2026, 8, 14, 12, tzinfo=timezone.utc),
    )
    base.update(overrides)
    return UpsertDeviceInventoryCommand(**base)


def _service() -> tuple[DeviceInventoryService, MemoryAgentRepository, MemoryDeviceInventoryRepository]:
    agents = MemoryAgentRepository()
    inventory = MemoryDeviceInventoryRepository()
    frozen = datetime(2026, 8, 14, 15, tzinfo=timezone.utc)
    service = DeviceInventoryService(agents, inventory, clock=lambda: frozen)
    return service, agents, inventory


def test_upsert_stores_inventory_for_registered_agent() -> None:
    service, agents, inventory = _service()
    agent = _agent()
    agents.add(agent)

    stored, created = service.upsert(_command(agent.agent_id))
    assert created is True
    assert stored.agent_id == agent.agent_id
    assert stored.machine_name == "DESKTOP-01"
    assert stored.received_at == datetime(2026, 8, 14, 15, tzinfo=timezone.utc)
    assert inventory.get(agent.agent_id) == stored


def test_upsert_rejects_unknown_agent() -> None:
    service, _, inventory = _service()
    unknown = uuid4()
    with pytest.raises(UnknownAgentError) as excinfo:
        service.upsert(_command(unknown))
    assert excinfo.value.agent_id == unknown
    assert excinfo.value.status_code == 404
    assert inventory.list() == []


def test_duplicate_upload_overwrites_latest() -> None:
    service, agents, inventory = _service()
    agent = _agent()
    agents.add(agent)

    first, created1 = service.upsert(_command(agent.agent_id, cpu_cores=8))
    assert created1 is True
    second, created2 = service.upsert(_command(agent.agent_id, cpu_cores=16, machine_name="RENAMED"))
    assert created2 is False
    assert second.cpu_cores == 16
    assert second.machine_name == "RENAMED"
    assert len(inventory.list()) == 1
    assert inventory.get(agent.agent_id).cpu_cores == 16


def test_entity_rejects_blank_required_fields() -> None:
    with pytest.raises(ValueError):
        DeviceInventory(
            agent_id=uuid4(),
            install_id=" ",
            machine_name="M",
            hostname="H",
            operating_system="OS",
            os_version="",
            windows_build="",
            architecture="",
            cpu_model="",
            cpu_cores=0,
            memory_bytes=0,
            disk_total_bytes=0,
            disk_free_bytes=0,
            manufacturer="",
            model="",
            serial_number="",
            boot_time=None,
            current_user="",
            agent_version="0.1.0",
            collected_at=datetime.now(timezone.utc),
            received_at=datetime.now(timezone.utc),
        )

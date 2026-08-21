from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID, uuid4

from fastapi.testclient import TestClient
from sqlalchemy import text

from app.domain.agent import Agent, AgentStatus
from app.domain.device import DeviceInventory
from app.infrastructure.postgres import (
    Database,
    PostgresAgentRepository,
    PostgresDeviceInventoryRepository,
)

REGISTER_URL = "/api/v1/agents/register"
DEVICES_URL = "/api/v1/devices"


def _register_payload(agent_id: str | None = None) -> dict:
    return {
        "agent_id": agent_id or str(uuid4()),
        "install_id": "install-1",
        "agent_version": "0.1.0",
        "machine_name": "ENDPOINT-01",
        "operating_system": "Windows 11",
        "created_at": datetime(2026, 8, 14, tzinfo=timezone.utc).isoformat(),
    }


def _inventory_payload(agent_id: str, **overrides) -> dict:
    body = {
        "agent_id": agent_id,
        "install_id": "install-1",
        "machine_name": "DESKTOP-01",
        "hostname": "DESKTOP-01",
        "operating_system": "Windows 11 Pro",
        "os_version": "10.0 (25H2)",
        "windows_build": "26200",
        "architecture": "x86_64",
        "cpu_model": "Intel Core Ultra 9",
        "cpu_cores": 24,
        "memory_bytes": 33736060928,
        "disk_total_bytes": 2015337902080,
        "disk_free_bytes": 1367635787776,
        "manufacturer": "MSI",
        "model": "Crosshair",
        "serial_number": "",
        "boot_time": datetime(2026, 8, 11, tzinfo=timezone.utc).isoformat(),
        "current_user": r"MSI\rohit",
        "agent_version": "0.1.0",
        "collected_at": datetime(2026, 8, 14, 12, tzinfo=timezone.utc).isoformat(),
    }
    body.update(overrides)
    return body


def test_postgres_repository_upsert_and_get(pg_database: Database) -> None:
    agents = PostgresAgentRepository(pg_database.session_factory)
    inventory = PostgresDeviceInventoryRepository(pg_database.session_factory)
    agent_id = uuid4()
    agents.add(
        Agent(
            agent_id=agent_id,
            install_id="i",
            agent_version="0.1.0",
            machine_name="M",
            operating_system="OS",
            display_name=None,
            created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
            registered_at=datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
            status=AgentStatus.registered,
        )
    )

    row = DeviceInventory(
        agent_id=agent_id,
        install_id="i",
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
        manufacturer="OEM",
        model="Model",
        serial_number="",
        boot_time=datetime(2026, 8, 11, tzinfo=timezone.utc),
        current_user="user",
        agent_version="0.1.0",
        collected_at=datetime(2026, 8, 14, 12, tzinfo=timezone.utc),
        received_at=datetime(2026, 8, 14, 13, tzinfo=timezone.utc),
    )
    stored, created = inventory.upsert(row)
    assert created is True
    assert inventory.get(agent_id) == stored

    updated = DeviceInventory(
        agent_id=agent_id,
        install_id="i",
        machine_name="DESKTOP",
        hostname="DESKTOP",
        operating_system="Windows 11",
        os_version="10.0",
        windows_build="26200",
        architecture="x86_64",
        cpu_model="CPU",
        cpu_cores=16,
        memory_bytes=16,
        disk_total_bytes=100,
        disk_free_bytes=40,
        manufacturer="OEM",
        model="Model",
        serial_number="",
        boot_time=datetime(2026, 8, 11, tzinfo=timezone.utc),
        current_user="user",
        agent_version="0.1.0",
        collected_at=datetime(2026, 8, 14, 12, tzinfo=timezone.utc),
        received_at=datetime(2026, 8, 14, 14, tzinfo=timezone.utc),
    )
    stored2, created2 = inventory.upsert(updated)
    assert created2 is False
    assert inventory.get(agent_id).cpu_cores == 16
    assert len(inventory.list()) == 1


def test_endpoint_persists_row_in_postgres(
    pg_client: TestClient, pg_database: Database
) -> None:
    agent_id = str(uuid4())
    assert pg_client.post(REGISTER_URL, json=_register_payload(agent_id)).status_code == 201
    response = pg_client.post(DEVICES_URL, json=_inventory_payload(agent_id))
    assert response.status_code == 201, response.text

    with pg_database.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT machine_name, hostname, cpu_cores, memory_bytes, windows_build "
                "FROM device_inventory WHERE agent_id = :id"
            ),
            {"id": UUID(agent_id)},
        ).one()
    assert row.machine_name == "DESKTOP-01"
    assert row.hostname == "DESKTOP-01"
    assert row.cpu_cores == 24
    assert row.memory_bytes == 33736060928
    assert row.windows_build == "26200"


def test_duplicate_upload_keeps_single_row(
    pg_client: TestClient, pg_database: Database
) -> None:
    agent_id = str(uuid4())
    assert pg_client.post(REGISTER_URL, json=_register_payload(agent_id)).status_code == 201
    assert pg_client.post(DEVICES_URL, json=_inventory_payload(agent_id, cpu_cores=4)).status_code == 201
    assert pg_client.post(DEVICES_URL, json=_inventory_payload(agent_id, cpu_cores=12)).status_code == 200

    with pg_database.engine.connect() as connection:
        count = connection.execute(text("SELECT count(*) FROM device_inventory")).scalar_one()
        cores = connection.execute(
            text("SELECT cpu_cores FROM device_inventory WHERE agent_id = :id"),
            {"id": UUID(agent_id)},
        ).scalar_one()
    assert count == 1
    assert cores == 12


def test_unknown_agent_writes_nothing(pg_client: TestClient, pg_database: Database) -> None:
    response = pg_client.post(DEVICES_URL, json=_inventory_payload(str(uuid4())))
    assert response.status_code == 404
    with pg_database.engine.connect() as connection:
        count = connection.execute(text("SELECT count(*) FROM device_inventory")).scalar_one()
    assert count == 0

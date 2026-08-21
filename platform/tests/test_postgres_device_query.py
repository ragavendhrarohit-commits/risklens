from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.agent import Agent, AgentStatus
from app.domain.device import DeviceInventory, ListDevicesQuery
from app.infrastructure.postgres import (
    Database,
    PostgresAgentRepository,
    PostgresDeviceInventoryRepository,
    PostgresDeviceQueryRepository,
)

REGISTER_URL = "/api/v1/agents/register"
DEVICES_URL = "/api/v1/devices"


def _seed(
    agents: PostgresAgentRepository,
    inventory: PostgresDeviceInventoryRepository,
    *,
    hostname: str,
    manufacturer: str = "MSI",
    operating_system: str = "Windows 11 Pro",
    current_user: str = r"MSI\alice",
    received_hour: int = 12,
):
    agent_id = uuid4()
    agents.add(
        Agent(
            agent_id=agent_id,
            install_id="i",
            agent_version="0.1.0",
            machine_name=hostname,
            operating_system=operating_system,
            display_name=None,
            created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
            registered_at=datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
            status=AgentStatus.registered,
        )
    )
    inventory.upsert(
        DeviceInventory(
            agent_id=agent_id,
            install_id="i",
            machine_name=hostname,
            hostname=hostname,
            operating_system=operating_system,
            os_version="10.0",
            windows_build="26200",
            architecture="x86_64",
            cpu_model="CPU",
            cpu_cores=8,
            memory_bytes=16,
            disk_total_bytes=100,
            disk_free_bytes=40,
            manufacturer=manufacturer,
            model="Model",
            serial_number="",
            boot_time=None,
            current_user=current_user,
            agent_version="0.1.0",
            collected_at=datetime(2026, 8, 14, 12, tzinfo=timezone.utc),
            received_at=datetime(2026, 8, 14, received_hour, tzinfo=timezone.utc),
        )
    )
    return agent_id


def test_postgres_query_repository_list_filter_sort_page(pg_database: Database) -> None:
    agents = PostgresAgentRepository(pg_database.session_factory)
    inventory = PostgresDeviceInventoryRepository(pg_database.session_factory)
    query = PostgresDeviceQueryRepository(pg_database.session_factory)

    _seed(agents, inventory, hostname="ALPHA", manufacturer="Dell", received_hour=10)
    b = _seed(agents, inventory, hostname="BRAVO", manufacturer="MSI", received_hour=11)
    _seed(
        agents,
        inventory,
        hostname="CHARLIE",
        manufacturer="MSI",
        operating_system="Windows 10 Home",
        current_user=r"CORP\bob",
        received_hour=12,
    )

    page = query.list(ListDevicesQuery(manufacturer="msi", sort="hostname"))
    assert page.total == 2
    assert [item.hostname for item in page.items] == ["BRAVO", "CHARLIE"]
    assert all(item.registration_status == "registered" for item in page.items)

    filtered = query.list(ListDevicesQuery(hostname="alpha"))
    assert filtered.total == 1
    assert filtered.items[0].manufacturer == "Dell"

    paged = query.list(ListDevicesQuery(sort="hostname", limit=1, offset=1))
    assert paged.total == 3
    assert len(paged.items) == 1
    assert paged.items[0].hostname == "BRAVO"

    got = query.get(b)
    assert got is not None
    assert got.hostname == "BRAVO"
    assert query.get(uuid4()) is None


def test_postgres_query_endpoints(pg_client: TestClient) -> None:
    agent_id = str(uuid4())
    assert (
        pg_client.post(
            REGISTER_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "agent_version": "0.1.0",
                "machine_name": "ENDPOINT-01",
                "operating_system": "Windows 11",
                "created_at": datetime(2026, 8, 14, tzinfo=timezone.utc).isoformat(),
            },
        ).status_code
        == 201
    )
    assert (
        pg_client.post(
            DEVICES_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "machine_name": "PG-HOST",
                "hostname": "PG-HOST",
                "operating_system": "Windows 11 Pro",
                "os_version": "10.0",
                "windows_build": "26200",
                "architecture": "x86_64",
                "cpu_model": "CPU",
                "cpu_cores": 8,
                "memory_bytes": 16,
                "disk_total_bytes": 100,
                "disk_free_bytes": 40,
                "manufacturer": "MSI",
                "model": "Model",
                "serial_number": "",
                "current_user": r"MSI\rohit",
                "agent_version": "0.1.0",
                "collected_at": datetime(2026, 8, 14, 12, tzinfo=timezone.utc).isoformat(),
            },
        ).status_code
        == 201
    )

    listed = pg_client.get(DEVICES_URL, params={"hostname": "PG-HOST"})
    assert listed.status_code == 200, listed.text
    body = listed.json()
    assert body["total"] == 1
    assert body["items"][0]["agent_id"] == agent_id
    assert body["items"][0]["registration_status"] == "registered"

    single = pg_client.get(f"{DEVICES_URL}/{agent_id}")
    assert single.status_code == 200
    assert single.json()["hostname"] == "PG-HOST"

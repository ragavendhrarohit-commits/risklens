from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from app.domain.agent import Agent, AgentStatus
from app.domain.software import (
    InstalledApplication,
    ListSoftwareQuery,
    SoftwareApplicationInput,
    UnknownAgentError,
    UpsertSoftwareCommand,
    software_id,
)
from app.infrastructure.memory import MemoryAgentRepository, MemorySoftwareRepository
from app.services.software_service import SoftwareInventoryService

NOW = datetime(2026, 8, 14, 12, tzinfo=timezone.utc)
REGISTER_URL = "/api/v1/agents/register"
SOFTWARE_URL = "/api/v1/software"


def _agent(agent_id=None) -> Agent:
    return Agent(
        agent_id=agent_id or uuid4(),
        install_id="install-1",
        agent_version="0.1.0",
        machine_name="HOST-01",
        operating_system="Windows 11",
        display_name=None,
        created_at=NOW,
        registered_at=NOW,
        status=AgentStatus.registered,
    )


def _sample_app(agent_id, name="Chrome", version="120.0"):
    return InstalledApplication(
        id=software_id(agent_id, name, version),
        agent_id=agent_id,
        application_name=name,
        version=version,
        publisher="Google LLC",
        install_date="2026-01-15",
        install_location=r"C:\Program Files\Chrome",
        estimated_size=524288000,
        architecture="x64",
        system_component=False,
        windows_component=False,
        registry_path=r"HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\Chrome",
        collected_at=NOW,
        received_at=NOW,
    )


def test_repository_upsert_get_and_list() -> None:
    agents = MemoryAgentRepository()
    repo = MemorySoftwareRepository(agents)
    agent = _agent()
    agents.add(agent)

    assert repo.upsert_many([_sample_app(agent.agent_id)]) == 1
    page = repo.list(ListSoftwareQuery(application_name="chr"))
    assert page.total == 1
    assert page.items[0].machine_name == "HOST-01"
    assert repo.get(page.items[0].id).application_name == "Chrome"


def test_service_rejects_unknown_agent() -> None:
    agents = MemoryAgentRepository()
    repo = MemorySoftwareRepository(agents)
    service = SoftwareInventoryService(agents, repo, clock=lambda: NOW)
    with pytest.raises(UnknownAgentError):
        service.upsert(
            UpsertSoftwareCommand(
                agent_id=uuid4(),
                install_id="i",
                collected_at=NOW,
                applications=[SoftwareApplicationInput(application_name="X")],
            )
        )


def test_service_dedupes_duplicates_in_payload() -> None:
    agents = MemoryAgentRepository()
    repo = MemorySoftwareRepository(agents)
    service = SoftwareInventoryService(agents, repo, clock=lambda: NOW)
    agent = _agent()
    agents.add(agent)
    stored, _ = service.upsert(
        UpsertSoftwareCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            applications=[
                SoftwareApplicationInput(application_name="Chrome", version="120.0"),
                SoftwareApplicationInput(application_name="Chrome", version="120.0"),
                SoftwareApplicationInput(application_name="7-Zip", version="23.01"),
                SoftwareApplicationInput(application_name="  ", version=""),  # dropped
            ],
        )
    )
    assert stored == 2
    listing = service.list(ListSoftwareQuery())
    assert listing.total == 2


def test_service_upsert_replaces_previous_snapshot() -> None:
    agents = MemoryAgentRepository()
    repo = MemorySoftwareRepository(agents)
    service = SoftwareInventoryService(agents, repo, clock=lambda: NOW)
    agent = _agent()
    agents.add(agent)
    service.upsert(
        UpsertSoftwareCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            applications=[
                SoftwareApplicationInput(application_name="Chrome", version="120.0", publisher="Old"),
            ],
        )
    )
    stored, _ = service.upsert(
        UpsertSoftwareCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            applications=[
                SoftwareApplicationInput(application_name="Chrome", version="120.0", publisher="Google"),
            ],
        )
    )
    assert stored == 1
    page = service.list(ListSoftwareQuery())
    assert page.total == 1
    assert page.items[0].publisher == "Google"


def test_filter_by_architecture_and_publisher() -> None:
    agents = MemoryAgentRepository()
    repo = MemorySoftwareRepository(agents)
    service = SoftwareInventoryService(agents, repo, clock=lambda: NOW)
    agent = _agent()
    agents.add(agent)
    service.upsert(
        UpsertSoftwareCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            applications=[
                SoftwareApplicationInput(
                    application_name="Chrome",
                    version="120",
                    publisher="Google",
                    architecture="x64",
                ),
                SoftwareApplicationInput(
                    application_name="OldTool",
                    version="1",
                    publisher="Legacy Inc",
                    architecture="x86",
                ),
            ],
        )
    )
    assert service.list(ListSoftwareQuery(architecture="x86")).total == 1
    assert service.list(ListSoftwareQuery(publisher="google")).total == 1
    assert service.list(ListSoftwareQuery(version="120")).total == 1


def test_software_endpoint_list_get_filter(client: TestClient) -> None:
    agent_id = str(uuid4())
    assert (
        client.post(
            REGISTER_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "agent_version": "0.1.0",
                "machine_name": "LAB-PC",
                "operating_system": "Windows 11",
                "created_at": NOW.isoformat(),
            },
        ).status_code
        == 201
    )
    assert (
        client.post(
            SOFTWARE_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "applications": [
                    {
                        "application_name": "Chrome",
                        "version": "120.0",
                        "publisher": "Google LLC",
                        "architecture": "x64",
                    },
                    {
                        "application_name": "7-Zip",
                        "version": "23.01",
                        "publisher": "Igor Pavlov",
                        "architecture": "x64",
                    },
                ],
            },
        ).status_code
        == 200
    )

    listed = client.get(SOFTWARE_URL, params={"application_name": "chr", "sort": "application_name"})
    assert listed.status_code == 200
    body = listed.json()
    assert body["total"] == 1
    assert body["items"][0]["machine_name"] == "LAB-PC"
    item_id = body["items"][0]["id"]

    detail = client.get(f"{SOFTWARE_URL}/{item_id}")
    assert detail.status_code == 200
    assert detail.json()["application_name"] == "Chrome"

    missing = client.get(f"{SOFTWARE_URL}/does-not-exist")
    assert missing.status_code == 404


def test_upload_unknown_agent_returns_404(client: TestClient) -> None:
    resp = client.post(
        SOFTWARE_URL,
        json={
            "agent_id": str(uuid4()),
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "applications": [{"application_name": "Anything", "version": "1"}],
        },
    )
    assert resp.status_code == 404


def test_concurrent_upload(client: TestClient) -> None:
    agent_id = str(uuid4())
    assert (
        client.post(
            REGISTER_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "agent_version": "0.1.0",
                "machine_name": "HOST-01",
                "operating_system": "Windows 11",
                "created_at": NOW.isoformat(),
            },
        ).status_code
        == 201
    )

    def upload(index: int) -> int:
        return client.post(
            SOFTWARE_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "applications": [
                    {
                        "application_name": f"App-{index}",
                        "version": "1.0",
                        "publisher": "Concurrent",
                        "architecture": "x64",
                    }
                ],
            },
        ).status_code

    with ThreadPoolExecutor(max_workers=8) as pool:
        codes = list(pool.map(upload, range(20)))
    assert all(code == 200 for code in codes)
    listed = client.get(SOFTWARE_URL, params={"limit": 100}).json()
    assert listed["total"] >= 20


def test_invalid_sort_returns_422(client: TestClient) -> None:
    resp = client.get(SOFTWARE_URL, params={"sort": "not_a_field"})
    assert resp.status_code == 422

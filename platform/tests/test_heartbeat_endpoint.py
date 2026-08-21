from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

REGISTER_URL = "/api/v1/agents/register"
DEVICES_URL = "/api/v1/devices"
NOW = datetime(2026, 8, 14, 12, 0, 0, tzinfo=timezone.utc)


def _register(client: TestClient, agent_id: str | None = None) -> str:
    agent_id = agent_id or str(uuid4())
    response = client.post(
        REGISTER_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "agent_version": "0.1.0",
            "machine_name": "ENDPOINT-01",
            "operating_system": "Windows 11",
            "created_at": NOW.isoformat(),
        },
    )
    assert response.status_code == 201, response.text
    return agent_id


def _upload(client: TestClient, agent_id: str) -> None:
    response = client.post(
        DEVICES_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "machine_name": "DESKTOP-01",
            "hostname": "DESKTOP-01",
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
            "collected_at": NOW.isoformat(),
        },
    )
    assert response.status_code in (200, 201), response.text


def test_heartbeat_endpoint_success(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
        },
    )
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["accepted"] is True
    assert body["heartbeat_status"] == "ok"
    assert body["agent_id"] == agent_id


def test_heartbeat_unknown_agent(client: TestClient) -> None:
    agent_id = str(uuid4())
    response = client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
        },
    )
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "unknown_agent"


def test_heartbeat_agent_id_mismatch(client: TestClient) -> None:
    agent_id = _register(client)
    other = str(uuid4())
    response = client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": other,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
        },
    )
    assert response.status_code == 422
    assert response.json()["error"]["code"] == "agent_id_mismatch"


def test_devices_list_includes_presence_after_heartbeat(client: TestClient) -> None:
    agent_id = _register(client)
    _upload(client, agent_id)

    before = client.get(DEVICES_URL).json()["items"][0]
    assert before["status"] == "offline"
    assert before["last_seen"] is None

    assert (
        client.post(
            f"/api/v1/agents/{agent_id}/heartbeat",
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "timestamp": datetime.now(timezone.utc).isoformat(),
            },
        ).status_code
        == 200
    )

    after = client.get(DEVICES_URL).json()["items"][0]
    assert after["status"] == "online"
    assert after["last_seen"] is not None
    assert after["heartbeat_age_seconds"] is not None
    assert after["heartbeat_age_seconds"] <= 60


def test_openapi_documents_heartbeat(client: TestClient) -> None:
    schema = client.get("/openapi.json").json()
    assert "/api/v1/agents/{agent_id}/heartbeat" in schema["paths"]

from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

REGISTER_URL = "/api/v1/agents/register"
DEVICES_URL = "/api/v1/devices"


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
            "display_name": "Lab",
            "created_at": datetime(2026, 8, 14, tzinfo=timezone.utc).isoformat(),
        },
    )
    assert response.status_code == 201, response.text
    return agent_id


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
        "manufacturer": "Micro-Star International Co., Ltd.",
        "model": "Crosshair 16 HX AI D2XWGKG",
        "serial_number": "",
        "boot_time": datetime(2026, 8, 11, 6, 9, 58, tzinfo=timezone.utc).isoformat(),
        "current_user": r"MSI\rohit",
        "agent_version": "0.1.0",
        "collected_at": datetime(2026, 8, 14, 12, 0, 0, tzinfo=timezone.utc).isoformat(),
    }
    body.update(overrides)
    return body


def test_upload_returns_201_for_first_inventory(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.post(DEVICES_URL, json=_inventory_payload(agent_id))
    assert response.status_code == 201, response.text
    body = response.json()
    assert body["accepted"] is True
    assert body["created"] is True
    assert body["agent_id"] == agent_id
    assert body["inventory_status"] == "stored"
    assert body["server_version"] == "0.1.0"


def test_duplicate_upload_returns_200_and_overwrites(client: TestClient) -> None:
    agent_id = _register(client)
    first = client.post(DEVICES_URL, json=_inventory_payload(agent_id, cpu_cores=8))
    assert first.status_code == 201
    second = client.post(DEVICES_URL, json=_inventory_payload(agent_id, cpu_cores=16))
    assert second.status_code == 200
    assert second.json()["created"] is False


def test_unknown_agent_returns_404(client: TestClient) -> None:
    response = client.post(DEVICES_URL, json=_inventory_payload(str(uuid4())))
    assert response.status_code == 404
    body = response.json()
    assert body["error"]["code"] == "unknown_agent"


def test_missing_agent_id_returns_422(client: TestClient) -> None:
    payload = _inventory_payload(str(uuid4()))
    payload.pop("agent_id")
    response = client.post(DEVICES_URL, json=payload)
    assert response.status_code == 422
    assert response.json()["error"]["code"] == "validation_error"


def test_missing_machine_name_returns_422(client: TestClient) -> None:
    agent_id = _register(client)
    payload = _inventory_payload(agent_id)
    payload.pop("machine_name")
    response = client.post(DEVICES_URL, json=payload)
    assert response.status_code == 422


def test_blank_hostname_returns_422(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.post(DEVICES_URL, json=_inventory_payload(agent_id, hostname="   "))
    assert response.status_code == 422


def test_negative_memory_returns_422(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.post(DEVICES_URL, json=_inventory_payload(agent_id, memory_bytes=-1))
    assert response.status_code == 422


def test_extra_fields_rejected(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.post(
        DEVICES_URL,
        json=_inventory_payload(agent_id, rogue="nope"),
    )
    assert response.status_code == 422


def test_openapi_documents_devices_endpoint(client: TestClient) -> None:
    schema = client.get("/openapi.json").json()
    assert "/api/v1/devices" in schema["paths"]
    endpoint = schema["paths"]["/api/v1/devices"]["post"]
    assert "201" in endpoint["responses"] or "200" in endpoint["responses"]
    assert "404" in endpoint["responses"]
    assert "422" in endpoint["responses"]
    assert "get" in schema["paths"]["/api/v1/devices"]

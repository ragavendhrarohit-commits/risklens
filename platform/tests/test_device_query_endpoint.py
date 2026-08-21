from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

REGISTER_URL = "/api/v1/agents/register"
DEVICES_URL = "/api/v1/devices"


def _register(client: TestClient, agent_id: str | None = None, **overrides) -> str:
    agent_id = agent_id or str(uuid4())
    body = {
        "agent_id": agent_id,
        "install_id": "install-1",
        "agent_version": "0.1.0",
        "machine_name": "ENDPOINT-01",
        "operating_system": "Windows 11",
        "display_name": "Lab",
        "created_at": datetime(2026, 8, 14, tzinfo=timezone.utc).isoformat(),
    }
    body.update(overrides)
    response = client.post(REGISTER_URL, json=body)
    assert response.status_code == 201, response.text
    return agent_id


def _upload(client: TestClient, agent_id: str, **overrides) -> None:
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
    response = client.post(DEVICES_URL, json=body)
    assert response.status_code in (200, 201), response.text


def test_list_devices_returns_uploaded_inventory(client: TestClient) -> None:
    agent_id = _register(client)
    _upload(client, agent_id, hostname="QUERY-HOST", machine_name="QUERY-HOST")

    response = client.get(DEVICES_URL)
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["total"] == 1
    assert body["limit"] == 50
    assert body["offset"] == 0
    assert len(body["items"]) == 1
    item = body["items"][0]
    assert item["agent_id"] == agent_id
    assert item["hostname"] == "QUERY-HOST"
    assert item["machine_name"] == "QUERY-HOST"
    assert item["operating_system"] == "Windows 11 Pro"
    assert item["registration_status"] == "registered"
    assert item["agent_version"] == "0.1.0"
    assert "collected_at" in item
    assert "received_at" in item


def test_get_single_device(client: TestClient) -> None:
    agent_id = _register(client)
    _upload(client, agent_id, hostname="SINGLE")

    response = client.get(f"{DEVICES_URL}/{agent_id}")
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["agent_id"] == agent_id
    assert body["hostname"] == "SINGLE"
    assert body["registration_status"] == "registered"


def test_get_unknown_agent_returns_404(client: TestClient) -> None:
    response = client.get(f"{DEVICES_URL}/{uuid4()}")
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "device_not_found"


def test_list_filtering(client: TestClient) -> None:
    a1 = _register(client)
    a2 = _register(client)
    _upload(
        client,
        a1,
        hostname="FILTER-A",
        operating_system="Windows 11 Pro",
        manufacturer="Dell Inc.",
        current_user=r"CORP\bob",
    )
    _upload(
        client,
        a2,
        hostname="FILTER-B",
        operating_system="Windows 10 Home",
        manufacturer="MSI",
        current_user=r"MSI\alice",
    )

    by_host = client.get(DEVICES_URL, params={"hostname": "FILTER-A"})
    assert by_host.status_code == 200
    assert by_host.json()["total"] == 1
    assert by_host.json()["items"][0]["hostname"] == "FILTER-A"

    by_os = client.get(DEVICES_URL, params={"operating_system": "Windows 10"})
    assert by_os.json()["total"] == 1

    by_mfr = client.get(DEVICES_URL, params={"manufacturer": "Dell"})
    assert by_mfr.json()["total"] == 1

    by_user = client.get(DEVICES_URL, params={"current_user": r"MSI\alice"})
    assert by_user.json()["total"] == 1


def test_list_pagination(client: TestClient) -> None:
    for index in range(3):
        agent_id = _register(client)
        _upload(client, agent_id, hostname=f"PAGE-{index}")

    first = client.get(DEVICES_URL, params={"limit": 2, "offset": 0, "sort": "hostname"})
    assert first.status_code == 200
    body = first.json()
    assert body["total"] == 3
    assert body["limit"] == 2
    assert body["offset"] == 0
    assert len(body["items"]) == 2

    second = client.get(DEVICES_URL, params={"limit": 2, "offset": 2, "sort": "hostname"})
    assert second.json()["total"] == 3
    assert len(second.json()["items"]) == 1


def test_list_sorting(client: TestClient) -> None:
    for name in ("ZED", "ACE", "MID"):
        agent_id = _register(client)
        _upload(client, agent_id, hostname=name)

    asc = client.get(DEVICES_URL, params={"sort": "hostname"})
    assert [item["hostname"] for item in asc.json()["items"]] == ["ACE", "MID", "ZED"]

    desc = client.get(DEVICES_URL, params={"sort": "-hostname"})
    assert [item["hostname"] for item in desc.json()["items"]] == ["ZED", "MID", "ACE"]


def test_invalid_sort_returns_422(client: TestClient) -> None:
    response = client.get(DEVICES_URL, params={"sort": "not_real"})
    assert response.status_code == 422
    assert response.json()["error"]["code"] == "invalid_query"


def test_openapi_documents_query_endpoints(client: TestClient) -> None:
    schema = client.get("/openapi.json").json()
    paths = schema["paths"]["/api/v1/devices"]
    assert "get" in paths
    assert "post" in paths
    assert "/api/v1/devices/{agent_id}" in schema["paths"]
    get_one = schema["paths"]["/api/v1/devices/{agent_id}"]["get"]
    assert "404" in get_one["responses"]

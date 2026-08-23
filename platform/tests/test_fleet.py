from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

REGISTER_URL = "/api/v1/agents/register"
DEVICES_URL = "/api/v1/devices"
USERS_URL = "/api/v1/users"
SOFTWARE_URL = "/api/v1/software"
FLEET_URL = "/api/v1/fleet"
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


def _upload_device(client: TestClient, agent_id: str) -> None:
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


def _heartbeat(client: TestClient, agent_id: str) -> None:
    response = client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": datetime.now(timezone.utc).isoformat(),
        },
    )
    assert response.status_code == 200, response.text


def test_fleet_lists_registered_agent_without_inventory(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.get(FLEET_URL)
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["summary"]["total_agents"] == 1
    assert body["summary"]["never_seen"] == 1
    assert body["summary"]["online"] == 0
    assert body["total"] == 1
    item = body["items"][0]
    assert item["agent_id"] == agent_id
    assert item["online_status"] == "never_seen"
    assert item["policy_name"] == "Default Windows Policy"
    assert item["tags"] == []
    assert item["ip_address"] is None


def test_fleet_online_and_healthy_after_heartbeat_and_inventory(client: TestClient) -> None:
    agent_id = _register(client)
    _upload_device(client, agent_id)
    _heartbeat(client, agent_id)

    body = client.get(FLEET_URL).json()
    item = body["items"][0]
    assert item["online_status"] == "online"
    assert item["hostname"] == "DESKTOP-01"
    assert item["last_seen"] is not None
    assert item["last_inventory_at"] is not None
    assert body["summary"]["online"] == 1
    assert body["summary"]["healthy"] == 1


def test_fleet_detail_users_software_health(client: TestClient) -> None:
    agent_id = _register(client)
    _upload_device(client, agent_id)
    assert (
        client.post(
            USERS_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "users": [
                    {
                        "username": "alice",
                        "sid": "S-1-5-21-1-2-3-1001",
                        "account_type": "Local",
                        "enabled": True,
                    }
                ],
            },
        ).status_code
        == 200
    )
    assert (
        client.post(
            SOFTWARE_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "applications": [
                    {"application_name": "Chrome", "version": "1.0", "publisher": "Google"}
                ],
            },
        ).status_code
        == 200
    )

    detail = client.get(f"{FLEET_URL}/{agent_id}")
    assert detail.status_code == 200, detail.text
    assert detail.json()["machine_name"] == "DESKTOP-01"
    assert detail.json()["manufacturer"] == "MSI"

    users = client.get(f"{FLEET_URL}/{agent_id}/users")
    assert users.status_code == 200
    assert users.json()["total"] == 1
    assert users.json()["items"][0]["username"] == "alice"

    software = client.get(f"{FLEET_URL}/{agent_id}/software")
    assert software.status_code == 200
    assert software.json()["total"] == 1
    assert software.json()["items"][0]["application_name"] == "Chrome"

    session_id = str(uuid4())
    assert (
        client.post(
            "/api/v1/sessions",
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "sessions": [
                    {
                        "session_id": session_id,
                        "username": "alice",
                        "account_sid": "S-1-5-21-1-2-3-1001",
                        "logon_at": NOW.isoformat(),
                        "status": "active",
                    }
                ],
            },
        ).status_code
        == 200
    )
    assert (
        client.post(
            "/api/v1/applications",
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "activities": [
                    {
                        "session_id": session_id,
                        "application_name": "Cursor",
                        "executable_name": "Cursor.exe",
                        "window_title": "main.go",
                        "started_at": NOW.isoformat(),
                        "duration_seconds": 10,
                    }
                ],
            },
        ).status_code
        == 200
    )
    sessions = client.get(f"{FLEET_URL}/{agent_id}/sessions")
    assert sessions.status_code == 200
    assert sessions.json()["total"] == 1
    assert sessions.json()["items"][0]["username"] == "alice"
    applications = client.get(f"{FLEET_URL}/{agent_id}/applications")
    assert applications.status_code == 200
    assert applications.json()["total"] == 1
    assert applications.json()["items"][0]["application_name"] == "Cursor"
    assert applications.json()["items"][0]["username"] == "alice"

    health = client.get(f"{FLEET_URL}/{agent_id}/health")
    assert health.status_code == 200
    body = health.json()
    assert body["cpu_percent"] is None
    assert body["memory"] is None
    assert body["disks"] == []
    assert body["memory_bytes"] == 16
    assert body["disk_total_bytes"] == 100

    beat = client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
            "health": {
                "cpu_percent": 18.5,
                "cpu_model": "Intel Core Ultra 9 275HX",
                "logical_cpus": 24,
                "physical_cores": 16,
                "memory": {
                    "total": 33722208256,
                    "used": 15200000000,
                    "free": 18522208256,
                    "percent": 45.1,
                },
                "disks": [
                    {
                        "name": "C:",
                        "total": 2000398934016,
                        "used": 332800000000,
                        "free": 1667598934016,
                        "percent": 16.6,
                    },
                    {
                        "name": "D:",
                        "total": 500000000000,
                        "used": 100000000000,
                        "free": 400000000000,
                        "percent": 20.0,
                    },
                ],
            },
        },
    )
    assert beat.status_code == 200, beat.text
    live = client.get(f"{FLEET_URL}/{agent_id}/health")
    assert live.status_code == 200
    live_body = live.json()
    assert live_body["cpu_percent"] == 18.5
    assert live_body["cpu_model"] == "Intel Core Ultra 9 275HX"
    assert live_body["logical_cpus"] == 24
    assert live_body["physical_cores"] == 16
    assert live_body["memory"]["percent"] == 45.1
    assert live_body["disks"][0]["name"] == "C:"
    assert live_body["disks"][1]["name"] == "D:"
    assert "Not implemented yet" not in live.text


def test_fleet_unknown_agent_is_404(client: TestClient) -> None:
    missing = str(uuid4())
    assert client.get(f"{FLEET_URL}/{missing}").status_code == 404
    assert client.get(f"{FLEET_URL}/{missing}/users").status_code == 404
    assert client.get(f"{FLEET_URL}/{missing}/software").status_code == 404
    assert client.get(f"{FLEET_URL}/{missing}/sessions").status_code == 404
    assert client.get(f"{FLEET_URL}/{missing}/applications").status_code == 404
    assert client.get(f"{FLEET_URL}/{missing}/website-usage").status_code == 404
    assert client.get(f"{FLEET_URL}/{missing}/browser-activity").status_code == 404
    assert client.get(f"{FLEET_URL}/{missing}/health").status_code == 404


def test_sprint12_devices_users_software_still_work(client: TestClient) -> None:
    agent_id = _register(client)
    _upload_device(client, agent_id)
    devices = client.get(DEVICES_URL)
    assert devices.status_code == 200
    assert devices.json()["total"] == 1
    assert devices.json()["items"][0]["status"] in {"online", "stale", "offline"}

    users = client.get(USERS_URL)
    assert users.status_code == 200
    software = client.get(SOFTWARE_URL)
    assert software.status_code == 200


def test_openapi_documents_fleet(client: TestClient) -> None:
    schema = client.get("/openapi.json").json()
    paths = schema["paths"]
    assert "/api/v1/fleet" in paths
    assert "/api/v1/fleet/{agent_id}" in paths
    assert "/api/v1/fleet/{agent_id}/users" in paths
    assert "/api/v1/fleet/{agent_id}/software" in paths
    assert "/api/v1/fleet/{agent_id}/sessions" in paths
    assert "/api/v1/fleet/{agent_id}/applications" in paths
    assert "/api/v1/fleet/{agent_id}/website-usage" in paths
    assert "/api/v1/fleet/{agent_id}/browser-activity" in paths
    assert "/api/v1/fleet/{agent_id}/health" in paths
    assert "/api/v1/fleet/{agent_id}/policy" not in paths
    assert "/api/v1/devices" in paths


def test_postgres_fleet_lists_registered_agent(pg_client: TestClient) -> None:
    agent_id = _register(pg_client)
    response = pg_client.get(FLEET_URL)
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["summary"]["total_agents"] == 1
    assert body["items"][0]["agent_id"] == agent_id
    assert body["items"][0]["online_status"] == "never_seen"
    assert body["items"][0]["policy_name"] == "Default Windows Policy"

    _upload_device(pg_client, agent_id)
    _heartbeat(pg_client, agent_id)
    online = pg_client.get(FLEET_URL).json()["items"][0]
    assert online["online_status"] == "online"
    assert pg_client.get(DEVICES_URL).status_code == 200
    assert pg_client.get(USERS_URL).status_code == 200
    assert pg_client.get(SOFTWARE_URL).status_code == 200

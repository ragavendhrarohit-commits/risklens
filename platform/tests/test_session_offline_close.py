from __future__ import annotations

from datetime import datetime, timedelta, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.heartbeat import STALE_MAX_SECONDS

REGISTER_URL = "/api/v1/agents/register"
SESSIONS_URL = "/api/v1/sessions"
APPLICATIONS_URL = "/api/v1/applications"
FLEET_URL = "/api/v1/fleet"
NOW = datetime(2026, 8, 16, 9, 0, tzinfo=timezone.utc)


def _register(client: TestClient) -> str:
    agent_id = str(uuid4())
    response = client.post(
        REGISTER_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "agent_version": "0.1.0",
            "machine_name": "LAPTOP-B",
            "operating_system": "Windows 11",
            "created_at": NOW.isoformat(),
        },
    )
    assert response.status_code == 201, response.text
    return agent_id


def _heartbeat(client: TestClient, agent_id: str, when: datetime) -> None:
    response = client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": when.isoformat(),
        },
    )
    assert response.status_code == 200, response.text


def _open_session(client: TestClient, agent_id: str, session_id: str) -> None:
    response = client.post(
        SESSIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "sessions": [
                {
                    "session_id": session_id,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": NOW.isoformat(),
                    "logoff_at": None,
                    "status": "active",
                }
            ],
        },
    )
    assert response.status_code == 200, response.text


def _activity(client: TestClient, agent_id: str, session_id: str) -> None:
    response = client.post(
        APPLICATIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "activities": [
                {
                    "session_id": session_id,
                    "application_name": "Chrome",
                    "executable_name": "chrome.exe",
                    "window_title": "Inbox",
                    "started_at": NOW.isoformat(),
                    "stopped_at": None,
                    "duration_seconds": 0,
                }
            ],
        },
    )
    assert response.status_code == 200, response.text


def test_login_activity_shutdown_closes_session_before_offline(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    _heartbeat(client, agent_id, datetime.now(timezone.utc))
    _open_session(client, agent_id, session_id)
    _activity(client, agent_id, session_id)

    logoff = datetime.now(timezone.utc)
    shutdown = client.post(
        SESSIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": logoff.isoformat(),
            "sessions": [
                {
                    "session_id": session_id,
                    "username": "ROHIT",
                    "account_sid": "S-1-5-21-1",
                    "logon_at": NOW.isoformat(),
                    "logoff_at": logoff.isoformat(),
                    "status": "closed",
                }
            ],
        },
    )
    assert shutdown.status_code == 200, shutdown.text

    closed = client.get(f"{SESSIONS_URL}/{session_id}")
    assert closed.status_code == 200
    body = closed.json()
    assert body["status"] == "closed"
    assert body["logoff_at"] is not None
    assert body["duration_seconds"] >= 0

    fleet = client.get(f"{FLEET_URL}/{agent_id}")
    assert fleet.status_code == 200
    assert fleet.json()["online_status"] == "online"
    assert closed.json()["status"] == "closed"


def test_powered_off_computer_closes_abandoned_session(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    last_seen = datetime.now(timezone.utc) - timedelta(seconds=STALE_MAX_SECONDS + 30)
    _heartbeat(client, agent_id, last_seen)
    _open_session(client, agent_id, session_id)

    fleet = client.get(f"{FLEET_URL}/{agent_id}")
    assert fleet.status_code == 200
    assert fleet.json()["online_status"] == "offline"

    page = client.get(f"{FLEET_URL}/{agent_id}/sessions")
    assert page.status_code == 200
    items = page.json()["items"]
    assert len(items) == 1
    assert items[0]["status"] == "closed"
    assert items[0]["logoff_at"] is not None
    assert items[0]["duration_seconds"] >= 0
    assert items[0]["logon_at"].startswith("2026-08-16T09:00:00")

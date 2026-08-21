from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from app.domain.agent import Agent, AgentStatus
from app.domain.sessions import (
    ListSessionsQuery,
    SessionInput,
    UnknownAgentError,
    UpsertSessionsCommand,
)
from app.infrastructure.memory import MemoryAgentRepository, MemorySessionRepository
from app.services.session_service import SessionService

NOW = datetime(2026, 8, 15, 4, tzinfo=timezone.utc)
REGISTER_URL = "/api/v1/agents/register"
SESSIONS_URL = "/api/v1/sessions"


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


def _register(client: TestClient, agent_id: str | None = None) -> str:
    agent_id = agent_id or str(uuid4())
    response = client.post(
        REGISTER_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "agent_version": "0.1.0",
            "machine_name": "LAB-PC",
            "operating_system": "Windows 11",
            "created_at": NOW.isoformat(),
        },
    )
    assert response.status_code == 201, response.text
    return agent_id


def test_repository_upsert_and_list() -> None:
    agents = MemoryAgentRepository()
    repo = MemorySessionRepository(agents)
    agent = _agent()
    agents.add(agent)
    session_id = uuid4()
    from app.domain.sessions import ComputerSession

    assert (
        repo.upsert_many(
            [
                ComputerSession(
                    session_id=session_id,
                    agent_id=agent.agent_id,
                    username="rohit",
                    account_sid="S-1-5-21-1",
                    logon_at=NOW,
                    logoff_at=None,
                    status="active",
                    collected_at=NOW,
                    received_at=NOW,
                )
            ]
        )
        == 1
    )
    page = repo.list(ListSessionsQuery(username="roh"))
    assert page.total == 1
    assert page.items[0].machine_name == "HOST-01"
    assert repo.get(session_id).username == "rohit"


def test_service_rejects_unknown_agent() -> None:
    agents = MemoryAgentRepository()
    sessions = MemorySessionRepository(agents)
    service = SessionService(agents, sessions, clock=lambda: NOW)
    with pytest.raises(UnknownAgentError):
        service.upsert(
            UpsertSessionsCommand(
                agent_id=uuid4(),
                install_id="i",
                collected_at=NOW,
                sessions=[SessionInput(session_id=uuid4(), username="rohit", logon_at=NOW)],
            )
        )


def test_sessions_endpoint_post_list_get(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    assert (
        client.post(
            SESSIONS_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "sessions": [
                    {
                        "session_id": session_id,
                        "username": "rohit",
                        "account_sid": "S-1-5-21-1",
                        "logon_at": NOW.isoformat(),
                        "logoff_at": None,
                        "status": "active",
                    }
                ],
            },
        ).status_code
        == 200
    )

    listed = client.get(SESSIONS_URL, params={"agent_id": agent_id})
    assert listed.status_code == 200
    body = listed.json()
    assert body["total"] == 1
    assert body["items"][0]["username"] == "rohit"
    assert body["items"][0]["status"] == "active"
    assert body["items"][0]["machine_name"] == "LAB-PC"

    detail = client.get(f"{SESSIONS_URL}/{session_id}")
    assert detail.status_code == 200
    assert detail.json()["session_id"] == session_id

    missing = client.get(f"{SESSIONS_URL}/{uuid4()}")
    assert missing.status_code == 404


def test_closed_session_keeps_login_and_computes_duration(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    logoff = datetime(2026, 8, 15, 17, 30, tzinfo=timezone.utc)
    assert (
        client.post(
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
        ).status_code
        == 200
    )
    detail = client.get(f"{SESSIONS_URL}/{session_id}")
    assert detail.status_code == 200
    body = detail.json()
    assert body["status"] == "closed"
    assert body["logon_at"].startswith("2026-08-15T04:00:00")
    assert body["logoff_at"].startswith("2026-08-15T17:30:00")
    assert body["duration_seconds"] == 13 * 3600 + 30 * 60


def test_upload_unknown_agent_returns_404(client: TestClient) -> None:
    resp = client.post(
        SESSIONS_URL,
        json={
            "agent_id": str(uuid4()),
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "sessions": [
                {
                    "session_id": str(uuid4()),
                    "username": "rohit",
                    "logon_at": NOW.isoformat(),
                    "status": "active",
                }
            ],
        },
    )
    assert resp.status_code == 404
    assert resp.json()["error"]["code"] == "unknown_agent"

from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from app.domain.agent import Agent, AgentStatus
from app.domain.applications import (
    ApplicationActivityInput,
    UnknownAgentError,
    UnknownSessionError,
    UpsertApplicationActivityCommand,
    activity_id,
)
from app.domain.sessions import SessionInput, UpsertSessionsCommand
from app.infrastructure.memory import (
    MemoryAgentRepository,
    MemoryApplicationActivityRepository,
    MemorySessionRepository,
)
from app.services.application_activity_service import ApplicationActivityService
from app.services.session_service import SessionService

NOW = datetime(2026, 8, 15, 4, tzinfo=timezone.utc)
REGISTER_URL = "/api/v1/agents/register"
SESSIONS_URL = "/api/v1/sessions"
APPLICATIONS_URL = "/api/v1/applications"


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


def test_service_rejects_unknown_agent() -> None:
    agents = MemoryAgentRepository()
    sessions = MemorySessionRepository(agents)
    activities = MemoryApplicationActivityRepository(agents, sessions)
    service = ApplicationActivityService(agents, sessions, activities, clock=lambda: NOW)
    with pytest.raises(UnknownAgentError):
        service.upsert(
            UpsertApplicationActivityCommand(
                agent_id=uuid4(),
                install_id="i",
                collected_at=NOW,
                activities=[
                    ApplicationActivityInput(
                        session_id=uuid4(),
                        application_name="Cursor",
                        started_at=NOW,
                    )
                ],
            )
        )


def test_service_rejects_unknown_session() -> None:
    agents = MemoryAgentRepository()
    sessions = MemorySessionRepository(agents)
    activities = MemoryApplicationActivityRepository(agents, sessions)
    service = ApplicationActivityService(agents, sessions, activities, clock=lambda: NOW)
    agent = _agent()
    agents.add(agent)
    with pytest.raises(UnknownSessionError):
        service.upsert(
            UpsertApplicationActivityCommand(
                agent_id=agent.agent_id,
                install_id="i",
                collected_at=NOW,
                activities=[
                    ApplicationActivityInput(
                        session_id=uuid4(),
                        application_name="Cursor",
                        started_at=NOW,
                    )
                ],
            )
        )


def test_service_upsert_and_list() -> None:
    agents = MemoryAgentRepository()
    sessions = MemorySessionRepository(agents)
    activities = MemoryApplicationActivityRepository(agents, sessions)
    session_service = SessionService(agents, sessions, clock=lambda: NOW)
    app_service = ApplicationActivityService(agents, sessions, activities, clock=lambda: NOW)
    agent = _agent()
    agents.add(agent)
    session_id = uuid4()
    session_service.upsert(
        UpsertSessionsCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            sessions=[SessionInput(session_id=session_id, username="rohit", logon_at=NOW)],
        )
    )
    stored, _ = app_service.upsert(
        UpsertApplicationActivityCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            activities=[
                ApplicationActivityInput(
                    session_id=session_id,
                    application_name="Cursor",
                    executable_name="Cursor.exe",
                    window_title="plan.md",
                    started_at=NOW,
                )
            ],
        )
    )
    assert stored == 1
    from app.domain.applications import ListApplicationActivityQuery

    page = app_service.list(ListApplicationActivityQuery(agent_id=agent.agent_id))
    assert page.total == 1
    assert page.items[0].username == "rohit"
    assert page.items[0].id == activity_id(session_id, "Cursor.exe", NOW)


def test_applications_require_existing_session(client: TestClient) -> None:
    agent_id = _register(client)
    resp = client.post(
        APPLICATIONS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "activities": [
                {
                    "session_id": str(uuid4()),
                    "application_name": "Cursor",
                    "executable_name": "Cursor.exe",
                    "started_at": NOW.isoformat(),
                    "duration_seconds": 12,
                }
            ],
        },
    )
    assert resp.status_code == 422
    assert resp.json()["error"]["code"] == "unknown_session"


def test_applications_endpoint_post_list_get(client: TestClient) -> None:
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
                        "status": "active",
                    }
                ],
            },
        ).status_code
        == 200
    )
    assert (
        client.post(
            APPLICATIONS_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "activities": [
                    {
                        "session_id": session_id,
                        "application_name": "Cursor",
                        "executable_name": "Cursor.exe",
                        "window_title": "plan.md",
                        "started_at": NOW.isoformat(),
                        "duration_seconds": 42,
                    }
                ],
            },
        ).status_code
        == 200
    )

    listed = client.get(APPLICATIONS_URL, params={"agent_id": agent_id})
    assert listed.status_code == 200
    body = listed.json()
    assert body["total"] == 1
    assert body["items"][0]["application_name"] == "Cursor"
    assert body["items"][0]["username"] == "rohit"
    activity_row_id = body["items"][0]["id"]

    by_session = client.get(APPLICATIONS_URL, params={"session_id": session_id})
    assert by_session.status_code == 200
    assert by_session.json()["total"] == 1

    detail = client.get(f"{APPLICATIONS_URL}/{activity_row_id}")
    assert detail.status_code == 200
    assert detail.json()["executable_name"] == "Cursor.exe"

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from app.domain.browser import (
    ApplicationInterval,
    BrowserActivityInput,
    BrowserLabel,
    InvalidBrowserActivityError,
    ListBrowserActivityQuery,
    UnknownAgentError,
    UpsertBrowserActivityCommand,
    application_total_seconds,
    join_application_interval,
)
from app.domain.classification import (
    BusinessCategory,
    ClassificationInput,
    ProductivityClass,
    default_classification_engine,
)
from app.domain.agent import Agent, AgentStatus
from app.domain.sessions import SessionInput, UpsertSessionsCommand
from app.infrastructure.memory import (
    MemoryAgentRepository,
    MemoryBrowserActivityRepository,
    MemorySessionRepository,
)
from app.services.browser_activity_service import BrowserActivityService
from app.services.session_service import SessionService

NOW = datetime(2026, 8, 21, 10, tzinfo=timezone.utc)
REGISTER_URL = "/api/v1/agents/register"
SESSIONS_URL = "/api/v1/sessions"
APPLICATIONS_URL = "/api/v1/applications"
BROWSER_URL = "/api/v1/browser"


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


def _open_session(client: TestClient, agent_id: str) -> str:
    session_id = str(uuid4())
    resp = client.post(
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
    )
    assert resp.status_code == 200, resp.text
    return session_id


def test_browser_requires_existing_session(client: TestClient) -> None:
    agent_id = _register(client)
    resp = client.post(
        BROWSER_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "activities": [
                {
                    "session_id": str(uuid4()),
                    "executable_name": "chrome.exe",
                    "domain": "github.com",
                    "started_at": NOW.isoformat(),
                }
            ],
        },
    )
    assert resp.status_code == 422
    assert resp.json()["error"]["code"] == "unknown_session"


def test_browser_rejects_non_browser_executable(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = _open_session(client, agent_id)
    resp = client.post(
        BROWSER_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "activities": [
                {
                    "session_id": session_id,
                    "executable_name": "excel.exe",
                    "domain": "github.com",
                    "started_at": NOW.isoformat(),
                }
            ],
        },
    )
    assert resp.status_code == 422
    assert resp.json()["error"]["code"] == "invalid_browser_activity"


def test_browser_endpoint_post_list_get_strips_url(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = _open_session(client, agent_id)
    resp = client.post(
        BROWSER_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "collected_at": NOW.isoformat(),
            "activities": [
                {
                    "session_id": session_id,
                    "executable_name": "chrome.exe",
                    "domain": "github.com",
                    "url": "https://user:pass@github.com/org/repo?token=secret#frag",
                    "title": "GitHub",
                    "window_id": 1,
                    "tab_id": 42,
                    "started_at": NOW.isoformat(),
                    "duration_seconds": 120,
                }
            ],
        },
    )
    assert resp.status_code == 200, resp.text
    listed = client.get(BROWSER_URL, params={"agent_id": agent_id})
    assert listed.status_code == 200
    body = listed.json()
    assert body["total"] == 1
    item = body["items"][0]
    assert item["domain"] == "github.com"
    assert item["url"] == "https://github.com"
    assert "token" not in item["url"]
    assert item["tab_id"] == 42
    detail = client.get(f"{BROWSER_URL}/{item['id']}")
    assert detail.status_code == 200
    assert detail.json()["executable_name"] == "chrome.exe"


def test_application_activity_still_works_alongside_browser(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = _open_session(client, agent_id)
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
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "GitHub",
                        "started_at": NOW.isoformat(),
                        "duration_seconds": 3600,
                    }
                ],
            },
        ).status_code
        == 200
    )
    assert (
        client.post(
            BROWSER_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "activities": [
                    {
                        "session_id": session_id,
                        "executable_name": "chrome.exe",
                        "domain": "github.com",
                        "started_at": NOW.isoformat(),
                        "duration_seconds": 3600,
                    }
                ],
            },
        ).status_code
        == 200
    )
    apps = client.get(APPLICATIONS_URL, params={"agent_id": agent_id}).json()
    browsers = client.get(BROWSER_URL, params={"agent_id": agent_id}).json()
    assert apps["total"] == 1
    assert browsers["total"] == 1
    app_seconds = apps["items"][0]["duration_seconds"]
    assert app_seconds == 3600
    labeled_total = app_seconds
    assert labeled_total == 3600
    assert app_seconds + 0 == 3600


def test_join_never_increases_total_time() -> None:
    agent_id = uuid4()
    session_id = uuid4()
    start = NOW
    stop = NOW + timedelta(hours=1)
    app = ApplicationInterval(
        agent_id=agent_id,
        session_id=session_id,
        executable="chrome.exe",
        started_at=start,
        stopped_at=stop,
        duration_seconds=3600,
    )
    labels = [
        BrowserLabel(
            agent_id=agent_id,
            session_id=session_id,
            executable="chrome.exe",
            host="github.com",
            started_at=start,
            stopped_at=start + timedelta(minutes=20),
        ),
        BrowserLabel(
            agent_id=agent_id,
            session_id=session_id,
            executable="chrome.exe",
            host="youtube.com",
            started_at=start + timedelta(minutes=20),
            stopped_at=stop,
        ),
    ]
    slices = join_application_interval(app, labels)
    assert application_total_seconds([app]) == 3600
    assert sum(item.duration_seconds for item in slices) == 3600
    hosts = {item.host for item in slices if item.duration_seconds}
    assert hosts == {"github.com", "youtube.com"}


def test_join_does_not_label_non_browser_executables() -> None:
    agent_id = uuid4()
    session_id = uuid4()
    app = ApplicationInterval(
        agent_id=agent_id,
        session_id=session_id,
        executable="Code.exe",
        started_at=NOW,
        stopped_at=NOW + timedelta(hours=1),
        duration_seconds=3600,
    )
    labels = [
        BrowserLabel(
            agent_id=agent_id,
            session_id=session_id,
            executable="chrome.exe",
            host="github.com",
            started_at=NOW,
            stopped_at=NOW + timedelta(hours=1),
        )
    ]
    slices = join_application_interval(app, labels)
    assert sum(item.duration_seconds for item in slices) == 3600
    assert slices[0].host is None


@pytest.mark.parametrize(
    ("host", "category"),
    [
        ("github.com", BusinessCategory.DEVELOPMENT),
        ("office.com", BusinessCategory.OFFICE),
        ("chat.openai.com", BusinessCategory.AI_ASSISTANT),
        ("youtube.com", BusinessCategory.ENTERTAINMENT),
    ],
)
def test_manual_chrome_domain_labels(host: str, category: BusinessCategory) -> None:
    agent_id = uuid4()
    session_id = uuid4()
    app = ApplicationInterval(
        agent_id=agent_id,
        session_id=session_id,
        executable="chrome.exe",
        started_at=NOW,
        stopped_at=NOW + timedelta(hours=1),
        duration_seconds=3600,
    )
    labels = [
        BrowserLabel(
            agent_id=agent_id,
            session_id=session_id,
            executable="chrome.exe",
            host=host,
            started_at=NOW,
            stopped_at=NOW + timedelta(hours=1),
        )
    ]
    slices = join_application_interval(app, labels)
    assert sum(item.duration_seconds for item in slices) == 3600
    engine = default_classification_engine()
    labeled = next(item for item in slices if item.host == host)
    result = engine.classify(
        ClassificationInput(executable=labeled.executable, browser_domain=labeled.host)
    )
    assert result.category is category
    if category is BusinessCategory.ENTERTAINMENT:
        assert result.productivity is ProductivityClass.UNPRODUCTIVE
    else:
        assert result.productivity is ProductivityClass.PRODUCTIVE


def test_service_rejects_unknown_agent() -> None:
    agents = MemoryAgentRepository()
    sessions = MemorySessionRepository(agents)
    service = BrowserActivityService(
        agents, sessions, MemoryBrowserActivityRepository(), clock=lambda: NOW
    )
    with pytest.raises(UnknownAgentError):
        service.upsert(
            UpsertBrowserActivityCommand(
                agent_id=uuid4(),
                install_id="i",
                collected_at=NOW,
                activities=[
                    BrowserActivityInput(
                        session_id=uuid4(),
                        executable_name="chrome.exe",
                        domain="github.com",
                        started_at=NOW,
                    )
                ],
            )
        )


def test_service_strips_and_stores() -> None:
    agents = MemoryAgentRepository()
    sessions = MemorySessionRepository(agents)
    activities = MemoryBrowserActivityRepository()
    session_service = SessionService(agents, sessions, clock=lambda: NOW)
    service = BrowserActivityService(agents, sessions, activities, clock=lambda: NOW)
    agent = Agent(
        agent_id=uuid4(),
        install_id="install-1",
        agent_version="0.1.0",
        machine_name="HOST-01",
        operating_system="Windows 11",
        display_name=None,
        created_at=NOW,
        registered_at=NOW,
        status=AgentStatus.registered,
    )
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
    stored, _ = service.upsert(
        UpsertBrowserActivityCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            activities=[
                BrowserActivityInput(
                    session_id=session_id,
                    executable_name="msedge.exe",
                    domain="office.com",
                    url="https://www.office.com/mail?q=1",
                    title="Mail",
                    window_id=3,
                    tab_id=7,
                    started_at=NOW,
                )
            ],
        )
    )
    assert stored == 1
    page = service.list(ListBrowserActivityQuery(agent_id=agent.agent_id))
    assert page.total == 1
    assert page.items[0].domain == "office.com"
    assert page.items[0].url == "https://office.com"
    assert page.items[0].executable_name == "msedge.exe"


def test_service_rejects_non_browser() -> None:
    agents = MemoryAgentRepository()
    sessions = MemorySessionRepository(agents)
    service = BrowserActivityService(
        agents, sessions, MemoryBrowserActivityRepository(), clock=lambda: NOW
    )
    agent = Agent(
        agent_id=uuid4(),
        install_id="install-1",
        agent_version="0.1.0",
        machine_name="HOST-01",
        operating_system="Windows 11",
        display_name=None,
        created_at=NOW,
        registered_at=NOW,
        status=AgentStatus.registered,
    )
    agents.add(agent)
    session_id = uuid4()
    SessionService(agents, sessions, clock=lambda: NOW).upsert(
        UpsertSessionsCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            sessions=[SessionInput(session_id=session_id, username="rohit", logon_at=NOW)],
        )
    )
    with pytest.raises(InvalidBrowserActivityError):
        service.upsert(
            UpsertBrowserActivityCommand(
                agent_id=agent.agent_id,
                install_id="i",
                collected_at=NOW,
                activities=[
                    BrowserActivityInput(
                        session_id=session_id,
                        executable_name="notepad.exe",
                        domain="github.com",
                        started_at=NOW,
                    )
                ],
            )
        )

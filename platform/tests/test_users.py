from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from app.domain.agent import Agent, AgentStatus
from app.domain.user import (
    ListUsersQuery,
    UnknownAgentError,
    UpsertUsersCommand,
    UserAccount,
    UserAccountInput,
)
from app.infrastructure.memory import MemoryAgentRepository, MemoryUserRepository
from app.services.user_service import UserInventoryService

NOW = datetime(2026, 8, 14, 12, tzinfo=timezone.utc)
REGISTER_URL = "/api/v1/agents/register"
USERS_URL = "/api/v1/users"


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


def test_repository_upsert_and_list() -> None:
    agents = MemoryAgentRepository()
    repo = MemoryUserRepository(agents)
    agent = _agent()
    agents.add(agent)
    account = UserAccount(
        sid="S-1-5-21-1-2-3-1001",
        agent_id=agent.agent_id,
        username="alice",
        display_name="Alice",
        account_type="Local",
        enabled=True,
        builtin=False,
        password_required=True,
        last_logon=None,
        source="Local",
        collected_at=NOW,
        received_at=NOW,
    )
    assert repo.upsert_many([account]) == 1
    page = repo.list(ListUsersQuery(username="ali"))
    assert page.total == 1
    assert page.items[0].machine_name == "HOST-01"
    assert repo.get(account.sid).username == "alice"


def test_service_rejects_unknown_agent() -> None:
    agents = MemoryAgentRepository()
    users = MemoryUserRepository(agents)
    service = UserInventoryService(agents, users, clock=lambda: NOW)
    with pytest.raises(UnknownAgentError):
        service.upsert(
            UpsertUsersCommand(
                agent_id=uuid4(),
                install_id="i",
                collected_at=NOW,
                users=[UserAccountInput(username="a", sid="S-1-5-21-1")],
            )
        )


def test_service_upsert_and_query() -> None:
    agents = MemoryAgentRepository()
    users = MemoryUserRepository(agents)
    service = UserInventoryService(agents, users, clock=lambda: NOW)
    agent = _agent()
    agents.add(agent)
    stored, _ = service.upsert(
        UpsertUsersCommand(
            agent_id=agent.agent_id,
            install_id="i",
            collected_at=NOW,
            users=[
                UserAccountInput(username="bob", sid="S-1-5-21-9", account_type="Local", enabled=True),
                UserAccountInput(username="guest", sid="S-1-5-21-501", account_type="Guest", enabled=False),
            ],
        )
    )
    assert stored == 2
    page = service.list(ListUsersQuery(enabled=True))
    assert page.total == 1
    assert service.get("S-1-5-21-9").username == "bob"


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
        response = client.post(
            USERS_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "users": [
                    {
                        "username": f"user{index}",
                        "sid": f"S-1-5-21-concurrent-{index}",
                        "display_name": f"User {index}",
                        "account_type": "Local",
                        "enabled": True,
                        "builtin": False,
                        "password_required": True,
                        "source": "Local",
                    }
                ],
            },
        )
        return response.status_code

    with ThreadPoolExecutor(max_workers=8) as pool:
        codes = list(pool.map(upload, range(20)))
    assert all(code == 200 for code in codes)
    listed = client.get(USERS_URL, params={"limit": 100}).json()
    assert listed["total"] >= 20


def test_users_endpoint_list_get_filter(client: TestClient) -> None:
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
            USERS_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "collected_at": NOW.isoformat(),
                "users": [
                    {
                        "username": "alice",
                        "sid": "S-1-5-21-api-alice",
                        "display_name": "Alice",
                        "account_type": "Local",
                        "enabled": True,
                        "builtin": False,
                        "password_required": True,
                        "source": "Local",
                    }
                ],
            },
        ).status_code
        == 200
    )

    listed = client.get(USERS_URL, params={"username": "ali", "sort": "username"})
    assert listed.status_code == 200
    body = listed.json()
    assert body["total"] == 1
    assert body["items"][0]["machine_name"] == "LAB-PC"

    detail = client.get(f"{USERS_URL}/S-1-5-21-api-alice")
    assert detail.status_code == 200
    assert detail.json()["username"] == "alice"

    missing = client.get(f"{USERS_URL}/S-1-5-21-missing")
    assert missing.status_code == 404

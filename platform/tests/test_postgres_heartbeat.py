from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.agent import Agent, AgentStatus
from app.domain.heartbeat import Heartbeat, HeartbeatRowStatus
from app.infrastructure.postgres import (
    Database,
    PostgresAgentRepository,
    PostgresHeartbeatRepository,
)

REGISTER_URL = "/api/v1/agents/register"


def test_postgres_heartbeat_repository_appends_history(pg_database: Database) -> None:
    agents = PostgresAgentRepository(pg_database.session_factory)
    heartbeats = PostgresHeartbeatRepository(pg_database.session_factory)
    agent_id = uuid4()
    agents.add(
        Agent(
            agent_id=agent_id,
            install_id="i",
            agent_version="0.1.0",
            machine_name="M",
            operating_system="OS",
            display_name=None,
            created_at=datetime(2026, 8, 14, tzinfo=timezone.utc),
            registered_at=datetime(2026, 8, 14, 1, tzinfo=timezone.utc),
            status=AgentStatus.registered,
        )
    )

    first = heartbeats.append(
        Heartbeat(
            agent_id=agent_id,
            heartbeat_time=datetime(2026, 8, 14, 12, tzinfo=timezone.utc),
            received_at=datetime(2026, 8, 14, 12, tzinfo=timezone.utc),
            status=HeartbeatRowStatus.ok,
        )
    )
    second = heartbeats.append(
        Heartbeat(
            agent_id=agent_id,
            heartbeat_time=datetime(2026, 8, 14, 12, 1, tzinfo=timezone.utc),
            received_at=datetime(2026, 8, 14, 12, 1, tzinfo=timezone.utc),
            status=HeartbeatRowStatus.ok,
        )
    )
    assert first.agent_id == agent_id
    rows = heartbeats.list_for_agent(agent_id)
    assert len(rows) == 2
    assert heartbeats.get_latest(agent_id).heartbeat_time == second.heartbeat_time


def test_postgres_heartbeat_endpoint(pg_client: TestClient) -> None:
    agent_id = str(uuid4())
    assert (
        pg_client.post(
            REGISTER_URL,
            json={
                "agent_id": agent_id,
                "install_id": "install-1",
                "agent_version": "0.1.0",
                "machine_name": "ENDPOINT-01",
                "operating_system": "Windows 11",
                "created_at": datetime(2026, 8, 14, tzinfo=timezone.utc).isoformat(),
            },
        ).status_code
        == 201
    )
    response = pg_client.post(
        f"/api/v1/agents/{agent_id}/heartbeat",
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": datetime(2026, 8, 14, 12, tzinfo=timezone.utc).isoformat(),
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["heartbeat_status"] == "ok"

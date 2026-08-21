"""Endpoint tests: POST /api/v1/agents/register backed by PostgreSQL.

The assertions deliberately repeat the in-memory endpoint suite. Identical
HTTP behaviour on both adapters is the whole point of Sprint 3: the only
observable difference is that rows now survive in the database.
"""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from uuid import UUID, uuid4

from fastapi.testclient import TestClient
from sqlalchemy import text

from app.config import Settings
from app.infrastructure.postgres import Database
from app.main import create_app

REGISTER_URL = "/api/v1/agents/register"


def _payload(**overrides) -> dict:
    body = {
        "agent_id": str(uuid4()),
        "install_id": "install-1",
        "agent_version": "0.1.0",
        "machine_name": "ENDPOINT-01",
        "operating_system": "Windows 11",
        "display_name": "Lab machine",
        "created_at": datetime(2026, 8, 14, tzinfo=timezone.utc).isoformat(),
    }
    body.update(overrides)
    return body


def _row_count(database: Database) -> int:
    with database.engine.connect() as connection:
        return connection.execute(text("SELECT count(*) FROM agents")).scalar_one()


def test_register_agent_returns_201_and_full_envelope(pg_client: TestClient) -> None:
    payload = _payload()

    response = pg_client.post(REGISTER_URL, json=payload)

    assert response.status_code == 201, response.text
    body = response.json()
    assert body["accepted"] is True
    assert body["agent_id"] == payload["agent_id"]
    assert body["registration_status"] == "registered"
    assert body["server_version"] == "0.1.0"
    assert body["server_time"].endswith("+00:00") or body["server_time"].endswith("Z")


def test_registration_writes_a_row_to_postgres(
    pg_client: TestClient, pg_database: Database
) -> None:
    payload = _payload(display_name="Finance PC")

    assert pg_client.post(REGISTER_URL, json=payload).status_code == 201

    with pg_database.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT install_id, agent_version, machine_name, operating_system,"
                " display_name, created_at, registered_at, status"
                " FROM agents WHERE agent_id = :id"
            ),
            {"id": UUID(payload["agent_id"])},
        ).one()

    assert row.install_id == "install-1"
    assert row.agent_version == "0.1.0"
    assert row.machine_name == "ENDPOINT-01"
    assert row.operating_system == "Windows 11"
    assert row.display_name == "Finance PC"
    assert row.created_at == datetime(2026, 8, 14, tzinfo=timezone.utc)
    assert row.status == "registered"
    assert row.registered_at.tzinfo is not None


def test_registration_survives_an_application_restart(
    pg_client: TestClient, pg_settings: Settings
) -> None:
    """The persistence guarantee: a brand-new app instance still sees the agent."""
    payload = _payload()
    assert pg_client.post(REGISTER_URL, json=payload).status_code == 201

    restarted = create_app(pg_settings)
    with TestClient(restarted) as second_client:
        conflict = second_client.post(REGISTER_URL, json=payload)

    assert conflict.status_code == 409
    assert conflict.json()["error"]["code"] == "duplicate_agent"


def test_duplicate_registration_returns_409_envelope(
    pg_client: TestClient, pg_database: Database
) -> None:
    payload = _payload()

    assert pg_client.post(REGISTER_URL, json=payload).status_code == 201

    second = pg_client.post(REGISTER_URL, json=payload)
    assert second.status_code == 409
    body = second.json()
    assert body["error"]["code"] == "duplicate_agent"
    assert body["error"]["details"]["agent_id"] == payload["agent_id"]
    assert body["error"]["request_id"]
    assert _row_count(pg_database) == 1


def test_register_agent_echoes_request_id_header(pg_client: TestClient) -> None:
    response = pg_client.post(
        REGISTER_URL, json=_payload(), headers={"X-Request-ID": "reg-abc"}
    )
    assert response.status_code == 201
    assert response.headers.get("X-Request-ID") == "reg-abc"


def test_invalid_uuid_returns_422_and_writes_nothing(
    pg_client: TestClient, pg_database: Database
) -> None:
    response = pg_client.post(REGISTER_URL, json=_payload(agent_id="not-a-uuid"))
    assert response.status_code == 422
    assert response.json()["error"]["code"] == "validation_error"
    assert _row_count(pg_database) == 0


def test_missing_install_id_returns_422(pg_client: TestClient) -> None:
    payload = _payload()
    payload.pop("install_id")
    response = pg_client.post(REGISTER_URL, json=payload)
    assert response.status_code == 422
    fields = {tuple(item["loc"]) for item in response.json()["error"]["details"]}
    assert ("body", "install_id") in fields


def test_missing_machine_name_returns_422(pg_client: TestClient) -> None:
    payload = _payload()
    payload.pop("machine_name")
    response = pg_client.post(REGISTER_URL, json=payload)
    assert response.status_code == 422
    fields = {tuple(item["loc"]) for item in response.json()["error"]["details"]}
    assert ("body", "machine_name") in fields


def test_missing_agent_id_returns_422(pg_client: TestClient) -> None:
    payload = _payload()
    payload.pop("agent_id")
    response = pg_client.post(REGISTER_URL, json=payload)
    assert response.status_code == 422
    fields = {tuple(item["loc"]) for item in response.json()["error"]["details"]}
    assert ("body", "agent_id") in fields


def test_blank_install_id_returns_422(pg_client: TestClient, pg_database: Database) -> None:
    assert pg_client.post(REGISTER_URL, json=_payload(install_id="   ")).status_code == 422
    assert _row_count(pg_database) == 0


def test_extra_fields_are_rejected(pg_client: TestClient, pg_database: Database) -> None:
    response = pg_client.post(REGISTER_URL, json=_payload(rogue_field="not allowed"))
    assert response.status_code == 422
    assert _row_count(pg_database) == 0


def test_openapi_documents_agents_endpoint(pg_client: TestClient) -> None:
    schema = pg_client.get("/openapi.json").json()
    endpoint = schema["paths"][REGISTER_URL]["post"]
    assert set(endpoint["responses"]) >= {"201", "409", "422"}
    assert "agents" in endpoint["tags"]


def test_many_distinct_agents_all_persist(
    pg_client: TestClient, pg_database: Database
) -> None:
    for index in range(10):
        response = pg_client.post(REGISTER_URL, json=_payload(machine_name=f"HOST-{index}"))
        assert response.status_code == 201, response.text

    assert _row_count(pg_database) == 10


def test_concurrent_duplicate_registrations_yield_exactly_one_201(
    pg_client: TestClient, pg_database: Database
) -> None:
    """Two agents racing with the same AgentID: one 201, the rest 409."""
    payload = _payload()

    def post(_: int) -> int:
        return pg_client.post(REGISTER_URL, json=payload).status_code

    with ThreadPoolExecutor(max_workers=6) as pool:
        codes = list(pool.map(post, range(6)))

    assert codes.count(201) == 1
    assert codes.count(409) == 5
    assert _row_count(pg_database) == 1


def test_health_endpoint_still_works_with_postgres_backend(pg_client: TestClient) -> None:
    assert pg_client.get("/health").json()["status"] == "healthy"

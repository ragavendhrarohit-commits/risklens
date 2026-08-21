from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient


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


def test_register_agent_returns_201_and_full_envelope(client: TestClient) -> None:
    payload = _payload()

    response = client.post("/api/v1/agents/register", json=payload)

    assert response.status_code == 201, response.text
    body = response.json()
    assert body["accepted"] is True
    assert body["agent_id"] == payload["agent_id"]
    assert body["registration_status"] == "registered"
    assert body["server_version"] == "0.1.0"
    assert body["server_time"].endswith("+00:00") or body["server_time"].endswith("Z")


def test_register_agent_echoes_request_id_header(client: TestClient) -> None:
    response = client.post(
        "/api/v1/agents/register",
        json=_payload(),
        headers={"X-Request-ID": "reg-abc"},
    )
    assert response.status_code == 201
    assert response.headers.get("X-Request-ID") == "reg-abc"


def test_duplicate_registration_returns_409_envelope(client: TestClient) -> None:
    payload = _payload()

    first = client.post("/api/v1/agents/register", json=payload)
    assert first.status_code == 201

    second = client.post("/api/v1/agents/register", json=payload)
    assert second.status_code == 409
    body = second.json()
    assert body["error"]["code"] == "duplicate_agent"
    assert body["error"]["details"]["agent_id"] == payload["agent_id"]
    assert body["error"]["request_id"]


def test_invalid_uuid_returns_422(client: TestClient) -> None:
    response = client.post(
        "/api/v1/agents/register",
        json=_payload(agent_id="not-a-uuid"),
    )
    assert response.status_code == 422
    body = response.json()
    assert body["error"]["code"] == "validation_error"


def test_missing_install_id_returns_422(client: TestClient) -> None:
    payload = _payload()
    payload.pop("install_id")
    response = client.post("/api/v1/agents/register", json=payload)
    assert response.status_code == 422
    body = response.json()
    assert body["error"]["code"] == "validation_error"
    fields = {tuple(item["loc"]) for item in body["error"]["details"]}
    assert ("body", "install_id") in fields


def test_missing_machine_name_returns_422(client: TestClient) -> None:
    payload = _payload()
    payload.pop("machine_name")
    response = client.post("/api/v1/agents/register", json=payload)
    assert response.status_code == 422
    fields = {tuple(item["loc"]) for item in response.json()["error"]["details"]}
    assert ("body", "machine_name") in fields


def test_missing_agent_id_returns_422(client: TestClient) -> None:
    payload = _payload()
    payload.pop("agent_id")
    response = client.post("/api/v1/agents/register", json=payload)
    assert response.status_code == 422
    fields = {tuple(item["loc"]) for item in response.json()["error"]["details"]}
    assert ("body", "agent_id") in fields


def test_blank_install_id_returns_422(client: TestClient) -> None:
    response = client.post("/api/v1/agents/register", json=_payload(install_id="   "))
    assert response.status_code == 422


def test_extra_fields_are_rejected(client: TestClient) -> None:
    response = client.post(
        "/api/v1/agents/register",
        json=_payload(rogue_field="not allowed"),
    )
    assert response.status_code == 422


def test_openapi_documents_agents_endpoint(client: TestClient) -> None:
    schema = client.get("/openapi.json").json()
    assert "/api/v1/agents/register" in schema["paths"]
    endpoint = schema["paths"]["/api/v1/agents/register"]["post"]
    responses = endpoint["responses"]
    assert "201" in responses
    assert "409" in responses
    assert "422" in responses
    assert "agents" in endpoint["tags"]


def test_registrations_are_isolated_per_app_instance(client: TestClient) -> None:
    """Verifies the DI container is fresh per TestClient (fresh in-memory repo per test)."""
    payload = _payload()
    r = client.post("/api/v1/agents/register", json=payload)
    assert r.status_code == 201, r.text

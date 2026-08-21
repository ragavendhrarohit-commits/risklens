from __future__ import annotations

from fastapi.testclient import TestClient


def test_root_returns_service_metadata(client: TestClient) -> None:
    response = client.get("/")
    assert response.status_code == 200
    payload = response.json()
    assert payload == {
        "service": "RiskLens Backend",
        "status": "running",
        "version": "0.1.0",
    }


def test_root_sets_request_id_header(client: TestClient) -> None:
    response = client.get("/")
    assert response.headers.get("X-Request-ID"), response.headers
    assert response.headers.get("X-Response-Time-ms")


def test_root_echoes_client_request_id(client: TestClient) -> None:
    response = client.get("/", headers={"X-Request-ID": "abc-123"})
    assert response.headers.get("X-Request-ID") == "abc-123"

from __future__ import annotations

from fastapi.testclient import TestClient


def test_version_returns_build_metadata(client: TestClient) -> None:
    response = client.get("/version")
    assert response.status_code == 200
    payload = response.json()
    assert payload == {
        "version": "0.1.0",
        "build_time": "2026-08-14T00:00:00Z",
        "environment": "testing",
    }


def test_openapi_is_available(client: TestClient) -> None:
    response = client.get("/openapi.json")
    assert response.status_code == 200
    schema = response.json()
    assert schema["info"]["title"] == "RiskLens Backend"
    tags = {tag["name"] for tag in schema.get("tags", [])}
    assert {"meta", "health"} <= tags


def test_docs_reachable(client: TestClient) -> None:
    response = client.get("/docs")
    assert response.status_code == 200


def test_not_found_uses_error_envelope(client: TestClient) -> None:
    response = client.get("/does-not-exist")
    assert response.status_code == 404
    body = response.json()
    assert body["error"]["code"] == "not_found"
    assert body["error"]["request_id"]

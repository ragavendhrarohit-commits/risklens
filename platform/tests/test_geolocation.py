from __future__ import annotations

from datetime import datetime, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.geolocation import is_public_ip

AGENTS_URL = "/api/v1/agents/register"
HEARTBEAT_URL = "/api/v1/agents/{agent_id}/heartbeat"
GEO_URL = "/api/v1/geolocation"
NOW = datetime(2026, 8, 22, 10, 0, tzinfo=timezone.utc)
MELBOURNE_TEST_IP = "49.177.1.1"
SYDNEY_TEST_IP = "1.2.3.4"


def _register(client: TestClient, *, machine_name: str = "MSI-Laptop") -> str:
    agent_id = str(uuid4())
    response = client.post(
        AGENTS_URL,
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "agent_version": "0.1.0",
            "machine_name": machine_name,
            "operating_system": "Windows 11",
            "created_at": NOW.isoformat(),
        },
    )
    assert response.status_code == 201, response.text
    return agent_id


def test_is_public_ip_helpers() -> None:
    assert is_public_ip(MELBOURNE_TEST_IP) is True
    assert is_public_ip("192.168.1.10") is False
    assert is_public_ip("127.0.0.1") is False
    assert is_public_ip("") is False


def test_geolocation_ingested_from_heartbeat(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.post(
        HEARTBEAT_URL.format(agent_id=agent_id),
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
            "public_ip": MELBOURNE_TEST_IP,
            "private_ip": "192.168.1.25",
            "hostname": "MSI-Laptop",
        },
    )
    assert response.status_code == 200, response.text

    listing = client.get(GEO_URL)
    assert listing.status_code == 200, listing.text
    body = listing.json()
    assert body["summary"]["managed_devices"] == 1
    assert body["summary"]["online_devices"] == 1
    assert body["summary"]["countries"] == 1
    assert body["items"][0]["computer"] == "MSI-Laptop"
    assert body["items"][0]["city"] == "Melbourne"
    assert body["items"][0]["country"] == "Australia"
    assert body["items"][0]["isp"] == "Aussie Broadband"
    assert body["items"][0]["public_ip"] == MELBOURNE_TEST_IP
    assert body["items"][0]["private_ip"] == "192.168.1.25"

    detail = client.get(f"{GEO_URL}/{agent_id}")
    assert detail.status_code == 200, detail.text
    assert detail.json()["latitude"] == -37.8136


def test_geolocation_lookup_cached_until_public_ip_changes(client: TestClient) -> None:
    agent_id = _register(client, machine_name="John-PC")
    first = client.post(
        HEARTBEAT_URL.format(agent_id=agent_id),
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
            "public_ip": SYDNEY_TEST_IP,
            "private_ip": "10.0.0.8",
            "hostname": "John-PC",
        },
    )
    assert first.status_code == 200, first.text
    initial = client.get(f"{GEO_URL}/{agent_id}").json()
    assert initial["city"] == "Sydney"

    second = client.post(
        HEARTBEAT_URL.format(agent_id=agent_id),
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": (NOW.replace(minute=1)).isoformat(),
            "public_ip": SYDNEY_TEST_IP,
            "private_ip": "10.0.0.9",
            "hostname": "John-PC",
        },
    )
    assert second.status_code == 200, second.text
    cached = client.get(f"{GEO_URL}/{agent_id}").json()
    assert cached["city"] == "Sydney"
    assert cached["private_ip"] == "10.0.0.9"

    third = client.post(
        HEARTBEAT_URL.format(agent_id=agent_id),
        json={
            "agent_id": agent_id,
            "install_id": "install-1",
            "timestamp": (NOW.replace(minute=2)).isoformat(),
            "public_ip": MELBOURNE_TEST_IP,
            "private_ip": "10.0.0.9",
            "hostname": "John-PC",
        },
    )
    assert third.status_code == 200, third.text
    refreshed = client.get(f"{GEO_URL}/{agent_id}").json()
    assert refreshed["city"] == "Melbourne"


def test_geolocation_search_and_filters(client: TestClient) -> None:
    melbourne = _register(client, machine_name="MSI-Laptop")
    sydney = _register(client, machine_name="John-PC")
    client.post(
        HEARTBEAT_URL.format(agent_id=melbourne),
        json={
            "agent_id": melbourne,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
            "public_ip": MELBOURNE_TEST_IP,
            "private_ip": "192.168.1.25",
            "hostname": "MSI-Laptop",
        },
    )
    client.post(
        HEARTBEAT_URL.format(agent_id=sydney),
        json={
            "agent_id": sydney,
            "install_id": "install-1",
            "timestamp": NOW.isoformat(),
            "public_ip": SYDNEY_TEST_IP,
            "private_ip": "10.0.0.8",
            "hostname": "John-PC",
        },
    )

    by_city = client.get(GEO_URL, params={"city": "Sydney"})
    assert by_city.status_code == 200
    assert len(by_city.json()["items"]) == 1
    assert by_city.json()["items"][0]["computer"] == "John-PC"

    by_search = client.get(GEO_URL, params={"q": "Melbourne"})
    assert by_search.status_code == 200
    assert len(by_search.json()["items"]) == 1
    assert by_search.json()["items"][0]["computer"] == "MSI-Laptop"

    missing = client.get(f"{GEO_URL}/{uuid4()}")
    assert missing.status_code == 404

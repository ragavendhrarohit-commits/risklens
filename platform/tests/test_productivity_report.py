from __future__ import annotations

from datetime import datetime, timedelta, timezone
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.productivity import (
    ApplicationAggregate,
    IntervalPoint,
    TitleAggregate,
    application_usage_rows,
    average_session_length_seconds,
    idle_time_seconds,
    merge_span_metrics,
    rollup_websites,
)

AGENTS_URL = "/api/v1/agents/register"
SESSIONS_URL = "/api/v1/sessions"
APPLICATIONS_URL = "/api/v1/applications"
REPORT_URL = "/api/v1/reports/productivity"
NOW = datetime(2026, 8, 21, 10, 0, tzinfo=timezone.utc)


def _register(client: TestClient) -> str:
    agent_id = str(uuid4())
    response = client.post(
        AGENTS_URL,
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


def test_productivity_helpers() -> None:
    assert idle_time_seconds(3600, 1200) == 2400
    assert average_session_length_seconds(100, 4) == 25
    apps = application_usage_rows(
        [
            ApplicationAggregate("Cursor", 4200, 3),
            ApplicationAggregate("Chrome", 1800, 5),
        ],
        active_time_seconds=6000,
    )
    assert apps[0][0] == "Cursor"
    assert apps[0][2] == 70.0
    sites = rollup_websites(
        [
            TitleAggregate("GitHub - Google Chrome", "chrome.exe", 600, 2),
            TitleAggregate("ChatGPT - Google Chrome", "chrome.exe", 300, 1),
            TitleAggregate("Budget - Excel", "EXCEL.EXE", 999, 1),
        ]
    )
    assert sites[0][0] == "github.com"
    assert sites[0][2] == 2
    span = merge_span_metrics(
        first_activity=NOW,
        last_activity=NOW + timedelta(hours=2),
        active_time_seconds=3600,
        segment_count=2,
        applications_used=2,
        intervals=[
            IntervalPoint(NOW, NOW + timedelta(minutes=30)),
            IntervalPoint(NOW + timedelta(hours=1), NOW + timedelta(hours=2)),
        ],
    )
    assert span.working_time_seconds == 7200
    assert span.longest_idle_period_seconds == 1800
    assert span.longest_continuous_working_session_seconds == 3600


def test_productivity_report_endpoint(client: TestClient) -> None:
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
                        "window_title": "app.js",
                        "started_at": NOW.isoformat(),
                        "stopped_at": (NOW + timedelta(hours=1)).isoformat(),
                        "duration_seconds": 3600,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "GitHub - RiskLens/platform - Google Chrome",
                        "started_at": (NOW + timedelta(hours=1, minutes=10)).isoformat(),
                        "stopped_at": (NOW + timedelta(hours=1, minutes=40)).isoformat(),
                        "duration_seconds": 1800,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "ChatGPT - Google Chrome",
                        "started_at": (NOW + timedelta(hours=1, minutes=45)).isoformat(),
                        "stopped_at": (NOW + timedelta(hours=2)).isoformat(),
                        "duration_seconds": 900,
                    },
                ],
            },
        ).status_code
        == 200
    )

    response = client.get(
        REPORT_URL,
        params={
            "agent_id": agent_id,
            "started_at_from": (NOW - timedelta(hours=1)).isoformat(),
            "started_at_to": (NOW + timedelta(hours=3)).isoformat(),
        },
    )
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["empty"] is False
    assert body["employee"]["computer"] == "LAB-PC"
    assert body["summary"]["applications_used"] == 2
    assert body["summary"]["websites_visited"] >= 2
    assert body["summary"]["active_time_seconds"] == 6300
    assert body["applications"][0]["application"] == "Cursor"
    assert body["applications"][0]["launch_count"] == 1
    websites = {row["website"]: row for row in body["websites"]}
    assert "github.com" in websites
    assert "chat.openai.com" in websites
    assert body["activity"]["most_used_application"] == "Cursor"
    assert body["activity"]["most_used_website"] in {"github.com", "chat.openai.com"}


def test_productivity_report_empty_period(client: TestClient) -> None:
    agent_id = _register(client)
    response = client.get(
        REPORT_URL,
        params={
            "agent_id": agent_id,
            "started_at_from": NOW.isoformat(),
            "started_at_to": (NOW + timedelta(hours=1)).isoformat(),
        },
    )
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["empty"] is True
    assert body["summary"]["applications_used"] == 0
    assert body["applications"] == []


def test_productivity_report_unknown_agent(client: TestClient) -> None:
    response = client.get(REPORT_URL, params={"agent_id": str(uuid4())})
    assert response.status_code == 404

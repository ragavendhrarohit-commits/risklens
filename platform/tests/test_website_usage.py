from __future__ import annotations

from datetime import datetime, timedelta, timezone
from types import SimpleNamespace
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.website_usage import (
    UNKNOWN_WEBSITE,
    format_total_time,
    match_website,
    summarize_website_usage,
)

NOW = datetime(2026, 8, 21, 10, tzinfo=timezone.utc)
REGISTER_URL = "/api/v1/agents/register"
SESSIONS_URL = "/api/v1/sessions"
APPLICATIONS_URL = "/api/v1/applications"


def test_keyword_matching_examples() -> None:
    assert match_website("YouTube - Google Chrome").website == "YouTube"
    assert match_website("GitHub").website == "GitHub"
    assert match_website("ChatGPT").website == "ChatGPT"
    assert match_website("LinkedIn").website == "LinkedIn"
    assert match_website("Proxmox VE").website == "Proxmox"
    assert match_website("GeeksforGeeks").website == "GeeksforGeeks"
    assert match_website("Mail - Outlook - Microsoft Office").website == "Microsoft Office"
    assert match_website("Jira").website == "Jira"
    assert match_website("Confluence").website == "Confluence"
    assert match_website("random tab").website == UNKNOWN_WEBSITE


def test_keyword_matching_is_case_insensitive() -> None:
    assert match_website("github - mozilla firefox").website == "GitHub"
    assert match_website("CHATGPT").category == "AI Assistant"


def test_unknown_title_is_other() -> None:
    matched = match_website("New Tab - Google Chrome")
    assert matched.website == "Other"
    assert matched.category == "Other"


def test_excel_is_not_website_usage() -> None:
    rows = summarize_website_usage(
        [
            SimpleNamespace(
                executable_name="EXCEL.EXE",
                window_title="Budget - Excel",
                duration_seconds=3600,
            )
        ]
    )
    assert rows == []


def test_summary_aggregates_time_and_does_not_double_count() -> None:
    rows = summarize_website_usage(
        [
            SimpleNamespace(
                executable_name="chrome.exe",
                window_title="GitHub - Google Chrome",
                duration_seconds=3600 + 12 * 60,
            ),
            SimpleNamespace(
                executable_name="chrome.exe",
                window_title="another GitHub issue",
                duration_seconds=60,
            ),
            SimpleNamespace(
                executable_name="chrome.exe",
                window_title="ChatGPT",
                duration_seconds=3600 + 5 * 60,
            ),
            SimpleNamespace(
                executable_name="chrome.exe",
                window_title="Proxmox VE",
                duration_seconds=42 * 60,
            ),
            SimpleNamespace(
                executable_name="chrome.exe",
                window_title="LinkedIn",
                duration_seconds=18 * 60,
            ),
            SimpleNamespace(
                executable_name="chrome.exe",
                window_title="YouTube",
                duration_seconds=22 * 60,
            ),
            SimpleNamespace(
                executable_name="chrome.exe",
                window_title="GeeksforGeeks",
                duration_seconds=40 * 60,
            ),
        ]
    )
    by_site = {row.website: row for row in rows}
    assert by_site["GitHub"].browser == "Chrome"
    assert by_site["GitHub"].duration_seconds == 3600 + 12 * 60 + 60
    assert by_site["GitHub"].category == "Development"
    assert by_site["ChatGPT"].category == "AI Assistant"
    assert by_site["ChatGPT"].total_time == "1h 05m"
    assert by_site["Proxmox"].category == "Infrastructure"
    assert by_site["Proxmox"].total_time == "42m"
    assert by_site["LinkedIn"].category == "Social Media"
    assert by_site["YouTube"].category == "Entertainment"
    assert by_site["GeeksforGeeks"].category == "Learning"
    assert format_total_time(3 * 3600 + 13 * 60) == "3h 13m"


def test_open_application_activity_still_counts_existing_duration() -> None:
    rows = summarize_website_usage(
        [
            SimpleNamespace(
                executable_name="msedge.exe",
                window_title="Jira - Microsoft Edge",
                duration_seconds=90,
            )
        ]
    )
    assert len(rows) == 1
    assert rows[0].browser == "Edge"
    assert rows[0].website == "Jira"
    assert rows[0].duration_seconds == 90


def _register(client: TestClient) -> str:
    agent_id = str(uuid4())
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


def test_fleet_website_usage_uses_application_activity(client: TestClient) -> None:
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
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "GitHub - Google Chrome",
                        "started_at": NOW.isoformat(),
                        "duration_seconds": 3 * 3600 + 12 * 60,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "ChatGPT",
                        "started_at": (NOW + timedelta(minutes=1)).isoformat(),
                        "duration_seconds": 3600 + 5 * 60,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Cursor",
                        "executable_name": "Cursor.exe",
                        "window_title": "main.go",
                        "started_at": (NOW + timedelta(minutes=2)).isoformat(),
                        "duration_seconds": 999,
                    },
                ],
            },
        ).status_code
        == 200
    )
    apps = client.get(f"/api/v1/fleet/{agent_id}/applications")
    assert apps.status_code == 200
    assert apps.json()["total"] == 3

    usage = client.get(f"/api/v1/fleet/{agent_id}/website-usage")
    assert usage.status_code == 200, usage.text
    body = usage.json()
    assert body["total"] == 2
    by_site = {item["website"]: item for item in body["items"]}
    assert by_site["GitHub"]["browser"] == "Chrome"
    assert by_site["GitHub"]["category"] == "Development"
    assert by_site["GitHub"]["total_time"] == "3h 12m"
    assert by_site["ChatGPT"]["category"] == "AI Assistant"
    assert by_site["ChatGPT"]["total_time"] == "1h 05m"
    assert "Cursor" not in by_site


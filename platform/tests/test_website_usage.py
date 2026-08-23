from __future__ import annotations

from datetime import datetime, timedelta, timezone
from types import SimpleNamespace
from uuid import uuid4

from fastapi.testclient import TestClient

from app.domain.website_usage import (
    UNKNOWN_BROWSER_WEBSITE,
    UNKNOWN_WEBSITE,
    browser_activity_segments,
    format_total_time,
    match_website,
    parse_browser_window_title,
    parse_page_title,
    parse_website_from_title,
    strip_browser_suffix,
    summarize_browser_activity,
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


def test_parse_website_from_window_title() -> None:
    assert parse_website_from_title("GitHub - Google Chrome") == "github.com"
    assert parse_website_from_title("ChatGPT") == "chat.openai.com"
    assert parse_website_from_title("Mail - Outlook - Microsoft Office - Microsoft Edge") == "office.com"
    assert parse_website_from_title("docs.google.com - Google Chrome") == "docs.google.com"
    assert parse_website_from_title("New Tab - Google Chrome") == UNKNOWN_BROWSER_WEBSITE
    assert strip_browser_suffix("GitHub - Google Chrome") == "GitHub"


def test_parse_browser_window_title_from_application_activity() -> None:
    examples = [
        (
            "GitHub - RiskLens/platform - Google Chrome",
            "github.com",
            "GitHub - RiskLens/platform",
        ),
        (
            "Docker Networking Tutorial - YouTube - Google Chrome",
            "youtube.com",
            "Docker Networking Tutorial - YouTube",
        ),
        (
            "Inbox (5) - Gmail - Google Chrome",
            "gmail.com",
            "Inbox (5) - Gmail",
        ),
        (
            "ChatGPT - Google Chrome",
            "chat.openai.com",
            "ChatGPT",
        ),
        (
            "LinkedIn: Rohit Ragavendhra | LinkedIn - Google Chrome",
            "linkedin.com",
            "LinkedIn: Rohit Ragavendhra | LinkedIn",
        ),
        (
            "Recording Session - YouTube - Google Chrome",
            "youtube.com",
            "Recording Session - YouTube",
        ),
        (
            "Google Search - Google Chrome",
            "google.com",
            "Google Search",
        ),
        (
            "what is portforwarding - Google Search - Google Chrome",
            "google.com",
            "what is portforwarding - Google Search",
        ),
        (
            "RiskLens - Google Chrome",
            "risklens.local",
            "RiskLens",
        ),
        (
            "AI FDE learning roadmap - Claude - Google Chrome",
            "claude.ai",
            "AI FDE learning roadmap - Claude",
        ),
        (
            "Search results | Udemy - Google Chrome",
            "udemy.com",
            "Search results | Udemy",
        ),
        ("GitHub - RiskLens/platform", "github.com", "GitHub - RiskLens/platform"),
        ("Recording Session - YouTube - Google Chrome", "youtube.com", "Recording Session - YouTube"),
        ("Gmail", "gmail.com", "Gmail"),
        ("LinkedIn", "linkedin.com", "LinkedIn"),
        ("ChatGPT", "chat.openai.com", "ChatGPT"),
        ("Google Search", "google.com", "Google Search"),
        ("Stack Overflow", "stackoverflow.com", "Stack Overflow"),
    ]
    for window_title, website, page_title in examples:
        assert parse_browser_window_title(window_title) == (website, page_title)
        assert parse_website_from_title(window_title) == website
        assert parse_page_title(window_title) == page_title
    assert parse_website_from_title("New Tab - Google Chrome") == UNKNOWN_BROWSER_WEBSITE
    assert parse_page_title("New Tab - Google Chrome") == "New Tab"
    assert parse_website_from_title("Google Chrome") != "google.com"


def test_browser_activity_keeps_github_and_youtube_titles() -> None:
    session_id = uuid4()
    rows = browser_activity_segments(
        [
            SimpleNamespace(
                id="1",
                session_id=session_id,
                executable_name="chrome.exe",
                window_title="GitHub - RiskLens/platform - Google Chrome",
                started_at=NOW,
                stopped_at=NOW,
                duration_seconds=600,
                username="rohit",
                machine_name="LAB-PC",
            ),
            SimpleNamespace(
                id="2",
                session_id=session_id,
                executable_name="chrome.exe",
                window_title="Recording Session of Ala Bolelo Song - YouTube - Google Chrome",
                started_at=NOW,
                stopped_at=NOW,
                duration_seconds=900,
                username="rohit",
                machine_name="LAB-PC",
            ),
        ]
    )
    by_site = {row.website: row for row in rows}
    assert by_site["github.com"].page_title == "GitHub - RiskLens/platform"
    assert by_site["youtube.com"].page_title == "Recording Session of Ala Bolelo Song - YouTube"
    assert UNKNOWN_BROWSER_WEBSITE not in by_site


def test_stored_domain_is_used_when_present() -> None:
    session_id = uuid4()
    rows = browser_activity_segments(
        [
            SimpleNamespace(
                id="1",
                session_id=session_id,
                executable_name="chrome.exe",
                window_title="random tab - Google Chrome",
                domain="github.com",
                started_at=NOW,
                stopped_at=NOW,
                duration_seconds=60,
                username="rohit",
                machine_name="LAB-PC",
            ),
            SimpleNamespace(
                id="2",
                session_id=session_id,
                executable_name="chrome.exe",
                window_title="Gmail - Google Chrome",
                domain="Unknown Website",
                started_at=NOW,
                stopped_at=NOW,
                duration_seconds=60,
                username="rohit",
                machine_name="LAB-PC",
            ),
        ]
    )
    assert rows[0].website == "github.com"
    assert rows[1].website == "gmail.com"


def test_browser_summary_groups_visits_by_website_and_browser() -> None:
    session_id = uuid4()
    first = SimpleNamespace(
        id="1",
        session_id=session_id,
        executable_name="chrome.exe",
        window_title="GitHub - Google Chrome",
        started_at=NOW,
        stopped_at=NOW + timedelta(minutes=20),
        duration_seconds=20 * 60,
        username="rohit",
        machine_name="LAB-PC",
    )
    second = SimpleNamespace(
        id="2",
        session_id=session_id,
        executable_name="chrome.exe",
        window_title="GitHub - Google Chrome",
        started_at=NOW + timedelta(hours=1),
        stopped_at=None,
        duration_seconds=25 * 60,
        username="rohit",
        machine_name="LAB-PC",
    )
    rows = summarize_browser_activity(browser_activity_segments([first, second]))
    assert len(rows) == 1
    assert rows[0].website == "github.com"
    assert rows[0].browser == "Chrome"
    assert rows[0].visits == 2
    assert rows[0].duration_seconds == 45 * 60
    assert rows[0].status == "active"
    assert len(rows[0].timeline) == 2
    assert rows[0].total_time == "45m"
    assert rows[0].timeline[0].status == "active"
    assert rows[0].timeline[0].page_title == "GitHub"
    assert rows[0].page_title == "GitHub"
    assert rows[0].timeline[1].status == "inactive"


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


def test_fleet_browser_activity_uses_application_activity_titles(client: TestClient) -> None:
    agent_id = _register(client)
    session_id = str(uuid4())
    earlier = NOW - timedelta(days=1)
    later = NOW
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
                        "started_at": later.isoformat(),
                        "stopped_at": (later + timedelta(hours=2, minutes=15)).isoformat(),
                        "duration_seconds": 2 * 3600 + 15 * 60,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "ChatGPT",
                        "started_at": (later + timedelta(minutes=1)).isoformat(),
                        "duration_seconds": 3600 + 20 * 60,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Microsoft Edge",
                        "executable_name": "msedge.exe",
                        "window_title": "Mail - Outlook - Microsoft Office - Microsoft Edge",
                        "started_at": (later + timedelta(minutes=2)).isoformat(),
                        "stopped_at": (later + timedelta(minutes=47)).isoformat(),
                        "duration_seconds": 45 * 60,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "New Tab - Google Chrome",
                        "started_at": (later + timedelta(minutes=3)).isoformat(),
                        "duration_seconds": 30,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Google Chrome",
                        "executable_name": "chrome.exe",
                        "window_title": "YouTube - Google Chrome",
                        "started_at": earlier.isoformat(),
                        "stopped_at": (earlier + timedelta(minutes=25)).isoformat(),
                        "duration_seconds": 25 * 60,
                    },
                    {
                        "session_id": session_id,
                        "application_name": "Excel",
                        "executable_name": "EXCEL.EXE",
                        "window_title": "Budget - Excel",
                        "started_at": later.isoformat(),
                        "duration_seconds": 9999,
                    },
                ],
            },
        ).status_code
        == 200
    )

    page = client.get(
        f"/api/v1/fleet/{agent_id}/browser-activity",
        params={
            "started_at_from": later.isoformat(),
            "started_at_to": (later + timedelta(hours=1)).isoformat(),
            "sort": "-duration_seconds",
        },
    )
    assert page.status_code == 200, page.text
    body = page.json()
    assert body["total"] == 4
    websites = [item["website"] for item in body["items"]]
    assert websites == ["github.com", "chat.openai.com", "office.com", UNKNOWN_BROWSER_WEBSITE]
    github = body["items"][0]
    assert github["browser"] == "Chrome"
    assert github["visits"] == 1
    assert github["status"] == "inactive"
    assert github["website"] == "github.com"
    assert github["total_time"] == "2h 15m"
    assert len(github["timeline"]) == 1
    assert github["timeline"][0]["status"] == "inactive"
    assert github["timeline"][0]["page_title"] == "GitHub"
    assert github["timeline"][0]["website"] == "github.com"
    assert github["timeline"][0]["browser"] == "Chrome"
    assert github["page_title"] == "GitHub"
    assert body["items"][1]["status"] == "active"
    assert body["items"][2]["browser"] == "Edge"
    assert body["items"][3]["website"] == UNKNOWN_BROWSER_WEBSITE
    assert body["websites_visited"] == 4
    assert body["active_website"] == UNKNOWN_BROWSER_WEBSITE
    assert body["active_browser"] == "Chrome"
    assert body["total_browser_time"] == "4h 20m"

    empty_range = client.get(
        f"/api/v1/fleet/{agent_id}/browser-activity",
        params={
            "started_at_from": (later + timedelta(days=1)).isoformat(),
            "started_at_to": (later + timedelta(days=2)).isoformat(),
        },
    )
    assert empty_range.status_code == 200
    assert empty_range.json()["total"] == 0

    inverted = client.get(
        f"/api/v1/fleet/{agent_id}/browser-activity",
        params={"started_at_from": later.isoformat(), "started_at_to": earlier.isoformat()},
    )
    assert inverted.status_code == 422

    invalid_sort = client.get(
        f"/api/v1/fleet/{agent_id}/browser-activity",
        params={"sort": "password"},
    )
    assert invalid_sort.status_code == 422


def test_excel_is_not_browser_activity() -> None:
    rows = browser_activity_segments(
        [
            SimpleNamespace(
                id="1",
                session_id=uuid4(),
                executable_name="EXCEL.EXE",
                window_title="Budget - Excel",
                started_at=NOW,
                stopped_at=None,
                duration_seconds=3600,
                username="rohit",
                machine_name="LAB-PC",
            )
        ]
    )
    assert rows == []


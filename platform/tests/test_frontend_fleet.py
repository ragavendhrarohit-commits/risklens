from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
APP_JS = ROOT / "frontend" / "assets" / "app.js"
INDEX_HTML = ROOT / "frontend" / "index.html"


def test_frontend_fleet_replaces_devices_nav() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert ">Fleet<" in html or "Fleet</span>" in html
    assert 'data-view="fleet"' in html
    assert "view-fleet" in html
    assert "view-fleet-detail" in html
    assert "/api/v1/fleet" in js
    assert "function loadFleet" in js
    assert "function loadFleetDetail" in js
    assert "#/fleet/" in js
    assert "view-fleet-detail" in html
    assert 'data-tab="overview"' in html
    assert 'data-tab="accounts"' in html
    assert 'data-tab="sessions"' in html
    assert 'data-tab="applications"' in html
    assert 'data-tab="browser"' in html
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/website-usage" in js
    assert 'data-tab="network"' in html
    assert 'data-tab="policies"' in html
    assert 'data-tab="health"' in html
    assert ">Accounts<" in html
    assert "Installed Applications" in html
    assert 'data-tab="users"' not in html
    assert "function loadUsers" in js
    assert "function loadSoftware" in js
    assert "/api/v1/users" in js
    assert "/api/v1/software" in js


def test_frontend_application_activity_uses_global_time_range() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    assert "function applicationActivityTimeQuery" in js
    assert "started_at_from: range.startTime" in js
    assert "started_at_to: range.endTime" in js
    assert "No application activity during the selected time range." in js
    assert "bindApplicationActivityTimeFilter" in js
    assert 'f.tab === "applications"' in js
    assert "onApplicationActivityTimeRangeChange" in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/sessions`, { limit: 200 }" in js
    assert "started_at_from" not in js.split("/website-usage")[1][:400]
    assert 'tab === "sessions" && !f.sessions' in js
    assert 'tab === "browser"' in js
    assert "loadFleetTabData" in js


def test_frontend_fleet_software_refetches_on_tab() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    assert 'tab === "software" && !f.software' not in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/software" in js


def test_frontend_session_lifecycle_display() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    assert "function formatSessionStatus" in js
    assert 'if (key === "active") return "Active"' in js
    assert 'if (key === "closed") return "Closed"' in js
    assert "Login Time" in js
    assert "Logout Time" in js
    assert "data-session-duration-live" in js
    assert "function startSessionDurationTicker" in js


def test_frontend_presence_rendering_helpers() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    assert "function formatPresenceStatus" in js
    assert "🟢 Online" in js
    assert "🟡 Warning" in js or "🟡 Stale" in js
    assert "🔴 Offline" in js

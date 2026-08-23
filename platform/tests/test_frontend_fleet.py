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
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/browser-activity" in js
    assert 'data-tab="processes"' not in html
    assert 'data-tab="services"' not in html
    assert 'data-tab="network"' not in html
    assert 'data-tab="policies"' not in html
    assert 'data-tab="health"' not in html
    assert "function renderOverviewHealthSection" in js
    assert "function manageHealthAutoRefresh" in js
    assert "function refreshOverviewHealth" in js
    assert "health-usage-track" in js
    assert "cpu_percent" in js
    assert 'state.fleet.tab !== "overview"' in js or "tab !== \"overview\"" in js
    assert 'REMOVED_FLEET_TABS' in js
    assert '"health"' in js.split("REMOVED_FLEET_TABS")[1][:200]
    assert 'data-tab="alerts"' not in html
    assert 'data-group="security"' not in html
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
    assert "No browser activity during the selected time range." in js
    assert "data-browser-summary-key" in js
    assert "function renderBrowserSessionTable" in js
    assert "function flattenBrowserSessions" in js
    assert "function filterBrowserSessions" in js
    assert "sessions.filter" in js
    assert "Page Title" in js
    assert "Browser Activity (Websites)" not in js
    assert "Click a website" not in js
    assert "All Websites" not in js
    assert "function renderBrowserDetailTable" not in js
    assert "window_title" not in js.split("/browser-activity")[1][:400]
    assert "bindApplicationActivityTimeFilter" in js
    assert 'f.tab === "applications"' in js
    assert 'f.tab === "browser"' in js
    assert "onApplicationActivityTimeRangeChange" in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/sessions`, { limit: 200 }" in js
    assert "...applicationActivityTimeQuery()" in js.split("/browser-activity")[1][:250]
    assert "sort: f.browserSort" in js
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


def test_frontend_productivity_report_page() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert 'id="view-productivity"' in html
    assert 'data-view="reports"' in html
    assert "Productivity Report" in html
    assert "/api/v1/reports/productivity" in js
    assert "function loadProductivityReport" in js
    assert "function exportProductivityExcel" in js
    assert "function productivityDonutChart" in js
    assert "function productivityStackedTimeBar" in js
    assert "function ensureProductivityChartStyles" in js
    assert "No productivity data available for the selected period." in js
    assert 'view === "productivity"' in js
    assert "state.view === \"productivity\"" in js or "state.view === 'productivity'" in js
    assert "Login Time" in js
    assert "Logout Time" in js
    assert "data-session-duration-live" in js
    assert "function startSessionDurationTicker" in js


def test_frontend_reports_opens_productivity() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert 'id="view-reports"' not in html
    assert 'id="view-productivity"' in html
    assert 'data-view="reports"' in html
    assert 'if (view === "reports")' in js
    assert 'return "#/productivity"' in js
    assert 'data-open-view="users"' not in html
    assert 'data-open-view="software"' not in html
    assert "Application Usage" not in html
    assert "Website Usage" not in html
    assert "Computer Usage" not in html
    assert "Productivity Report" in html


def test_frontend_presence_rendering_helpers() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    assert "function formatPresenceStatus" in js
    assert "🟢 Online" in js
    assert "🟡 Warning" in js or "🟡 Stale" in js
    assert "🔴 Offline" in js


def test_frontend_geolocation_page() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert 'data-view="geolocation"' in html
    assert "Geo Location" in html
    assert "view-geolocation" in html
    assert "geolocation-map" in html
    assert "No managed devices with location information." in js
    assert "/api/v1/geolocation" in js
    assert "function loadGeolocationPage" in js
    assert "function renderGeolocationMap" in js
    assert "function applyAppHash" in js
    assert "30000" in js

from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
INDEX_HTML = ROOT / "frontend" / "index.html"
APP_JS = ROOT / "frontend" / "assets" / "app.js"


def test_fleet_detail_groups_inventory_activity() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    start = html.find('id="fleet-detail-nav"')
    end = html.find('id="fleet-detail-body"', start)
    nav = html[start:end]
    assert "fleet-detail-shell" in html
    assert "fleet-detail-nav" in html
    assert "fleet-detail-content" in html
    assert 'id="fleet-detail-tabs"' not in html
    assert 'class="fleet-tab"' not in html
    assert 'data-group="inventory"' in nav
    assert 'data-group="activity"' in nav
    assert 'data-group="security"' not in nav
    assert 'data-tab="alerts"' not in nav
    assert 'data-tab="detections"' not in nav
    assert 'data-tab="response"' not in nav
    assert 'data-tab="threat-timeline"' not in nav
    assert 'data-group="future"' not in nav
    assert 'data-tab="usb"' not in nav
    assert 'data-tab="files"' not in nav
    assert 'data-tab="scheduled-tasks"' not in nav
    assert 'data-tab="startup"' not in nav
    assert "USB Devices" not in nav
    assert "File Activity" not in nav
    assert "Scheduled Tasks" not in nav
    assert "Startup Programs" not in nav
    order = [
        'data-tab="overview"',
        'data-tab="accounts"',
        'data-tab="software"',
        'data-tab="sessions"',
        'data-tab="applications"',
        'data-tab="browser"',
    ]
    positions = [nav.find(token) for token in order]
    assert all(index >= 0 for index in positions)
    assert positions == sorted(positions)
    assert 'data-tab="processes"' not in nav
    assert 'data-tab="services"' not in nav
    assert 'data-tab="network"' not in nav
    assert 'data-tab="policies"' not in nav
    assert 'data-tab="timeline"' not in nav
    assert "function renderOverviewHealthSection" in APP_JS.read_text(encoding="utf-8")
    assert "fleet-nav-item" in APP_JS.read_text(encoding="utf-8")


def test_accounts_tab_still_calls_users_api() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    assert 'tab === "accounts"' in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/users" in js or "/users" in js
    assert "/api/v1/users" in js


def test_dashboard_is_structure_only() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    assert 'id="view-dashboard"' in html
    assert "dash-card-total" in html
    assert "Compliance" in html
    assert "Recent Activity" in html
    assert "Top Applications" in html
    assert "Policy Status" in html

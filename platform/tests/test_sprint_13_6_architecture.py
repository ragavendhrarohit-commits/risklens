from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
INDEX_HTML = ROOT / "frontend" / "index.html"
APP_JS = ROOT / "frontend" / "assets" / "app.js"


def test_fleet_detail_groups_inventory_activity_security() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    start = html.find('id="fleet-detail-tabs"')
    end = html.find('id="fleet-detail-error"', start)
    tabs = html[start:end]
    assert 'data-group="inventory"' in tabs
    assert 'data-group="activity"' in tabs
    assert 'data-group="security"' in tabs
    assert 'data-group="future"' not in tabs
    assert 'data-tab="usb"' not in tabs
    assert 'data-tab="files"' not in tabs
    assert 'data-tab="scheduled-tasks"' not in tabs
    assert 'data-tab="startup"' not in tabs
    assert "USB Devices" not in tabs
    assert "File Activity" not in tabs
    assert "Scheduled Tasks" not in tabs
    assert "Startup Programs" not in tabs
    order = [
        'data-tab="overview"',
        'data-tab="accounts"',
        'data-tab="software"',
        'data-tab="processes"',
        'data-tab="services"',
        'data-tab="network"',
        'data-tab="policies"',
        'data-tab="health"',
        'data-tab="sessions"',
        'data-tab="applications"',
        'data-tab="browser"',
        'data-tab="timeline"',
    ]
    positions = [tabs.find(token) for token in order]
    assert all(index >= 0 for index in positions)
    assert positions == sorted(positions)


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

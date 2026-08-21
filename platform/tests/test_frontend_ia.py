from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
APP_JS = ROOT / "frontend" / "assets" / "app.js"
INDEX_HTML = ROOT / "frontend" / "index.html"


def test_company_nav_is_computer_centric() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert 'data-view="dashboard"' in html
    assert 'data-view="fleet"' in html
    assert 'data-view="alerts"' in html
    assert 'data-view="incidents"' in html
    assert 'data-view="reports"' in html
    assert 'data-view="policies"' in html
    assert 'data-view="administration"' in html
    assert 'data-view="settings"' in html
    assert 'id="view-dashboard"' in html
    assert 'id="view-fleet"' in html
    assert 'id="view-users"' in html
    assert 'id="view-software"' in html
    assert "Account Inventory" in html
    assert "Software Inventory" in html
    assert "function primaryNavView" in js
    assert "view: \"fleet\"" in js or "view: 'fleet'" in js
    assert "function renderActivityArchitecture" in js
    assert "function renderTimelinePlaceholders" in js
    assert "/api/v1/users" in js
    assert "/api/v1/software" in js
    assert "/api/v1/fleet" in js


def test_inventory_pages_are_not_primary_nav() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    nav_start = html.find("<nav")
    nav_end = html.find("</nav>")
    nav = html[nav_start:nav_end]
    assert 'data-view="users"' not in nav
    assert 'data-view="software"' not in nav
    assert 'data-view="groups"' not in nav
    assert "Global Users" not in nav
    assert "Global Software" not in nav

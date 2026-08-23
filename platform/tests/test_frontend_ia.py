from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
APP_JS = ROOT / "frontend" / "assets" / "app.js"
INDEX_HTML = ROOT / "frontend" / "index.html"

PRIMARY_NAV = (
    "dashboard",
    "fleet",
    "reports",
    "geolocation",
    "settings",
)

REMOVED_NAV = (
    "alerts",
    "incidents",
    "policies",
    "administration",
    "groups",
)


def test_company_nav_is_computer_centric() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    for view in PRIMARY_NAV:
        assert f'data-view="{view}"' in html
    for view in PRIMARY_NAV:
        if view == "reports":
            assert 'id="view-productivity"' in html
            assert 'id="view-reports"' not in html
            continue
        assert f'id="view-{view}"' in html
    for view in REMOVED_NAV:
        assert f'data-view="{view}"' not in html
        assert f'id="view-{view}"' not in html
    assert 'id="view-users"' in html
    assert 'id="view-software"' in html
    assert "Account Inventory" in html
    assert "Software Inventory" in html
    assert "Productivity Report" in html
    assert "function primaryNavView" in js
    assert "REMOVED_NAV_VIEWS" in js
    assert 'if (view === "reports")' in js
    assert "view: \"fleet\"" in js or "view: 'fleet'" in js
    assert "function renderActivityArchitecture" in js
    assert 'data-tab="processes"' not in html
    assert 'data-tab="services"' not in html
    assert 'data-tab="network"' not in html
    assert 'data-tab="policies"' not in html
    assert 'data-tab="timeline"' not in html
    assert "/api/v1/users" in js
    assert "/api/v1/software" in js
    assert "/api/v1/fleet" in js
    assert "/api/v1/reports/productivity" in js


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
    for view in REMOVED_NAV:
        assert f'data-view="{view}"' not in nav

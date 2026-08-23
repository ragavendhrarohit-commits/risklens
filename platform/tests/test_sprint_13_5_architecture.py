from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOMAIN = ROOT / "platform" / "app" / "domain"
INDEX_HTML = ROOT / "frontend" / "index.html"
APP_JS = ROOT / "frontend" / "assets" / "app.js"

RESERVED = (
    "activity",
    "network",
    "alerts",
    "incidents",
    "rules",
    "reports",
    "ai",
    "groups",
    "administration",
)

REMOVED = (
    "usb",
    "files",
    "scheduled_tasks",
    "startup",
    "timeline",
)


def test_reserved_domain_packages_are_placeholders_only() -> None:
    for name in RESERVED:
        package = DOMAIN / name
        init = package / "__init__.py"
        assert init.is_file(), f"missing reserved package {name}"
        extras = [path.name for path in package.iterdir() if path.name != "__init__.py"]
        assert extras == [], f"{name} must stay empty: {extras}"
        text = init.read_text(encoding="utf-8")
        assert "No entities, commands, or business logic" in text
        assert "def " not in text
        assert "class " not in text


def test_removed_collector_domains_do_not_exist() -> None:
    for name in REMOVED:
        assert not (DOMAIN / name).exists(), f"{name} must not exist"


def test_dead_pre_fleet_views_removed() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert 'id="view-overview"' not in html
    assert 'id="view-anomalies"' not in html
    assert 'id="view-service-accounts"' not in html
    assert 'id="view-locations"' not in html
    assert 'id="view-data-sources"' not in html
    assert "PlatformApiService" not in js
    assert 'id="view-groups"' not in html
    assert 'id="view-alerts"' not in html
    assert 'id="view-incidents"' not in html
    assert 'id="view-policies"' not in html
    assert 'id="view-administration"' not in html
    assert 'id="view-fleet"' in html
    assert 'id="view-users"' in html
    assert 'id="view-software"' in html
    assert 'id="view-geolocation"' in html
    assert 'id="view-settings"' in html
    assert 'id="view-productivity"' in html
    assert 'id="view-reports"' not in html

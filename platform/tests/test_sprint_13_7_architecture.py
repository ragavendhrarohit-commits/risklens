from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
INDEX_HTML = ROOT / "frontend" / "index.html"
APP_JS = ROOT / "frontend" / "assets" / "app.js"
ARCH = ROOT / "docs" / "ARCHITECTURE.md"


def test_software_ui_label_is_installed_applications() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert 'data-tab="software">Installed Applications</button>' in html
    assert "/api/v1/software" in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/software" in js or "/software" in js


def test_sessions_and_application_activity_use_fleet_routes() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    js = APP_JS.read_text(encoding="utf-8")
    assert 'data-tab="sessions"' in html
    assert 'data-tab="applications"' in html
    assert 'data-tab="browser"' in html
    assert "Computer → Session → Account" in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/sessions" in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/applications" in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/browser-activity" in js
    assert "/api/v1/fleet/${encodeURIComponent(f.selectedId)}/browser`" not in js
    assert "function renderBrowserActivityTable" in js
    assert "Total Time" in js
    assert (
        "Browser Activity is website usage on this computer during a session. Collectors are not implemented."
        not in js
    )


def test_official_architecture_doc_exists() -> None:
    text = ARCH.read_text(encoding="utf-8")
    assert "Sprint 13.7" in text
    assert "Computer → Session → Account" in text
    assert "```mermaid" in text

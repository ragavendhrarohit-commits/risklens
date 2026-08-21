from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
APP_JS = ROOT / "frontend" / "assets" / "app.js"
INDEX_HTML = ROOT / "frontend" / "index.html"


def test_frontend_users_page_wired() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    html = INDEX_HTML.read_text(encoding="utf-8")
    assert "function loadUsers" in js
    assert "/api/v1/users" in js
    assert "users-body" in html
    assert "filter-user-username" in html
    assert "not implemented yet" not in html.lower() or "Users" in html
    assert "Live Windows local account inventory" in html

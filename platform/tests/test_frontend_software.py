from __future__ import annotations

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
APP_JS = ROOT / "frontend" / "assets" / "app.js"
INDEX_HTML = ROOT / "frontend" / "index.html"


def test_frontend_software_page_wired() -> None:
    js = APP_JS.read_text(encoding="utf-8")
    html = INDEX_HTML.read_text(encoding="utf-8")

    assert 'data-view="software"' in html
    assert 'id="view-software"' in html
    assert "software-body" in html
    assert "filter-software-name" in html
    assert "filter-software-publisher" in html
    assert "filter-software-version" in html
    assert "filter-software-arch" in html
    assert "software-refresh" in html
    assert "software-prev" in html
    assert "software-next" in html

    assert "function loadSoftware" in js
    assert "/api/v1/software" in js
    assert "software: {" in js
    assert "softwareSortParam" in js

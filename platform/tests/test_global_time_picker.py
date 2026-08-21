from __future__ import annotations

import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
INDEX_HTML = ROOT / "frontend" / "index.html"
APP_JS = ROOT / "frontend" / "assets" / "app.js"
CSS = ROOT / "frontend" / "assets" / "app.css"
PICKER_JS = ROOT / "frontend" / "assets" / "global-time-picker.js"
NODE_TEST = Path(__file__).resolve().parent / "js" / "run_global_time_picker.js"


def test_time_picker_is_in_header_and_reuses_phase1() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    picker = PICKER_JS.read_text(encoding="utf-8")
    css = CSS.read_text(encoding="utf-8")
    header = html[html.find("<header") : html.find("</header>")]
    assert 'id="global-time-picker"' in header
    assert 'id="gtp-toggle"' in header
    assert "gtp-calendar" in header
    assert "Last 15 Minutes" in header
    assert "gtp-caret" in header
    assert "Quick Select" in header
    assert "Recently Used" in header
    assert "Start Date" in header
    assert "Start Time" in header
    assert "End Date" in header
    assert "End Time" in header
    assert 'id="gtp-apply"' in header
    assert ">Apply<" in header
    assert ">Cancel<" in header
    assert "Refresh Every" in header
    assert 'src="/assets/global-time-filter.js"' in html
    assert 'src="/assets/global-time-picker.js"' in html
    assert html.index("/assets/global-time-filter.js") < html.index("/assets/global-time-picker.js")
    assert html.index("/assets/global-time-picker.js") < html.index("/assets/app.js")
    assert "createTimePickerController" in picker
    assert "getGlobalTimeStore" in picker
    assert "setCustomRange" in picker
    assert "shouldClosePicker" in picker
    assert "nextFocusIndex" in picker
    assert ".gtp-toggle" in css
    assert ".gtp.is-open .gtp-panel" in css
    assert "transition:" in css


def test_time_picker_labels_match_spec() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    picker = PICKER_JS.read_text(encoding="utf-8")
    for label in (
        "Today",
        "Yesterday",
        "Last 15 Minutes",
        "Last 30 Minutes",
        "Last 1 Hour",
        "Last 24 Hours",
        "Last 7 Days",
        "Last 30 Days",
        "Last 90 Days",
        "Last Year",
        "Custom Range",
        "Off",
        "30 Seconds",
        "1 Minute",
        "5 Minutes",
        "10 Minutes",
        "30 Minutes",
    ):
        assert label in picker
    assert "Refresh Every" in html


def test_time_picker_does_not_connect_pages_or_apis() -> None:
    app = APP_JS.read_text(encoding="utf-8")
    picker = PICKER_JS.read_text(encoding="utf-8")
    assert "createTimePickerController" not in app
    assert "loadFleet(" not in picker
    assert "loadFleetTabData" not in picker
    assert "/api/v1/" not in picker
    assert "fetch(" not in picker
    assert "location.reload" not in picker
    assert "website-usage" not in picker
    assert "/applications" not in picker
    assert "/sessions" not in picker


def test_time_picker_runtime_behaviour() -> None:
    completed = subprocess.run(
        ["node", str(NODE_TEST)],
        check=False,
        capture_output=True,
        text=True,
    )
    assert completed.returncode == 0, completed.stdout + completed.stderr
    assert "global time picker tests passed" in completed.stdout

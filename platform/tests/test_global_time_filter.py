from __future__ import annotations

import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
INDEX_HTML = ROOT / "frontend" / "index.html"
APP_JS = ROOT / "frontend" / "assets" / "app.js"
TIME_JS = ROOT / "frontend" / "assets" / "global-time-filter.js"
NODE_TEST = Path(__file__).resolve().parent / "js" / "run_global_time_filter.js"


def test_phase1_module_exists_and_only_application_activity_consumes_it() -> None:
    html = INDEX_HTML.read_text(encoding="utf-8")
    app = APP_JS.read_text(encoding="utf-8")
    module = TIME_JS.read_text(encoding="utf-8")

    assert TIME_JS.is_file()
    assert "createTimeStore" in module
    assert "resolveTimeRange" in module
    assert "getRecentlyUsed" in module
    assert 'src="/assets/global-time-filter.js"' in html
    assert 'src="/assets/app.js"' in html
    assert "function applicationActivityTimeQuery" in app
    assert "bindApplicationActivityTimeFilter" in app
    assert "createTimeStore" not in app
    assert "setCustomRange" not in app
    assert "setRefreshInterval" not in app


def test_phase1_defines_required_presets_and_refresh_intervals() -> None:
    module = TIME_JS.read_text(encoding="utf-8")
    for label in (
        "Today",
        "Yesterday",
        "Last 15 minutes",
        "Last 30 minutes",
        "Last 1 hour",
        "Last 24 hours",
        "Last 7 days",
        "Last 30 days",
        "Last 90 days",
        "Last Year",
        "Custom",
    ):
        assert f'label: "{label}"' in module
    for label in (
        "Off",
        "30 seconds",
        "1 minute",
        "5 minutes",
        "10 minutes",
        "30 minutes",
    ):
        assert f'label: "{label}"' in module
    for field in ("startTime", "endTime", "preset", "timezone", "refreshInterval"):
        assert field in module


def test_phase1_runtime_behaviour() -> None:
    completed = subprocess.run(
        ["node", str(NODE_TEST)],
        check=False,
        capture_output=True,
        text=True,
    )
    assert completed.returncode == 0, completed.stdout + completed.stderr
    assert "global time filter tests passed" in completed.stdout

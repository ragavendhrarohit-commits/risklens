from __future__ import annotations

import io
import json
import logging

import pytest

from app.logging.context import bind_request_id, clear_request_id, get_request_id
from app.logging.setup import JsonFormatter, configure_logging, get_logger


def test_json_formatter_produces_valid_json() -> None:
    record = logging.LogRecord(
        name="risklens.test",
        level=logging.INFO,
        pathname=__file__,
        lineno=1,
        msg="hello %s",
        args=("world",),
        exc_info=None,
    )
    record.custom = "value"
    output = JsonFormatter().format(record)
    payload = json.loads(output)
    assert payload["level"] == "INFO"
    assert payload["msg"] == "hello world"
    assert payload["logger"] == "risklens.test"
    assert payload["custom"] == "value"
    assert payload["time"].endswith("+00:00")


def test_json_formatter_includes_request_id() -> None:
    token = bind_request_id("req-42")
    try:
        record = logging.LogRecord("risklens", logging.INFO, __file__, 1, "hi", None, None)
        payload = json.loads(JsonFormatter().format(record))
        assert payload["request_id"] == "req-42"
    finally:
        clear_request_id(token)


def test_configure_logging_replaces_handlers(monkeypatch: pytest.MonkeyPatch) -> None:
    buffer = io.StringIO()
    configure_logging(level="DEBUG", json_output=True)
    root = logging.getLogger()
    assert len(root.handlers) == 1
    root.handlers[0].stream = buffer  # type: ignore[attr-defined]

    logger = get_logger("risklens.logging.test")
    logger.info("captured", extra={"kind": "unit"})

    line = buffer.getvalue().strip()
    payload = json.loads(line)
    assert payload["msg"] == "captured"
    assert payload["kind"] == "unit"


def test_request_id_context_isolated_per_run() -> None:
    assert get_request_id() is None
    token = bind_request_id("abc")
    assert get_request_id() == "abc"
    clear_request_id(token)
    assert get_request_id() is None

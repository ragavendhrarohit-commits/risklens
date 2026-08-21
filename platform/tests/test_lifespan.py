from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

from app.config.settings import Environment, Settings
from app.dependencies.container import Container
from app.main import create_app


def test_lifespan_installs_container_and_settings() -> None:
    settings = Settings(
        environment=Environment.testing,
        app_name="RiskLens Backend",
        app_version="0.1.0",
        build_time="2026-08-14T00:00:00Z",
    )
    app = create_app(settings)

    with TestClient(app):
        container = app.state.container
        assert isinstance(container, Container)
        assert container.resolve(Settings).app_name == "RiskLens Backend"
        assert app.state.settings.environment is Environment.testing


def test_lifespan_shutdown_clears_container() -> None:
    settings = Settings(environment=Environment.testing)
    app = create_app(settings)

    with TestClient(app):
        pass

    container: Container = app.state.container
    assert container.has(Settings) is False


def test_lifespan_logs_startup_and_shutdown(monkeypatch: pytest.MonkeyPatch) -> None:
    messages: list[str] = []

    class _FakeLogger:
        def info(self, msg: str, *args: object, **kwargs: object) -> None:
            messages.append(msg)

        def warning(self, msg: str, *args: object, **kwargs: object) -> None:
            return None

    monkeypatch.setattr("app.startup.lifespan.get_logger", lambda _name: _FakeLogger())
    settings = Settings(environment=Environment.testing, log_json=True, log_level="INFO")
    app = create_app(settings)

    with TestClient(app):
        pass

    assert "startup" in messages
    assert "shutdown" in messages

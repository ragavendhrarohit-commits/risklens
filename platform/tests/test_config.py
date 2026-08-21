from __future__ import annotations

import pytest

from app.config.settings import Environment, Settings


def test_defaults_load() -> None:
    settings = Settings(_env_file=None)  # type: ignore[call-arg]
    assert settings.app_name == "RiskLens Backend"
    assert settings.environment is Environment.development
    assert settings.log_level == "INFO"


def test_environment_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RISKLENS_ENVIRONMENT", "production")
    monkeypatch.setenv("RISKLENS_LOG_LEVEL", "warning")
    settings = Settings(_env_file=None)  # type: ignore[call-arg]
    assert settings.environment is Environment.production
    assert settings.log_level == "WARNING"
    assert settings.is_production is True


def test_allowed_hosts_csv(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RISKLENS_ALLOWED_HOSTS", "example.com, api.example.com")
    settings = Settings(_env_file=None)  # type: ignore[call-arg]
    assert settings.allowed_hosts == ["example.com", "api.example.com"]


def test_invalid_environment_rejected(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RISKLENS_ENVIRONMENT", "staging")
    with pytest.raises(Exception):
        Settings(_env_file=None)  # type: ignore[call-arg]


def test_invalid_log_level_rejected(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RISKLENS_LOG_LEVEL", "shout")
    with pytest.raises(Exception):
        Settings(_env_file=None)  # type: ignore[call-arg]


def test_invalid_port_rejected(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RISKLENS_PORT", "0")
    with pytest.raises(Exception):
        Settings(_env_file=None)  # type: ignore[call-arg]

from __future__ import annotations

from enum import Enum
from functools import lru_cache
from pathlib import Path
from typing import Annotated, Literal

from pydantic import Field, field_validator
from pydantic_settings import BaseSettings, NoDecode, SettingsConfigDict


PROJECT_ROOT = Path(__file__).resolve().parents[2]


class Environment(str, Enum):
    development = "development"
    testing = "testing"
    production = "production"


class RepositoryBackend(str, Enum):
    """Selects which AgentRepository adapter the container wires up."""

    memory = "memory"
    postgres = "postgres"


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=str(PROJECT_ROOT / ".env"),
        env_file_encoding="utf-8",
        env_prefix="RISKLENS_",
        case_sensitive=False,
        extra="ignore",
        env_ignore_empty=True,
    )

    environment: Environment = Environment.development

    app_name: str = "RiskLens Backend"
    app_version: str = "0.1.0"
    build_time: str = "unknown"

    host: str = "127.0.0.1"
    port: int = Field(default=8000, ge=1, le=65535)

    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"] = "INFO"
    log_json: bool = True

    allowed_hosts: Annotated[list[str], NoDecode] = Field(default_factory=lambda: ["*"])
    cors_origins: Annotated[list[str], NoDecode] = Field(
        default_factory=lambda: [
            "http://127.0.0.1:8080",
            "http://localhost:8080",
        ]
    )

    request_id_header: str = "X-Request-ID"
    gzip_min_size: int = Field(default=1024, ge=0)

    repository_backend: RepositoryBackend = RepositoryBackend.postgres

    database_url: str = "postgresql+psycopg://risklens:risklens@127.0.0.1:5432/risklens"
    database_pool_size: int = Field(default=5, ge=1, le=100)
    database_max_overflow: int = Field(default=10, ge=0, le=100)
    database_pool_timeout: int = Field(default=30, ge=1, le=300)
    database_pool_recycle: int = Field(default=1800, ge=-1)
    database_echo: bool = False
    database_connect_timeout: int = Field(default=10, ge=1, le=120)

    @field_validator("allowed_hosts", "cors_origins", mode="before")
    @classmethod
    def _split_csv(cls, value: object) -> object:
        if isinstance(value, str):
            items = [part.strip() for part in value.split(",") if part.strip()]
            return items
        return value

    @field_validator("log_level", mode="before")
    @classmethod
    def _upper_level(cls, value: object) -> object:
        if isinstance(value, str):
            return value.strip().upper()
        return value

    @field_validator("database_url")
    @classmethod
    def _require_supported_driver(cls, value: str) -> str:
        value = value.strip()
        if not value:
            raise ValueError("database_url must not be empty")
        if not value.startswith(("postgresql+psycopg://", "postgresql://", "sqlite://")):
            raise ValueError(
                "database_url must use postgresql+psycopg://, postgresql://, or sqlite://"
            )
        return value

    @property
    def is_production(self) -> bool:
        return self.environment is Environment.production

    @property
    def is_testing(self) -> bool:
        return self.environment is Environment.testing

    @property
    def uses_postgres(self) -> bool:
        return self.repository_backend is RepositoryBackend.postgres


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()

from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Iterator

import pytest
from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from app.config import Environment, RepositoryBackend, Settings, get_settings  # noqa: E402
from app.infrastructure.postgres import Database, PostgresAgentRepository  # noqa: E402
from app.main import create_app  # noqa: E402

DEFAULT_TEST_DATABASE_URL = (
    "postgresql+psycopg://risklens:risklens@127.0.0.1:5432/risklens_test"
)


@pytest.fixture(autouse=True)
def _clear_settings_cache() -> Iterator[None]:
    get_settings.cache_clear()
    yield
    get_settings.cache_clear()


@pytest.fixture
def settings() -> Settings:
    """Baseline settings for tests that do not need a database.

    Pinned to the memory backend so the Sprint 1/2 suites keep running on
    machines without PostgreSQL.
    """
    return Settings(
        environment=Environment.testing,
        app_name="RiskLens Backend",
        app_version="0.1.0",
        build_time="2026-08-14T00:00:00Z",
        log_json=True,
        log_level="INFO",
        allowed_hosts=["*"],
        cors_origins=[],
        request_id_header="X-Request-ID",
        repository_backend=RepositoryBackend.memory,
    )


@pytest.fixture
def client(settings: Settings) -> Iterator[TestClient]:
    app = create_app(settings)
    with TestClient(app) as test_client:
        yield test_client


# --------------------------------------------------------------------------
# PostgreSQL fixtures (Sprint 3)
# --------------------------------------------------------------------------


def _ensure_test_database(url: str) -> None:
    """Create the test database if it does not exist yet."""
    from sqlalchemy import create_engine, text
    from sqlalchemy.engine import make_url

    target = make_url(url)
    admin = target.set(database="postgres")
    engine = create_engine(
        admin,
        isolation_level="AUTOCOMMIT",
        future=True,
        connect_args={"connect_timeout": 5},
    )
    try:
        with engine.connect() as connection:
            exists = connection.execute(
                text("SELECT 1 FROM pg_database WHERE datname = :name"),
                {"name": target.database},
            ).first()
            if exists is None:
                connection.execute(text(f'CREATE DATABASE "{target.database}"'))
    finally:
        engine.dispose()


@pytest.fixture(scope="session")
def postgres_url() -> str:
    """Reachable test-database URL, or skip the whole PostgreSQL suite."""
    url = os.getenv("RISKLENS_TEST_DATABASE_URL", DEFAULT_TEST_DATABASE_URL)
    from sqlalchemy import create_engine, text
    from sqlalchemy.exc import SQLAlchemyError

    try:
        _ensure_test_database(url)
        engine = create_engine(url, future=True, connect_args={"connect_timeout": 5})
        try:
            with engine.connect() as connection:
                connection.execute(text("SELECT 1"))
        finally:
            engine.dispose()
    except SQLAlchemyError as exc:
        pytest.skip(f"PostgreSQL is not available at {url}: {exc}")
    return url


@pytest.fixture(scope="session")
def migrated_postgres(postgres_url: str) -> Iterator[str]:
    """Build the schema by running Alembic, exactly as production does."""
    from alembic import command
    from alembic.config import Config

    config = Config(str(ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(ROOT / "app/infrastructure/postgres/migrations"))
    config.set_main_option("sqlalchemy.url", postgres_url)

    command.downgrade(config, "base")
    command.upgrade(config, "head")
    yield postgres_url


@pytest.fixture
def pg_settings(settings: Settings, migrated_postgres: str) -> Settings:
    return settings.model_copy(
        update={
            "repository_backend": RepositoryBackend.postgres,
            "database_url": migrated_postgres,
        }
    )


@pytest.fixture
def pg_database(pg_settings: Settings) -> Iterator[Database]:
    from sqlalchemy import text

    database = Database.from_settings(pg_settings)
    with database.engine.begin() as connection:
        connection.execute(
            text(
                "TRUNCATE TABLE application_activity, computer_sessions,"
                " agent_tags, agent_policy, agent_status,"
                " software_inventory, user_inventory, agent_heartbeats,"
                " device_inventory, agents CASCADE"
            )
        )
    yield database
    database.dispose()


@pytest.fixture
def pg_repository(pg_database: Database) -> PostgresAgentRepository:
    return PostgresAgentRepository(pg_database.session_factory)


@pytest.fixture
def pg_client(pg_settings: Settings, pg_database: Database) -> Iterator[TestClient]:
    """TestClient backed by a freshly truncated PostgreSQL database."""
    app = create_app(pg_settings)
    with TestClient(app) as test_client:
        yield test_client

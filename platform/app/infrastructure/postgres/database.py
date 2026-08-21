from __future__ import annotations

from dataclasses import dataclass

from sqlalchemy import Engine, create_engine, text
from sqlalchemy.orm import Session, sessionmaker
from sqlalchemy.pool import StaticPool

from app.config import Settings
from app.logging.setup import get_logger

_logger = get_logger("risklens.database")


def build_engine(settings: Settings) -> Engine:
    """Create the SQLAlchemy engine from configuration.

    Pooling options only apply to server-backed databases. SQLite (used by a
    few offline schema tests) gets a StaticPool so an in-memory database
    survives across connections.
    """
    url = settings.database_url

    if url.startswith("sqlite"):
        connect_args = {"check_same_thread": False}
        return create_engine(
            url,
            echo=settings.database_echo,
            future=True,
            poolclass=StaticPool,
            connect_args=connect_args,
        )

    return create_engine(
        url,
        echo=settings.database_echo,
        future=True,
        pool_pre_ping=True,
        pool_size=settings.database_pool_size,
        max_overflow=settings.database_max_overflow,
        pool_timeout=settings.database_pool_timeout,
        pool_recycle=settings.database_pool_recycle,
        connect_args={"connect_timeout": settings.database_connect_timeout},
    )


def build_session_factory(engine: Engine) -> sessionmaker[Session]:
    return sessionmaker(
        bind=engine,
        class_=Session,
        expire_on_commit=False,
        autoflush=False,
        future=True,
    )


@dataclass(slots=True)
class Database:
    """Owns the engine + session factory for the process lifetime.

    Created once during application startup and disposed during shutdown.
    Repositories receive the session factory, not the engine, so they never
    manage connection lifecycle themselves.
    """

    engine: Engine
    session_factory: sessionmaker[Session]

    @classmethod
    def from_settings(cls, settings: Settings) -> "Database":
        engine = build_engine(settings)
        return cls(engine=engine, session_factory=build_session_factory(engine))

    def check_connection(self) -> None:
        """Fail fast if the database is unreachable.

        Raises the underlying SQLAlchemy error so startup aborts with a
        message the operator can act on.
        """
        with self.engine.connect() as connection:
            connection.execute(text("SELECT 1"))

    def dispose(self) -> None:
        self.engine.dispose()
        _logger.info("database connection pool disposed")

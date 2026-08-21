from __future__ import annotations

from uuid import UUID

from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session, sessionmaker

from app.domain.agent import Agent, DuplicateAgentError
from app.infrastructure.postgres.models import AgentRow
from app.repositories.agent_repository import AgentRepository


class PostgresAgentRepository(AgentRepository):
    """PostgreSQL adapter for the AgentRepository port.

    Each method opens a short-lived session so the repository is safe to
    share across request threads. Uniqueness is enforced by the database
    primary key rather than a read-then-write check, which removes the
    check/insert race entirely.
    """

    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def add(self, agent: Agent) -> None:
        row = AgentRow.from_domain(agent)
        try:
            with self._session_factory() as session:
                session.add(row)
                session.commit()
        except IntegrityError as exc:
            if _is_duplicate_primary_key(exc):
                raise DuplicateAgentError(agent.agent_id) from exc
            raise

    def get(self, agent_id: UUID) -> Agent | None:
        with self._session_factory() as session:
            row = session.get(AgentRow, agent_id)
            return row.to_domain() if row is not None else None

    def exists(self, agent_id: UUID) -> bool:
        with self._session_factory() as session:
            stmt = select(AgentRow.agent_id).where(AgentRow.agent_id == agent_id)
            return session.execute(stmt).first() is not None

    def list(self) -> list[Agent]:
        with self._session_factory() as session:
            stmt = select(AgentRow).order_by(AgentRow.registered_at, AgentRow.agent_id)
            return [row.to_domain() for row in session.execute(stmt).scalars()]


def _is_duplicate_primary_key(exc: IntegrityError) -> bool:
    """Detect a unique/PK violation across psycopg and sqlite drivers."""
    sqlstate = getattr(getattr(exc, "orig", None), "sqlstate", None)
    if sqlstate == "23505":  # PostgreSQL unique_violation
        return True
    message = str(getattr(exc, "orig", exc)).lower()
    return "unique constraint" in message or "duplicate key" in message

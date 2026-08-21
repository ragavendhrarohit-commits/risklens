from __future__ import annotations

from uuid import UUID

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from app.domain.heartbeat import Heartbeat
from app.infrastructure.postgres.models import AgentHeartbeatRow
from app.repositories.heartbeat_repository import HeartbeatRepository


class PostgresHeartbeatRepository(HeartbeatRepository):
    """PostgreSQL adapter that appends heartbeat history rows."""

    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def append(self, heartbeat: Heartbeat) -> Heartbeat:
        row = AgentHeartbeatRow.from_domain(heartbeat)
        with self._session_factory() as session:
            session.add(row)
            session.commit()
            session.refresh(row)
            return row.to_domain()

    def get_latest(self, agent_id: UUID) -> Heartbeat | None:
        with self._session_factory() as session:
            stmt = (
                select(AgentHeartbeatRow)
                .where(AgentHeartbeatRow.agent_id == agent_id)
                .order_by(
                    AgentHeartbeatRow.heartbeat_time.desc(),
                    AgentHeartbeatRow.id.desc(),
                )
                .limit(1)
            )
            row = session.execute(stmt).scalar_one_or_none()
            return row.to_domain() if row is not None else None

    def get_latest_map(self, agent_ids: list[UUID]) -> dict[UUID, Heartbeat]:
        if not agent_ids:
            return {}
        with self._session_factory() as session:
            result: dict[UUID, Heartbeat] = {}
            for agent_id in agent_ids:
                stmt = (
                    select(AgentHeartbeatRow)
                    .where(AgentHeartbeatRow.agent_id == agent_id)
                    .order_by(
                        AgentHeartbeatRow.heartbeat_time.desc(),
                        AgentHeartbeatRow.id.desc(),
                    )
                    .limit(1)
                )
                row = session.execute(stmt).scalar_one_or_none()
                if row is not None:
                    result[agent_id] = row.to_domain()
            return result

    def list_for_agent(self, agent_id: UUID) -> list[Heartbeat]:
        with self._session_factory() as session:
            stmt = (
                select(AgentHeartbeatRow)
                .where(AgentHeartbeatRow.agent_id == agent_id)
                .order_by(AgentHeartbeatRow.received_at.asc(), AgentHeartbeatRow.id.asc())
            )
            return [row.to_domain() for row in session.execute(stmt).scalars()]

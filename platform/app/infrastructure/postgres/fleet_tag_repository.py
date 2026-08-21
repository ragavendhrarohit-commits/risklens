from __future__ import annotations

from uuid import UUID

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from app.infrastructure.postgres.models import AgentTagRow
from app.repositories.fleet_repository import AgentTagRepository


class PostgresAgentTagRepository(AgentTagRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def list_for_agent(self, agent_id: UUID) -> list[str]:
        with self._session_factory() as session:
            stmt = (
                select(AgentTagRow.tag)
                .where(AgentTagRow.agent_id == agent_id)
                .order_by(AgentTagRow.tag)
            )
            return [tag for tag in session.execute(stmt).scalars()]

    def list_map(self, agent_ids: list[UUID]) -> dict[UUID, list[str]]:
        result: dict[UUID, list[str]] = {agent_id: [] for agent_id in agent_ids}
        if not agent_ids:
            return result
        with self._session_factory() as session:
            stmt = (
                select(AgentTagRow.agent_id, AgentTagRow.tag)
                .where(AgentTagRow.agent_id.in_(agent_ids))
                .order_by(AgentTagRow.tag)
            )
            for agent_id, tag in session.execute(stmt).all():
                result.setdefault(agent_id, []).append(tag)
        return result

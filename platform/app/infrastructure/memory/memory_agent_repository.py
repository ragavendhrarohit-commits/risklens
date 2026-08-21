from __future__ import annotations

from threading import RLock
from uuid import UUID

from app.domain.agent import Agent, DuplicateAgentError
from app.repositories.agent_repository import AgentRepository


class MemoryAgentRepository(AgentRepository):
    """In-memory adapter. Volatile; state disappears on process restart.

    Thread-safe: guarded by an RLock so that concurrent registrations cannot
    slip a duplicate past the exists->add gap.
    """

    def __init__(self) -> None:
        self._agents: dict[UUID, Agent] = {}
        self._lock = RLock()

    def add(self, agent: Agent) -> None:
        with self._lock:
            if agent.agent_id in self._agents:
                raise DuplicateAgentError(agent.agent_id)
            self._agents[agent.agent_id] = agent

    def get(self, agent_id: UUID) -> Agent | None:
        with self._lock:
            return self._agents.get(agent_id)

    def exists(self, agent_id: UUID) -> bool:
        with self._lock:
            return agent_id in self._agents

    def list(self) -> list[Agent]:
        with self._lock:
            return list(self._agents.values())

    def clear(self) -> None:
        """Testing convenience. Not part of the AgentRepository contract."""
        with self._lock:
            self._agents.clear()

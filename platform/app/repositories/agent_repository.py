from __future__ import annotations

from abc import ABC, abstractmethod
from uuid import UUID

from app.domain.agent import Agent


class AgentRepository(ABC):
    """Persistence port for Agent aggregates.

    Adapters live under `app.infrastructure`. Sprint 2 ships only
    `MemoryAgentRepository`. Later sprints add a durable adapter without
    touching the service layer.
    """

    @abstractmethod
    def add(self, agent: Agent) -> None:
        """Persist a new agent. Must raise DuplicateAgentError if the id exists."""

    @abstractmethod
    def get(self, agent_id: UUID) -> Agent | None:
        """Return the agent with this id, or None."""

    @abstractmethod
    def exists(self, agent_id: UUID) -> bool:
        """Return True if an agent with this id has been registered."""

    @abstractmethod
    def list(self) -> list[Agent]:
        """Return all registered agents (read-only snapshot)."""

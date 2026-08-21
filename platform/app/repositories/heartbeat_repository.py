from __future__ import annotations

from abc import ABC, abstractmethod
from uuid import UUID

from app.domain.heartbeat import Heartbeat


class HeartbeatRepository(ABC):
    """Append-only persistence port for agent heartbeats."""

    @abstractmethod
    def append(self, heartbeat: Heartbeat) -> Heartbeat:
        """Insert a new heartbeat row. Never overwrites history."""

    @abstractmethod
    def get_latest(self, agent_id: UUID) -> Heartbeat | None:
        """Return the most recent heartbeat for an agent, if any."""

    @abstractmethod
    def get_latest_map(self, agent_ids: list[UUID]) -> dict[UUID, Heartbeat]:
        """Return the latest heartbeat keyed by agent_id for the given set."""

    @abstractmethod
    def list_for_agent(self, agent_id: UUID) -> list[Heartbeat]:
        """Return all heartbeats for an agent (oldest first)."""

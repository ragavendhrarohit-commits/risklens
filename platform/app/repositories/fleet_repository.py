from __future__ import annotations

from abc import ABC, abstractmethod
from datetime import datetime
from uuid import UUID

from app.domain.fleet import AgentStatusSnapshot, Policy, PolicyAssignment


class AgentStatusRepository(ABC):
    """Current-state timestamps for Fleet. Does not store inventory rows."""

    @abstractmethod
    def ensure(self, agent_id: UUID) -> None:
        """Insert an empty status row if one does not exist."""

    @abstractmethod
    def record_seen(self, agent_id: UUID, last_seen: datetime, received_at: datetime) -> None:
        """Update last_seen and last_communication_at from a heartbeat."""

    @abstractmethod
    def record_inventory(self, agent_id: UUID, received_at: datetime) -> None:
        """Update last_inventory_at and last_communication_at from device inventory."""

    @abstractmethod
    def get(self, agent_id: UUID) -> AgentStatusSnapshot | None:
        """Return the status snapshot for one agent."""

    @abstractmethod
    def list_map(self) -> dict[UUID, AgentStatusSnapshot]:
        """Return all status snapshots keyed by agent_id."""


class PolicyRepository(ABC):
    """Policy catalog and per-agent assignment."""

    @abstractmethod
    def get_default_policy_id(self) -> UUID:
        """Return the Default Windows Policy id."""

    @abstractmethod
    def get_policy(self, policy_id: UUID) -> Policy | None:
        """Return a policy by id."""

    @abstractmethod
    def get_default_policy(self) -> Policy | None:
        """Return Default Windows Policy."""

    @abstractmethod
    def assign_default(self, agent_id: UUID, assigned_at: datetime) -> None:
        """Assign Default Windows Policy if the agent has no assignment."""

    @abstractmethod
    def get_assignment(self, agent_id: UUID) -> PolicyAssignment | None:
        """Return the assigned policy for an agent."""

    @abstractmethod
    def list_assignments(self) -> dict[UUID, PolicyAssignment]:
        """Return assignments keyed by agent_id."""


class AgentTagRepository(ABC):
    """Read-only tag listing. Sprint 13 has no write API."""

    @abstractmethod
    def list_for_agent(self, agent_id: UUID) -> list[str]:
        """Return tags for one agent, sorted."""

    @abstractmethod
    def list_map(self, agent_ids: list[UUID]) -> dict[UUID, list[str]]:
        """Return tags keyed by agent_id."""

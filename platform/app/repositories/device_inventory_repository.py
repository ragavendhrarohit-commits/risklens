from __future__ import annotations

from abc import ABC, abstractmethod
from uuid import UUID

from app.domain.device import DeviceInventory


class DeviceInventoryRepository(ABC):
    """Persistence port for the latest device inventory per agent.

    `upsert` replaces any previous row for the same AgentID so the table
    always holds the most recent snapshot.
    """

    @abstractmethod
    def upsert(self, inventory: DeviceInventory) -> tuple[DeviceInventory, bool]:
        """Store inventory. Returns (stored, created) where created is True on insert."""

    @abstractmethod
    def get(self, agent_id: UUID) -> DeviceInventory | None:
        """Return the latest inventory for this agent, or None."""

    @abstractmethod
    def list(self) -> list[DeviceInventory]:
        """Return all stored inventory rows."""

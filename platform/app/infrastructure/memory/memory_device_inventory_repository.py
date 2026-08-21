from __future__ import annotations

from threading import RLock
from uuid import UUID

from app.domain.device import DeviceInventory
from app.repositories.device_inventory_repository import DeviceInventoryRepository


class MemoryDeviceInventoryRepository(DeviceInventoryRepository):
    """In-memory adapter storing the latest inventory per AgentID."""

    def __init__(self) -> None:
        self._rows: dict[UUID, DeviceInventory] = {}
        self._lock = RLock()

    def upsert(self, inventory: DeviceInventory) -> tuple[DeviceInventory, bool]:
        with self._lock:
            created = inventory.agent_id not in self._rows
            self._rows[inventory.agent_id] = inventory
            return inventory, created

    def get(self, agent_id: UUID) -> DeviceInventory | None:
        with self._lock:
            return self._rows.get(agent_id)

    def list(self) -> list[DeviceInventory]:
        with self._lock:
            return list(self._rows.values())

    def clear(self) -> None:
        with self._lock:
            self._rows.clear()

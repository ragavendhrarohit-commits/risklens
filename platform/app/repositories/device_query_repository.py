from __future__ import annotations

from abc import ABC, abstractmethod
from uuid import UUID

from app.domain.device import DevicePage, DeviceRecord, ListDevicesQuery


class DeviceQueryRepository(ABC):
    """Read-only persistence port for device inventory queries.

    Does not mutate inventory. Upload remains owned by DeviceInventoryRepository.
    """

    @abstractmethod
    def get(self, agent_id: UUID) -> DeviceRecord | None:
        """Return the device record for this AgentID, or None."""

    @abstractmethod
    def list(self, query: ListDevicesQuery) -> DevicePage:
        """Return a filtered, sorted, paginated page of device records."""

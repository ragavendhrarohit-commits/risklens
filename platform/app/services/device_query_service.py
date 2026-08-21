from __future__ import annotations

from uuid import UUID

from app.domain.device import (
    DeviceNotFoundError,
    DevicePage,
    DeviceRecord,
    ListDevicesQuery,
)
from app.logging.setup import get_logger
from app.repositories.device_query_repository import DeviceQueryRepository

_logger = get_logger("risklens.device_query")


class DeviceQueryService:
    """Read API for the frontend Devices page.

    Sources inventory from DeviceQueryRepository only. Does not collect or
    upload inventory.
    """

    def __init__(self, repository: DeviceQueryRepository) -> None:
        self._repository = repository

    def get(self, agent_id: UUID) -> DeviceRecord:
        record = self._repository.get(agent_id)
        if record is None:
            raise DeviceNotFoundError(agent_id)
        return record

    def list(self, query: ListDevicesQuery) -> DevicePage:
        page = self._repository.list(query)
        _logger.debug(
            "device inventory listed",
            extra={
                "total": page.total,
                "limit": page.limit,
                "offset": page.offset,
                "returned": len(page.items),
            },
        )
        return page

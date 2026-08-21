from __future__ import annotations

from datetime import datetime, timezone

from app.domain.device import (
    DeviceInventory,
    UnknownAgentError,
    UpsertDeviceInventoryCommand,
)
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.device_inventory_repository import DeviceInventoryRepository

_logger = get_logger("risklens.device")


class DeviceInventoryService:
    """Accepts device inventory uploads from registered agents.

    Uniqueness of AgentID registration is owned by AgentRepository. This
    service only stores the latest inventory snapshot per known agent.
    """

    def __init__(
        self,
        agents: AgentRepository,
        inventory: DeviceInventoryRepository,
        *,
        clock=None,
        projection=None,
    ) -> None:
        self._agents = agents
        self._inventory = inventory
        self._clock = clock or _default_clock
        self._projection = projection

    def upsert(self, command: UpsertDeviceInventoryCommand) -> tuple[DeviceInventory, bool]:
        if not self._agents.exists(command.agent_id):
            raise UnknownAgentError(command.agent_id)

        inventory = DeviceInventory(
            agent_id=command.agent_id,
            install_id=command.install_id.strip(),
            machine_name=command.machine_name.strip(),
            hostname=command.hostname.strip(),
            operating_system=command.operating_system.strip(),
            os_version=command.os_version.strip(),
            windows_build=command.windows_build.strip(),
            architecture=command.architecture.strip(),
            cpu_model=command.cpu_model.strip(),
            cpu_cores=command.cpu_cores,
            memory_bytes=command.memory_bytes,
            disk_total_bytes=command.disk_total_bytes,
            disk_free_bytes=command.disk_free_bytes,
            manufacturer=command.manufacturer.strip(),
            model=command.model.strip(),
            serial_number=command.serial_number.strip(),
            boot_time=command.boot_time,
            current_user=command.current_user.strip(),
            agent_version=command.agent_version.strip(),
            collected_at=command.collected_at,
            received_at=self._clock(),
        )
        stored, created = self._inventory.upsert(inventory)
        if self._projection is not None:
            self._projection.on_inventory(stored.agent_id, stored.received_at)
        _logger.info(
            "device inventory stored",
            extra={
                "agent_id": str(stored.agent_id),
                "machine_name": stored.machine_name,
                "hostname": stored.hostname,
                "was_created": created,
            },
        )
        return stored, created


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)

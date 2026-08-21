from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True, slots=True)
class DeviceInventory:
    """Latest device inventory snapshot for a registered agent."""

    agent_id: UUID
    install_id: str
    machine_name: str
    hostname: str
    operating_system: str
    os_version: str
    windows_build: str
    architecture: str
    cpu_model: str
    cpu_cores: int
    memory_bytes: int
    disk_total_bytes: int
    disk_free_bytes: int
    manufacturer: str
    model: str
    serial_number: str
    boot_time: datetime | None
    current_user: str
    agent_version: str
    collected_at: datetime
    received_at: datetime

    def __post_init__(self) -> None:
        if not isinstance(self.agent_id, UUID):
            raise TypeError("agent_id must be a UUID")
        for name in ("install_id", "machine_name", "hostname", "operating_system", "agent_version"):
            value = getattr(self, name)
            if not isinstance(value, str) or not value.strip():
                raise ValueError(f"{name} must be a non-empty string")
        if self.cpu_cores < 0:
            raise ValueError("cpu_cores must be >= 0")
        for name in ("memory_bytes", "disk_total_bytes", "disk_free_bytes"):
            if getattr(self, name) < 0:
                raise ValueError(f"{name} must be >= 0")
        if not isinstance(self.collected_at, datetime):
            raise TypeError("collected_at must be a datetime")
        if not isinstance(self.received_at, datetime):
            raise TypeError("received_at must be a datetime")

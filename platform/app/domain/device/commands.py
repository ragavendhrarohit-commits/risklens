from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True, slots=True)
class UpsertDeviceInventoryCommand:
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

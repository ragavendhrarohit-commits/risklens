from __future__ import annotations

from datetime import datetime
from typing import Annotated
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

from app.domain.device import UpsertDeviceInventoryCommand

NonEmptyStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
BoundedStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=256)]
VersionStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=64)]
BuildStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=64)]
OSVersionStr = Annotated[str, StringConstraints(strip_whitespace=True, max_length=128)]


class DeviceInventoryRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID = Field(description="Registered agent identifier (UUID).")
    install_id: NonEmptyStr
    machine_name: NonEmptyStr
    hostname: NonEmptyStr
    operating_system: NonEmptyStr
    os_version: OSVersionStr = ""
    windows_build: BuildStr = ""
    architecture: BuildStr = ""
    cpu_model: BoundedStr = ""
    cpu_cores: int = Field(default=0, ge=0)
    memory_bytes: int = Field(default=0, ge=0)
    disk_total_bytes: int = Field(default=0, ge=0)
    disk_free_bytes: int = Field(default=0, ge=0)
    manufacturer: BoundedStr = ""
    model: BoundedStr = ""
    serial_number: BoundedStr = ""
    boot_time: datetime | None = None
    current_user: BoundedStr = ""
    agent_version: VersionStr
    collected_at: datetime

    def to_command(self) -> UpsertDeviceInventoryCommand:
        return UpsertDeviceInventoryCommand(
            agent_id=self.agent_id,
            install_id=self.install_id,
            machine_name=self.machine_name,
            hostname=self.hostname,
            operating_system=self.operating_system,
            os_version=self.os_version,
            windows_build=self.windows_build,
            architecture=self.architecture,
            cpu_model=self.cpu_model,
            cpu_cores=self.cpu_cores,
            memory_bytes=self.memory_bytes,
            disk_total_bytes=self.disk_total_bytes,
            disk_free_bytes=self.disk_free_bytes,
            manufacturer=self.manufacturer,
            model=self.model,
            serial_number=self.serial_number,
            boot_time=self.boot_time,
            current_user=self.current_user,
            agent_version=self.agent_version,
            collected_at=self.collected_at,
        )


class DeviceInventoryResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    created: bool = Field(description="True when this was the first inventory for the agent.")
    inventory_status: str = "stored"
    server_version: str
    server_time: datetime
    received_at: datetime


class DeviceItemResponse(BaseModel):
    """Single device row for the frontend Devices page."""

    agent_id: UUID
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
    current_user: str
    agent_version: str
    collected_at: datetime
    received_at: datetime
    registration_status: str
    status: str = Field(description="Presence: online | stale | offline")
    last_seen: datetime | None = None
    heartbeat_age_seconds: int | None = None


class DeviceListResponse(BaseModel):
    items: list[DeviceItemResponse]
    total: int
    limit: int
    offset: int

from __future__ import annotations

from datetime import datetime
from typing import Annotated, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, StringConstraints

from app.domain.agent import RegisterAgentCommand

NonEmptyStr = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=256)]
OptionalStr = Annotated[str | None, StringConstraints(strip_whitespace=True, max_length=256)]


class AgentRegistrationRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID = Field(description="Agent identifier (UUID v4).")
    install_id: NonEmptyStr = Field(description="Installation identifier assigned by the agent.")
    agent_version: NonEmptyStr = Field(description="Agent build version, e.g. 0.1.0.")
    machine_name: NonEmptyStr = Field(description="Reported hostname of the endpoint.")
    operating_system: NonEmptyStr = Field(description="Reported operating system string.")
    display_name: OptionalStr = Field(default=None, description="Optional friendly label.")
    created_at: datetime = Field(description="Local time when the agent identity was created.")

    def to_command(self) -> RegisterAgentCommand:
        return RegisterAgentCommand(
            agent_id=self.agent_id,
            install_id=self.install_id,
            agent_version=self.agent_version,
            machine_name=self.machine_name,
            operating_system=self.operating_system,
            display_name=self.display_name,
            created_at=self.created_at,
        )


class AgentRegistrationResponse(BaseModel):
    accepted: bool = Field(description="True when the agent was recorded by the backend.")
    agent_id: UUID
    registration_status: Literal["registered"] = "registered"
    server_version: str = Field(description="Backend build version.")
    server_time: datetime = Field(description="UTC server timestamp at response time.")


class HeartbeatMemoryPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    total: int = Field(ge=0)
    used: int = Field(ge=0)
    free: int = Field(ge=0)
    percent: float = Field(ge=0, le=100)


class HeartbeatDiskPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=32)]
    total: int = Field(ge=0)
    used: int = Field(ge=0)
    free: int = Field(ge=0)
    percent: float = Field(ge=0, le=100)


class HeartbeatHealthPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    cpu_percent: float = Field(ge=0, le=100)
    memory: HeartbeatMemoryPayload
    disks: list[HeartbeatDiskPayload] = Field(default_factory=list)
    cpu_model: OptionalStr = None
    logical_cpus: int | None = Field(default=None, ge=1)
    physical_cores: int | None = Field(default=None, ge=1)


class AgentHeartbeatRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID
    install_id: NonEmptyStr
    timestamp: datetime
    public_ip: OptionalStr = Field(default=None, description="Endpoint public IPv4/IPv6 address.")
    private_ip: OptionalStr = Field(default=None, description="Endpoint private LAN address.")
    hostname: OptionalStr = Field(default=None, description="Reported hostname.")
    health: HeartbeatHealthPayload | None = Field(
        default=None,
        description="Live CPU, memory, and disk metrics collected with the heartbeat.",
    )

    def to_command(self) -> "RecordHeartbeatCommand":
        from app.domain.heartbeat import (
            HeartbeatDiskMetrics,
            HeartbeatHealthMetrics,
            HeartbeatMemoryMetrics,
            RecordHeartbeatCommand,
        )

        health = None
        if self.health is not None:
            health = HeartbeatHealthMetrics(
                cpu_percent=self.health.cpu_percent,
                cpu_model=self.health.cpu_model,
                logical_cpus=self.health.logical_cpus,
                physical_cores=self.health.physical_cores,
                memory=HeartbeatMemoryMetrics(
                    total=self.health.memory.total,
                    used=self.health.memory.used,
                    free=self.health.memory.free,
                    percent=self.health.memory.percent,
                ),
                disks=tuple(
                    HeartbeatDiskMetrics(
                        name=disk.name,
                        total=disk.total,
                        used=disk.used,
                        free=disk.free,
                        percent=disk.percent,
                    )
                    for disk in self.health.disks
                ),
            )
        return RecordHeartbeatCommand(
            agent_id=self.agent_id,
            install_id=self.install_id.strip(),
            timestamp=self.timestamp,
            public_ip=self.public_ip,
            private_ip=self.private_ip,
            hostname=self.hostname,
            health=health,
        )


class AgentHeartbeatResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    heartbeat_status: Literal["ok"] = "ok"
    server_version: str
    server_time: datetime
    received_at: datetime

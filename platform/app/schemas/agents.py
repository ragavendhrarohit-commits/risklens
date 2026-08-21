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


class AgentHeartbeatRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    agent_id: UUID
    install_id: NonEmptyStr
    timestamp: datetime

    def to_command(self) -> "RecordHeartbeatCommand":
        from app.domain.heartbeat import RecordHeartbeatCommand

        return RecordHeartbeatCommand(
            agent_id=self.agent_id,
            install_id=self.install_id.strip(),
            timestamp=self.timestamp,
        )


class AgentHeartbeatResponse(BaseModel):
    accepted: bool = True
    agent_id: UUID
    heartbeat_status: Literal["ok"] = "ok"
    server_version: str
    server_time: datetime
    received_at: datetime

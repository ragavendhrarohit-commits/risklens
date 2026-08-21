from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from uuid import UUID


class AgentStatus(str, Enum):
    registered = "registered"


@dataclass(frozen=True, slots=True)
class Agent:
    """Domain entity. Constructed only from validated inputs."""

    agent_id: UUID
    install_id: str
    agent_version: str
    machine_name: str
    operating_system: str
    display_name: str | None
    created_at: datetime
    registered_at: datetime
    status: AgentStatus = AgentStatus.registered

    def __post_init__(self) -> None:
        if not isinstance(self.agent_id, UUID):
            raise TypeError("agent_id must be a UUID")
        for name in ("install_id", "agent_version", "machine_name", "operating_system"):
            value = getattr(self, name)
            if not isinstance(value, str) or not value.strip():
                raise ValueError(f"{name} must be a non-empty string")
        if not isinstance(self.created_at, datetime):
            raise TypeError("created_at must be a datetime")
        if not isinstance(self.registered_at, datetime):
            raise TypeError("registered_at must be a datetime")

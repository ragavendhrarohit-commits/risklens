from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True, slots=True)
class RegisterAgentCommand:
    """Framework-independent input to AgentRegistrationService.register.

    The API layer builds this from a validated request schema; tests build it
    directly. The service does not depend on Pydantic or FastAPI.
    """

    agent_id: UUID
    install_id: str
    agent_version: str
    machine_name: str
    operating_system: str
    display_name: str | None
    created_at: datetime

from __future__ import annotations

from uuid import UUID

from app.core.errors import AppError


class DuplicateAgentError(AppError):
    """Raised when an agent tries to register with an AgentID that already exists."""

    status_code = 409
    code = "duplicate_agent"

    def __init__(self, agent_id: UUID) -> None:
        super().__init__(
            f"Agent {agent_id} is already registered",
            details={"agent_id": str(agent_id)},
        )
        self.agent_id = agent_id

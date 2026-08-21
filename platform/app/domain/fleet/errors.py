from __future__ import annotations

from uuid import UUID

from app.core.errors import AppError


class FleetAgentNotFoundError(AppError):
    status_code = 404
    code = "unknown_agent"

    def __init__(self, agent_id: UUID) -> None:
        super().__init__(
            f"Agent {agent_id} is not registered",
            details={"agent_id": str(agent_id)},
        )
        self.agent_id = agent_id


class InvalidFleetQueryError(AppError):
    status_code = 422
    code = "invalid_query"

    def __init__(self, message: str) -> None:
        super().__init__(message)

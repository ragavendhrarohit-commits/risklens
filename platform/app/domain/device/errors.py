from __future__ import annotations

from uuid import UUID

from app.core.errors import AppError


class UnknownAgentError(AppError):
    """Raised when inventory is uploaded for an AgentID that is not registered."""

    status_code = 404
    code = "unknown_agent"

    def __init__(self, agent_id: UUID) -> None:
        super().__init__(
            f"Agent {agent_id} is not registered",
            details={"agent_id": str(agent_id)},
        )
        self.agent_id = agent_id


class DeviceNotFoundError(AppError):
    """Raised when a device inventory row does not exist for the given AgentID."""

    status_code = 404
    code = "device_not_found"

    def __init__(self, agent_id: UUID) -> None:
        super().__init__(
            f"No device inventory found for agent {agent_id}",
            details={"agent_id": str(agent_id)},
        )
        self.agent_id = agent_id


class InvalidDeviceQueryError(AppError):
    """Raised when list query parameters fail domain validation."""

    status_code = 422
    code = "invalid_query"

    def __init__(self, message: str) -> None:
        super().__init__(message)

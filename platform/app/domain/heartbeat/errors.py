from __future__ import annotations

from uuid import UUID

from app.core.errors import AppError


class UnknownAgentError(AppError):
    """Raised when a heartbeat targets an unregistered AgentID."""

    status_code = 404
    code = "unknown_agent"

    def __init__(self, agent_id: UUID) -> None:
        super().__init__(
            f"Agent {agent_id} is not registered",
            details={"agent_id": str(agent_id)},
        )
        self.agent_id = agent_id


class AgentIDMismatchError(AppError):
    """Raised when path AgentID does not match the request body."""

    status_code = 422
    code = "agent_id_mismatch"

    def __init__(self, path_id: UUID, body_id: UUID) -> None:
        super().__init__(
            "Path agent_id does not match body agent_id",
            details={"path_agent_id": str(path_id), "body_agent_id": str(body_id)},
        )

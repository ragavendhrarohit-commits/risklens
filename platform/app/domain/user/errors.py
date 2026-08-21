from __future__ import annotations

from uuid import UUID

from app.core.errors import AppError


class UnknownAgentError(AppError):
    status_code = 404
    code = "unknown_agent"

    def __init__(self, agent_id: UUID) -> None:
        super().__init__(
            f"Agent {agent_id} is not registered",
            details={"agent_id": str(agent_id)},
        )
        self.agent_id = agent_id


class UserNotFoundError(AppError):
    status_code = 404
    code = "user_not_found"

    def __init__(self, sid: str) -> None:
        super().__init__(
            f"No user inventory found for SID {sid}",
            details={"sid": sid},
        )
        self.sid = sid


class InvalidUserQueryError(AppError):
    status_code = 422
    code = "invalid_query"

    def __init__(self, message: str) -> None:
        super().__init__(message)

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


class UnknownSessionError(AppError):
    status_code = 422
    code = "unknown_session"

    def __init__(self, session_id: UUID) -> None:
        super().__init__(
            f"Session {session_id} is not recorded",
            details={"session_id": str(session_id)},
        )
        self.session_id = session_id


class ApplicationActivityNotFoundError(AppError):
    status_code = 404
    code = "application_activity_not_found"

    def __init__(self, activity_id: str) -> None:
        super().__init__(
            f"No application activity found for {activity_id}",
            details={"id": activity_id},
        )
        self.activity_id = activity_id


class InvalidApplicationQueryError(AppError):
    status_code = 422
    code = "invalid_query"

    def __init__(self, message: str) -> None:
        super().__init__(message)

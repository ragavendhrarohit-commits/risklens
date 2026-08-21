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


class SessionNotFoundError(AppError):
    status_code = 404
    code = "session_not_found"

    def __init__(self, session_id: UUID) -> None:
        super().__init__(
            f"No session found for {session_id}",
            details={"session_id": str(session_id)},
        )
        self.session_id = session_id


class InvalidSessionQueryError(AppError):
    status_code = 422
    code = "invalid_query"

    def __init__(self, message: str) -> None:
        super().__init__(message)


class MultipleActiveSessionsError(AppError):
    status_code = 500
    code = "multiple_active_sessions"

    def __init__(self, agent_id: UUID, details: dict) -> None:
        super().__init__(
            f"FATAL: agent {agent_id} has more than one ACTIVE session after login",
            details=details,
        )
        self.agent_id = agent_id

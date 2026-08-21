from __future__ import annotations

from abc import ABC, abstractmethod
from uuid import UUID

from app.domain.sessions import ComputerSession, ListSessionsQuery, SessionPage, SessionRecord


class SessionRepository(ABC):
    @abstractmethod
    def upsert_many(self, sessions: list[ComputerSession]) -> int:
        """Upsert sessions by session_id. Returns number of rows written."""

    @abstractmethod
    def get(self, session_id: UUID) -> SessionRecord | None:
        """Return one session record."""

    @abstractmethod
    def exists(self, session_id: UUID) -> bool:
        """True when the session row exists."""

    @abstractmethod
    def list(self, query: ListSessionsQuery) -> SessionPage:
        """Filtered, sorted, paginated sessions."""

    @abstractmethod
    def activate_current(self, session: ComputerSession) -> int:
        """Make this the sole ACTIVE session for the agent.

        In one transaction:
        1. Close every other ACTIVE row for the agent at this session's logon_at
        2. Insert or update this session as ACTIVE
        3. Abort if more than one ACTIVE row still exists
        """

    @abstractmethod
    def close_active_for_agent(
        self,
        agent_id: UUID,
        *,
        logoff_at,
        received_at,
        current_session_id: UUID | None = None,
    ) -> int:
        """Close the current active session for an agent.

        Updates at most one row, keyed by session_id. If more than one
        ACTIVE row exists, the operation is refused and zero rows are written.
        """

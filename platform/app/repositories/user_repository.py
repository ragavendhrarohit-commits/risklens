from __future__ import annotations

from abc import ABC, abstractmethod

from app.domain.user import ListUsersQuery, UserAccount, UserPage, UserRecord


class UserRepository(ABC):
    """Persistence port for Windows user inventory (SID primary key)."""

    @abstractmethod
    def upsert_many(self, accounts: list[UserAccount]) -> int:
        """Upsert accounts by SID. Returns number of rows written."""

    @abstractmethod
    def get(self, sid: str) -> UserRecord | None:
        """Return one user record (with machine_name when available)."""

    @abstractmethod
    def list(self, query: ListUsersQuery) -> UserPage:
        """Filtered, sorted, paginated user inventory."""

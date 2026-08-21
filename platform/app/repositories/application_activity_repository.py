from __future__ import annotations

from abc import ABC, abstractmethod

from app.domain.applications import (
    ApplicationActivity,
    ApplicationActivityPage,
    ApplicationActivityRecord,
    ListApplicationActivityQuery,
)


class ApplicationActivityRepository(ABC):
    @abstractmethod
    def upsert_many(self, activities: list[ApplicationActivity]) -> int:
        """Upsert activity rows by id. Returns number of rows written."""

    @abstractmethod
    def get(self, activity_id: str) -> ApplicationActivityRecord | None:
        """Return one activity record."""

    @abstractmethod
    def list(self, query: ListApplicationActivityQuery) -> ApplicationActivityPage:
        """Filtered, sorted, paginated application activity."""

from __future__ import annotations

from abc import ABC, abstractmethod

from app.domain.browser import (
    BrowserActivity,
    BrowserActivityPage,
    BrowserActivityRecord,
    ListBrowserActivityQuery,
)


class BrowserActivityRepository(ABC):
    @abstractmethod
    def upsert_many(self, activities: list[BrowserActivity]) -> int:
        """Upsert browser activity rows by id. Returns number of rows written."""

    @abstractmethod
    def get(self, activity_id: str) -> BrowserActivityRecord | None:
        """Return one browser activity record."""

    @abstractmethod
    def list(self, query: ListBrowserActivityQuery) -> BrowserActivityPage:
        """Filtered, sorted, paginated browser activity."""

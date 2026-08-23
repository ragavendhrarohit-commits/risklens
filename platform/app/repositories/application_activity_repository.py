from __future__ import annotations

from abc import ABC, abstractmethod
from datetime import datetime
from uuid import UUID

from app.domain.applications import (
    ApplicationActivity,
    ApplicationActivityPage,
    ApplicationActivityRecord,
    ListApplicationActivityQuery,
)
from app.domain.productivity import ApplicationAggregate, IntervalPoint, TitleAggregate


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

    @abstractmethod
    def productivity_application_usage(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[ApplicationAggregate]:
        """SQL/in-memory rollup of application usage for one agent and period."""

    @abstractmethod
    def productivity_browser_title_usage(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[TitleAggregate]:
        """Grouped browser window titles for website rollup without loading raw rows."""

    @abstractmethod
    def productivity_intervals(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[IntervalPoint]:
        """Lightweight activity intervals for working/idle span metrics."""

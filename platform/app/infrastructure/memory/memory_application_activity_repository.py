from __future__ import annotations

from datetime import datetime
from threading import RLock
from uuid import UUID

from app.domain.applications import (
    ApplicationActivity,
    ApplicationActivityPage,
    ApplicationActivityRecord,
    ListApplicationActivityQuery,
    parse_sort,
)
from app.domain.productivity import ApplicationAggregate, IntervalPoint, TitleAggregate, interval_end
from app.repositories.agent_repository import AgentRepository
from app.repositories.application_activity_repository import ApplicationActivityRepository
from app.repositories.session_repository import SessionRepository

_BROWSER_EXECUTABLES = frozenset({"chrome.exe", "msedge.exe", "firefox.exe"})


class MemoryApplicationActivityRepository(ApplicationActivityRepository):
    def __init__(
        self,
        agents: AgentRepository | None = None,
        sessions: SessionRepository | None = None,
    ) -> None:
        self._rows: dict[str, ApplicationActivity] = {}
        self._agents = agents
        self._sessions = sessions
        self._lock = RLock()

    def upsert_many(self, activities: list[ApplicationActivity]) -> int:
        with self._lock:
            for activity in activities:
                self._rows[activity.id] = activity
            return len(activities)

    def get(self, activity_id: str) -> ApplicationActivityRecord | None:
        with self._lock:
            row = self._rows.get(activity_id)
            if row is None:
                return None
            return self._to_record(row)

    def list(self, query: ListApplicationActivityQuery) -> ApplicationActivityPage:
        with self._lock:
            rows = [self._to_record(item) for item in self._rows.values()]
        filtered = [row for row in rows if self._matches(row, query)]
        field, descending = parse_sort(query.sort)
        filtered.sort(key=lambda r: _sort_key(r, field), reverse=descending)
        total = len(filtered)
        page = filtered[query.offset : query.offset + query.limit]
        return ApplicationActivityPage(
            items=page, total=total, limit=query.limit, offset=query.offset
        )

    def _to_record(self, activity: ApplicationActivity) -> ApplicationActivityRecord:
        machine_name = ""
        username = ""
        if self._agents is not None:
            agent = self._agents.get(activity.agent_id)
            if agent is not None:
                machine_name = agent.machine_name
        if self._sessions is not None:
            session = self._sessions.get(activity.session_id)
            if session is not None:
                username = session.username
        return ApplicationActivityRecord(
            id=activity.id,
            session_id=activity.session_id,
            agent_id=activity.agent_id,
            application_name=activity.application_name,
            executable_name=activity.executable_name,
            window_title=activity.window_title,
            started_at=activity.started_at,
            stopped_at=activity.stopped_at,
            duration_seconds=activity.duration_seconds,
            collected_at=activity.collected_at,
            received_at=activity.received_at,
            username=username,
            machine_name=machine_name,
        )

    def productivity_application_usage(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[ApplicationAggregate]:
        buckets: dict[str, list[int]] = {}
        for record in self._filtered_records(agent_id, started_at_from, started_at_to):
            name = (record.application_name or "").strip() or "Unknown Application"
            current = buckets.get(name)
            if current is None:
                buckets[name] = [max(0, int(record.duration_seconds or 0)), 1]
            else:
                current[0] += max(0, int(record.duration_seconds or 0))
                current[1] += 1
        rows = [
            ApplicationAggregate(application=name, duration_seconds=data[0], launch_count=data[1])
            for name, data in buckets.items()
        ]
        rows.sort(key=lambda row: (-row.duration_seconds, row.application))
        return rows

    def productivity_browser_title_usage(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[TitleAggregate]:
        buckets: dict[tuple[str, str], list[int]] = {}
        for record in self._filtered_records(agent_id, started_at_from, started_at_to):
            executable = (record.executable_name or "").strip().casefold()
            if executable not in _BROWSER_EXECUTABLES:
                continue
            key = (record.window_title or "", record.executable_name or "")
            current = buckets.get(key)
            if current is None:
                buckets[key] = [max(0, int(record.duration_seconds or 0)), 1]
            else:
                current[0] += max(0, int(record.duration_seconds or 0))
                current[1] += 1
        return [
            TitleAggregate(
                window_title=title,
                executable_name=executable,
                duration_seconds=data[0],
                visit_count=data[1],
            )
            for (title, executable), data in buckets.items()
        ]

    def productivity_intervals(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[IntervalPoint]:
        rows = self._filtered_records(agent_id, started_at_from, started_at_to)
        rows.sort(key=lambda row: (row.started_at, row.id))
        return [
            IntervalPoint(
                started_at=row.started_at,
                ended_at=interval_end(row.started_at, row.stopped_at, row.duration_seconds),
            )
            for row in rows
        ]

    def _filtered_records(
        self,
        agent_id: UUID,
        started_at_from: datetime | None,
        started_at_to: datetime | None,
    ) -> list[ApplicationActivityRecord]:
        with self._lock:
            rows = [self._to_record(item) for item in self._rows.values()]
        filtered: list[ApplicationActivityRecord] = []
        for row in rows:
            if row.agent_id != agent_id:
                continue
            if started_at_from is not None and row.started_at < started_at_from:
                continue
            if started_at_to is not None and row.started_at > started_at_to:
                continue
            filtered.append(row)
        return filtered

    @staticmethod
    def _matches(record: ApplicationActivityRecord, query: ListApplicationActivityQuery) -> bool:
        if query.agent_id is not None and record.agent_id != query.agent_id:
            return False
        if query.session_id is not None and record.session_id != query.session_id:
            return False
        if (
            query.application_name
            and query.application_name.casefold() not in record.application_name.casefold()
        ):
            return False
        if query.started_at_from is not None and record.started_at < query.started_at_from:
            return False
        if query.started_at_to is not None and record.started_at > query.started_at_to:
            return False
        return True


def _sort_key(record: ApplicationActivityRecord, field: str):
    value = getattr(record, field)
    if value is None:
        return ""
    if hasattr(value, "isoformat"):
        return value.isoformat()
    return value

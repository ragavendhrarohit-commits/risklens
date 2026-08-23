from __future__ import annotations

from threading import RLock

from app.domain.browser import (
    BrowserActivity,
    BrowserActivityPage,
    BrowserActivityRecord,
    ListBrowserActivityQuery,
    parse_sort,
)
from app.repositories.agent_repository import AgentRepository
from app.repositories.browser_activity_repository import BrowserActivityRepository
from app.repositories.session_repository import SessionRepository


class MemoryBrowserActivityRepository(BrowserActivityRepository):
    def __init__(
        self,
        agents: AgentRepository | None = None,
        sessions: SessionRepository | None = None,
    ) -> None:
        self._rows: dict[str, BrowserActivity] = {}
        self._agents = agents
        self._sessions = sessions
        self._lock = RLock()

    def upsert_many(self, activities: list[BrowserActivity]) -> int:
        with self._lock:
            for activity in activities:
                self._rows[activity.id] = activity
            return len(activities)

    def get(self, activity_id: str) -> BrowserActivityRecord | None:
        with self._lock:
            row = self._rows.get(activity_id)
            if row is None:
                return None
            return self._to_record(row)

    def list(self, query: ListBrowserActivityQuery) -> BrowserActivityPage:
        with self._lock:
            rows = [self._to_record(item) for item in self._rows.values()]
        filtered = [row for row in rows if _matches(row, query)]
        field, descending = parse_sort(query.sort)
        filtered.sort(key=lambda r: _sort_key(r, field), reverse=descending)
        total = len(filtered)
        page = filtered[query.offset : query.offset + query.limit]
        return BrowserActivityPage(
            items=page, total=total, limit=query.limit, offset=query.offset
        )

    def _to_record(self, activity: BrowserActivity) -> BrowserActivityRecord:
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
        return BrowserActivityRecord(
            id=activity.id,
            session_id=activity.session_id,
            agent_id=activity.agent_id,
            executable_name=activity.executable_name,
            domain=activity.domain,
            url=activity.url,
            title=activity.title,
            profile=activity.profile,
            window_id=activity.window_id,
            tab_id=activity.tab_id,
            started_at=activity.started_at,
            stopped_at=activity.stopped_at,
            duration_seconds=activity.duration_seconds,
            collected_at=activity.collected_at,
            received_at=activity.received_at,
            username=username,
            machine_name=machine_name,
        )


def _matches(record: BrowserActivityRecord, query: ListBrowserActivityQuery) -> bool:
    if query.agent_id is not None and record.agent_id != query.agent_id:
        return False
    if query.session_id is not None and record.session_id != query.session_id:
        return False
    if query.domain and query.domain.casefold() not in record.domain.casefold():
        return False
    if query.started_at_from is not None and record.started_at < query.started_at_from:
        return False
    if query.started_at_to is not None and record.started_at > query.started_at_to:
        return False
    return True


def _sort_key(record: BrowserActivityRecord, field: str):
    value = getattr(record, field)
    if value is None:
        return ""
    if hasattr(value, "isoformat"):
        return value.isoformat()
    return value

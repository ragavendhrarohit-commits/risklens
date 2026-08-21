from __future__ import annotations

from datetime import datetime, timezone

from app.domain.browser import (
    BrowserActivity,
    BrowserActivityNotFoundError,
    BrowserActivityPage,
    BrowserActivityRecord,
    InvalidBrowserActivityError,
    ListBrowserActivityQuery,
    UnknownAgentError,
    UnknownSessionError,
    UpsertBrowserActivityCommand,
    browser_activity_id,
    is_browser_executable,
)
from app.domain.browser.strip import strip_url
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.browser_activity_repository import BrowserActivityRepository
from app.repositories.session_repository import SessionRepository

_logger = get_logger("risklens.browser")


class BrowserActivityService:
    def __init__(
        self,
        agents: AgentRepository,
        sessions: SessionRepository,
        activities: BrowserActivityRepository,
        *,
        clock=None,
    ) -> None:
        self._agents = agents
        self._sessions = sessions
        self._activities = activities
        self._clock = clock or _default_clock

    def upsert(self, command: UpsertBrowserActivityCommand) -> tuple[int, datetime]:
        if not self._agents.exists(command.agent_id):
            raise UnknownAgentError(command.agent_id)

        received_at = self._clock()
        rows = []
        seen_sessions: set = set()
        for item in command.activities:
            if item.session_id not in seen_sessions:
                if not self._sessions.exists(item.session_id):
                    raise UnknownSessionError(item.session_id)
                seen_sessions.add(item.session_id)
            if not is_browser_executable(item.executable_name):
                raise InvalidBrowserActivityError(
                    "executable_name must be a supported browser",
                    details={"executable_name": item.executable_name},
                )
            started_at = item.started_at or command.collected_at
            try:
                host, origin = strip_url(item.domain or item.url)
            except ValueError as exc:
                raise InvalidBrowserActivityError(str(exc)) from exc
            url = origin
            if item.url:
                try:
                    _, url = strip_url(item.url)
                except ValueError as exc:
                    raise InvalidBrowserActivityError(str(exc)) from exc
            row_id = (item.id or "").strip() or browser_activity_id(
                item.session_id,
                item.executable_name,
                host,
                item.window_id,
                item.tab_id,
                started_at,
            )
            duration = item.duration_seconds
            if duration < 0:
                duration = 0
            if item.stopped_at is not None and duration == 0:
                duration = max(int((item.stopped_at - started_at).total_seconds()), 0)
            rows.append(
                BrowserActivity(
                    id=row_id,
                    session_id=item.session_id,
                    agent_id=command.agent_id,
                    executable_name=item.executable_name.strip(),
                    domain=host,
                    url=url,
                    title=(item.title or "").strip()[:512],
                    profile=(item.profile or "").strip()[:256],
                    window_id=item.window_id,
                    tab_id=item.tab_id,
                    started_at=started_at,
                    stopped_at=item.stopped_at,
                    duration_seconds=duration,
                    collected_at=command.collected_at,
                    received_at=received_at,
                )
            )
        stored = self._activities.upsert_many(rows)
        _logger.info(
            "browser activity stored",
            extra={"agent_id": str(command.agent_id), "stored": stored},
        )
        return stored, received_at

    def get(self, activity_id_value: str) -> BrowserActivityRecord:
        record = self._activities.get(activity_id_value)
        if record is None:
            raise BrowserActivityNotFoundError(activity_id_value)
        return record

    def list(self, query: ListBrowserActivityQuery) -> BrowserActivityPage:
        return self._activities.list(query)


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)

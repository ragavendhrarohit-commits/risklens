from __future__ import annotations

from datetime import datetime, timezone

from app.domain.applications import (
    ApplicationActivity,
    ApplicationActivityNotFoundError,
    ApplicationActivityPage,
    ApplicationActivityRecord,
    ListApplicationActivityQuery,
    UnknownAgentError,
    UnknownSessionError,
    UpsertApplicationActivityCommand,
    activity_id,
)
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.application_activity_repository import ApplicationActivityRepository
from app.repositories.session_repository import SessionRepository

_logger = get_logger("risklens.applications")


class ApplicationActivityService:
    def __init__(
        self,
        agents: AgentRepository,
        sessions: SessionRepository,
        activities: ApplicationActivityRepository,
        *,
        clock=None,
    ) -> None:
        self._agents = agents
        self._sessions = sessions
        self._activities = activities
        self._clock = clock or _default_clock

    def upsert(self, command: UpsertApplicationActivityCommand) -> tuple[int, datetime]:
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
            started_at = item.started_at or command.collected_at
            row_id = (item.id or "").strip() or activity_id(
                item.session_id, item.executable_name or item.application_name, started_at
            )
            duration = item.duration_seconds
            if duration < 0:
                duration = 0
            if item.stopped_at is not None and duration == 0:
                duration = max(int((item.stopped_at - started_at).total_seconds()), 0)
            rows.append(
                ApplicationActivity(
                    id=row_id,
                    session_id=item.session_id,
                    agent_id=command.agent_id,
                    application_name=item.application_name.strip(),
                    executable_name=(item.executable_name or "").strip(),
                    window_title=(item.window_title or "").strip(),
                    started_at=started_at,
                    stopped_at=item.stopped_at,
                    duration_seconds=duration,
                    collected_at=command.collected_at,
                    received_at=received_at,
                )
            )
        stored = self._activities.upsert_many(rows)
        _logger.info(
            "application activity stored",
            extra={"agent_id": str(command.agent_id), "stored": stored},
        )
        return stored, received_at

    def get(self, activity_id_value: str) -> ApplicationActivityRecord:
        record = self._activities.get(activity_id_value)
        if record is None:
            raise ApplicationActivityNotFoundError(activity_id_value)
        return record

    def list(self, query: ListApplicationActivityQuery) -> ApplicationActivityPage:
        return self._activities.list(query)


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)

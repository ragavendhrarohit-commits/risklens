from __future__ import annotations

from datetime import datetime, timezone

from app.domain.sessions import (
    MAX_LIMIT,
    SESSION_STATUS_ACTIVE,
    SESSION_STATUS_CLOSED,
    ComputerSession,
    ListSessionsQuery,
    SessionNotFoundError,
    SessionPage,
    SessionRecord,
    UnknownAgentError,
    UpsertSessionsCommand,
    can_reopen_closed_session,
    merge_fallback_close,
    merge_session,
    session_from_record,
    session_is_immutable,
)
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.session_repository import SessionRepository
from app.services import session_lifecycle_log as lifecycle

_logger = get_logger("risklens.sessions")


class SessionService:
    def __init__(
        self,
        agents: AgentRepository,
        sessions: SessionRepository,
        *,
        clock=None,
    ) -> None:
        self._agents = agents
        self._sessions = sessions
        self._clock = clock or _default_clock

    def upsert(self, command: UpsertSessionsCommand) -> tuple[int, datetime]:
        if not self._agents.exists(command.agent_id):
            raise UnknownAgentError(command.agent_id)

        received_at = self._clock()
        lifecycle.banner(
            "INCOMING PAYLOAD",
            agent_id=str(command.agent_id),
            payload=_payload_dict(command),
        )
        actives = self._active_records(command.agent_id)
        lifecycle.banner(
            "CURRENT ACTIVE SESSIONS",
            agent_id=str(command.agent_id),
            active_count=len(actives),
            active_session_ids=[str(record.session_id) for record in actives],
            rows=[
                {
                    "session_id": str(record.session_id),
                    "login_time": record.logon_at.isoformat(),
                    "logout_time": None if record.logoff_at is None else record.logoff_at.isoformat(),
                    "status": record.status,
                }
                for record in actives
            ],
        )

        closed_items = []
        opened_items = []
        for item in command.sessions:
            status = (item.status or SESSION_STATUS_ACTIVE).strip() or SESSION_STATUS_ACTIVE
            if status == SESSION_STATUS_CLOSED:
                closed_items.append(item)
            else:
                opened_items.append(item)

        stored = 0
        if closed_items and not opened_items:
            lifecycle.banner(
                "REPOSITORY METHOD",
                agent_id=str(command.agent_id),
                method="_close_current_session -> upsert_many",
                kind="SHUTDOWN",
            )
            stored += self._close_current_session(command, closed_items, received_at)
            self._repro_dump("AFTER SHUTDOWN", command.agent_id)
        else:
            for item in closed_items:
                lifecycle.banner(
                    "REPOSITORY METHOD",
                    agent_id=str(command.agent_id),
                    method="_upsert_one",
                    kind="CLOSED_ITEM",
                    session_id=str(item.session_id),
                )
                stored += self._upsert_one(command, item, SESSION_STATUS_CLOSED, received_at)
            login_seen = False
            for item in opened_items:
                was_login = self._is_login(command.agent_id, item.session_id)
                if was_login:
                    login_seen = True
                stored += self._activate_login(command, item, received_at)
            if login_seen:
                self._repro_dump("AFTER LOGIN", command.agent_id)

        _logger.info(
            "sessions stored",
            extra={"agent_id": str(command.agent_id), "stored": stored},
        )
        return stored, received_at

    def get(self, session_id) -> SessionRecord:
        record = self._sessions.get(session_id)
        if record is None:
            raise SessionNotFoundError(session_id)
        return record

    def list(self, query: ListSessionsQuery) -> SessionPage:
        return self._sessions.list(query)

    def close_open_for_agent(self, agent_id, logoff_at: datetime) -> int:
        received_at = self._clock()
        page = self._sessions.list(
            ListSessionsQuery(agent_id=agent_id, limit=MAX_LIMIT, sort="logon_at")
        )
        if not page.items:
            return 0
        latest = max(page.items, key=lambda item: _as_utc(item.logon_at))
        actives = [
            record
            for record in page.items
            if not session_is_immutable(record.status, record.logoff_at)
        ]
        if not actives:
            return 0
        if len(actives) != 1 or actives[0].session_id != latest.session_id:
            lifecycle.error(
                "OFFLINE RECONCILIATION REFUSED",
                agent_id=str(agent_id),
                current_session_id=str(latest.session_id),
                active_session_ids=[str(item.session_id) for item in actives],
                sql_where="agent_id = :agent_id AND status = 'active'",
                rows_affected=0,
                session_ids_affected=[],
            )
            return 0
        current = actives[0]
        before = current.status
        closed = merge_fallback_close(
            session_from_record(current),
            logoff_at,
            received_at,
            None,
        )
        if closed is None:
            return 0
        stored = self._sessions.upsert_many([closed])
        lifecycle.banner(
            "OFFLINE RECONCILIATION",
            agent_id=str(agent_id),
            session_id=str(current.session_id),
            current_session_id=str(current.session_id),
            status_before=before,
            status_after=closed.status,
            sql_where="session_id = :session_id AND status = 'active'",
            rows_affected=stored,
            session_ids_affected=[str(current.session_id)],
        )
        return stored

    def _upsert_one(
        self,
        command: UpsertSessionsCommand,
        item,
        status: str,
        received_at: datetime,
    ) -> int:
        logon_at = item.logon_at or command.collected_at
        incoming = ComputerSession(
            session_id=item.session_id,
            agent_id=command.agent_id,
            username=item.username.strip(),
            account_sid=(item.account_sid or "").strip(),
            logon_at=logon_at,
            logoff_at=item.logoff_at,
            status=status,
            collected_at=command.collected_at,
            received_at=received_at,
        )
        existing = self._sessions.get(incoming.session_id)
        known = self._sessions.list(
            ListSessionsQuery(agent_id=command.agent_id, limit=MAX_LIMIT, sort="logon_at")
        ).items
        if (
            existing is not None
            and session_is_immutable(existing.status, existing.logoff_at)
            and not can_reopen_closed_session(
                session_from_record(existing), incoming, known_sessions=known
            )
        ):
            _logger.info(
                "historical session unchanged",
                extra={
                    "agent_id": str(command.agent_id),
                    "session_id": str(existing.session_id),
                    "logoff_at": None if existing.logoff_at is None else existing.logoff_at.isoformat(),
                },
            )
            return 1
        next_logon = _next_logon_for(
            known,
            incoming.session_id,
            existing.logon_at if existing is not None else incoming.logon_at,
        )
        if incoming.status == SESSION_STATUS_CLOSED and next_logon is not None:
            incoming_logoff = incoming.logoff_at or incoming.collected_at
            if incoming_logoff is None or _as_utc(incoming_logoff) != _as_utc(next_logon):
                lifecycle.error(
                    "CLOSE REJECTED",
                    agent_id=str(command.agent_id),
                    session_id=str(incoming.session_id),
                    current_session_id=str(incoming.session_id),
                    status_before=None if existing is None else existing.status,
                    status_after=None if existing is None else existing.status,
                    sql_where="session_id = :session_id AND status = 'active'",
                    rows_affected=0,
                    session_ids_affected=[],
                    reason="shutdown/offline cannot close a session that is not current",
                )
                return 0 if existing is None else 1
        merged = merge_session(
            session_from_record(existing) if existing is not None else None,
            incoming,
            None,
            known_sessions=known,
        )
        if (
            session_is_immutable(merged.status, merged.logoff_at)
            and existing is not None
            and not can_reopen_closed_session(
                session_from_record(existing), incoming, known_sessions=known
            )
        ):
            if existing.logoff_at == merged.logoff_at and existing.status == merged.status:
                _logger.info(
                    "historical session unchanged",
                    extra={
                        "agent_id": str(command.agent_id),
                        "session_id": str(merged.session_id),
                        "logoff_at": None if merged.logoff_at is None else merged.logoff_at.isoformat(),
                    },
                )
                return 1
        self._sessions.upsert_many([merged])
        before = None if existing is None else existing.status
        operation = "CREATE SESSION" if existing is None else (
            "SHUTDOWN" if merged.status == SESSION_STATUS_CLOSED else "HEARTBEAT"
        )
        lifecycle.banner(
            operation,
            agent_id=str(command.agent_id),
            session_id=str(merged.session_id),
            current_session_id=str(merged.session_id),
            status_before=before,
            status_after=merged.status,
            sql_where="session_id = :session_id AND status != 'closed'",
            rows_affected=1,
            session_ids_affected=[str(merged.session_id)],
        )
        return 1

    def _activate_login(self, command: UpsertSessionsCommand, item, received_at: datetime) -> int:
        logon_at = item.logon_at or command.collected_at
        incoming = ComputerSession(
            session_id=item.session_id,
            agent_id=command.agent_id,
            username=item.username.strip(),
            account_sid=(item.account_sid or "").strip(),
            logon_at=logon_at,
            logoff_at=item.logoff_at,
            status=SESSION_STATUS_ACTIVE,
            collected_at=command.collected_at,
            received_at=received_at,
        )
        existing = self._sessions.get(incoming.session_id)
        known = self._sessions.list(
            ListSessionsQuery(agent_id=command.agent_id, limit=MAX_LIMIT, sort="logon_at")
        ).items
        if (
            existing is not None
            and session_is_immutable(existing.status, existing.logoff_at)
            and not can_reopen_closed_session(
                session_from_record(existing), incoming, known_sessions=known
            )
        ):
            return self._upsert_one(command, item, SESSION_STATUS_ACTIVE, received_at)

        actives_before = self._active_records(command.agent_id)
        current_active = None
        if actives_before:
            current_active = max(actives_before, key=lambda record: _as_utc(record.logon_at))
        is_login = current_active is None or current_active.session_id != incoming.session_id
        dump_before = self._session_dump(command.agent_id)
        kind = "LOGIN" if is_login else "SESSION_HEARTBEAT"
        lifecycle.banner(
            "LOGIN DETECTED" if is_login else "SESSION HEARTBEAT",
            agent_id=str(command.agent_id),
            kind=kind,
            incoming_session_id=str(incoming.session_id),
            current_session_id=("" if current_active is None else str(current_active.session_id)),
            current_active_sessions=[str(record.session_id) for record in actives_before],
            repository_method="activate_current",
            rows_before=dump_before,
        )
        lifecycle.banner(
            "REPOSITORY METHOD",
            agent_id=str(command.agent_id),
            method="activate_current",
            session_id=str(incoming.session_id),
            kind=kind,
        )
        written = self._sessions.activate_current(incoming)
        return written

    def _session_dump(self, agent_id) -> list[dict]:
        page = self._sessions.list(
            ListSessionsQuery(agent_id=agent_id, limit=MAX_LIMIT, sort="logon_at")
        )
        return [
            {
                "session_id": str(record.session_id),
                "login_time": record.logon_at.isoformat(),
                "logout_time": None if record.logoff_at is None else record.logoff_at.isoformat(),
                "status": record.status,
            }
            for record in page.items
        ]

    def _repro_dump(self, title: str, agent_id) -> None:
        lifecycle.dump_query(title, str(agent_id), self._session_dump(agent_id))

    def _is_login(self, agent_id, session_id) -> bool:
        existing = self._sessions.get(session_id)
        if existing is not None:
            return False
        actives = self._active_records(agent_id)
        if not actives:
            return True
        return all(record.session_id != session_id for record in actives)

    def _close_current_session(self, command: UpsertSessionsCommand, closed_items, received_at: datetime) -> int:
        current = max(closed_items, key=lambda item: item.logon_at or command.collected_at)
        current_id = current.session_id
        actives = self._active_records(command.agent_id)
        active_ids = [str(record.session_id) for record in actives]
        _logger.info(
            "shutdown close requested",
            extra={
                "sql_where": "session_id = :session_id AND agent_id = :agent_id AND status = 'active'",
                "session_id": str(current_id),
                "agent_id": str(command.agent_id),
                "current_session_id": str(current_id),
                "active_session_ids": active_ids,
                "rows_affected": 0,
                "payload_count": len(closed_items),
                "active_count": len(actives),
            },
        )
        if len(actives) > 1:
            lifecycle.error(
                "SHUTDOWN",
                agent_id=str(command.agent_id),
                session_id=str(current_id),
                current_session_id=str(current_id),
                sql_where="agent_id = :agent_id AND status = 'active'",
                rows_selected=len(actives),
                rows_updated=0,
                session_ids_updated=[],
                reason="more than one ACTIVE session; shutdown will update only current_session_id",
            )
        if len(closed_items) > 1:
            extra_ids = [
                str(item.session_id) for item in closed_items if item.session_id != current_id
            ]
            lifecycle.error(
                "SHUTDOWN",
                agent_id=str(command.agent_id),
                session_id=str(current_id),
                current_session_id=str(current_id),
                sql_where="session_id = :session_id AND status = 'active'",
                rows_selected=len(closed_items),
                rows_updated=0,
                session_ids_updated=[],
                rejected_session_ids=extra_ids,
                reason="shutdown payload contained multiple sessions",
            )
        if actives and all(record.session_id != current_id for record in actives):
            lifecycle.error(
                "SHUTDOWN",
                agent_id=str(command.agent_id),
                session_id=str(current_id),
                current_session_id=str(current_id),
                sql_where="session_id = :session_id AND status = 'active'",
                rows_selected=len(actives),
                rows_updated=0,
                session_ids_updated=[],
                active_session_ids=active_ids,
                reason="shutdown session is not the backend ACTIVE session",
            )
        lifecycle.banner(
            "SHUTDOWN",
            agent_id=str(command.agent_id),
            session_id=str(current_id),
            current_session_id=str(current_id),
            sql_where="session_id = :session_id AND agent_id = :agent_id AND status = 'active'",
            rows_selected=len(actives),
            rows_updated=1,
            session_ids_updated=[str(current_id)],
        )
        stored = self._upsert_one(command, current, SESSION_STATUS_CLOSED, received_at)
        return stored

    def _active_records(self, agent_id):
        return self._sessions.list(
            ListSessionsQuery(
                agent_id=agent_id,
                status=SESSION_STATUS_ACTIVE,
                limit=MAX_LIMIT,
                sort="logon_at",
            )
        ).items


def _payload_dict(command: UpsertSessionsCommand) -> dict:
    sessions = []
    for item in command.sessions:
        sessions.append(
            {
                "session_id": str(item.session_id),
                "username": item.username,
                "account_sid": item.account_sid,
                "logon_at": None if item.logon_at is None else item.logon_at.isoformat(),
                "logoff_at": None if item.logoff_at is None else item.logoff_at.isoformat(),
                "status": item.status,
            }
        )
    return {
        "agent_id": str(command.agent_id),
        "install_id": command.install_id,
        "collected_at": command.collected_at.isoformat(),
        "sessions": sessions,
    }


def _next_logon(records: list[SessionRecord], current: SessionRecord):
    return _next_logon_for(records, current.session_id, current.logon_at)


def _next_logon_for(records: list[SessionRecord], session_id, logon_at: datetime):
    logon_at = _as_utc(logon_at)
    later = [
        _as_utc(item.logon_at)
        for item in records
        if item.session_id != session_id and _as_utc(item.logon_at) > logon_at
    ]
    return min(later) if later else None


def _as_utc(value: datetime) -> datetime:
    if value.tzinfo is None:
        return value.replace(tzinfo=timezone.utc)
    return value.astimezone(timezone.utc)


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)

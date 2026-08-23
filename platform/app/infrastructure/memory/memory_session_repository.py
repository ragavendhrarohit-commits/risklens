from __future__ import annotations

from datetime import datetime, timezone
from threading import RLock
from uuid import UUID

from app.domain.sessions import (
    SESSION_STATUS_ACTIVE,
    SESSION_STATUS_CLOSED,
    ComputerSession,
    ListSessionsQuery,
    SessionPage,
    SessionRecord,
    can_reopen_closed_session,
    parse_sort,
    session_duration_seconds,
    session_is_immutable,
)
from app.domain.sessions.errors import MultipleActiveSessionsError
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.session_repository import SessionRepository

_logger = get_logger("risklens.sessions.memory")


class MemorySessionRepository(SessionRepository):
    def __init__(self, agents: AgentRepository | None = None) -> None:
        self._rows: dict[UUID, ComputerSession] = {}
        self._agents = agents
        self._lock = RLock()

    def upsert_many(self, sessions: list[ComputerSession]) -> int:
        with self._lock:
            written = 0
            for session in sessions:
                existing = self._rows.get(session.session_id)
                sql_where = "session_id = :session_id AND status != 'closed'"
                if (
                    existing is not None
                    and session_is_immutable(existing.status, existing.logoff_at)
                    and not can_reopen_closed_session(existing, session)
                ):
                    _logger.info(
                        "session upsert",
                        extra={
                            "sql_where": sql_where,
                            "session_id": str(session.session_id),
                            "agent_id": str(session.agent_id),
                            "current_session_id": str(session.session_id),
                            "rows_affected": 0,
                            "status": existing.status,
                        },
                    )
                    continue
                self._rows[session.session_id] = session
                written += 1
                _logger.info(
                    "REPRO SQL",
                    extra={
                        "repository_method": "upsert_many",
                        "sql_where": sql_where,
                        "session_id": str(session.session_id),
                        "agent_id": str(session.agent_id),
                        "current_session_id": str(session.session_id),
                        "rows_affected": 1,
                        "status": session.status,
                    },
                )
            _logger.info(
                "REPRO transaction commit",
                extra={
                    "repository_method": "upsert_many",
                    "rows_written": written,
                    "session_ids": [str(session.session_id) for session in sessions],
                },
            )
            return written

    def activate_current(self, incoming: ComputerSession) -> int:
        with self._lock:
            snapshot = dict(self._rows)
            try:
                return self._activate_current_locked(incoming)
            except Exception:
                self._rows = snapshot
                raise

    def _activate_current_locked(self, incoming: ComputerSession) -> int:
            closed_ids: list[str] = []
            for session_id, session in list(self._rows.items()):
                if session.agent_id != incoming.agent_id:
                    continue
                if session.session_id == incoming.session_id:
                    continue
                if session.status != SESSION_STATUS_ACTIVE or session.logoff_at is not None:
                    continue
                end = incoming.logon_at
                if end < session.logon_at:
                    end = session.logon_at
                self._rows[session_id] = ComputerSession(
                    session_id=session.session_id,
                    agent_id=session.agent_id,
                    username=session.username,
                    account_sid=session.account_sid,
                    logon_at=session.logon_at,
                    logoff_at=end,
                    status=SESSION_STATUS_CLOSED,
                    collected_at=incoming.received_at,
                    received_at=incoming.received_at,
                )
                closed_ids.append(str(session_id))
            existing = self._rows.get(incoming.session_id)
            inserted = 0
            updated = 0
            known = [
                session
                for session in self._rows.values()
                if session.agent_id == incoming.agent_id
            ]
            if existing is None:
                self._rows[incoming.session_id] = incoming
                inserted = 1
            elif session_is_immutable(existing.status, existing.logoff_at) and not (
                can_reopen_closed_session(existing, incoming, known_sessions=known)
            ):
                pass
            else:
                self._rows[incoming.session_id] = incoming
                updated = 1
            actives = [
                session
                for session in self._rows.values()
                if session.agent_id == incoming.agent_id and session.status == SESSION_STATUS_ACTIVE
            ]
            dump = [
                {
                    "session_id": str(session.session_id),
                    "status": session.status,
                    "logon_at": session.logon_at.isoformat(),
                    "logoff_at": None if session.logoff_at is None else session.logoff_at.isoformat(),
                }
                for session in sorted(
                    (item for item in self._rows.values() if item.agent_id == incoming.agent_id),
                    key=lambda item: item.logon_at,
                    reverse=True,
                )
            ]
            if len(actives) != 1 or actives[0].session_id != incoming.session_id:
                details = {
                    "current_session_id": str(incoming.session_id),
                    "active_session_ids": [str(session.session_id) for session in actives],
                    "sql": (
                        "SELECT session_id, status FROM computer_sessions "
                        "WHERE agent_id = :agent_id ORDER BY logon_at DESC"
                    ),
                    "rows": dump,
                    "decision": "ABORT",
                    "rows_updated": len(closed_ids) + updated,
                    "rows_inserted": inserted,
                }
                _logger.critical(
                    "FATAL: more than one ACTIVE session after login",
                    extra=details | {"agent_id": str(incoming.agent_id)},
                )
                raise MultipleActiveSessionsError(incoming.agent_id, details)
            _logger.info(
                "LOGIN activate_current",
                extra={
                    "agent_id": str(incoming.agent_id),
                    "incoming_session_id": str(incoming.session_id),
                    "decision": "CLOSE_OTHER_ACTIVES_THEN_INSERT",
                    "rows_updated": len(closed_ids) + updated,
                    "rows_inserted": inserted,
                    "closed_session_ids": closed_ids,
                    "sql": (
                        "SELECT session_id, status FROM computer_sessions "
                        "WHERE agent_id = :agent_id ORDER BY logon_at DESC"
                    ),
                    "rows": dump,
                },
            )
            return inserted + updated + len(closed_ids)

    def get(self, session_id: UUID) -> SessionRecord | None:
        with self._lock:
            row = self._rows.get(session_id)
            if row is None:
                return None
            return self._to_record(row)

    def exists(self, session_id: UUID) -> bool:
        with self._lock:
            return session_id in self._rows

    def close_active_for_agent(
        self,
        agent_id: UUID,
        *,
        logoff_at,
        received_at,
        current_session_id: UUID | None = None,
    ) -> int:
        sql_where = "agent_id = :agent_id AND status = 'active' AND logoff_at IS NULL"
        with self._lock:
            matches = [
                session
                for session in self._rows.values()
                if session.agent_id == agent_id
                and session.status == SESSION_STATUS_ACTIVE
                and session.logoff_at is None
            ]
            active_ids = [str(session.session_id) for session in matches]
            if len(matches) != 1:
                _logger.error(
                    "close_active_for_agent refused: expected exactly one active session",
                    extra={
                        "sql_where": sql_where,
                        "session_id": None,
                        "agent_id": str(agent_id),
                        "current_session_id": None if current_session_id is None else str(current_session_id),
                        "active_session_ids": active_ids,
                        "rows_affected": 0,
                    },
                )
                return 0
            session = matches[0]
            if current_session_id is not None and session.session_id != current_session_id:
                _logger.error(
                    "close_active_for_agent refused: active session is not current",
                    extra={
                        "sql_where": sql_where,
                        "session_id": str(session.session_id),
                        "agent_id": str(agent_id),
                        "current_session_id": str(current_session_id),
                        "rows_affected": 0,
                    },
                )
                return 0
            end = logoff_at if logoff_at >= session.logon_at else session.logon_at
            self._rows[session.session_id] = ComputerSession(
                session_id=session.session_id,
                agent_id=session.agent_id,
                username=session.username,
                account_sid=session.account_sid,
                logon_at=session.logon_at,
                logoff_at=end,
                status=SESSION_STATUS_CLOSED,
                collected_at=received_at,
                received_at=received_at,
            )
            _logger.info(
                "close_active_for_agent",
                extra={
                    "sql_where": "session_id = :session_id AND agent_id = :agent_id AND status = 'active'",
                    "session_id": str(session.session_id),
                    "agent_id": str(agent_id),
                    "current_session_id": str(session.session_id),
                    "rows_affected": 1,
                },
            )
            return 1

    def list(self, query: ListSessionsQuery) -> SessionPage:
        with self._lock:
            rows = [self._to_record(item) for item in self._rows.values()]
        filtered = [row for row in rows if self._matches(row, query)]
        field, descending = parse_sort(query.sort)
        filtered.sort(key=lambda r: _sort_key(r, field), reverse=descending)
        total = len(filtered)
        page = filtered[query.offset : query.offset + query.limit]
        return SessionPage(items=page, total=total, limit=query.limit, offset=query.offset)

    def _to_record(self, session: ComputerSession) -> SessionRecord:
        machine_name = ""
        if self._agents is not None:
            agent = self._agents.get(session.agent_id)
            if agent is not None:
                machine_name = agent.machine_name
        now = datetime.now(timezone.utc)
        return SessionRecord(
            session_id=session.session_id,
            agent_id=session.agent_id,
            username=session.username,
            account_sid=session.account_sid,
            logon_at=session.logon_at,
            logoff_at=session.logoff_at,
            status=session.status,
            collected_at=session.collected_at,
            received_at=session.received_at,
            machine_name=machine_name,
            duration_seconds=session_duration_seconds(session.logon_at, session.logoff_at, now),
        )

    @staticmethod
    def _matches(record: SessionRecord, query: ListSessionsQuery) -> bool:
        if query.agent_id is not None and record.agent_id != query.agent_id:
            return False
        if query.status and query.status != record.status:
            return False
        if query.username and query.username.casefold() not in record.username.casefold():
            return False
        return True


def _sort_key(record: SessionRecord, field: str):
    value = getattr(record, field)
    if value is None:
        return ""
    if hasattr(value, "isoformat"):
        return value.isoformat()
    if isinstance(value, bool):
        return int(value)
    return value

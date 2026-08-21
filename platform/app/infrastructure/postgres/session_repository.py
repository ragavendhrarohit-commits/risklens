from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from sqlalchemy import Select, asc, desc, func, select, update
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.orm import Session, sessionmaker

from app.domain.sessions import (
    SESSION_STATUS_ACTIVE,
    SESSION_STATUS_CLOSED,
    ComputerSession,
    ListSessionsQuery,
    SessionPage,
    SessionRecord,
    parse_sort,
    session_duration_seconds,
    session_is_immutable,
)
from app.domain.sessions.errors import MultipleActiveSessionsError
from app.infrastructure.postgres.models import AgentRow, ComputerSessionRow
from app.logging.setup import get_logger
from app.repositories.session_repository import SessionRepository

_logger = get_logger("risklens.sessions.postgres")

_SORT_COLUMNS = {
    "logon_at": ComputerSessionRow.logon_at,
    "logoff_at": ComputerSessionRow.logoff_at,
    "username": ComputerSessionRow.username,
    "status": ComputerSessionRow.status,
    "collected_at": ComputerSessionRow.collected_at,
    "received_at": ComputerSessionRow.received_at,
    "machine_name": AgentRow.machine_name,
}


class PostgresSessionRepository(SessionRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def upsert_many(self, sessions: list[ComputerSession]) -> int:
        if not sessions:
            return 0
        with self._session_factory() as session:
            dialect = session.bind.dialect.name if session.bind is not None else ""
            count = 0
            for item in sessions:
                row = ComputerSessionRow.from_domain(item)
                values = {
                    column.name: getattr(row, column.name)
                    for column in ComputerSessionRow.__table__.columns
                }
                if dialect == "postgresql":
                    stmt = pg_insert(ComputerSessionRow).values(**values)
                    update_cols = {
                        key: stmt.excluded[key] for key in values if key != "session_id"
                    }
                    sql_where = (
                        "computer_sessions.session_id = :session_id "
                        "AND computer_sessions.status != 'closed'"
                    )
                    stmt = stmt.on_conflict_do_update(
                        index_elements=[ComputerSessionRow.session_id],
                        set_=update_cols,
                        where=ComputerSessionRow.status != SESSION_STATUS_CLOSED,
                    )
                    result = session.execute(stmt)
                    rows_affected = int(result.rowcount or 0)
                    _logger.info(
                        "REPRO SQL",
                        extra={
                            "repository_method": "upsert_many",
                            "sql": (
                                "INSERT INTO computer_sessions (...) VALUES (...) "
                                "ON CONFLICT (session_id) DO UPDATE SET ... "
                                "WHERE computer_sessions.status != 'closed'"
                            ),
                            "sql_where": sql_where,
                            "session_id": str(item.session_id),
                            "agent_id": str(item.agent_id),
                            "current_session_id": str(item.session_id),
                            "rows_affected": rows_affected,
                            "status": item.status,
                        },
                    )
                    count += rows_affected
                else:
                    existing = session.get(ComputerSessionRow, item.session_id)
                    if existing is None:
                        session.add(row)
                        count += 1
                        rows_affected = 1
                    elif session_is_immutable(existing.status, existing.logoff_at):
                        rows_affected = 0
                    else:
                        for key, value in values.items():
                            if key != "session_id":
                                setattr(existing, key, value)
                        count += 1
                        rows_affected = 1
                    _logger.info(
                        "REPRO SQL",
                        extra={
                            "repository_method": "upsert_many",
                            "sql": "INSERT/UPDATE computer_sessions WHERE session_id = :session_id AND status != 'closed'",
                            "session_id": str(item.session_id),
                            "agent_id": str(item.agent_id),
                            "rows_affected": rows_affected,
                            "status": item.status,
                        },
                    )
            session.commit()
            _logger.info(
                "REPRO transaction commit",
                extra={
                    "repository_method": "upsert_many",
                    "rows_written": count,
                    "session_ids": [str(item.session_id) for item in sessions],
                },
            )
            return count

    def activate_current(self, incoming: ComputerSession) -> int:
        with self._session_factory() as session:
            existing_rows = session.scalars(
                select(ComputerSessionRow).where(
                    ComputerSessionRow.agent_id == incoming.agent_id,
                    ComputerSessionRow.status == SESSION_STATUS_ACTIVE,
                )
            ).all()
            closed_ids: list[str] = []
            for row in existing_rows:
                if row.session_id == incoming.session_id:
                    continue
                end = incoming.logon_at
                if end < row.logon_at:
                    end = row.logon_at
                before = row.status
                row.logoff_at = end
                row.status = SESSION_STATUS_CLOSED
                row.collected_at = incoming.received_at
                row.received_at = incoming.received_at
                closed_ids.append(str(row.session_id))
                _logger.info(
                    "LOGIN close previous ACTIVE",
                    extra={
                        "sql_where": (
                            "session_id = :session_id AND agent_id = :agent_id "
                            "AND status = 'active'"
                        ),
                        "session_id": str(row.session_id),
                        "agent_id": str(incoming.agent_id),
                        "current_session_id": str(incoming.session_id),
                        "status_before": before,
                        "status_after": SESSION_STATUS_CLOSED,
                        "rows_affected": 1,
                    },
                )
            target = session.get(ComputerSessionRow, incoming.session_id)
            inserted = 0
            updated = 0
            if target is None:
                session.add(ComputerSessionRow.from_domain(incoming))
                inserted = 1
            elif session_is_immutable(target.status, target.logoff_at):
                pass
            else:
                incoming_row = ComputerSessionRow.from_domain(incoming)
                for column in ComputerSessionRow.__table__.columns:
                    if column.name == "session_id":
                        continue
                    setattr(target, column.name, getattr(incoming_row, column.name))
                updated = 1
            session.flush()
            actives = session.scalars(
                select(ComputerSessionRow).where(
                    ComputerSessionRow.agent_id == incoming.agent_id,
                    ComputerSessionRow.status == SESSION_STATUS_ACTIVE,
                )
            ).all()
            dump = [
                {
                    "session_id": str(row.session_id),
                    "status": row.status,
                    "logon_at": row.logon_at.isoformat(),
                    "logoff_at": None if row.logoff_at is None else row.logoff_at.isoformat(),
                }
                for row in session.scalars(
                    select(ComputerSessionRow)
                    .where(ComputerSessionRow.agent_id == incoming.agent_id)
                    .order_by(ComputerSessionRow.logon_at.desc())
                ).all()
            ]
            if len(actives) != 1 or actives[0].session_id != incoming.session_id:
                details = {
                    "current_session_id": str(incoming.session_id),
                    "active_session_ids": [str(row.session_id) for row in actives],
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
            session.commit()
            return inserted + updated + len(closed_ids)

    def get(self, session_id: UUID) -> SessionRecord | None:
        with self._session_factory() as session:
            stmt = (
                select(ComputerSessionRow, AgentRow.machine_name)
                .outerjoin(AgentRow, AgentRow.agent_id == ComputerSessionRow.agent_id)
                .where(ComputerSessionRow.session_id == session_id)
            )
            row = session.execute(stmt).first()
            if row is None:
                return None
            item, machine_name = row
            return _to_record(item, machine_name or "")

    def close_active_for_agent(
        self,
        agent_id: UUID,
        *,
        logoff_at,
        received_at,
        current_session_id: UUID | None = None,
    ) -> int:
        sql_where = (
            "agent_id = :agent_id AND status = 'active' AND logoff_at IS NULL"
        )
        with self._session_factory() as session:
            rows = session.scalars(
                select(ComputerSessionRow).where(
                    ComputerSessionRow.agent_id == agent_id,
                    ComputerSessionRow.status == SESSION_STATUS_ACTIVE,
                    ComputerSessionRow.logoff_at.is_(None),
                )
            ).all()
            active_ids = [str(row.session_id) for row in rows]
            if len(rows) != 1:
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
            row = rows[0]
            if current_session_id is not None and row.session_id != current_session_id:
                _logger.error(
                    "close_active_for_agent refused: active session is not current",
                    extra={
                        "sql_where": sql_where,
                        "session_id": str(row.session_id),
                        "agent_id": str(agent_id),
                        "current_session_id": str(current_session_id),
                        "rows_affected": 0,
                    },
                )
                return 0
            end = logoff_at if logoff_at >= row.logon_at else row.logon_at
            result = session.execute(
                update(ComputerSessionRow)
                .where(
                    ComputerSessionRow.session_id == row.session_id,
                    ComputerSessionRow.agent_id == agent_id,
                    ComputerSessionRow.status == SESSION_STATUS_ACTIVE,
                )
                .values(
                    logoff_at=end,
                    status=SESSION_STATUS_CLOSED,
                    collected_at=received_at,
                    received_at=received_at,
                )
            )
            rows_affected = int(result.rowcount or 0)
            session.commit()
            _logger.info(
                "close_active_for_agent",
                extra={
                    "sql_where": "session_id = :session_id AND agent_id = :agent_id AND status = 'active'",
                    "session_id": str(row.session_id),
                    "agent_id": str(agent_id),
                    "current_session_id": str(row.session_id),
                    "rows_affected": rows_affected,
                },
            )
            return rows_affected

    def exists(self, session_id: UUID) -> bool:
        with self._session_factory() as session:
            return session.get(ComputerSessionRow, session_id) is not None

    def list(self, query: ListSessionsQuery) -> SessionPage:
        with self._session_factory() as session:
            base = select(ComputerSessionRow, AgentRow.machine_name).outerjoin(
                AgentRow, AgentRow.agent_id == ComputerSessionRow.agent_id
            )
            base = _apply_filters(base, query)
            count_stmt = select(func.count()).select_from(base.subquery())
            total = int(session.execute(count_stmt).scalar_one())
            field, descending = parse_sort(query.sort)
            column = _SORT_COLUMNS[field]
            order = desc(column) if descending else asc(column)
            stmt = (
                base.order_by(order, ComputerSessionRow.session_id)
                .offset(query.offset)
                .limit(query.limit)
            )
            items = [
                _to_record(item, machine_name or "")
                for item, machine_name in session.execute(stmt).all()
            ]
            return SessionPage(
                items=items, total=total, limit=query.limit, offset=query.offset
            )


def _apply_filters(stmt: Select, query: ListSessionsQuery) -> Select:
    if query.agent_id is not None:
        stmt = stmt.where(ComputerSessionRow.agent_id == query.agent_id)
    if query.status:
        stmt = stmt.where(ComputerSessionRow.status == query.status)
    if query.username:
        stmt = stmt.where(ComputerSessionRow.username.ilike(f"%{query.username}%"))
    return stmt


def _to_record(row: ComputerSessionRow, machine_name: str) -> SessionRecord:
    now = datetime.now(timezone.utc)
    return SessionRecord(
        session_id=row.session_id,
        agent_id=row.agent_id,
        username=row.username,
        account_sid=row.account_sid,
        logon_at=row.logon_at,
        logoff_at=row.logoff_at,
        status=row.status,
        collected_at=row.collected_at,
        received_at=row.received_at,
        machine_name=machine_name,
        duration_seconds=session_duration_seconds(row.logon_at, row.logoff_at, now),
    )

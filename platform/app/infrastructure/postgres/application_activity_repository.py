from __future__ import annotations

from sqlalchemy import Select, asc, desc, func, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.orm import Session, sessionmaker

from app.domain.applications import (
    ApplicationActivity,
    ApplicationActivityPage,
    ApplicationActivityRecord,
    ListApplicationActivityQuery,
    parse_sort,
)
from app.infrastructure.postgres.models import AgentRow, ApplicationActivityRow, ComputerSessionRow
from app.repositories.application_activity_repository import ApplicationActivityRepository

_SORT_COLUMNS = {
    "started_at": ApplicationActivityRow.started_at,
    "stopped_at": ApplicationActivityRow.stopped_at,
    "application_name": ApplicationActivityRow.application_name,
    "executable_name": ApplicationActivityRow.executable_name,
    "duration_seconds": ApplicationActivityRow.duration_seconds,
    "collected_at": ApplicationActivityRow.collected_at,
    "received_at": ApplicationActivityRow.received_at,
    "username": ComputerSessionRow.username,
}


class PostgresApplicationActivityRepository(ApplicationActivityRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def upsert_many(self, activities: list[ApplicationActivity]) -> int:
        if not activities:
            return 0
        with self._session_factory() as session:
            dialect = session.bind.dialect.name if session.bind is not None else ""
            count = 0
            for item in activities:
                row = ApplicationActivityRow.from_domain(item)
                values = {
                    column.name: getattr(row, column.name)
                    for column in ApplicationActivityRow.__table__.columns
                }
                if dialect == "postgresql":
                    stmt = pg_insert(ApplicationActivityRow).values(**values)
                    update_cols = {key: stmt.excluded[key] for key in values if key != "id"}
                    stmt = stmt.on_conflict_do_update(
                        index_elements=[ApplicationActivityRow.id],
                        set_=update_cols,
                    )
                    session.execute(stmt)
                else:
                    existing = session.get(ApplicationActivityRow, item.id)
                    if existing is None:
                        session.add(row)
                    else:
                        for key, value in values.items():
                            if key != "id":
                                setattr(existing, key, value)
                count += 1
            session.commit()
            return count

    def get(self, activity_id: str) -> ApplicationActivityRecord | None:
        with self._session_factory() as session:
            stmt = (
                select(ApplicationActivityRow, AgentRow.machine_name, ComputerSessionRow.username)
                .outerjoin(AgentRow, AgentRow.agent_id == ApplicationActivityRow.agent_id)
                .outerjoin(
                    ComputerSessionRow,
                    ComputerSessionRow.session_id == ApplicationActivityRow.session_id,
                )
                .where(ApplicationActivityRow.id == activity_id)
            )
            row = session.execute(stmt).first()
            if row is None:
                return None
            item, machine_name, username = row
            return _to_record(item, machine_name or "", username or "")

    def list(self, query: ListApplicationActivityQuery) -> ApplicationActivityPage:
        with self._session_factory() as session:
            base = (
                select(ApplicationActivityRow, AgentRow.machine_name, ComputerSessionRow.username)
                .outerjoin(AgentRow, AgentRow.agent_id == ApplicationActivityRow.agent_id)
                .outerjoin(
                    ComputerSessionRow,
                    ComputerSessionRow.session_id == ApplicationActivityRow.session_id,
                )
            )
            base = _apply_filters(base, query)
            count_stmt = select(func.count()).select_from(base.subquery())
            total = int(session.execute(count_stmt).scalar_one())
            field, descending = parse_sort(query.sort)
            column = _SORT_COLUMNS[field]
            order = desc(column) if descending else asc(column)
            stmt = (
                base.order_by(order, ApplicationActivityRow.id)
                .offset(query.offset)
                .limit(query.limit)
            )
            items = [
                _to_record(item, machine_name or "", username or "")
                for item, machine_name, username in session.execute(stmt).all()
            ]
            return ApplicationActivityPage(
                items=items, total=total, limit=query.limit, offset=query.offset
            )


def _apply_filters(stmt: Select, query: ListApplicationActivityQuery) -> Select:
    if query.agent_id is not None:
        stmt = stmt.where(ApplicationActivityRow.agent_id == query.agent_id)
    if query.session_id is not None:
        stmt = stmt.where(ApplicationActivityRow.session_id == query.session_id)
    if query.application_name:
        stmt = stmt.where(
            ApplicationActivityRow.application_name.ilike(f"%{query.application_name}%")
        )
    return stmt


def _to_record(
    row: ApplicationActivityRow, machine_name: str, username: str
) -> ApplicationActivityRecord:
    return ApplicationActivityRecord(
        id=row.id,
        session_id=row.session_id,
        agent_id=row.agent_id,
        application_name=row.application_name,
        executable_name=row.executable_name,
        window_title=row.window_title,
        started_at=row.started_at,
        stopped_at=row.stopped_at,
        duration_seconds=row.duration_seconds,
        collected_at=row.collected_at,
        received_at=row.received_at,
        username=username,
        machine_name=machine_name,
    )

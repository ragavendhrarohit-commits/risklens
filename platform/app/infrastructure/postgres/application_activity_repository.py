from __future__ import annotations

from datetime import datetime
from uuid import UUID

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
from app.domain.productivity import ApplicationAggregate, IntervalPoint, TitleAggregate, interval_end
from app.infrastructure.postgres.models import AgentRow, ApplicationActivityRow, ComputerSessionRow
from app.repositories.application_activity_repository import ApplicationActivityRepository

_BROWSER_EXECUTABLES = ("chrome.exe", "msedge.exe", "firefox.exe")

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

    def productivity_application_usage(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[ApplicationAggregate]:
        with self._session_factory() as session:
            stmt = (
                select(
                    ApplicationActivityRow.application_name,
                    func.coalesce(func.sum(ApplicationActivityRow.duration_seconds), 0),
                    func.count(),
                )
                .where(ApplicationActivityRow.agent_id == agent_id)
                .group_by(ApplicationActivityRow.application_name)
                .order_by(
                    desc(func.coalesce(func.sum(ApplicationActivityRow.duration_seconds), 0)),
                    ApplicationActivityRow.application_name,
                )
            )
            stmt = _apply_time_filters(stmt, started_at_from, started_at_to)
            return [
                ApplicationAggregate(
                    application=name or "",
                    duration_seconds=int(duration or 0),
                    launch_count=int(launches or 0),
                )
                for name, duration, launches in session.execute(stmt).all()
            ]

    def productivity_browser_title_usage(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[TitleAggregate]:
        with self._session_factory() as session:
            stmt = (
                select(
                    ApplicationActivityRow.window_title,
                    ApplicationActivityRow.executable_name,
                    func.coalesce(func.sum(ApplicationActivityRow.duration_seconds), 0),
                    func.count(),
                )
                .where(ApplicationActivityRow.agent_id == agent_id)
                .where(func.lower(ApplicationActivityRow.executable_name).in_(_BROWSER_EXECUTABLES))
                .group_by(
                    ApplicationActivityRow.window_title,
                    ApplicationActivityRow.executable_name,
                )
            )
            stmt = _apply_time_filters(stmt, started_at_from, started_at_to)
            return [
                TitleAggregate(
                    window_title=title or "",
                    executable_name=executable or "",
                    duration_seconds=int(duration or 0),
                    visit_count=int(visits or 0),
                )
                for title, executable, duration, visits in session.execute(stmt).all()
            ]

    def productivity_intervals(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> list[IntervalPoint]:
        with self._session_factory() as session:
            stmt = (
                select(
                    ApplicationActivityRow.started_at,
                    ApplicationActivityRow.stopped_at,
                    ApplicationActivityRow.duration_seconds,
                )
                .where(ApplicationActivityRow.agent_id == agent_id)
                .order_by(ApplicationActivityRow.started_at, ApplicationActivityRow.id)
            )
            stmt = _apply_time_filters(stmt, started_at_from, started_at_to)
            return [
                IntervalPoint(
                    started_at=started_at,
                    ended_at=interval_end(started_at, stopped_at, int(duration or 0)),
                )
                for started_at, stopped_at, duration in session.execute(stmt).all()
            ]


def _apply_time_filters(
    stmt: Select,
    started_at_from: datetime | None,
    started_at_to: datetime | None,
) -> Select:
    if started_at_from is not None:
        stmt = stmt.where(ApplicationActivityRow.started_at >= started_at_from)
    if started_at_to is not None:
        stmt = stmt.where(ApplicationActivityRow.started_at <= started_at_to)
    return stmt


def _apply_filters(stmt: Select, query: ListApplicationActivityQuery) -> Select:
    if query.agent_id is not None:
        stmt = stmt.where(ApplicationActivityRow.agent_id == query.agent_id)
    if query.session_id is not None:
        stmt = stmt.where(ApplicationActivityRow.session_id == query.session_id)
    if query.application_name:
        stmt = stmt.where(
            ApplicationActivityRow.application_name.ilike(f"%{query.application_name}%")
        )
    if query.started_at_from is not None:
        stmt = stmt.where(ApplicationActivityRow.started_at >= query.started_at_from)
    if query.started_at_to is not None:
        stmt = stmt.where(ApplicationActivityRow.started_at <= query.started_at_to)
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

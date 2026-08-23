from __future__ import annotations

from sqlalchemy import Select, asc, desc, func, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.orm import Session, sessionmaker

from app.domain.browser import (
    BrowserActivity,
    BrowserActivityPage,
    BrowserActivityRecord,
    ListBrowserActivityQuery,
    parse_sort,
)
from app.infrastructure.postgres.models import AgentRow, BrowserActivityRow, ComputerSessionRow
from app.repositories.browser_activity_repository import BrowserActivityRepository

_SORT_COLUMNS = {
    "started_at": BrowserActivityRow.started_at,
    "stopped_at": BrowserActivityRow.stopped_at,
    "executable_name": BrowserActivityRow.executable_name,
    "domain": BrowserActivityRow.domain,
    "duration_seconds": BrowserActivityRow.duration_seconds,
    "collected_at": BrowserActivityRow.collected_at,
    "received_at": BrowserActivityRow.received_at,
}


class PostgresBrowserActivityRepository(BrowserActivityRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def upsert_many(self, activities: list[BrowserActivity]) -> int:
        if not activities:
            return 0
        with self._session_factory() as session:
            dialect = session.bind.dialect.name if session.bind is not None else ""
            count = 0
            for item in activities:
                row = BrowserActivityRow.from_domain(item)
                values = {
                    column.name: getattr(row, column.name)
                    for column in BrowserActivityRow.__table__.columns
                }
                if dialect == "postgresql":
                    stmt = pg_insert(BrowserActivityRow).values(**values)
                    update_cols = {key: stmt.excluded[key] for key in values if key != "id"}
                    stmt = stmt.on_conflict_do_update(
                        index_elements=[BrowserActivityRow.id],
                        set_=update_cols,
                    )
                    session.execute(stmt)
                else:
                    existing = session.get(BrowserActivityRow, item.id)
                    if existing is None:
                        session.add(row)
                    else:
                        for key, value in values.items():
                            if key != "id":
                                setattr(existing, key, value)
                count += 1
            session.commit()
            return count

    def get(self, activity_id: str) -> BrowserActivityRecord | None:
        with self._session_factory() as session:
            stmt = (
                select(BrowserActivityRow, AgentRow.machine_name, ComputerSessionRow.username)
                .outerjoin(AgentRow, AgentRow.agent_id == BrowserActivityRow.agent_id)
                .outerjoin(
                    ComputerSessionRow,
                    ComputerSessionRow.session_id == BrowserActivityRow.session_id,
                )
                .where(BrowserActivityRow.id == activity_id)
            )
            row = session.execute(stmt).first()
            if row is None:
                return None
            item, machine_name, username = row
            return _to_record(item, machine_name or "", username or "")

    def list(self, query: ListBrowserActivityQuery) -> BrowserActivityPage:
        with self._session_factory() as session:
            base = (
                select(BrowserActivityRow, AgentRow.machine_name, ComputerSessionRow.username)
                .outerjoin(AgentRow, AgentRow.agent_id == BrowserActivityRow.agent_id)
                .outerjoin(
                    ComputerSessionRow,
                    ComputerSessionRow.session_id == BrowserActivityRow.session_id,
                )
            )
            base = _apply_filters(base, query)
            count_stmt = select(func.count()).select_from(base.subquery())
            total = int(session.execute(count_stmt).scalar_one())
            field, descending = parse_sort(query.sort)
            column = _SORT_COLUMNS[field]
            order = desc(column) if descending else asc(column)
            stmt = (
                base.order_by(order, BrowserActivityRow.id)
                .offset(query.offset)
                .limit(query.limit)
            )
            items = [
                _to_record(item, machine_name or "", username or "")
                for item, machine_name, username in session.execute(stmt).all()
            ]
            return BrowserActivityPage(
                items=items, total=total, limit=query.limit, offset=query.offset
            )


def _apply_filters(stmt: Select, query: ListBrowserActivityQuery) -> Select:
    if query.agent_id is not None:
        stmt = stmt.where(BrowserActivityRow.agent_id == query.agent_id)
    if query.session_id is not None:
        stmt = stmt.where(BrowserActivityRow.session_id == query.session_id)
    if query.domain:
        stmt = stmt.where(BrowserActivityRow.domain.ilike(f"%{query.domain}%"))
    if query.started_at_from is not None:
        stmt = stmt.where(BrowserActivityRow.started_at >= query.started_at_from)
    if query.started_at_to is not None:
        stmt = stmt.where(BrowserActivityRow.started_at <= query.started_at_to)
    return stmt


def _to_record(
    row: BrowserActivityRow, machine_name: str, username: str
) -> BrowserActivityRecord:
    return BrowserActivityRecord(
        id=row.id,
        session_id=row.session_id,
        agent_id=row.agent_id,
        executable_name=row.executable_name,
        domain=row.domain,
        url=row.url,
        title=row.title,
        profile=row.profile,
        window_id=int(row.window_id),
        tab_id=int(row.tab_id),
        started_at=row.started_at,
        stopped_at=row.stopped_at,
        duration_seconds=row.duration_seconds,
        collected_at=row.collected_at,
        received_at=row.received_at,
        username=username,
        machine_name=machine_name,
    )

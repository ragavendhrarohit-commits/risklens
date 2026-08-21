from __future__ import annotations

from sqlalchemy import Select, asc, desc, func, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.orm import Session, sessionmaker

from app.domain.software import (
    InstalledApplication,
    ListSoftwareQuery,
    SoftwarePage,
    SoftwareRecord,
    parse_sort,
)
from app.infrastructure.postgres.models import AgentRow, SoftwareInventoryRow
from app.repositories.software_repository import SoftwareRepository

_SORT_COLUMNS = {
    "application_name": SoftwareInventoryRow.application_name,
    "version": SoftwareInventoryRow.version,
    "publisher": SoftwareInventoryRow.publisher,
    "install_date": SoftwareInventoryRow.install_date,
    "architecture": SoftwareInventoryRow.architecture,
    "estimated_size": SoftwareInventoryRow.estimated_size,
    "machine_name": AgentRow.machine_name,
    "collected_at": SoftwareInventoryRow.collected_at,
    "received_at": SoftwareInventoryRow.received_at,
}


class PostgresSoftwareRepository(SoftwareRepository):
    def __init__(self, session_factory: sessionmaker[Session]) -> None:
        self._session_factory = session_factory

    def upsert_many(self, applications: list[InstalledApplication]) -> int:
        if not applications:
            return 0
        with self._session_factory() as session:
            dialect = session.bind.dialect.name if session.bind is not None else ""
            count = 0
            for app in applications:
                row = SoftwareInventoryRow.from_domain(app)
                values = {
                    column.name: getattr(row, column.name)
                    for column in SoftwareInventoryRow.__table__.columns
                }
                if dialect == "postgresql":
                    stmt = pg_insert(SoftwareInventoryRow).values(**values)
                    update_cols = {
                        key: stmt.excluded[key] for key in values if key != "id"
                    }
                    stmt = stmt.on_conflict_do_update(
                        index_elements=[SoftwareInventoryRow.id],
                        set_=update_cols,
                    )
                    session.execute(stmt)
                else:
                    existing = session.get(SoftwareInventoryRow, app.id)
                    if existing is None:
                        session.add(row)
                    else:
                        for key, value in values.items():
                            if key != "id":
                                setattr(existing, key, value)
                count += 1
            session.commit()
            return count

    def get(self, software_id: str) -> SoftwareRecord | None:
        with self._session_factory() as session:
            stmt = (
                select(SoftwareInventoryRow, AgentRow.machine_name)
                .outerjoin(AgentRow, AgentRow.agent_id == SoftwareInventoryRow.agent_id)
                .where(SoftwareInventoryRow.id == software_id)
            )
            row = session.execute(stmt).first()
            if row is None:
                return None
            app, machine_name = row
            return _to_record(app, machine_name or "")

    def list(self, query: ListSoftwareQuery) -> SoftwarePage:
        with self._session_factory() as session:
            base = select(SoftwareInventoryRow, AgentRow.machine_name).outerjoin(
                AgentRow, AgentRow.agent_id == SoftwareInventoryRow.agent_id
            )
            base = _apply_filters(base, query)
            count_stmt = select(func.count()).select_from(base.subquery())
            total = int(session.execute(count_stmt).scalar_one())

            field, descending = parse_sort(query.sort)
            column = _SORT_COLUMNS[field]
            order = desc(column) if descending else asc(column)
            stmt = (
                base.order_by(order, SoftwareInventoryRow.id)
                .offset(query.offset)
                .limit(query.limit)
            )
            items = [
                _to_record(app, machine_name or "")
                for app, machine_name in session.execute(stmt).all()
            ]
            return SoftwarePage(
                items=items,
                total=total,
                limit=query.limit,
                offset=query.offset,
            )


def _apply_filters(stmt: Select, query: ListSoftwareQuery) -> Select:
    if query.application_name:
        stmt = stmt.where(SoftwareInventoryRow.application_name.ilike(f"%{query.application_name}%"))
    if query.publisher:
        stmt = stmt.where(SoftwareInventoryRow.publisher.ilike(f"%{query.publisher}%"))
    if query.version:
        stmt = stmt.where(SoftwareInventoryRow.version.ilike(f"%{query.version}%"))
    if query.architecture:
        stmt = stmt.where(SoftwareInventoryRow.architecture.ilike(query.architecture))
    if query.agent_id is not None:
        stmt = stmt.where(SoftwareInventoryRow.agent_id == query.agent_id)
    return stmt


def _to_record(row: SoftwareInventoryRow, machine_name: str) -> SoftwareRecord:
    return SoftwareRecord(
        id=row.id,
        agent_id=row.agent_id,
        application_name=row.application_name,
        version=row.version,
        publisher=row.publisher,
        install_date=row.install_date,
        install_location=row.install_location,
        estimated_size=row.estimated_size,
        architecture=row.architecture,
        system_component=row.system_component,
        windows_component=row.windows_component,
        registry_path=row.registry_path,
        collected_at=row.collected_at,
        received_at=row.received_at,
        machine_name=machine_name,
    )

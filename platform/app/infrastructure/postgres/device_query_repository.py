from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from sqlalchemy import Select, asc, desc, func, select
from sqlalchemy.orm import Session, sessionmaker

from app.domain.device import DevicePage, DeviceRecord, ListDevicesQuery, parse_sort
from app.domain.heartbeat import compute_presence
from app.infrastructure.postgres.models import AgentHeartbeatRow, AgentRow, DeviceInventoryRow
from app.repositories.device_query_repository import DeviceQueryRepository

_SORT_COLUMNS = {
    "agent_id": DeviceInventoryRow.agent_id,
    "machine_name": DeviceInventoryRow.machine_name,
    "hostname": DeviceInventoryRow.hostname,
    "operating_system": DeviceInventoryRow.operating_system,
    "manufacturer": DeviceInventoryRow.manufacturer,
    "model": DeviceInventoryRow.model,
    "current_user": DeviceInventoryRow.current_user,
    "agent_version": DeviceInventoryRow.agent_version,
    "cpu_cores": DeviceInventoryRow.cpu_cores,
    "memory_bytes": DeviceInventoryRow.memory_bytes,
    "disk_total_bytes": DeviceInventoryRow.disk_total_bytes,
    "disk_free_bytes": DeviceInventoryRow.disk_free_bytes,
    "collected_at": DeviceInventoryRow.collected_at,
    "received_at": DeviceInventoryRow.received_at,
    "registration_status": AgentRow.status,
}


class PostgresDeviceQueryRepository(DeviceQueryRepository):
    """PostgreSQL read adapter with latest heartbeat presence fields."""

    def __init__(self, session_factory: sessionmaker[Session], *, clock=None) -> None:
        self._session_factory = session_factory
        self._clock = clock or (lambda: datetime.now(timezone.utc))

    def get(self, agent_id: UUID) -> DeviceRecord | None:
        with self._session_factory() as session:
            latest = _latest_heartbeat_subquery()
            stmt = (
                select(DeviceInventoryRow, AgentRow.status, latest.c.last_seen)
                .join(AgentRow, AgentRow.agent_id == DeviceInventoryRow.agent_id)
                .outerjoin(latest, latest.c.agent_id == DeviceInventoryRow.agent_id)
                .where(DeviceInventoryRow.agent_id == agent_id)
            )
            row = session.execute(stmt).first()
            if row is None:
                return None
            inventory, registration_status, last_seen = row
            return _to_record(inventory, registration_status, last_seen, self._clock())

    def list(self, query: ListDevicesQuery) -> DevicePage:
        with self._session_factory() as session:
            latest = _latest_heartbeat_subquery()
            base = (
                select(DeviceInventoryRow, AgentRow.status, latest.c.last_seen)
                .join(AgentRow, AgentRow.agent_id == DeviceInventoryRow.agent_id)
                .outerjoin(latest, latest.c.agent_id == DeviceInventoryRow.agent_id)
            )
            base = _apply_filters(base, query)

            count_stmt = select(func.count()).select_from(base.subquery())
            total = int(session.execute(count_stmt).scalar_one())

            field, descending = parse_sort(query.sort)
            now = self._clock()

            # Presence fields are derived in Python; for those sorts, fetch filtered
            # rows, sort in memory, then page. Inventory volume is agent-scale.
            if field in {"status", "last_seen", "heartbeat_age_seconds"}:
                rows = session.execute(base).all()
                items = [
                    _to_record(inventory, registration_status, last_seen, now)
                    for inventory, registration_status, last_seen in rows
                ]
                items.sort(key=lambda r: _presence_sort_key(r, field), reverse=descending)
                page_items = items[query.offset : query.offset + query.limit]
                return DevicePage(
                    items=page_items,
                    total=total,
                    limit=query.limit,
                    offset=query.offset,
                )

            column = _SORT_COLUMNS[field]
            order = desc(column) if descending else asc(column)
            stmt = (
                base.order_by(order, DeviceInventoryRow.agent_id)
                .offset(query.offset)
                .limit(query.limit)
            )
            items = [
                _to_record(inventory, registration_status, last_seen, now)
                for inventory, registration_status, last_seen in session.execute(stmt).all()
            ]
            return DevicePage(
                items=items,
                total=total,
                limit=query.limit,
                offset=query.offset,
            )


def _latest_heartbeat_subquery():
    return (
        select(
            AgentHeartbeatRow.agent_id.label("agent_id"),
            func.max(AgentHeartbeatRow.heartbeat_time).label("last_seen"),
        )
        .group_by(AgentHeartbeatRow.agent_id)
        .subquery()
    )


def _apply_filters(stmt: Select, query: ListDevicesQuery) -> Select:
    if query.hostname:
        stmt = stmt.where(DeviceInventoryRow.hostname.ilike(f"%{query.hostname}%"))
    if query.operating_system:
        stmt = stmt.where(
            DeviceInventoryRow.operating_system.ilike(f"%{query.operating_system}%")
        )
    if query.manufacturer:
        stmt = stmt.where(
            DeviceInventoryRow.manufacturer.ilike(f"%{query.manufacturer}%")
        )
    if query.current_user:
        stmt = stmt.where(
            DeviceInventoryRow.current_user.ilike(f"%{query.current_user}%")
        )
    return stmt


def _to_record(
    inventory: DeviceInventoryRow,
    registration_status: str,
    last_seen: datetime | None,
    now: datetime,
) -> DeviceRecord:
    presence, age = compute_presence(last_seen, now=now)
    return DeviceRecord(
        agent_id=inventory.agent_id,
        machine_name=inventory.machine_name,
        hostname=inventory.hostname,
        operating_system=inventory.operating_system,
        os_version=inventory.os_version,
        windows_build=inventory.windows_build,
        architecture=inventory.architecture,
        cpu_model=inventory.cpu_model,
        cpu_cores=inventory.cpu_cores,
        memory_bytes=inventory.memory_bytes,
        disk_total_bytes=inventory.disk_total_bytes,
        disk_free_bytes=inventory.disk_free_bytes,
        manufacturer=inventory.manufacturer,
        model=inventory.model,
        serial_number=inventory.serial_number,
        current_user=inventory.current_user,
        agent_version=inventory.agent_version,
        collected_at=inventory.collected_at,
        received_at=inventory.received_at,
        registration_status=registration_status,
        status=presence.value,
        last_seen=last_seen,
        heartbeat_age_seconds=age,
    )


def _presence_sort_key(record: DeviceRecord, field: str):
    value = getattr(record, field)
    if isinstance(value, str):
        return value.casefold()
    if value is None:
        return ""
    return value

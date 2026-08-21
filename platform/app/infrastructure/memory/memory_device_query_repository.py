from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from app.domain.agent import AgentStatus
from app.domain.device import DeviceInventory, DevicePage, DeviceRecord, ListDevicesQuery, parse_sort
from app.domain.heartbeat import compute_presence
from app.repositories.agent_repository import AgentRepository
from app.repositories.device_inventory_repository import DeviceInventoryRepository
from app.repositories.device_query_repository import DeviceQueryRepository
from app.repositories.heartbeat_repository import HeartbeatRepository


class MemoryDeviceQueryRepository(DeviceQueryRepository):
    """In-memory read adapter over inventory, agents, and heartbeats."""

    def __init__(
        self,
        inventory: DeviceInventoryRepository,
        agents: AgentRepository,
        heartbeats: HeartbeatRepository,
        *,
        clock=None,
    ) -> None:
        self._inventory = inventory
        self._agents = agents
        self._heartbeats = heartbeats
        self._clock = clock or (lambda: datetime.now(timezone.utc))

    def get(self, agent_id: UUID) -> DeviceRecord | None:
        row = self._inventory.get(agent_id)
        if row is None:
            return None
        return self._to_record(row)

    def list(self, query: ListDevicesQuery) -> DevicePage:
        rows = [self._to_record(item) for item in self._inventory.list()]
        filtered = [row for row in rows if self._matches(row, query)]
        field, descending = parse_sort(query.sort)
        filtered.sort(key=lambda r: self._sort_key(r, field), reverse=descending)
        total = len(filtered)
        page_items = filtered[query.offset : query.offset + query.limit]
        return DevicePage(
            items=page_items,
            total=total,
            limit=query.limit,
            offset=query.offset,
        )

    def _to_record(self, inventory: DeviceInventory) -> DeviceRecord:
        agent = self._agents.get(inventory.agent_id)
        registration = (
            agent.status.value
            if agent is not None
            else AgentStatus.registered.value
        )
        latest = self._heartbeats.get_latest(inventory.agent_id)
        last_seen = latest.heartbeat_time if latest is not None else None
        presence, age = compute_presence(last_seen, now=self._clock())
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
            registration_status=registration,
            status=presence.value,
            last_seen=last_seen,
            heartbeat_age_seconds=age,
        )

    @staticmethod
    def _matches(record: DeviceRecord, query: ListDevicesQuery) -> bool:
        return (
            _contains(record.hostname, query.hostname)
            and _contains(record.operating_system, query.operating_system)
            and _contains(record.manufacturer, query.manufacturer)
            and _contains(record.current_user, query.current_user)
        )

    @staticmethod
    def _sort_key(record: DeviceRecord, field: str):
        value = getattr(record, field)
        if isinstance(value, str):
            return value.casefold()
        if value is None:
            return ""
        return value


def _contains(haystack: str, needle: str | None) -> bool:
    if needle is None or needle == "":
        return True
    return needle.casefold() in haystack.casefold()

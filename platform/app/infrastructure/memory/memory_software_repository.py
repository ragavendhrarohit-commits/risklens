from __future__ import annotations

from threading import RLock

from app.domain.software import (
    InstalledApplication,
    ListSoftwareQuery,
    SoftwarePage,
    SoftwareRecord,
    parse_sort,
)
from app.repositories.agent_repository import AgentRepository
from app.repositories.software_repository import SoftwareRepository


class MemorySoftwareRepository(SoftwareRepository):
    def __init__(self, agents: AgentRepository | None = None) -> None:
        self._rows: dict[str, InstalledApplication] = {}
        self._agents = agents
        self._lock = RLock()

    def upsert_many(self, applications: list[InstalledApplication]) -> int:
        with self._lock:
            for app in applications:
                self._rows[app.id] = app
            return len(applications)

    def get(self, software_id: str) -> SoftwareRecord | None:
        with self._lock:
            row = self._rows.get(software_id)
            if row is None:
                return None
            return self._to_record(row)

    def list(self, query: ListSoftwareQuery) -> SoftwarePage:
        with self._lock:
            rows = [self._to_record(item) for item in self._rows.values()]
        filtered = [row for row in rows if self._matches(row, query)]
        field, descending = parse_sort(query.sort)
        filtered.sort(key=lambda r: _sort_key(r, field), reverse=descending)
        total = len(filtered)
        page = filtered[query.offset : query.offset + query.limit]
        return SoftwarePage(items=page, total=total, limit=query.limit, offset=query.offset)

    def clear(self) -> None:
        with self._lock:
            self._rows.clear()

    def _to_record(self, app: InstalledApplication) -> SoftwareRecord:
        machine_name = ""
        if self._agents is not None:
            agent = self._agents.get(app.agent_id)
            if agent is not None:
                machine_name = agent.machine_name
        return SoftwareRecord(
            id=app.id,
            agent_id=app.agent_id,
            application_name=app.application_name,
            version=app.version,
            publisher=app.publisher,
            install_date=app.install_date,
            install_location=app.install_location,
            estimated_size=app.estimated_size,
            architecture=app.architecture,
            system_component=app.system_component,
            windows_component=app.windows_component,
            registry_path=app.registry_path,
            collected_at=app.collected_at,
            received_at=app.received_at,
            machine_name=machine_name,
        )

    @staticmethod
    def _matches(record: SoftwareRecord, query: ListSoftwareQuery) -> bool:
        if query.application_name and query.application_name.casefold() not in record.application_name.casefold():
            return False
        if query.publisher and query.publisher.casefold() not in record.publisher.casefold():
            return False
        if query.version and query.version.casefold() not in record.version.casefold():
            return False
        if query.architecture and query.architecture.casefold() != record.architecture.casefold():
            return False
        if query.agent_id is not None and record.agent_id != query.agent_id:
            return False
        return True


def _sort_key(record: SoftwareRecord, field: str):
    value = getattr(record, field)
    if isinstance(value, str):
        return value.casefold()
    if value is None:
        return ""
    if isinstance(value, bool):
        return int(value)
    return value

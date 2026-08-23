from __future__ import annotations

from threading import RLock
from uuid import UUID

from app.domain.geolocation import AgentGeolocation
from app.repositories.geolocation_repository import GeolocationRepository


class MemoryGeolocationRepository(GeolocationRepository):
    def __init__(self) -> None:
        self._rows: dict[UUID, AgentGeolocation] = {}
        self._lock = RLock()

    def get(self, agent_id: UUID) -> AgentGeolocation | None:
        with self._lock:
            return self._rows.get(agent_id)

    def list_all(self) -> list[AgentGeolocation]:
        with self._lock:
            return list(self._rows.values())

    def upsert(self, row: AgentGeolocation) -> AgentGeolocation:
        with self._lock:
            self._rows[row.agent_id] = row
            return row

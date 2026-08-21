from __future__ import annotations

from threading import RLock
from uuid import UUID

from app.domain.heartbeat import Heartbeat
from app.repositories.heartbeat_repository import HeartbeatRepository


class MemoryHeartbeatRepository(HeartbeatRepository):
    """In-memory append-only heartbeat store."""

    def __init__(self) -> None:
        self._rows: list[Heartbeat] = []
        self._lock = RLock()

    def append(self, heartbeat: Heartbeat) -> Heartbeat:
        with self._lock:
            self._rows.append(heartbeat)
            return heartbeat

    def get_latest(self, agent_id: UUID) -> Heartbeat | None:
        with self._lock:
            matches = [row for row in self._rows if row.agent_id == agent_id]
            if not matches:
                return None
            return max(matches, key=lambda row: (row.heartbeat_time, row.received_at))

    def get_latest_map(self, agent_ids: list[UUID]) -> dict[UUID, Heartbeat]:
        result: dict[UUID, Heartbeat] = {}
        for agent_id in agent_ids:
            latest = self.get_latest(agent_id)
            if latest is not None:
                result[agent_id] = latest
        return result

    def list_for_agent(self, agent_id: UUID) -> list[Heartbeat]:
        with self._lock:
            rows = [row for row in self._rows if row.agent_id == agent_id]
            rows.sort(key=lambda row: (row.received_at, row.heartbeat_time))
            return rows

    def clear(self) -> None:
        with self._lock:
            self._rows.clear()

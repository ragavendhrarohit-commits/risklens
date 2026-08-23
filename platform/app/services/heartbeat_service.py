from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from app.domain.heartbeat import (
    AgentIDMismatchError,
    Heartbeat,
    HeartbeatRowStatus,
    RecordHeartbeatCommand,
    UnknownAgentError,
)
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.heartbeat_repository import HeartbeatRepository

_logger = get_logger("risklens.heartbeat")


class HeartbeatService:
    """Records agent heartbeats for registered endpoints."""

    def __init__(
        self,
        agents: AgentRepository,
        heartbeats: HeartbeatRepository,
        *,
        clock=None,
        projection=None,
        geolocation=None,
    ) -> None:
        self._agents = agents
        self._heartbeats = heartbeats
        self._clock = clock or _default_clock
        self._projection = projection
        self._geolocation = geolocation

    def record(
        self,
        path_agent_id: UUID,
        command: RecordHeartbeatCommand,
        *,
        client_ip: str | None = None,
    ) -> Heartbeat:
        if path_agent_id != command.agent_id:
            raise AgentIDMismatchError(path_agent_id, command.agent_id)
        if not self._agents.exists(command.agent_id):
            raise UnknownAgentError(command.agent_id)

        row = Heartbeat(
            agent_id=command.agent_id,
            heartbeat_time=command.timestamp,
            received_at=self._clock(),
            status=HeartbeatRowStatus.ok,
            health=command.health,
        )
        stored = self._heartbeats.append(row)
        if self._projection is not None:
            self._projection.on_heartbeat(stored.agent_id, stored.heartbeat_time, stored.received_at)
        if self._geolocation is not None:
            self._geolocation.ingest_heartbeat(command, client_ip=client_ip)
        _logger.info(
            "heartbeat recorded",
            extra={
                "agent_id": str(stored.agent_id),
                "heartbeat_time": stored.heartbeat_time.isoformat(),
            },
        )
        return stored


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)

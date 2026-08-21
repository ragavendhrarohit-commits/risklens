from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from enum import Enum
from uuid import UUID


class HeartbeatRowStatus(str, Enum):
    """Persisted status for an accepted heartbeat row."""

    ok = "ok"


class PresenceStatus(str, Enum):
    """Derived endpoint presence for the Devices page."""

    online = "online"
    stale = "stale"
    offline = "offline"


ONLINE_MAX_SECONDS = 60
STALE_MAX_SECONDS = 300


@dataclass(frozen=True, slots=True)
class Heartbeat:
    agent_id: UUID
    heartbeat_time: datetime
    received_at: datetime
    status: HeartbeatRowStatus = HeartbeatRowStatus.ok


@dataclass(frozen=True, slots=True)
class RecordHeartbeatCommand:
    agent_id: UUID
    install_id: str
    timestamp: datetime


def compute_presence(
    last_seen: datetime | None,
    *,
    now: datetime | None = None,
) -> tuple[PresenceStatus, int | None]:
    """Return (status, heartbeat_age_seconds).

    Online  <= 60s
    Stale   61–300s
    Offline > 300s or never seen
    """
    clock = now or datetime.now(timezone.utc)
    if clock.tzinfo is None:
        clock = clock.replace(tzinfo=timezone.utc)
    if last_seen is None:
        return PresenceStatus.offline, None

    seen = last_seen
    if seen.tzinfo is None:
        seen = seen.replace(tzinfo=timezone.utc)

    age = int((clock - seen).total_seconds())
    if age < 0:
        age = 0
    if age <= ONLINE_MAX_SECONDS:
        return PresenceStatus.online, age
    if age <= STALE_MAX_SECONDS:
        return PresenceStatus.stale, age
    return PresenceStatus.offline, age

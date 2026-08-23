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
class HeartbeatMemoryMetrics:
    total: int
    used: int
    free: int
    percent: float


@dataclass(frozen=True, slots=True)
class HeartbeatDiskMetrics:
    name: str
    total: int
    used: int
    free: int
    percent: float


@dataclass(frozen=True, slots=True)
class HeartbeatHealthMetrics:
    cpu_percent: float
    memory: HeartbeatMemoryMetrics
    disks: tuple[HeartbeatDiskMetrics, ...] = ()
    cpu_model: str | None = None
    logical_cpus: int | None = None
    physical_cores: int | None = None


@dataclass(frozen=True, slots=True)
class Heartbeat:
    agent_id: UUID
    heartbeat_time: datetime
    received_at: datetime
    status: HeartbeatRowStatus = HeartbeatRowStatus.ok
    health: HeartbeatHealthMetrics | None = None


@dataclass(frozen=True, slots=True)
class RecordHeartbeatCommand:
    agent_id: UUID
    install_id: str
    timestamp: datetime
    public_ip: str | None = None
    private_ip: str | None = None
    hostname: str | None = None
    health: HeartbeatHealthMetrics | None = None


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


def health_metrics_to_dict(health: HeartbeatHealthMetrics | None) -> dict | None:
    if health is None:
        return None
    return {
        "cpu_percent": health.cpu_percent,
        "cpu_model": health.cpu_model,
        "logical_cpus": health.logical_cpus,
        "physical_cores": health.physical_cores,
        "memory": {
            "total": health.memory.total,
            "used": health.memory.used,
            "free": health.memory.free,
            "percent": health.memory.percent,
        },
        "disks": [
            {
                "name": disk.name,
                "total": disk.total,
                "used": disk.used,
                "free": disk.free,
                "percent": disk.percent,
            }
            for disk in health.disks
        ],
    }


def health_metrics_from_dict(raw: dict | None) -> HeartbeatHealthMetrics | None:
    if not raw or not isinstance(raw, dict):
        return None
    memory_raw = raw.get("memory") or {}
    if not isinstance(memory_raw, dict):
        return None
    try:
        memory = HeartbeatMemoryMetrics(
            total=int(memory_raw["total"]),
            used=int(memory_raw["used"]),
            free=int(memory_raw["free"]),
            percent=float(memory_raw["percent"]),
        )
    except (KeyError, TypeError, ValueError):
        return None
    disks: list[HeartbeatDiskMetrics] = []
    for item in raw.get("disks") or []:
        if not isinstance(item, dict):
            continue
        try:
            disks.append(
                HeartbeatDiskMetrics(
                    name=str(item["name"]).strip(),
                    total=int(item["total"]),
                    used=int(item["used"]),
                    free=int(item["free"]),
                    percent=float(item["percent"]),
                )
            )
        except (KeyError, TypeError, ValueError):
            continue
    logical = raw.get("logical_cpus")
    physical = raw.get("physical_cores")
    try:
        return HeartbeatHealthMetrics(
            cpu_percent=float(raw.get("cpu_percent", 0)),
            memory=memory,
            disks=tuple(disk for disk in disks if disk.name and disk.total >= 0),
            cpu_model=(str(raw["cpu_model"]).strip() or None) if raw.get("cpu_model") is not None else None,
            logical_cpus=int(logical) if logical is not None else None,
            physical_cores=int(physical) if physical is not None else None,
        )
    except (TypeError, ValueError):
        return None

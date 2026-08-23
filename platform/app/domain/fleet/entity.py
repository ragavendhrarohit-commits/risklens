from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from uuid import UUID

from app.domain.heartbeat import PresenceStatus, compute_presence

DEFAULT_WINDOWS_POLICY_ID = UUID("11111111-1111-4111-8111-111111111111")
DEFAULT_WINDOWS_POLICY_NAME = "Default Windows Policy"
NOT_IMPLEMENTED = "Not implemented yet"

ALLOWED_TAGS: frozenset[str] = frozenset(
    {"Finance", "HR", "Production", "Development", "Servers", "Lab"}
)

ALLOWED_STATUS_FILTERS: frozenset[str] = frozenset(
    {"online", "warning", "offline", "never_seen", "healthy"}
)

ALLOWED_SORT_FIELDS: frozenset[str] = frozenset(
    {
        "agent_id",
        "hostname",
        "machine_name",
        "agent_version",
        "operating_system",
        "current_user",
        "registration_status",
        "online_status",
        "last_seen",
        "last_inventory_at",
        "policy_name",
    }
)

DEFAULT_SORT = "hostname"
DEFAULT_LIMIT = 50
MAX_LIMIT = 200


class FleetOnlineStatus(str, Enum):
    online = "online"
    warning = "warning"
    offline = "offline"
    never_seen = "never_seen"


def compute_fleet_status(
    last_seen: datetime | None,
    *,
    has_inventory: bool,
    now: datetime | None = None,
) -> tuple[FleetOnlineStatus, int | None, bool]:
    """Return (online_status, heartbeat_age_seconds, healthy)."""
    if last_seen is None:
        return FleetOnlineStatus.never_seen, None, False
    presence, age = compute_presence(last_seen, now=now)
    if presence is PresenceStatus.online:
        return FleetOnlineStatus.online, age, has_inventory
    if presence is PresenceStatus.stale:
        return FleetOnlineStatus.warning, age, False
    return FleetOnlineStatus.offline, age, False


@dataclass(frozen=True, slots=True)
class AgentStatusSnapshot:
    agent_id: UUID
    last_seen: datetime | None = None
    last_inventory_at: datetime | None = None
    last_communication_at: datetime | None = None
    ip_address: str | None = None


@dataclass(frozen=True, slots=True)
class Policy:
    id: UUID
    name: str
    heartbeat_interval_seconds: int
    collect_devices: bool
    collect_users: bool
    collect_software: bool
    collect_processes: bool
    collect_services: bool
    collect_event_logs: bool
    created_at: datetime


@dataclass(frozen=True, slots=True)
class PolicyAssignment:
    agent_id: UUID
    policy_id: UUID
    policy_name: str
    assigned_at: datetime
    policy: Policy


@dataclass(frozen=True, slots=True)
class FleetRecord:
    agent_id: UUID
    hostname: str
    machine_name: str
    agent_version: str
    operating_system: str
    current_user: str
    registration_status: str
    online_status: str
    last_seen: datetime | None
    last_inventory_at: datetime | None
    ip_address: str | None
    tags: list[str]
    policy_name: str
    policy_id: UUID | None = None
    heartbeat_age_seconds: int | None = None
    last_communication_at: datetime | None = None
    healthy: bool = False
    os_version: str = ""
    architecture: str = ""
    manufacturer: str = ""
    model: str = ""
    windows_build: str = ""
    cpu_model: str = ""
    cpu_cores: int | None = None
    memory_bytes: int | None = None
    disk_total_bytes: int | None = None
    disk_free_bytes: int | None = None
    serial_number: str = ""


@dataclass(frozen=True, slots=True)
class FleetSummary:
    total_agents: int
    online: int
    offline: int
    warning: int
    never_seen: int
    healthy: int


@dataclass(frozen=True, slots=True)
class ListFleetQuery:
    q: str | None = None
    status: str | None = None
    tag: str | None = None
    policy_id: UUID | None = None
    limit: int = DEFAULT_LIMIT
    offset: int = 0
    sort: str = DEFAULT_SORT

    def __post_init__(self) -> None:
        if self.limit < 1 or self.limit > MAX_LIMIT:
            raise ValueError(f"limit must be between 1 and {MAX_LIMIT}")
        if self.offset < 0:
            raise ValueError("offset must be >= 0")
        if self.status and self.status not in ALLOWED_STATUS_FILTERS:
            raise ValueError(f"unsupported status filter: {self.status}")
        if self.tag and self.tag not in ALLOWED_TAGS:
            raise ValueError(f"unsupported tag: {self.tag}")
        field, _ = parse_sort(self.sort)
        if field not in ALLOWED_SORT_FIELDS:
            raise ValueError(f"unsupported sort field: {field}")


@dataclass(frozen=True, slots=True)
class FleetPage:
    items: list[FleetRecord]
    summary: FleetSummary
    total: int
    limit: int
    offset: int


@dataclass(frozen=True, slots=True)
class FleetMemoryHealth:
    total: int
    used: int
    free: int
    percent: float


@dataclass(frozen=True, slots=True)
class FleetDiskHealth:
    name: str
    total: int
    used: int
    free: int
    percent: float


@dataclass(frozen=True, slots=True)
class FleetHealth:
    status: str
    heartbeat_age_seconds: int | None
    last_inventory_at: datetime | None
    last_communication_at: datetime | None
    cpu_percent: float | None = None
    cpu_model: str | None = None
    logical_cpus: int | None = None
    physical_cores: int | None = None
    memory: FleetMemoryHealth | None = None
    disks: tuple[FleetDiskHealth, ...] = ()
    memory_bytes: int | None = None
    disk_total_bytes: int | None = None
    disk_free_bytes: int | None = None


def parse_sort(sort: str) -> tuple[str, bool]:
    raw = (sort or DEFAULT_SORT).strip() or DEFAULT_SORT
    descending = raw.startswith("-")
    field = (raw[1:] if descending else raw).strip()
    if not field:
        raise ValueError("sort field must not be empty")
    return field, descending

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

# Sort keys accepted by the Devices Query API (prefix "-" for descending).
ALLOWED_SORT_FIELDS: frozenset[str] = frozenset(
    {
        "agent_id",
        "machine_name",
        "hostname",
        "operating_system",
        "manufacturer",
        "model",
        "current_user",
        "agent_version",
        "cpu_cores",
        "memory_bytes",
        "disk_total_bytes",
        "disk_free_bytes",
        "collected_at",
        "received_at",
        "registration_status",
        "status",
        "last_seen",
        "heartbeat_age_seconds",
    }
)

DEFAULT_SORT = "-received_at"
DEFAULT_LIMIT = 50
MAX_LIMIT = 200


@dataclass(frozen=True, slots=True)
class DeviceRecord:
    """Read model for the Devices page (inventory + registration status)."""

    agent_id: UUID
    machine_name: str
    hostname: str
    operating_system: str
    os_version: str
    windows_build: str
    architecture: str
    cpu_model: str
    cpu_cores: int
    memory_bytes: int
    disk_total_bytes: int
    disk_free_bytes: int
    manufacturer: str
    model: str
    serial_number: str
    current_user: str
    agent_version: str
    collected_at: datetime
    received_at: datetime
    registration_status: str
    status: str = "offline"
    last_seen: datetime | None = None
    heartbeat_age_seconds: int | None = None


@dataclass(frozen=True, slots=True)
class ListDevicesQuery:
    """Filters, pagination, and sort for listing device inventory."""

    hostname: str | None = None
    operating_system: str | None = None
    manufacturer: str | None = None
    current_user: str | None = None
    limit: int = DEFAULT_LIMIT
    offset: int = 0
    sort: str = DEFAULT_SORT

    def __post_init__(self) -> None:
        if self.limit < 1 or self.limit > MAX_LIMIT:
            raise ValueError(f"limit must be between 1 and {MAX_LIMIT}")
        if self.offset < 0:
            raise ValueError("offset must be >= 0")
        field, _ = parse_sort(self.sort)
        if field not in ALLOWED_SORT_FIELDS:
            raise ValueError(f"unsupported sort field: {field}")


@dataclass(frozen=True, slots=True)
class DevicePage:
    """Paginated list result."""

    items: list[DeviceRecord]
    total: int
    limit: int
    offset: int


def parse_sort(sort: str) -> tuple[str, bool]:
    """Return (field_name, descending)."""
    raw = (sort or DEFAULT_SORT).strip()
    if not raw:
        raw = DEFAULT_SORT
    descending = raw.startswith("-")
    field = raw[1:] if descending else raw
    field = field.strip()
    if not field:
        raise ValueError("sort field must not be empty")
    return field, descending

from __future__ import annotations

import hashlib
from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


def software_id(agent_id: UUID, application_name: str, version: str) -> str:
    """Deterministic upsert key: (agent_id, application_name, version).

    Using SHA1 hex means the id is:
      - stable across restarts (idempotent upserts),
      - safe for URLs (hex only),
      - fixed length so the DB column has a small index footprint.
    """
    canonical = f"{agent_id}\0{(application_name or '').strip().lower()}\0{(version or '').strip().lower()}"
    return hashlib.sha1(canonical.encode("utf-8")).hexdigest()


@dataclass(frozen=True, slots=True)
class InstalledApplication:
    id: str
    agent_id: UUID
    application_name: str
    version: str
    publisher: str
    install_date: str
    install_location: str
    estimated_size: int
    architecture: str
    system_component: bool
    windows_component: bool
    registry_path: str
    collected_at: datetime
    received_at: datetime

    def __post_init__(self) -> None:
        if not isinstance(self.id, str) or not self.id.strip():
            raise ValueError("id must be a non-empty string")
        if not isinstance(self.agent_id, UUID):
            raise TypeError("agent_id must be a UUID")
        if not isinstance(self.application_name, str) or not self.application_name.strip():
            raise ValueError("application_name must be a non-empty string")
        if self.estimated_size < 0:
            raise ValueError("estimated_size must be >= 0")


@dataclass(frozen=True, slots=True)
class SoftwareApplicationInput:
    application_name: str
    version: str = ""
    publisher: str = ""
    install_date: str = ""
    install_location: str = ""
    estimated_size: int = 0
    architecture: str = ""
    system_component: bool = False
    windows_component: bool = False
    registry_path: str = ""


@dataclass(frozen=True, slots=True)
class UpsertSoftwareCommand:
    agent_id: UUID
    install_id: str
    collected_at: datetime
    applications: list[SoftwareApplicationInput]


ALLOWED_SORT_FIELDS: frozenset[str] = frozenset(
    {
        "application_name",
        "version",
        "publisher",
        "install_date",
        "architecture",
        "estimated_size",
        "machine_name",
        "collected_at",
        "received_at",
    }
)

DEFAULT_SORT = "application_name"
DEFAULT_LIMIT = 50
MAX_LIMIT = 200


@dataclass(frozen=True, slots=True)
class SoftwareRecord:
    id: str
    agent_id: UUID
    application_name: str
    version: str
    publisher: str
    install_date: str
    install_location: str
    estimated_size: int
    architecture: str
    system_component: bool
    windows_component: bool
    registry_path: str
    collected_at: datetime
    received_at: datetime
    machine_name: str = ""


@dataclass(frozen=True, slots=True)
class ListSoftwareQuery:
    application_name: str | None = None
    publisher: str | None = None
    version: str | None = None
    architecture: str | None = None
    agent_id: UUID | None = None
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
class SoftwarePage:
    items: list[SoftwareRecord]
    total: int
    limit: int
    offset: int


def parse_sort(sort: str) -> tuple[str, bool]:
    raw = (sort or DEFAULT_SORT).strip() or DEFAULT_SORT
    descending = raw.startswith("-")
    field = (raw[1:] if descending else raw).strip()
    if not field:
        raise ValueError("sort field must not be empty")
    return field, descending

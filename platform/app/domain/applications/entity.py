from __future__ import annotations

import hashlib
from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


def activity_id(session_id: UUID, executable_name: str, started_at: datetime) -> str:
    started = started_at.isoformat()
    canonical = f"{session_id}\0{(executable_name or '').strip().lower()}\0{started}"
    return hashlib.sha1(canonical.encode("utf-8")).hexdigest()


ALLOWED_SORT_FIELDS: frozenset[str] = frozenset(
    {
        "started_at",
        "stopped_at",
        "application_name",
        "executable_name",
        "duration_seconds",
        "collected_at",
        "received_at",
        "username",
    }
)
DEFAULT_SORT = "-started_at"
DEFAULT_LIMIT = 50
MAX_LIMIT = 200


@dataclass(frozen=True, slots=True)
class ApplicationActivity:
    id: str
    session_id: UUID
    agent_id: UUID
    application_name: str
    executable_name: str
    window_title: str
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int
    collected_at: datetime
    received_at: datetime

    def __post_init__(self) -> None:
        if not isinstance(self.id, str) or not self.id.strip():
            raise ValueError("id must be a non-empty string")
        if not isinstance(self.session_id, UUID):
            raise TypeError("session_id must be a UUID")
        if not isinstance(self.agent_id, UUID):
            raise TypeError("agent_id must be a UUID")
        if not isinstance(self.application_name, str) or not self.application_name.strip():
            raise ValueError("application_name must be a non-empty string")
        if self.duration_seconds < 0:
            raise ValueError("duration_seconds must be >= 0")


@dataclass(frozen=True, slots=True)
class ApplicationActivityInput:
    session_id: UUID
    application_name: str
    executable_name: str = ""
    window_title: str = ""
    started_at: datetime | None = None
    stopped_at: datetime | None = None
    duration_seconds: int = 0
    id: str = ""


@dataclass(frozen=True, slots=True)
class UpsertApplicationActivityCommand:
    agent_id: UUID
    install_id: str
    collected_at: datetime
    activities: list[ApplicationActivityInput]


@dataclass(frozen=True, slots=True)
class ApplicationActivityRecord:
    id: str
    session_id: UUID
    agent_id: UUID
    application_name: str
    executable_name: str
    window_title: str
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int
    collected_at: datetime
    received_at: datetime
    username: str = ""
    machine_name: str = ""


@dataclass(frozen=True, slots=True)
class ListApplicationActivityQuery:
    agent_id: UUID | None = None
    session_id: UUID | None = None
    application_name: str | None = None
    started_at_from: datetime | None = None
    started_at_to: datetime | None = None
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
        if (
            self.started_at_from is not None
            and self.started_at_to is not None
            and self.started_at_from > self.started_at_to
        ):
            raise ValueError("started_at_from must be before started_at_to")


@dataclass(frozen=True, slots=True)
class ApplicationActivityPage:
    items: list[ApplicationActivityRecord]
    total: int
    limit: int
    offset: int


def parse_sort(sort: str) -> tuple[str, bool]:
    raw = (sort or DEFAULT_SORT).strip() or DEFAULT_SORT
    descending = raw.startswith("-")
    field = (raw[1:] if descending else raw).strip()
    return field or DEFAULT_SORT.lstrip("-"), descending

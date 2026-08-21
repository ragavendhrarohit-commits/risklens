from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True, slots=True)
class UserAccount:
    sid: str
    agent_id: UUID
    username: str
    display_name: str
    account_type: str
    enabled: bool
    builtin: bool
    password_required: bool
    last_logon: datetime | None
    source: str
    collected_at: datetime
    received_at: datetime

    def __post_init__(self) -> None:
        if not isinstance(self.sid, str) or not self.sid.strip():
            raise ValueError("sid must be a non-empty string")
        if not isinstance(self.agent_id, UUID):
            raise TypeError("agent_id must be a UUID")
        if not isinstance(self.username, str) or not self.username.strip():
            raise ValueError("username must be a non-empty string")


@dataclass(frozen=True, slots=True)
class UserAccountInput:
    username: str
    sid: str
    display_name: str = ""
    account_type: str = "Local"
    enabled: bool = True
    builtin: bool = False
    password_required: bool = True
    last_logon: datetime | None = None
    source: str = "Local"


@dataclass(frozen=True, slots=True)
class UpsertUsersCommand:
    agent_id: UUID
    install_id: str
    collected_at: datetime
    users: list[UserAccountInput]


ALLOWED_SORT_FIELDS: frozenset[str] = frozenset(
    {
        "username",
        "display_name",
        "sid",
        "account_type",
        "enabled",
        "last_logon",
        "machine_name",
        "collected_at",
        "received_at",
    }
)

DEFAULT_SORT = "username"
DEFAULT_LIMIT = 50
MAX_LIMIT = 200


@dataclass(frozen=True, slots=True)
class UserRecord:
    sid: str
    agent_id: UUID
    username: str
    display_name: str
    account_type: str
    enabled: bool
    builtin: bool
    password_required: bool
    last_logon: datetime | None
    source: str
    collected_at: datetime
    received_at: datetime
    machine_name: str = ""


@dataclass(frozen=True, slots=True)
class ListUsersQuery:
    username: str | None = None
    account_type: str | None = None
    enabled: bool | None = None
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
class UserPage:
    items: list[UserRecord]
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

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from uuid import UUID

SESSION_STATUS_ACTIVE = "active"
SESSION_STATUS_CLOSED = "closed"
ALLOWED_SESSION_STATUSES = frozenset({SESSION_STATUS_ACTIVE, SESSION_STATUS_CLOSED})

ALLOWED_SORT_FIELDS: frozenset[str] = frozenset(
    {
        "logon_at",
        "logoff_at",
        "username",
        "status",
        "collected_at",
        "received_at",
        "machine_name",
    }
)
DEFAULT_SORT = "-logon_at"
DEFAULT_LIMIT = 50
MAX_LIMIT = 200


@dataclass(frozen=True, slots=True)
class ComputerSession:
    session_id: UUID
    agent_id: UUID
    username: str
    account_sid: str
    logon_at: datetime
    logoff_at: datetime | None
    status: str
    collected_at: datetime
    received_at: datetime

    def __post_init__(self) -> None:
        if not isinstance(self.session_id, UUID):
            raise TypeError("session_id must be a UUID")
        if not isinstance(self.agent_id, UUID):
            raise TypeError("agent_id must be a UUID")
        if not isinstance(self.username, str) or not self.username.strip():
            raise ValueError("username must be a non-empty string")
        if self.status not in ALLOWED_SESSION_STATUSES:
            raise ValueError(f"unsupported session status: {self.status}")


@dataclass(frozen=True, slots=True)
class SessionInput:
    session_id: UUID
    username: str
    account_sid: str = ""
    logon_at: datetime | None = None
    logoff_at: datetime | None = None
    status: str = SESSION_STATUS_ACTIVE


@dataclass(frozen=True, slots=True)
class UpsertSessionsCommand:
    agent_id: UUID
    install_id: str
    collected_at: datetime
    sessions: list[SessionInput]


@dataclass(frozen=True, slots=True)
class SessionRecord:
    session_id: UUID
    agent_id: UUID
    username: str
    account_sid: str
    logon_at: datetime
    logoff_at: datetime | None
    status: str
    collected_at: datetime
    received_at: datetime
    machine_name: str = ""
    duration_seconds: int = 0


@dataclass(frozen=True, slots=True)
class ListSessionsQuery:
    agent_id: UUID | None = None
    status: str | None = None
    username: str | None = None
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
class SessionPage:
    items: list[SessionRecord]
    total: int
    limit: int
    offset: int


def parse_sort(sort: str) -> tuple[str, bool]:
    raw = (sort or DEFAULT_SORT).strip() or DEFAULT_SORT
    descending = raw.startswith("-")
    field = (raw[1:] if descending else raw).strip()
    return field or DEFAULT_SORT.lstrip("-"), descending


def session_duration_seconds(logon_at: datetime, logoff_at: datetime | None, now: datetime) -> int:
    end = logoff_at or now
    delta = int((end - logon_at).total_seconds())
    return max(delta, 0)


def earlier_datetime(left: datetime | None, right: datetime | None) -> datetime | None:
    if left is None:
        return right
    if right is None:
        return left
    return left if left <= right else right


def session_is_immutable(status: str, logoff_at: datetime | None) -> bool:
    """Historical sessions must never change once they have been closed."""
    if status == SESSION_STATUS_CLOSED:
        return True
    return logoff_at is not None


def _as_utc(value: datetime) -> datetime:
    if value.tzinfo is None:
        return value.replace(tzinfo=timezone.utc)
    return value.astimezone(timezone.utc)


def can_reopen_closed_session(
    existing: ComputerSession,
    incoming: ComputerSession,
    *,
    known_sessions: list[ComputerSession] | list | None = None,
) -> bool:
    """Allow reopen when the agent reports the same Windows logon as still active.

    Offline reconciliation may close a session while the user is still logged in.
    Only the chronologically latest session for an agent may reopen — older closed
    rows stay historical even if an ACTIVE heartbeat is replayed.
    """
    if existing.status != SESSION_STATUS_CLOSED and existing.logoff_at is None:
        return False
    if incoming.status != SESSION_STATUS_ACTIVE:
        return False
    if incoming.logoff_at is not None:
        return False
    if _as_utc(existing.logon_at) != _as_utc(incoming.logon_at):
        return False
    if known_sessions:
        existing_logon = _as_utc(existing.logon_at)
        for other in known_sessions:
            other_id = getattr(other, "session_id", None)
            if other_id == existing.session_id:
                continue
            other_logon = getattr(other, "logon_at", None)
            if other_logon is not None and _as_utc(other_logon) > existing_logon:
                return False
    return True


def cap_logoff_at(
    logon_at: datetime,
    requested: datetime,
    next_logon_at: datetime | None,
) -> datetime:
    """Bound a fallback logout so it cannot overlap a later session."""
    end = requested
    if next_logon_at is not None and next_logon_at < end:
        end = next_logon_at
    if end < logon_at:
        return logon_at
    return end


def merge_closed_logoff(
    logon_at: datetime,
    existing_logoff: datetime | None,
    incoming_logoff: datetime | None,
    next_logon_at: datetime | None,
) -> datetime | None:
    """Choose a logout time for an agent close of an ACTIVE session."""
    if existing_logoff is not None:
        return existing_logoff
    if incoming_logoff is None:
        return None
    return cap_logoff_at(logon_at, incoming_logoff, next_logon_at)


def merge_fallback_close(
    existing: ComputerSession,
    logoff_at: datetime,
    received_at: datetime,
    next_logon_at: datetime | None,
) -> ComputerSession | None:
    """Offline / next-login close. Never touch a CLOSED session."""
    if session_is_immutable(existing.status, existing.logoff_at):
        return None
    if existing.status != SESSION_STATUS_ACTIVE:
        return None
    end = cap_logoff_at(existing.logon_at, logoff_at, next_logon_at)
    return ComputerSession(
        session_id=existing.session_id,
        agent_id=existing.agent_id,
        username=existing.username,
        account_sid=existing.account_sid,
        logon_at=existing.logon_at,
        logoff_at=end,
        status=SESSION_STATUS_CLOSED,
        collected_at=received_at,
        received_at=received_at,
    )


def merge_session(
    existing: ComputerSession | None,
    incoming: ComputerSession,
    next_logon_at: datetime | None = None,
    *,
    known_sessions: list[ComputerSession] | list | None = None,
) -> ComputerSession:
    """Idempotent agent upsert.

    - Keep the original login time. Never invent a login timestamp.
    - Closed sessions are immutable unless the agent reports the same latest logon as ACTIVE.
    - Next-login / offline close is a fallback only while the row is Active.
    """
    if existing is None:
        return incoming
    if can_reopen_closed_session(existing, incoming, known_sessions=known_sessions):
        return ComputerSession(
            session_id=existing.session_id,
            agent_id=existing.agent_id,
            username=incoming.username or existing.username,
            account_sid=incoming.account_sid or existing.account_sid,
            logon_at=existing.logon_at,
            logoff_at=None,
            status=SESSION_STATUS_ACTIVE,
            collected_at=incoming.collected_at,
            received_at=incoming.received_at,
        )
    if session_is_immutable(existing.status, existing.logoff_at):
        return existing
    logon_at = existing.logon_at
    if incoming.status == SESSION_STATUS_CLOSED:
        incoming_logoff = incoming.logoff_at or incoming.collected_at
        return ComputerSession(
            session_id=existing.session_id,
            agent_id=existing.agent_id,
            username=incoming.username or existing.username,
            account_sid=incoming.account_sid or existing.account_sid,
            logon_at=logon_at,
            logoff_at=cap_logoff_at(logon_at, incoming_logoff, next_logon_at),
            status=SESSION_STATUS_CLOSED,
            collected_at=incoming.collected_at,
            received_at=incoming.received_at,
        )
    return ComputerSession(
        session_id=incoming.session_id,
        agent_id=incoming.agent_id,
        username=incoming.username,
        account_sid=incoming.account_sid,
        logon_at=logon_at,
        logoff_at=None,
        status=SESSION_STATUS_ACTIVE,
        collected_at=incoming.collected_at,
        received_at=incoming.received_at,
    )


def session_from_record(record: SessionRecord) -> ComputerSession:
    return ComputerSession(
        session_id=record.session_id,
        agent_id=record.agent_id,
        username=record.username,
        account_sid=record.account_sid,
        logon_at=record.logon_at,
        logoff_at=record.logoff_at,
        status=record.status,
        collected_at=record.collected_at,
        received_at=record.received_at,
    )

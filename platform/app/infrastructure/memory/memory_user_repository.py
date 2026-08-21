from __future__ import annotations

from threading import RLock

from app.domain.user import (
    ListUsersQuery,
    UserAccount,
    UserPage,
    UserRecord,
    parse_sort,
)
from app.repositories.agent_repository import AgentRepository
from app.repositories.user_repository import UserRepository


class MemoryUserRepository(UserRepository):
    def __init__(self, agents: AgentRepository | None = None) -> None:
        self._rows: dict[str, UserAccount] = {}
        self._agents = agents
        self._lock = RLock()

    def upsert_many(self, accounts: list[UserAccount]) -> int:
        with self._lock:
            for account in accounts:
                self._rows[account.sid] = account
            return len(accounts)

    def get(self, sid: str) -> UserRecord | None:
        with self._lock:
            row = self._rows.get(sid)
            if row is None:
                return None
            return self._to_record(row)

    def list(self, query: ListUsersQuery) -> UserPage:
        with self._lock:
            rows = [self._to_record(item) for item in self._rows.values()]
        filtered = [row for row in rows if self._matches(row, query)]
        field, descending = parse_sort(query.sort)
        filtered.sort(key=lambda r: _sort_key(r, field), reverse=descending)
        total = len(filtered)
        page = filtered[query.offset : query.offset + query.limit]
        return UserPage(items=page, total=total, limit=query.limit, offset=query.offset)

    def clear(self) -> None:
        with self._lock:
            self._rows.clear()

    def _to_record(self, account: UserAccount) -> UserRecord:
        machine_name = ""
        if self._agents is not None:
            agent = self._agents.get(account.agent_id)
            if agent is not None:
                machine_name = agent.machine_name
        return UserRecord(
            sid=account.sid,
            agent_id=account.agent_id,
            username=account.username,
            display_name=account.display_name,
            account_type=account.account_type,
            enabled=account.enabled,
            builtin=account.builtin,
            password_required=account.password_required,
            last_logon=account.last_logon,
            source=account.source,
            collected_at=account.collected_at,
            received_at=account.received_at,
            machine_name=machine_name,
        )

    @staticmethod
    def _matches(record: UserRecord, query: ListUsersQuery) -> bool:
        if query.username and query.username.casefold() not in record.username.casefold():
            return False
        if query.account_type and query.account_type.casefold() not in record.account_type.casefold():
            return False
        if query.enabled is not None and record.enabled is not query.enabled:
            return False
        if query.agent_id is not None and record.agent_id != query.agent_id:
            return False
        return True


def _sort_key(record: UserRecord, field: str):
    value = getattr(record, field)
    if isinstance(value, str):
        return value.casefold()
    if value is None:
        return ""
    if isinstance(value, bool):
        return int(value)
    return value

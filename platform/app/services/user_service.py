from __future__ import annotations

from datetime import datetime, timezone

from app.domain.user import (
    UnknownAgentError,
    UpsertUsersCommand,
    UserAccount,
    UserNotFoundError,
    UserPage,
    UserRecord,
    ListUsersQuery,
)
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.user_repository import UserRepository

_logger = get_logger("risklens.users")


class UserInventoryService:
    """Accepts and queries Windows user inventory from registered agents."""

    def __init__(
        self,
        agents: AgentRepository,
        users: UserRepository,
        *,
        clock=None,
    ) -> None:
        self._agents = agents
        self._users = users
        self._clock = clock or _default_clock

    def upsert(self, command: UpsertUsersCommand) -> tuple[int, datetime]:
        if not self._agents.exists(command.agent_id):
            raise UnknownAgentError(command.agent_id)

        received_at = self._clock()
        accounts = [
            UserAccount(
                sid=item.sid.strip(),
                agent_id=command.agent_id,
                username=item.username.strip(),
                display_name=(item.display_name or "").strip(),
                account_type=(item.account_type or "Local").strip() or "Local",
                enabled=bool(item.enabled),
                builtin=bool(item.builtin),
                password_required=bool(item.password_required),
                last_logon=item.last_logon,
                source=(item.source or "Local").strip() or "Local",
                collected_at=command.collected_at,
                received_at=received_at,
            )
            for item in command.users
            if item.sid.strip() and item.username.strip()
        ]
        stored = self._users.upsert_many(accounts)
        _logger.info(
            "user inventory stored",
            extra={"agent_id": str(command.agent_id), "stored": stored},
        )
        return stored, received_at

    def get(self, sid: str) -> UserRecord:
        record = self._users.get(sid)
        if record is None:
            raise UserNotFoundError(sid)
        return record

    def list(self, query: ListUsersQuery) -> UserPage:
        return self._users.list(query)


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)

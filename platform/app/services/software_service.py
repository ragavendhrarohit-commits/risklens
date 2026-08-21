from __future__ import annotations

from datetime import datetime, timezone

from app.domain.software import (
    InstalledApplication,
    ListSoftwareQuery,
    SoftwareNotFoundError,
    SoftwarePage,
    SoftwareRecord,
    UnknownAgentError,
    UpsertSoftwareCommand,
    software_id,
)
from app.logging.setup import get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.software_repository import SoftwareRepository

_logger = get_logger("risklens.software")


class SoftwareInventoryService:
    """Accepts and queries installed software inventory from registered agents."""

    def __init__(
        self,
        agents: AgentRepository,
        software: SoftwareRepository,
        *,
        clock=None,
    ) -> None:
        self._agents = agents
        self._software = software
        self._clock = clock or _default_clock

    def upsert(self, command: UpsertSoftwareCommand) -> tuple[int, datetime]:
        if not self._agents.exists(command.agent_id):
            raise UnknownAgentError(command.agent_id)

        received_at = self._clock()
        seen: dict[str, InstalledApplication] = {}
        for item in command.applications:
            name = (item.application_name or "").strip()
            if not name:
                continue
            version = (item.version or "").strip()
            key = software_id(command.agent_id, name, version)
            app = InstalledApplication(
                id=key,
                agent_id=command.agent_id,
                application_name=name,
                version=version,
                publisher=(item.publisher or "").strip(),
                install_date=(item.install_date or "").strip(),
                install_location=(item.install_location or "").strip(),
                estimated_size=max(int(item.estimated_size or 0), 0),
                architecture=(item.architecture or "").strip(),
                system_component=bool(item.system_component),
                windows_component=bool(item.windows_component),
                registry_path=(item.registry_path or "").strip(),
                collected_at=command.collected_at,
                received_at=received_at,
            )
            # Duplicate rows within one payload collapse to the last-seen entry.
            seen[key] = app

        applications = list(seen.values())
        stored = self._software.upsert_many(applications)
        _logger.info(
            "software inventory stored",
            extra={"agent_id": str(command.agent_id), "stored": stored},
        )
        return stored, received_at

    def get(self, software_id_value: str) -> SoftwareRecord:
        record = self._software.get(software_id_value)
        if record is None:
            raise SoftwareNotFoundError(software_id_value)
        return record

    def list(self, query: ListSoftwareQuery) -> SoftwarePage:
        return self._software.list(query)


def _default_clock() -> datetime:
    return datetime.now(timezone.utc)

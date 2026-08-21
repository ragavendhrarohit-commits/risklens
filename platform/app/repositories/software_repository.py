from __future__ import annotations

from abc import ABC, abstractmethod

from app.domain.software import (
    InstalledApplication,
    ListSoftwareQuery,
    SoftwarePage,
    SoftwareRecord,
)


class SoftwareRepository(ABC):
    """Persistence port for installed software inventory.

    Rows are keyed by (agent_id, application_name, version) via a deterministic
    ``id`` hash; ``upsert_many`` writes the latest snapshot for each unique
    tuple within one agent's inventory.
    """

    @abstractmethod
    def upsert_many(self, applications: list[InstalledApplication]) -> int:
        """Upsert applications by id. Returns number of rows written."""

    @abstractmethod
    def get(self, software_id: str) -> SoftwareRecord | None:
        """Return one application record (with machine_name when available)."""

    @abstractmethod
    def list(self, query: ListSoftwareQuery) -> SoftwarePage:
        """Filtered, sorted, paginated software inventory."""

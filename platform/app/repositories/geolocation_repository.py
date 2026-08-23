from __future__ import annotations

from abc import ABC, abstractmethod
from uuid import UUID

from app.domain.geolocation import AgentGeolocation


class GeolocationRepository(ABC):
    @abstractmethod
    def get(self, agent_id: UUID) -> AgentGeolocation | None: ...

    @abstractmethod
    def list_all(self) -> list[AgentGeolocation]: ...

    @abstractmethod
    def upsert(self, row: AgentGeolocation) -> AgentGeolocation: ...

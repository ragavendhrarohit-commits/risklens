from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True, slots=True)
class AgentGeolocation:
    """Approximate IP-based location for one managed endpoint."""

    agent_id: UUID
    public_ip: str | None = None
    private_ip: str | None = None
    country: str | None = None
    state: str | None = None
    city: str | None = None
    latitude: float | None = None
    longitude: float | None = None
    timezone: str | None = None
    isp: str | None = None
    asn: str | None = None
    last_updated: datetime | None = None


@dataclass(frozen=True, slots=True)
class GeolocationDevice:
    """Fleet-enriched location row for the Geo Location console."""

    agent_id: UUID
    computer: str
    user: str
    hostname: str
    status: str
    city: str | None
    state: str | None
    country: str | None
    public_ip: str | None
    private_ip: str | None
    isp: str | None
    timezone: str | None
    latitude: float | None
    longitude: float | None
    asn: str | None
    last_seen: datetime | None
    operating_system: str
    online: bool


@dataclass(frozen=True, slots=True)
class GeolocationSummary:
    managed_devices: int
    online_devices: int
    countries: int
    cities: int


@dataclass(frozen=True, slots=True)
class GeolocationPage:
    summary: GeolocationSummary
    items: list[GeolocationDevice]
    total: int


@dataclass(frozen=True, slots=True)
class ListGeolocationQuery:
    q: str = ""
    status: str = ""
    country: str = ""
    city: str = ""
    isp: str = ""


@dataclass(frozen=True, slots=True)
class UpsertGeolocationCommand:
    agent_id: UUID
    public_ip: str | None
    private_ip: str | None
    hostname: str | None
    at: datetime

from __future__ import annotations

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class GeolocationSummaryResponse(BaseModel):
    managed_devices: int
    online_devices: int
    countries: int
    cities: int


class GeolocationItemResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    agent_id: UUID
    computer: str
    user: str
    hostname: str
    status: str
    city: str | None = None
    state: str | None = None
    country: str | None = None
    public_ip: str | None = None
    private_ip: str | None = None
    isp: str | None = None
    timezone: str | None = None
    latitude: float | None = None
    longitude: float | None = None
    asn: str | None = None
    last_seen: datetime | None = None
    operating_system: str = ""
    online: bool = False


class GeolocationListResponse(BaseModel):
    summary: GeolocationSummaryResponse
    items: list[GeolocationItemResponse]
    total: int


class GeolocationDetailResponse(GeolocationItemResponse):
    pass

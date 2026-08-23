from __future__ import annotations

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class FleetSummaryResponse(BaseModel):
    total_agents: int
    online: int
    offline: int
    warning: int
    never_seen: int
    healthy: int


class FleetItemResponse(BaseModel):
    agent_id: UUID
    hostname: str
    machine_name: str
    agent_version: str
    operating_system: str
    current_user: str
    registration_status: str
    online_status: str
    last_seen: datetime | None
    last_inventory_at: datetime | None
    ip_address: str | None
    tags: list[str]
    policy_name: str
    heartbeat_age_seconds: int | None = None
    last_communication_at: datetime | None = None
    os_version: str = ""
    architecture: str = ""
    manufacturer: str = ""
    model: str = ""
    windows_build: str = ""
    cpu_model: str = ""
    cpu_cores: int | None = None
    memory_bytes: int | None = None
    disk_total_bytes: int | None = None
    disk_free_bytes: int | None = None
    serial_number: str = ""


class FleetListResponse(BaseModel):
    summary: FleetSummaryResponse
    items: list[FleetItemResponse]
    total: int
    limit: int
    offset: int


class FleetMemoryHealthResponse(BaseModel):
    total: int
    used: int
    free: int
    percent: float


class FleetDiskHealthResponse(BaseModel):
    name: str
    total: int
    used: int
    free: int
    percent: float


class FleetHealthResponse(BaseModel):
    status: str
    heartbeat_age_seconds: int | None
    last_inventory_at: datetime | None
    last_communication_at: datetime | None
    cpu_percent: float | None = None
    cpu_model: str | None = None
    logical_cpus: int | None = None
    physical_cores: int | None = None
    memory: FleetMemoryHealthResponse | None = None
    disks: list[FleetDiskHealthResponse] = Field(default_factory=list)
    memory_bytes: int | None = None
    disk_total_bytes: int | None = None
    disk_free_bytes: int | None = None


class WebsiteUsageItemResponse(BaseModel):
    browser: str
    website: str
    duration_seconds: int
    category: str
    total_time: str


class WebsiteUsageListResponse(BaseModel):
    items: list[WebsiteUsageItemResponse]
    total: int


class BrowserVisitResponse(BaseModel):
    started_at: datetime
    stopped_at: datetime | None
    duration_seconds: int
    status: str
    page_title: str = ""
    window_title: str = ""
    website: str = ""
    browser: str = ""


class BrowserActivityItemResponse(BaseModel):
    website: str
    browser: str
    page_title: str = ""
    duration_seconds: int
    total_time: str
    visits: int
    last_visited: datetime
    status: str
    timeline: list[BrowserVisitResponse]


class BrowserActivityListResponse(BaseModel):
    items: list[BrowserActivityItemResponse]
    total: int
    limit: int
    offset: int
    total_browser_time_seconds: int
    total_browser_time: str
    websites_visited: int
    active_website: str | None = None
    active_browser: str | None = None

from __future__ import annotations

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

from app.domain.fleet import NOT_IMPLEMENTED


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


class FleetHealthResponse(BaseModel):
    status: str
    heartbeat_age_seconds: int | None
    last_inventory_at: datetime | None
    last_communication_at: datetime | None
    cpu_usage: str = NOT_IMPLEMENTED
    memory_usage: str = NOT_IMPLEMENTED
    disk_usage: str = NOT_IMPLEMENTED
    memory_bytes: int | None = None
    disk_total_bytes: int | None = None
    disk_free_bytes: int | None = None


class FleetPolicyResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    id: UUID
    name: str
    heartbeat_interval_seconds: int
    collect_devices: bool
    collect_users: bool
    collect_software: bool
    collect_processes: bool = Field(description="Reserved for a later sprint.")
    collect_services: bool = Field(description="Reserved for a later sprint.")
    collect_event_logs: bool = Field(description="Reserved for a later sprint.")
    created_at: datetime


class FleetTimelineResponse(BaseModel):
    status: str = NOT_IMPLEMENTED


class WebsiteUsageItemResponse(BaseModel):
    browser: str
    website: str
    duration_seconds: int
    category: str
    total_time: str


class WebsiteUsageListResponse(BaseModel):
    items: list[WebsiteUsageItemResponse]
    total: int

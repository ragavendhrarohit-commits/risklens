from __future__ import annotations

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class ProductivityEmployeeResponse(BaseModel):
    agent_id: UUID
    computer: str
    user: str
    department: str | None = None
    report_period_start: datetime | None = None
    report_period_end: datetime | None = None


class ProductivitySummaryResponse(BaseModel):
    working_time_seconds: int
    working_time: str
    active_time_seconds: int
    active_time: str
    idle_time_seconds: int
    idle_time: str
    applications_used: int
    websites_visited: int
    average_session_length_seconds: int
    average_session_length: str


class ProductivityApplicationRowResponse(BaseModel):
    application: str
    duration_seconds: int
    percentage: float
    launch_count: int
    total_time: str


class ProductivityWebsiteRowResponse(BaseModel):
    website: str
    duration_seconds: int
    visits: int
    total_time: str


class ProductivityActivitySummaryResponse(BaseModel):
    first_activity: datetime | None = None
    last_activity: datetime | None = None
    longest_continuous_working_session_seconds: int = 0
    longest_continuous_working_session: str = "0s"
    longest_idle_period_seconds: int = 0
    longest_idle_period: str = "0s"
    most_used_application: str | None = None
    most_used_website: str | None = None


class ProductivityReportResponse(BaseModel):
    employee: ProductivityEmployeeResponse
    summary: ProductivitySummaryResponse
    applications: list[ProductivityApplicationRowResponse] = Field(default_factory=list)
    websites: list[ProductivityWebsiteRowResponse] = Field(default_factory=list)
    activity: ProductivityActivitySummaryResponse
    empty: bool = False

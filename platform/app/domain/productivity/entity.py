from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True, slots=True)
class ApplicationUsageRow:
    application: str
    duration_seconds: int
    percentage: float
    launch_count: int
    total_time: str


@dataclass(frozen=True, slots=True)
class WebsiteUsageRow:
    website: str
    duration_seconds: int
    visits: int
    total_time: str


@dataclass(frozen=True, slots=True)
class ProductivityEmployee:
    agent_id: UUID
    computer: str
    user: str
    department: str | None
    report_period_start: datetime | None
    report_period_end: datetime | None


@dataclass(frozen=True, slots=True)
class ProductivitySummary:
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


@dataclass(frozen=True, slots=True)
class ProductivityActivitySummary:
    first_activity: datetime | None
    last_activity: datetime | None
    longest_continuous_working_session_seconds: int
    longest_continuous_working_session: str
    longest_idle_period_seconds: int
    longest_idle_period: str
    most_used_application: str | None
    most_used_website: str | None


@dataclass(frozen=True, slots=True)
class ProductivityReport:
    employee: ProductivityEmployee
    summary: ProductivitySummary
    applications: list[ApplicationUsageRow] = field(default_factory=list)
    websites: list[WebsiteUsageRow] = field(default_factory=list)
    activity: ProductivityActivitySummary | None = None
    empty: bool = False

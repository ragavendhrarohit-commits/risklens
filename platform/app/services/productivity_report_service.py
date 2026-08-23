from __future__ import annotations

from datetime import datetime
from uuid import UUID

from app.domain.fleet import FleetAgentNotFoundError
from app.domain.productivity import (
    ApplicationUsageRow,
    ProductivityActivitySummary,
    ProductivityEmployee,
    ProductivityReport,
    ProductivitySummary,
    WebsiteUsageRow,
    application_usage_rows,
    average_session_length_seconds,
    idle_time_seconds,
    merge_span_metrics,
    rollup_websites,
)
from app.domain.website_usage import format_total_time
from app.repositories.agent_repository import AgentRepository
from app.repositories.application_activity_repository import ApplicationActivityRepository
from app.repositories.device_inventory_repository import DeviceInventoryRepository
from app.repositories.fleet_repository import AgentTagRepository


class ProductivityReportService:
    """Build manager Productivity Reports from existing Application Activity."""

    def __init__(
        self,
        agents: AgentRepository,
        inventory: DeviceInventoryRepository,
        applications: ApplicationActivityRepository,
        tags: AgentTagRepository,
    ) -> None:
        self._agents = agents
        self._inventory = inventory
        self._applications = applications
        self._tags = tags

    def build(
        self,
        agent_id: UUID,
        *,
        started_at_from: datetime | None = None,
        started_at_to: datetime | None = None,
    ) -> ProductivityReport:
        agent = self._agents.get(agent_id)
        if agent is None:
            raise FleetAgentNotFoundError(agent_id)

        inventory = self._inventory.get(agent_id)
        computer = (
            (inventory.hostname if inventory and inventory.hostname else "")
            or (inventory.machine_name if inventory and inventory.machine_name else "")
            or agent.machine_name
            or agent.display_name
            or str(agent_id)
        )
        user = (inventory.current_user if inventory is not None else "") or ""
        tag_list = self._tags.list_for_agent(agent_id)
        department = ", ".join(tag_list) if tag_list else None

        app_aggs = self._applications.productivity_application_usage(
            agent_id,
            started_at_from=started_at_from,
            started_at_to=started_at_to,
        )
        title_aggs = self._applications.productivity_browser_title_usage(
            agent_id,
            started_at_from=started_at_from,
            started_at_to=started_at_to,
        )
        intervals = self._applications.productivity_intervals(
            agent_id,
            started_at_from=started_at_from,
            started_at_to=started_at_to,
        )

        active_time = sum(max(0, int(row.duration_seconds or 0)) for row in app_aggs)
        first_activity = intervals[0].started_at if intervals else None
        last_activity = max((row.ended_at for row in intervals), default=None)
        span = merge_span_metrics(
            first_activity=first_activity,
            last_activity=last_activity,
            active_time_seconds=active_time,
            segment_count=sum(max(0, int(row.launch_count or 0)) for row in app_aggs),
            applications_used=len(app_aggs),
            intervals=intervals,
        )

        app_rows = [
            ApplicationUsageRow(
                application=name,
                duration_seconds=duration,
                percentage=percentage,
                launch_count=launches,
                total_time=total_time,
            )
            for name, duration, percentage, launches, total_time in application_usage_rows(
                app_aggs,
                active_time_seconds=span.active_time_seconds,
            )
        ]
        website_rows = [
            WebsiteUsageRow(
                website=website,
                duration_seconds=duration,
                visits=visits,
                total_time=format_total_time(duration),
            )
            for website, duration, visits in rollup_websites(title_aggs)
        ]

        avg_seconds = average_session_length_seconds(
            span.active_time_seconds, span.segment_count
        )
        idle_seconds = idle_time_seconds(span.working_time_seconds, span.active_time_seconds)
        empty = not intervals

        employee = ProductivityEmployee(
            agent_id=agent_id,
            computer=computer,
            user=user or "—",
            department=department,
            report_period_start=started_at_from,
            report_period_end=started_at_to,
        )
        summary = ProductivitySummary(
            working_time_seconds=span.working_time_seconds,
            working_time=format_total_time(span.working_time_seconds),
            active_time_seconds=span.active_time_seconds,
            active_time=format_total_time(span.active_time_seconds),
            idle_time_seconds=idle_seconds,
            idle_time=format_total_time(idle_seconds),
            applications_used=span.applications_used,
            websites_visited=len(website_rows),
            average_session_length_seconds=avg_seconds,
            average_session_length=format_total_time(avg_seconds),
        )
        activity = ProductivityActivitySummary(
            first_activity=span.first_activity,
            last_activity=span.last_activity,
            longest_continuous_working_session_seconds=span.longest_continuous_working_session_seconds,
            longest_continuous_working_session=format_total_time(
                span.longest_continuous_working_session_seconds
            ),
            longest_idle_period_seconds=span.longest_idle_period_seconds,
            longest_idle_period=format_total_time(span.longest_idle_period_seconds),
            most_used_application=app_rows[0].application if app_rows else None,
            most_used_website=website_rows[0].website if website_rows else None,
        )
        return ProductivityReport(
            employee=employee,
            summary=summary,
            applications=app_rows,
            websites=website_rows,
            activity=activity,
            empty=empty,
        )

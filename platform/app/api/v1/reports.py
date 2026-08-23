from __future__ import annotations

from datetime import datetime
from uuid import UUID

from fastapi import APIRouter, Depends, Query

from app.dependencies.container import Container, get_container
from app.domain.fleet import FleetAgentNotFoundError
from app.schemas.errors import ErrorEnvelope
from app.schemas.reports import (
    ProductivityActivitySummaryResponse,
    ProductivityApplicationRowResponse,
    ProductivityEmployeeResponse,
    ProductivityReportResponse,
    ProductivitySummaryResponse,
    ProductivityWebsiteRowResponse,
)
from app.services.productivity_report_service import ProductivityReportService

router = APIRouter()


def get_productivity_report_service(
    container: Container = Depends(get_container),
) -> ProductivityReportService:
    return container.resolve(ProductivityReportService)


@router.get(
    "/productivity",
    response_model=ProductivityReportResponse,
    summary="Productivity Report for one computer over a time range",
    responses={404: {"model": ErrorEnvelope}, 422: {"model": ErrorEnvelope}},
)
def get_productivity_report(
    agent_id: UUID = Query(..., description="Fleet agent / computer id"),
    started_at_from: datetime | None = Query(default=None),
    started_at_to: datetime | None = Query(default=None),
    service: ProductivityReportService = Depends(get_productivity_report_service),
) -> ProductivityReportResponse:
    if (
        started_at_from is not None
        and started_at_to is not None
        and started_at_from > started_at_to
    ):
        from fastapi import HTTPException

        raise HTTPException(status_code=422, detail="started_at_from must be before started_at_to")
    try:
        report = service.build(
            agent_id,
            started_at_from=started_at_from,
            started_at_to=started_at_to,
        )
    except FleetAgentNotFoundError:
        raise
    return ProductivityReportResponse(
        employee=ProductivityEmployeeResponse(
            agent_id=report.employee.agent_id,
            computer=report.employee.computer,
            user=report.employee.user,
            department=report.employee.department,
            report_period_start=report.employee.report_period_start,
            report_period_end=report.employee.report_period_end,
        ),
        summary=ProductivitySummaryResponse(
            working_time_seconds=report.summary.working_time_seconds,
            working_time=report.summary.working_time,
            active_time_seconds=report.summary.active_time_seconds,
            active_time=report.summary.active_time,
            idle_time_seconds=report.summary.idle_time_seconds,
            idle_time=report.summary.idle_time,
            applications_used=report.summary.applications_used,
            websites_visited=report.summary.websites_visited,
            average_session_length_seconds=report.summary.average_session_length_seconds,
            average_session_length=report.summary.average_session_length,
        ),
        applications=[
            ProductivityApplicationRowResponse(
                application=row.application,
                duration_seconds=row.duration_seconds,
                percentage=row.percentage,
                launch_count=row.launch_count,
                total_time=row.total_time,
            )
            for row in report.applications
        ],
        websites=[
            ProductivityWebsiteRowResponse(
                website=row.website,
                duration_seconds=row.duration_seconds,
                visits=row.visits,
                total_time=row.total_time,
            )
            for row in report.websites
        ],
        activity=ProductivityActivitySummaryResponse(
            first_activity=report.activity.first_activity if report.activity else None,
            last_activity=report.activity.last_activity if report.activity else None,
            longest_continuous_working_session_seconds=(
                report.activity.longest_continuous_working_session_seconds if report.activity else 0
            ),
            longest_continuous_working_session=(
                report.activity.longest_continuous_working_session if report.activity else "0s"
            ),
            longest_idle_period_seconds=(
                report.activity.longest_idle_period_seconds if report.activity else 0
            ),
            longest_idle_period=report.activity.longest_idle_period if report.activity else "0s",
            most_used_application=(
                report.activity.most_used_application if report.activity else None
            ),
            most_used_website=report.activity.most_used_website if report.activity else None,
        ),
        empty=report.empty,
    )

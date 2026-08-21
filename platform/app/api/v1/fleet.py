from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, Query

from app.dependencies.container import Container, get_container
from app.domain.fleet import (
    DEFAULT_LIMIT,
    DEFAULT_SORT,
    MAX_LIMIT,
    NOT_IMPLEMENTED,
    FleetRecord,
    InvalidFleetQueryError,
    ListFleetQuery,
)
from app.domain.applications import DEFAULT_SORT as APPLICATION_SORT
from app.domain.sessions import DEFAULT_SORT as SESSION_SORT
from app.domain.software import DEFAULT_SORT as SOFTWARE_SORT
from app.domain.user import DEFAULT_SORT as USER_SORT
from app.schemas.applications import (
    ApplicationActivityItemResponse,
    ApplicationActivityListResponse,
)
from app.schemas.errors import ErrorEnvelope
from app.schemas.sessions import SessionItemResponse, SessionListResponse
from app.schemas.fleet import (
    FleetHealthResponse,
    FleetItemResponse,
    FleetListResponse,
    FleetPolicyResponse,
    FleetSummaryResponse,
    FleetTimelineResponse,
    WebsiteUsageItemResponse,
    WebsiteUsageListResponse,
)
from app.schemas.software import SoftwareItemResponse, SoftwareListResponse
from app.schemas.users import UserItemResponse, UserListResponse
from app.services.fleet_service import FleetService

router = APIRouter()


def get_fleet_service(container: Container = Depends(get_container)) -> FleetService:
    return container.resolve(FleetService)


def _to_item(record: FleetRecord) -> FleetItemResponse:
    return FleetItemResponse(
        agent_id=record.agent_id,
        hostname=record.hostname,
        machine_name=record.machine_name,
        agent_version=record.agent_version,
        operating_system=record.operating_system,
        current_user=record.current_user,
        registration_status=record.registration_status,
        online_status=record.online_status,
        last_seen=record.last_seen,
        last_inventory_at=record.last_inventory_at,
        ip_address=record.ip_address,
        tags=record.tags,
        policy_name=record.policy_name,
        heartbeat_age_seconds=record.heartbeat_age_seconds,
        last_communication_at=record.last_communication_at,
        os_version=record.os_version,
        architecture=record.architecture,
        manufacturer=record.manufacturer,
        model=record.model,
        windows_build=record.windows_build,
        cpu_model=record.cpu_model,
        cpu_cores=record.cpu_cores,
        memory_bytes=record.memory_bytes,
        disk_total_bytes=record.disk_total_bytes,
        disk_free_bytes=record.disk_free_bytes,
        serial_number=record.serial_number,
    )


@router.get(
    "",
    response_model=FleetListResponse,
    summary="List managed endpoints",
    responses={422: {"model": ErrorEnvelope}},
)
def list_fleet(
    q: str | None = Query(default=None, max_length=256),
    status: str | None = Query(default=None, max_length=32),
    tag: str | None = Query(default=None, max_length=64),
    policy_id: UUID | None = Query(default=None),
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=DEFAULT_SORT, max_length=64),
    service: FleetService = Depends(get_fleet_service),
) -> FleetListResponse:
    try:
        query = ListFleetQuery(
            q=q,
            status=status,
            tag=tag,
            policy_id=policy_id,
            limit=limit,
            offset=offset,
            sort=sort,
        )
    except ValueError as exc:
        raise InvalidFleetQueryError(str(exc)) from exc
    page = service.list(query)
    return FleetListResponse(
        summary=FleetSummaryResponse(
            total_agents=page.summary.total_agents,
            online=page.summary.online,
            offline=page.summary.offline,
            warning=page.summary.warning,
            never_seen=page.summary.never_seen,
            healthy=page.summary.healthy,
        ),
        items=[_to_item(item) for item in page.items],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{agent_id}/users",
    response_model=UserListResponse,
    summary="List user inventory for one agent",
    responses={404: {"model": ErrorEnvelope}, 422: {"model": ErrorEnvelope}},
)
def list_fleet_users(
    agent_id: UUID,
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=USER_SORT, max_length=64),
    service: FleetService = Depends(get_fleet_service),
) -> UserListResponse:
    try:
        page = service.users(agent_id, limit=limit, offset=offset, sort=sort)
    except ValueError as exc:
        raise InvalidFleetQueryError(str(exc)) from exc
    return UserListResponse(
        items=[
            UserItemResponse(
                sid=item.sid,
                agent_id=item.agent_id,
                username=item.username,
                display_name=item.display_name,
                account_type=item.account_type,
                enabled=item.enabled,
                builtin=item.builtin,
                password_required=item.password_required,
                last_logon=item.last_logon,
                source=item.source,
                collected_at=item.collected_at,
                received_at=item.received_at,
                machine_name=item.machine_name,
            )
            for item in page.items
        ],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{agent_id}/software",
    response_model=SoftwareListResponse,
    summary="List software inventory for one agent",
    responses={404: {"model": ErrorEnvelope}, 422: {"model": ErrorEnvelope}},
)
def list_fleet_software(
    agent_id: UUID,
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=SOFTWARE_SORT, max_length=64),
    service: FleetService = Depends(get_fleet_service),
) -> SoftwareListResponse:
    try:
        page = service.software(agent_id, limit=limit, offset=offset, sort=sort)
    except ValueError as exc:
        raise InvalidFleetQueryError(str(exc)) from exc
    return SoftwareListResponse(
        items=[
            SoftwareItemResponse(
                id=item.id,
                agent_id=item.agent_id,
                application_name=item.application_name,
                version=item.version,
                publisher=item.publisher,
                install_date=item.install_date,
                install_location=item.install_location,
                estimated_size=item.estimated_size,
                architecture=item.architecture,
                system_component=item.system_component,
                windows_component=item.windows_component,
                registry_path=item.registry_path,
                collected_at=item.collected_at,
                received_at=item.received_at,
                machine_name=item.machine_name,
            )
            for item in page.items
        ],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{agent_id}/sessions",
    response_model=SessionListResponse,
    summary="List computer sessions for one agent",
    responses={404: {"model": ErrorEnvelope}, 422: {"model": ErrorEnvelope}},
)
def list_fleet_sessions(
    agent_id: UUID,
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=SESSION_SORT, max_length=64),
    service: FleetService = Depends(get_fleet_service),
) -> SessionListResponse:
    try:
        page = service.sessions(agent_id, limit=limit, offset=offset, sort=sort)
    except ValueError as exc:
        raise InvalidFleetQueryError(str(exc)) from exc
    return SessionListResponse(
        items=[
            SessionItemResponse(
                session_id=item.session_id,
                agent_id=item.agent_id,
                username=item.username,
                account_sid=item.account_sid,
                logon_at=item.logon_at,
                logoff_at=item.logoff_at,
                status=item.status,
                duration_seconds=item.duration_seconds,
                collected_at=item.collected_at,
                received_at=item.received_at,
                machine_name=item.machine_name,
            )
            for item in page.items
        ],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{agent_id}/applications",
    response_model=ApplicationActivityListResponse,
    summary="List application activity for one agent",
    responses={404: {"model": ErrorEnvelope}, 422: {"model": ErrorEnvelope}},
)
def list_fleet_applications(
    agent_id: UUID,
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=APPLICATION_SORT, max_length=64),
    service: FleetService = Depends(get_fleet_service),
) -> ApplicationActivityListResponse:
    try:
        page = service.applications(agent_id, limit=limit, offset=offset, sort=sort)
    except ValueError as exc:
        raise InvalidFleetQueryError(str(exc)) from exc
    return ApplicationActivityListResponse(
        items=[
            ApplicationActivityItemResponse(
                id=item.id,
                session_id=item.session_id,
                agent_id=item.agent_id,
                application_name=item.application_name,
                executable_name=item.executable_name,
                window_title=item.window_title,
                started_at=item.started_at,
                stopped_at=item.stopped_at,
                duration_seconds=item.duration_seconds,
                collected_at=item.collected_at,
                received_at=item.received_at,
                username=item.username,
                machine_name=item.machine_name,
            )
            for item in page.items
        ],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{agent_id}/website-usage",
    response_model=WebsiteUsageListResponse,
    summary="Website usage summary from Application Activity window titles",
    responses={404: {"model": ErrorEnvelope}},
)
def list_fleet_website_usage(
    agent_id: UUID,
    service: FleetService = Depends(get_fleet_service),
) -> WebsiteUsageListResponse:
    items = service.website_usage(agent_id)
    return WebsiteUsageListResponse(
        items=[
            WebsiteUsageItemResponse(
                browser=item.browser,
                website=item.website,
                duration_seconds=item.duration_seconds,
                category=item.category,
                total_time=item.total_time,
            )
            for item in items
        ],
        total=len(items),
    )


@router.get(
    "/{agent_id}/health",
    response_model=FleetHealthResponse,
    summary="Agent health for Fleet",
    responses={404: {"model": ErrorEnvelope}},
)
def get_fleet_health(
    agent_id: UUID,
    service: FleetService = Depends(get_fleet_service),
) -> FleetHealthResponse:
    health = service.health(agent_id)
    return FleetHealthResponse(
        status=health.status,
        heartbeat_age_seconds=health.heartbeat_age_seconds,
        last_inventory_at=health.last_inventory_at,
        last_communication_at=health.last_communication_at,
        cpu_usage=health.cpu_usage,
        memory_usage=health.memory_usage,
        disk_usage=health.disk_usage,
        memory_bytes=health.memory_bytes,
        disk_total_bytes=health.disk_total_bytes,
        disk_free_bytes=health.disk_free_bytes,
    )


@router.get(
    "/{agent_id}/policy",
    response_model=FleetPolicyResponse,
    summary="Assigned agent policy (read-only)",
    responses={404: {"model": ErrorEnvelope}},
)
def get_fleet_policy(
    agent_id: UUID,
    service: FleetService = Depends(get_fleet_service),
) -> FleetPolicyResponse:
    policy = service.policy(agent_id)
    return FleetPolicyResponse(
        id=policy.id,
        name=policy.name,
        heartbeat_interval_seconds=policy.heartbeat_interval_seconds,
        collect_devices=policy.collect_devices,
        collect_users=policy.collect_users,
        collect_software=policy.collect_software,
        collect_processes=policy.collect_processes,
        collect_services=policy.collect_services,
        collect_event_logs=policy.collect_event_logs,
        created_at=policy.created_at,
    )


@router.get(
    "/{agent_id}/timeline",
    response_model=FleetTimelineResponse,
    summary="Agent timeline (not implemented)",
    responses={404: {"model": ErrorEnvelope}},
)
def get_fleet_timeline(
    agent_id: UUID,
    service: FleetService = Depends(get_fleet_service),
) -> FleetTimelineResponse:
    service.require_agent(agent_id)
    return FleetTimelineResponse(status=NOT_IMPLEMENTED)


@router.get(
    "/{agent_id}",
    response_model=FleetItemResponse,
    summary="Get one managed endpoint",
    responses={404: {"model": ErrorEnvelope}},
)
def get_fleet_agent(
    agent_id: UUID,
    service: FleetService = Depends(get_fleet_service),
) -> FleetItemResponse:
    return _to_item(service.get(agent_id))

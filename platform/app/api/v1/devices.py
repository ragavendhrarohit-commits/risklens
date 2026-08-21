from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Response, status

from app.config import Settings
from app.dependencies import get_settings_dep
from app.dependencies.container import Container, get_container
from app.domain.device import (
    DEFAULT_LIMIT,
    DEFAULT_SORT,
    MAX_LIMIT,
    DeviceRecord,
    InvalidDeviceQueryError,
    ListDevicesQuery,
)
from app.schemas.devices import (
    DeviceInventoryRequest,
    DeviceInventoryResponse,
    DeviceItemResponse,
    DeviceListResponse,
)
from app.schemas.errors import ErrorEnvelope
from app.services.device_query_service import DeviceQueryService
from app.services.device_service import DeviceInventoryService

router = APIRouter()


def get_device_inventory_service(
    container: Container = Depends(get_container),
) -> DeviceInventoryService:
    return container.resolve(DeviceInventoryService)


def get_device_query_service(
    container: Container = Depends(get_container),
) -> DeviceQueryService:
    return container.resolve(DeviceQueryService)


def _to_item(record: DeviceRecord) -> DeviceItemResponse:
    return DeviceItemResponse(
        agent_id=record.agent_id,
        machine_name=record.machine_name,
        hostname=record.hostname,
        operating_system=record.operating_system,
        os_version=record.os_version,
        windows_build=record.windows_build,
        architecture=record.architecture,
        cpu_model=record.cpu_model,
        cpu_cores=record.cpu_cores,
        memory_bytes=record.memory_bytes,
        disk_total_bytes=record.disk_total_bytes,
        disk_free_bytes=record.disk_free_bytes,
        manufacturer=record.manufacturer,
        model=record.model,
        serial_number=record.serial_number,
        current_user=record.current_user,
        agent_version=record.agent_version,
        collected_at=record.collected_at,
        received_at=record.received_at,
        registration_status=record.registration_status,
        status=record.status,
        last_seen=record.last_seen,
        heartbeat_age_seconds=record.heartbeat_age_seconds,
    )


@router.get(
    "",
    response_model=DeviceListResponse,
    summary="List device inventory",
    description=(
        "Returns the latest device inventory rows for the frontend Devices page. "
        "Supports hostname / operating_system / manufacturer / current_user filters, "
        "offset pagination, and sort (prefix '-' for descending)."
    ),
    responses={
        422: {"model": ErrorEnvelope, "description": "Invalid query parameters."},
    },
)
def list_devices(
    hostname: str | None = Query(default=None, max_length=256),
    operating_system: str | None = Query(default=None, max_length=256),
    manufacturer: str | None = Query(default=None, max_length=256),
    current_user: str | None = Query(default=None, max_length=256),
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=DEFAULT_SORT, max_length=64),
    service: DeviceQueryService = Depends(get_device_query_service),
) -> DeviceListResponse:
    try:
        query = ListDevicesQuery(
            hostname=hostname,
            operating_system=operating_system,
            manufacturer=manufacturer,
            current_user=current_user,
            limit=limit,
            offset=offset,
            sort=sort,
        )
    except ValueError as exc:
        raise InvalidDeviceQueryError(str(exc)) from exc

    page = service.list(query)
    return DeviceListResponse(
        items=[_to_item(item) for item in page.items],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{agent_id}",
    response_model=DeviceItemResponse,
    summary="Get device inventory by AgentID",
    description="Returns the latest inventory snapshot for a single registered agent.",
    responses={
        404: {"model": ErrorEnvelope, "description": "No inventory for this AgentID."},
        422: {"model": ErrorEnvelope, "description": "Invalid AgentID."},
    },
)
def get_device(
    agent_id: UUID,
    service: DeviceQueryService = Depends(get_device_query_service),
) -> DeviceItemResponse:
    return _to_item(service.get(agent_id))


@router.post(
    "",
    response_model=DeviceInventoryResponse,
    status_code=status.HTTP_200_OK,
    summary="Upload device inventory from an agent",
    description=(
        "Stores the latest static device inventory for a registered agent. "
        "Repeated uploads overwrite the previous snapshot for that AgentID."
    ),
    responses={
        200: {"description": "Existing inventory updated."},
        201: {"description": "First inventory stored for this agent."},
        404: {"model": ErrorEnvelope, "description": "AgentID is not registered."},
        422: {"model": ErrorEnvelope, "description": "Request failed schema validation."},
    },
)
def upload_device_inventory(
    payload: DeviceInventoryRequest,
    response: Response,
    service: DeviceInventoryService = Depends(get_device_inventory_service),
    settings: Settings = Depends(get_settings_dep),
) -> DeviceInventoryResponse:
    inventory, created = service.upsert(payload.to_command())
    response.status_code = status.HTTP_201_CREATED if created else status.HTTP_200_OK
    return DeviceInventoryResponse(
        accepted=True,
        agent_id=inventory.agent_id,
        created=created,
        inventory_status="stored",
        server_version=settings.app_version,
        server_time=datetime.now(timezone.utc),
        received_at=inventory.received_at,
    )

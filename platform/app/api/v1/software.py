from __future__ import annotations

from datetime import datetime, timezone

from fastapi import APIRouter, Depends, Query, status

from app.config import Settings
from app.dependencies import get_settings_dep
from app.dependencies.container import Container, get_container
from app.domain.software import (
    DEFAULT_LIMIT,
    DEFAULT_SORT,
    MAX_LIMIT,
    InvalidSoftwareQueryError,
    ListSoftwareQuery,
    SoftwareRecord,
)
from app.schemas.errors import ErrorEnvelope
from app.schemas.software import (
    SoftwareInventoryRequest,
    SoftwareInventoryResponse,
    SoftwareItemResponse,
    SoftwareListResponse,
)
from app.services.software_service import SoftwareInventoryService

router = APIRouter()


def get_software_inventory_service(
    container: Container = Depends(get_container),
) -> SoftwareInventoryService:
    return container.resolve(SoftwareInventoryService)


def _to_item(record: SoftwareRecord) -> SoftwareItemResponse:
    return SoftwareItemResponse(
        id=record.id,
        agent_id=record.agent_id,
        application_name=record.application_name,
        version=record.version,
        publisher=record.publisher,
        install_date=record.install_date,
        install_location=record.install_location,
        estimated_size=record.estimated_size,
        architecture=record.architecture,
        system_component=record.system_component,
        windows_component=record.windows_component,
        registry_path=record.registry_path,
        collected_at=record.collected_at,
        received_at=record.received_at,
        machine_name=record.machine_name,
    )


@router.post(
    "",
    response_model=SoftwareInventoryResponse,
    status_code=status.HTTP_200_OK,
    summary="Upload installed software inventory",
    responses={
        404: {"model": ErrorEnvelope, "description": "AgentID is not registered."},
        422: {"model": ErrorEnvelope, "description": "Validation failed."},
    },
)
def upload_software(
    payload: SoftwareInventoryRequest,
    service: SoftwareInventoryService = Depends(get_software_inventory_service),
    settings: Settings = Depends(get_settings_dep),
) -> SoftwareInventoryResponse:
    stored, received_at = service.upsert(payload.to_command())
    return SoftwareInventoryResponse(
        accepted=True,
        agent_id=payload.agent_id,
        stored=stored,
        inventory_status="stored",
        server_version=settings.app_version,
        server_time=datetime.now(timezone.utc),
        received_at=received_at,
    )


@router.get(
    "",
    response_model=SoftwareListResponse,
    summary="List installed software inventory",
    responses={422: {"model": ErrorEnvelope}},
)
def list_software(
    application_name: str | None = Query(default=None, max_length=512),
    publisher: str | None = Query(default=None, max_length=256),
    version: str | None = Query(default=None, max_length=128),
    architecture: str | None = Query(default=None, max_length=16),
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=DEFAULT_SORT, max_length=64),
    service: SoftwareInventoryService = Depends(get_software_inventory_service),
) -> SoftwareListResponse:
    try:
        query = ListSoftwareQuery(
            application_name=application_name,
            publisher=publisher,
            version=version,
            architecture=architecture,
            limit=limit,
            offset=offset,
            sort=sort,
        )
    except ValueError as exc:
        raise InvalidSoftwareQueryError(str(exc)) from exc
    page = service.list(query)
    return SoftwareListResponse(
        items=[_to_item(item) for item in page.items],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{software_id}",
    response_model=SoftwareItemResponse,
    summary="Get installed software by id",
    responses={404: {"model": ErrorEnvelope}},
)
def get_software(
    software_id: str,
    service: SoftwareInventoryService = Depends(get_software_inventory_service),
) -> SoftwareItemResponse:
    return _to_item(service.get(software_id))

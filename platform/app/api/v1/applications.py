from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status

from app.config import Settings
from app.dependencies import get_settings_dep
from app.dependencies.container import Container, get_container
from app.domain.applications import (
    DEFAULT_LIMIT,
    DEFAULT_SORT,
    MAX_LIMIT,
    ApplicationActivityRecord,
    InvalidApplicationQueryError,
    ListApplicationActivityQuery,
)
from app.schemas.applications import (
    ApplicationActivityItemResponse,
    ApplicationActivityListResponse,
    ApplicationActivityUploadRequest,
    ApplicationActivityUploadResponse,
)
from app.schemas.errors import ErrorEnvelope
from app.services.application_activity_service import ApplicationActivityService

router = APIRouter()


def get_application_activity_service(
    container: Container = Depends(get_container),
) -> ApplicationActivityService:
    return container.resolve(ApplicationActivityService)


def _to_item(record: ApplicationActivityRecord) -> ApplicationActivityItemResponse:
    return ApplicationActivityItemResponse(
        id=record.id,
        session_id=record.session_id,
        agent_id=record.agent_id,
        application_name=record.application_name,
        executable_name=record.executable_name,
        window_title=record.window_title,
        started_at=record.started_at,
        stopped_at=record.stopped_at,
        duration_seconds=record.duration_seconds,
        collected_at=record.collected_at,
        received_at=record.received_at,
        username=record.username,
        machine_name=record.machine_name,
    )


@router.post(
    "",
    response_model=ApplicationActivityUploadResponse,
    status_code=status.HTTP_200_OK,
    summary="Upload application activity",
    responses={
        404: {"model": ErrorEnvelope, "description": "AgentID is not registered."},
        422: {"model": ErrorEnvelope, "description": "Unknown session or validation failed."},
    },
)
def upload_application_activity(
    payload: ApplicationActivityUploadRequest,
    service: ApplicationActivityService = Depends(get_application_activity_service),
    settings: Settings = Depends(get_settings_dep),
) -> ApplicationActivityUploadResponse:
    stored, received_at = service.upsert(payload.to_command())
    return ApplicationActivityUploadResponse(
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
    response_model=ApplicationActivityListResponse,
    summary="List application activity",
    responses={422: {"model": ErrorEnvelope}},
)
def list_application_activity(
    agent_id: UUID | None = Query(default=None),
    session_id: UUID | None = Query(default=None),
    application_name: str | None = Query(default=None, max_length=512),
    started_at_from: datetime | None = Query(default=None),
    started_at_to: datetime | None = Query(default=None),
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=DEFAULT_SORT, max_length=64),
    service: ApplicationActivityService = Depends(get_application_activity_service),
) -> ApplicationActivityListResponse:
    try:
        query = ListApplicationActivityQuery(
            agent_id=agent_id,
            session_id=session_id,
            application_name=application_name,
            started_at_from=started_at_from,
            started_at_to=started_at_to,
            limit=limit,
            offset=offset,
            sort=sort,
        )
    except ValueError as exc:
        raise InvalidApplicationQueryError(str(exc)) from exc
    page = service.list(query)
    return ApplicationActivityListResponse(
        items=[_to_item(item) for item in page.items],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{activity_id}",
    response_model=ApplicationActivityItemResponse,
    summary="Get one application activity row",
    responses={404: {"model": ErrorEnvelope}},
)
def get_application_activity(
    activity_id: str,
    service: ApplicationActivityService = Depends(get_application_activity_service),
) -> ApplicationActivityItemResponse:
    return _to_item(service.get(activity_id))

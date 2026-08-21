from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status

from app.config import Settings
from app.dependencies import get_settings_dep
from app.dependencies.container import Container, get_container
from app.domain.browser import (
    DEFAULT_LIMIT,
    DEFAULT_SORT,
    MAX_LIMIT,
    BrowserActivityRecord,
    InvalidBrowserQueryError,
    ListBrowserActivityQuery,
)
from app.schemas.browser import (
    BrowserActivityItemResponse,
    BrowserActivityListResponse,
    BrowserActivityUploadRequest,
    BrowserActivityUploadResponse,
)
from app.schemas.errors import ErrorEnvelope
from app.services.browser_activity_service import BrowserActivityService

router = APIRouter()


def get_browser_activity_service(
    container: Container = Depends(get_container),
) -> BrowserActivityService:
    return container.resolve(BrowserActivityService)


def _to_item(record: BrowserActivityRecord) -> BrowserActivityItemResponse:
    return BrowserActivityItemResponse(
        id=record.id,
        session_id=record.session_id,
        agent_id=record.agent_id,
        executable_name=record.executable_name,
        domain=record.domain,
        url=record.url,
        title=record.title,
        profile=record.profile,
        window_id=record.window_id,
        tab_id=record.tab_id,
        started_at=record.started_at,
        stopped_at=record.stopped_at,
        duration_seconds=record.duration_seconds,
        collected_at=record.collected_at,
        received_at=record.received_at,
    )


@router.post(
    "",
    response_model=BrowserActivityUploadResponse,
    status_code=status.HTTP_200_OK,
    summary="Upload browser activity",
    responses={
        404: {"model": ErrorEnvelope, "description": "AgentID is not registered."},
        422: {"model": ErrorEnvelope, "description": "Unknown session or validation failed."},
    },
)
def upload_browser_activity(
    payload: BrowserActivityUploadRequest,
    service: BrowserActivityService = Depends(get_browser_activity_service),
    settings: Settings = Depends(get_settings_dep),
) -> BrowserActivityUploadResponse:
    stored, received_at = service.upsert(payload.to_command())
    return BrowserActivityUploadResponse(
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
    response_model=BrowserActivityListResponse,
    summary="List browser activity",
    responses={422: {"model": ErrorEnvelope}},
)
def list_browser_activity(
    agent_id: UUID | None = Query(default=None),
    session_id: UUID | None = Query(default=None),
    domain: str | None = Query(default=None, max_length=256),
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=DEFAULT_SORT, max_length=64),
    service: BrowserActivityService = Depends(get_browser_activity_service),
) -> BrowserActivityListResponse:
    try:
        query = ListBrowserActivityQuery(
            agent_id=agent_id,
            session_id=session_id,
            domain=domain,
            limit=limit,
            offset=offset,
            sort=sort,
        )
    except ValueError as exc:
        raise InvalidBrowserQueryError(str(exc)) from exc
    page = service.list(query)
    return BrowserActivityListResponse(
        items=[_to_item(item) for item in page.items],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{activity_id}",
    response_model=BrowserActivityItemResponse,
    summary="Get one browser activity row",
    responses={404: {"model": ErrorEnvelope}},
)
def get_browser_activity(
    activity_id: str,
    service: BrowserActivityService = Depends(get_browser_activity_service),
) -> BrowserActivityItemResponse:
    return _to_item(service.get(activity_id))

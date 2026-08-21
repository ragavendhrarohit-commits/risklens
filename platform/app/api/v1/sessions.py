from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status

from app.config import Settings
from app.dependencies import get_settings_dep
from app.dependencies.container import Container, get_container
from app.domain.sessions import (
    DEFAULT_LIMIT,
    DEFAULT_SORT,
    MAX_LIMIT,
    InvalidSessionQueryError,
    ListSessionsQuery,
    SessionRecord,
)
from app.schemas.errors import ErrorEnvelope
from app.schemas.sessions import (
    SessionItemResponse,
    SessionListResponse,
    SessionUploadRequest,
    SessionUploadResponse,
)
from app.services import session_lifecycle_log as lifecycle
from app.services.session_service import SessionService

router = APIRouter()


def get_session_service(container: Container = Depends(get_container)) -> SessionService:
    return container.resolve(SessionService)


def _to_item(record: SessionRecord) -> SessionItemResponse:
    return SessionItemResponse(
        session_id=record.session_id,
        agent_id=record.agent_id,
        username=record.username,
        account_sid=record.account_sid,
        logon_at=record.logon_at,
        logoff_at=record.logoff_at,
        status=record.status,
        duration_seconds=record.duration_seconds,
        collected_at=record.collected_at,
        received_at=record.received_at,
        machine_name=record.machine_name,
    )


@router.post(
    "",
    response_model=SessionUploadResponse,
    status_code=status.HTTP_200_OK,
    summary="Upload computer sessions",
    responses={
        404: {"model": ErrorEnvelope, "description": "AgentID is not registered."},
        422: {"model": ErrorEnvelope, "description": "Validation failed."},
    },
)
def upload_sessions(
    payload: SessionUploadRequest,
    service: SessionService = Depends(get_session_service),
    settings: Settings = Depends(get_settings_dep),
) -> SessionUploadResponse:
    lifecycle.banner(
        "INCOMING PAYLOAD",
        agent_id=str(payload.agent_id),
        payload=payload.model_dump(mode="json"),
    )
    stored, received_at = service.upsert(payload.to_command())
    return SessionUploadResponse(
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
    response_model=SessionListResponse,
    summary="List computer sessions",
    responses={422: {"model": ErrorEnvelope}},
)
def list_sessions(
    agent_id: UUID | None = Query(default=None),
    status_filter: str | None = Query(default=None, alias="status", max_length=32),
    username: str | None = Query(default=None, max_length=256),
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=DEFAULT_SORT, max_length=64),
    service: SessionService = Depends(get_session_service),
) -> SessionListResponse:
    try:
        query = ListSessionsQuery(
            agent_id=agent_id,
            status=status_filter,
            username=username,
            limit=limit,
            offset=offset,
            sort=sort,
        )
    except ValueError as exc:
        raise InvalidSessionQueryError(str(exc)) from exc
    page = service.list(query)
    return SessionListResponse(
        items=[_to_item(item) for item in page.items],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{session_id}",
    response_model=SessionItemResponse,
    summary="Get one computer session",
    responses={404: {"model": ErrorEnvelope}},
)
def get_session(
    session_id: UUID,
    service: SessionService = Depends(get_session_service),
) -> SessionItemResponse:
    return _to_item(service.get(session_id))

from __future__ import annotations

from datetime import datetime, timezone

from fastapi import APIRouter, Depends, Query, status

from app.config import Settings
from app.dependencies import get_settings_dep
from app.dependencies.container import Container, get_container
from app.domain.user import (
    DEFAULT_LIMIT,
    DEFAULT_SORT,
    MAX_LIMIT,
    InvalidUserQueryError,
    ListUsersQuery,
    UserRecord,
)
from app.schemas.errors import ErrorEnvelope
from app.schemas.users import (
    UserInventoryRequest,
    UserInventoryResponse,
    UserItemResponse,
    UserListResponse,
)
from app.services.user_service import UserInventoryService

router = APIRouter()


def get_user_inventory_service(
    container: Container = Depends(get_container),
) -> UserInventoryService:
    return container.resolve(UserInventoryService)


def _to_item(record: UserRecord) -> UserItemResponse:
    return UserItemResponse(
        sid=record.sid,
        agent_id=record.agent_id,
        username=record.username,
        display_name=record.display_name,
        account_type=record.account_type,
        enabled=record.enabled,
        builtin=record.builtin,
        password_required=record.password_required,
        last_logon=record.last_logon,
        source=record.source,
        collected_at=record.collected_at,
        received_at=record.received_at,
        machine_name=record.machine_name,
    )


@router.post(
    "",
    response_model=UserInventoryResponse,
    status_code=status.HTTP_200_OK,
    summary="Upload Windows user inventory",
    responses={
        404: {"model": ErrorEnvelope, "description": "AgentID is not registered."},
        422: {"model": ErrorEnvelope, "description": "Validation failed."},
    },
)
def upload_users(
    payload: UserInventoryRequest,
    service: UserInventoryService = Depends(get_user_inventory_service),
    settings: Settings = Depends(get_settings_dep),
) -> UserInventoryResponse:
    stored, received_at = service.upsert(payload.to_command())
    return UserInventoryResponse(
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
    response_model=UserListResponse,
    summary="List Windows user inventory",
    responses={422: {"model": ErrorEnvelope}},
)
def list_users(
    username: str | None = Query(default=None, max_length=256),
    account_type: str | None = Query(default=None, max_length=64),
    enabled: bool | None = Query(default=None),
    limit: int = Query(default=DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    offset: int = Query(default=0, ge=0),
    sort: str = Query(default=DEFAULT_SORT, max_length=64),
    service: UserInventoryService = Depends(get_user_inventory_service),
) -> UserListResponse:
    try:
        query = ListUsersQuery(
            username=username,
            account_type=account_type,
            enabled=enabled,
            limit=limit,
            offset=offset,
            sort=sort,
        )
    except ValueError as exc:
        raise InvalidUserQueryError(str(exc)) from exc
    page = service.list(query)
    return UserListResponse(
        items=[_to_item(item) for item in page.items],
        total=page.total,
        limit=page.limit,
        offset=page.offset,
    )


@router.get(
    "/{sid}",
    response_model=UserItemResponse,
    summary="Get user inventory by SID",
    responses={404: {"model": ErrorEnvelope}},
)
def get_user(
    sid: str,
    service: UserInventoryService = Depends(get_user_inventory_service),
) -> UserItemResponse:
    return _to_item(service.get(sid))

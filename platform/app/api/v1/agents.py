from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, status

from app.config import Settings
from app.dependencies import get_settings_dep
from app.dependencies.container import Container, get_container
from app.schemas.agents import (
    AgentHeartbeatRequest,
    AgentHeartbeatResponse,
    AgentRegistrationRequest,
    AgentRegistrationResponse,
)
from app.schemas.errors import ErrorEnvelope
from app.services import session_lifecycle_log as lifecycle
from app.services.agent_service import AgentRegistrationService
from app.services.heartbeat_service import HeartbeatService

router = APIRouter()


def get_agent_registration_service(
    container: Container = Depends(get_container),
) -> AgentRegistrationService:
    return container.resolve(AgentRegistrationService)


def get_heartbeat_service(
    container: Container = Depends(get_container),
) -> HeartbeatService:
    return container.resolve(HeartbeatService)


@router.post(
    "/register",
    response_model=AgentRegistrationResponse,
    status_code=status.HTTP_201_CREATED,
    summary="Register an agent with the platform",
    description=(
        "Accepts an agent's local identity and records it in the backend. "
        "Registrations are persisted through the configured AgentRepository."
    ),
    responses={
        409: {"model": ErrorEnvelope, "description": "AgentID is already registered."},
        422: {"model": ErrorEnvelope, "description": "Request failed schema validation."},
    },
)
def register_agent(
    payload: AgentRegistrationRequest,
    service: AgentRegistrationService = Depends(get_agent_registration_service),
    settings: Settings = Depends(get_settings_dep),
) -> AgentRegistrationResponse:
    agent = service.register(payload.to_command())
    return AgentRegistrationResponse(
        accepted=True,
        agent_id=agent.agent_id,
        registration_status="registered",
        server_version=settings.app_version,
        server_time=datetime.now(timezone.utc),
    )


@router.post(
    "/{agent_id}/heartbeat",
    response_model=AgentHeartbeatResponse,
    status_code=status.HTTP_200_OK,
    summary="Record an agent heartbeat",
    description=(
        "Appends a heartbeat history row for a registered agent. "
        "Used by the Devices page to derive Online / Stale / Offline presence."
    ),
    responses={
        404: {"model": ErrorEnvelope, "description": "AgentID is not registered."},
        422: {"model": ErrorEnvelope, "description": "Validation or agent_id mismatch."},
    },
)
def post_heartbeat(
    agent_id: UUID,
    payload: AgentHeartbeatRequest,
    service: HeartbeatService = Depends(get_heartbeat_service),
    settings: Settings = Depends(get_settings_dep),
) -> AgentHeartbeatResponse:
    lifecycle.banner(
        "HEARTBEAT PAYLOAD",
        agent_id=str(agent_id),
        payload=payload.model_dump(mode="json"),
    )
    stored = service.record(agent_id, payload.to_command())
    return AgentHeartbeatResponse(
        accepted=True,
        agent_id=stored.agent_id,
        heartbeat_status="ok",
        server_version=settings.app_version,
        server_time=datetime.now(timezone.utc),
        received_at=stored.received_at,
    )

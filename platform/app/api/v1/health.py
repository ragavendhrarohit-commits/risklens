from __future__ import annotations

from fastapi import APIRouter

from app.schemas.responses import HealthResponse

router = APIRouter(tags=["health"])


@router.get(
    "/health",
    response_model=HealthResponse,
    summary="Liveness probe",
    description="Returns healthy when the API process is up. Does not check external systems.",
)
async def read_health() -> HealthResponse:
    return HealthResponse(status="healthy")

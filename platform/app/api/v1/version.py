from __future__ import annotations

from fastapi import APIRouter, Depends

from app.config import Settings
from app.dependencies import get_settings_dep
from app.schemas.responses import VersionResponse

router = APIRouter(tags=["meta"])


@router.get(
    "/version",
    response_model=VersionResponse,
    summary="Build metadata",
    description="Returns the deployed application version, build time, and environment.",
)
async def read_version(settings: Settings = Depends(get_settings_dep)) -> VersionResponse:
    return VersionResponse(
        version=settings.app_version,
        build_time=settings.build_time,
        environment=settings.environment.value,
    )

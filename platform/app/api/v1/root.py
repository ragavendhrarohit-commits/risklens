from __future__ import annotations

from fastapi import APIRouter, Depends

from app.config import Settings
from app.dependencies import get_settings_dep
from app.schemas.responses import RootResponse

router = APIRouter(tags=["meta"])


@router.get(
    "/",
    response_model=RootResponse,
    summary="Service metadata",
    description="Returns the service name, current status, and version.",
)
async def read_root(settings: Settings = Depends(get_settings_dep)) -> RootResponse:
    return RootResponse(service=settings.app_name, status="running", version=settings.app_version)

from __future__ import annotations

from fastapi import FastAPI
from fastapi.openapi.utils import get_openapi

from app.config import Settings


def customize_openapi(app: FastAPI, settings: Settings) -> None:
    def _openapi() -> dict:
        if app.openapi_schema:
            return app.openapi_schema
        schema = get_openapi(
            title=settings.app_name,
            version=settings.app_version,
            summary="RiskLens Platform Backend",
            description=(
                "Foundation API for the RiskLens platform.\n\n"
                "Exposes agent registration, device inventory, user inventory, "
                "and installed software inventory endpoints."
            ),
            contact={"name": "RiskLens", "url": "https://example.invalid/risklens"},
            license_info={"name": "Proprietary"},
            tags=[
                {"name": "meta", "description": "Service metadata."},
                {"name": "health", "description": "Liveness and readiness."},
                {"name": "agents", "description": "Agent registration and heartbeats."},
                {"name": "devices", "description": "Device inventory."},
                {"name": "users", "description": "Windows user account inventory."},
                {"name": "software", "description": "Installed software inventory."},
            ],
            routes=app.routes,
        )
        app.openapi_schema = schema
        return schema

    app.openapi = _openapi

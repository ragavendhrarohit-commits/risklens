from __future__ import annotations

from fastapi import APIRouter

from app.api.v1 import agents, applications, browser, devices, fleet, health, root, sessions, software, users, version


def build_api_router() -> APIRouter:
    router = APIRouter()
    router.include_router(root.router)
    router.include_router(health.router)
    router.include_router(version.router)

    router.include_router(agents.router, prefix="/api/v1/agents", tags=["agents"])
    router.include_router(devices.router, prefix="/api/v1/devices", tags=["devices"])
    router.include_router(users.router, prefix="/api/v1/users", tags=["users"])
    router.include_router(software.router, prefix="/api/v1/software", tags=["software"])
    router.include_router(sessions.router, prefix="/api/v1/sessions", tags=["sessions"])
    router.include_router(applications.router, prefix="/api/v1/applications", tags=["applications"])
    router.include_router(browser.router, prefix="/api/v1/browser", tags=["browser"])
    router.include_router(fleet.router, prefix="/api/v1/fleet", tags=["fleet"])

    # Reserved prefixes (not mounted): activity, network, alerts,
    # incidents, rules, timeline, reports, ai, groups, administration.

    return router

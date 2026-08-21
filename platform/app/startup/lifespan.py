from __future__ import annotations

import logging
from contextlib import asynccontextmanager
from typing import AsyncIterator, Callable

from fastapi import FastAPI

from app.config import RepositoryBackend, Settings
from app.dependencies.container import Container
from app.infrastructure.memory import MemoryAgentRepository
from app.infrastructure.memory.memory_device_inventory_repository import (
    MemoryDeviceInventoryRepository,
)
from app.infrastructure.memory.memory_device_query_repository import (
    MemoryDeviceQueryRepository,
)
from app.infrastructure.memory.memory_fleet_repository import (
    MemoryAgentStatusRepository,
    MemoryAgentTagRepository,
    MemoryPolicyRepository,
)
from app.infrastructure.memory.memory_heartbeat_repository import (
    MemoryHeartbeatRepository,
)
from app.infrastructure.memory.memory_application_activity_repository import (
    MemoryApplicationActivityRepository,
)
from app.infrastructure.memory.memory_browser_activity_repository import (
    MemoryBrowserActivityRepository,
)
from app.infrastructure.memory.memory_session_repository import MemorySessionRepository
from app.infrastructure.memory.memory_software_repository import (
    MemorySoftwareRepository,
)
from app.infrastructure.memory.memory_user_repository import MemoryUserRepository
from app.infrastructure.postgres import Database, PostgresAgentRepository
from app.infrastructure.postgres.device_inventory_repository import (
    PostgresDeviceInventoryRepository,
)
from app.infrastructure.postgres.device_query_repository import (
    PostgresDeviceQueryRepository,
)
from app.infrastructure.postgres.fleet_repository import (
    PostgresAgentStatusRepository,
    PostgresPolicyRepository,
)
from app.infrastructure.postgres.fleet_tag_repository import PostgresAgentTagRepository
from app.infrastructure.postgres.heartbeat_repository import (
    PostgresHeartbeatRepository,
)
from app.infrastructure.postgres.application_activity_repository import (
    PostgresApplicationActivityRepository,
)
from app.infrastructure.postgres.browser_activity_repository import (
    PostgresBrowserActivityRepository,
)
from app.infrastructure.postgres.session_repository import PostgresSessionRepository
from app.infrastructure.postgres.software_repository import (
    PostgresSoftwareRepository,
)
from app.infrastructure.postgres.user_repository import PostgresUserRepository
from app.logging.setup import configure_logging, get_logger
from app.repositories.agent_repository import AgentRepository
from app.repositories.device_inventory_repository import DeviceInventoryRepository
from app.repositories.device_query_repository import DeviceQueryRepository
from app.repositories.fleet_repository import (
    AgentStatusRepository,
    AgentTagRepository,
    PolicyRepository,
)
from app.repositories.heartbeat_repository import HeartbeatRepository
from app.repositories.application_activity_repository import ApplicationActivityRepository
from app.repositories.browser_activity_repository import BrowserActivityRepository
from app.repositories.session_repository import SessionRepository
from app.repositories.software_repository import SoftwareRepository
from app.repositories.user_repository import UserRepository
from app.services.agent_service import AgentRegistrationService
from app.services.device_query_service import DeviceQueryService
from app.services.device_service import DeviceInventoryService
from app.services.fleet_service import FleetProjection, FleetService
from app.services.heartbeat_service import HeartbeatService
from app.services.application_activity_service import ApplicationActivityService
from app.services.browser_activity_service import BrowserActivityService
from app.services.session_service import SessionService
from app.services.software_service import SoftwareInventoryService
from app.services.user_service import UserInventoryService


def build_lifespan(settings: Settings) -> Callable[[FastAPI], "AsyncIterator[None]"]:
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        configure_logging(level=settings.log_level, json_output=settings.log_json)
        logger = get_logger("risklens.startup")

        container = Container(settings=settings)
        database = _register_services(container, settings, logger)
        app.state.container = container
        app.state.settings = settings
        app.state.database = database

        logger.info(
            "startup",
            extra={
                "app": settings.app_name,
                "version": settings.app_version,
                "environment": settings.environment.value,
                "build_time": settings.build_time,
                "repository_backend": settings.repository_backend.value,
                "cors_origins": settings.cors_origins,
                "user_middleware": [m.cls.__name__ for m in app.user_middleware],
            },
        )
        try:
            yield
        finally:
            logger.info("shutdown")
            container.close()
            if database is not None:
                database.dispose()
            logging.shutdown()

    return lifespan


def _register_services(
    container: Container,
    settings: Settings,
    logger: logging.Logger,
) -> Database | None:
    """Wire repository ports to the configured adapters."""
    database: Database | None = None

    if settings.repository_backend is RepositoryBackend.postgres:
        database = Database.from_settings(settings)
        database.check_connection()
        container.register_instance(Database, database)
        container.register_factory(
            AgentRepository,
            lambda c: PostgresAgentRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            DeviceInventoryRepository,
            lambda c: PostgresDeviceInventoryRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            HeartbeatRepository,
            lambda c: PostgresHeartbeatRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            UserRepository,
            lambda c: PostgresUserRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            SoftwareRepository,
            lambda c: PostgresSoftwareRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            DeviceQueryRepository,
            lambda c: PostgresDeviceQueryRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            AgentStatusRepository,
            lambda c: PostgresAgentStatusRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            PolicyRepository,
            lambda c: PostgresPolicyRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            AgentTagRepository,
            lambda c: PostgresAgentTagRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            SessionRepository,
            lambda c: PostgresSessionRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            ApplicationActivityRepository,
            lambda c: PostgresApplicationActivityRepository(c.resolve(Database).session_factory),
        )
        container.register_factory(
            BrowserActivityRepository,
            lambda c: PostgresBrowserActivityRepository(c.resolve(Database).session_factory),
        )
        logger.info("repositories bound to postgres")
    else:
        container.register_factory(
            AgentRepository,
            lambda _c: MemoryAgentRepository(),
        )
        container.register_factory(
            DeviceInventoryRepository,
            lambda _c: MemoryDeviceInventoryRepository(),
        )
        container.register_factory(
            HeartbeatRepository,
            lambda _c: MemoryHeartbeatRepository(),
        )
        container.register_factory(
            UserRepository,
            lambda c: MemoryUserRepository(c.resolve(AgentRepository)),
        )
        container.register_factory(
            SoftwareRepository,
            lambda c: MemorySoftwareRepository(c.resolve(AgentRepository)),
        )
        container.register_factory(
            DeviceQueryRepository,
            lambda c: MemoryDeviceQueryRepository(
                c.resolve(DeviceInventoryRepository),
                c.resolve(AgentRepository),
                c.resolve(HeartbeatRepository),
            ),
        )
        container.register_factory(
            AgentStatusRepository,
            lambda _c: MemoryAgentStatusRepository(),
        )
        container.register_factory(
            PolicyRepository,
            lambda _c: MemoryPolicyRepository(),
        )
        container.register_factory(
            AgentTagRepository,
            lambda _c: MemoryAgentTagRepository(),
        )
        container.register_factory(
            SessionRepository,
            lambda c: MemorySessionRepository(c.resolve(AgentRepository)),
        )
        container.register_factory(
            ApplicationActivityRepository,
            lambda c: MemoryApplicationActivityRepository(
                c.resolve(AgentRepository),
                c.resolve(SessionRepository),
            ),
        )
        container.register_factory(
            BrowserActivityRepository,
            lambda _c: MemoryBrowserActivityRepository(),
        )
        logger.info("repositories bound to memory")

    container.register_factory(
        FleetProjection,
        lambda c: FleetProjection(
            c.resolve(AgentStatusRepository),
            c.resolve(PolicyRepository),
        ),
    )
    container.register_factory(
        AgentRegistrationService,
        lambda c: AgentRegistrationService(
            c.resolve(AgentRepository),
            projection=c.resolve(FleetProjection),
        ),
    )
    container.register_factory(
        DeviceInventoryService,
        lambda c: DeviceInventoryService(
            c.resolve(AgentRepository),
            c.resolve(DeviceInventoryRepository),
            projection=c.resolve(FleetProjection),
        ),
    )
    container.register_factory(
        HeartbeatService,
        lambda c: HeartbeatService(
            c.resolve(AgentRepository),
            c.resolve(HeartbeatRepository),
            projection=c.resolve(FleetProjection),
        ),
    )
    container.register_factory(
        UserInventoryService,
        lambda c: UserInventoryService(
            c.resolve(AgentRepository),
            c.resolve(UserRepository),
        ),
    )
    container.register_factory(
        SoftwareInventoryService,
        lambda c: SoftwareInventoryService(
            c.resolve(AgentRepository),
            c.resolve(SoftwareRepository),
        ),
    )
    container.register_factory(
        SessionService,
        lambda c: SessionService(
            c.resolve(AgentRepository),
            c.resolve(SessionRepository),
        ),
    )
    container.register_factory(
        ApplicationActivityService,
        lambda c: ApplicationActivityService(
            c.resolve(AgentRepository),
            c.resolve(SessionRepository),
            c.resolve(ApplicationActivityRepository),
        ),
    )
    container.register_factory(
        BrowserActivityService,
        lambda c: BrowserActivityService(
            c.resolve(AgentRepository),
            c.resolve(SessionRepository),
            c.resolve(BrowserActivityRepository),
        ),
    )
    container.register_factory(
        DeviceQueryService,
        lambda c: DeviceQueryService(c.resolve(DeviceQueryRepository)),
    )
    container.register_factory(
        FleetService,
        lambda c: FleetService(
            c.resolve(AgentRepository),
            c.resolve(DeviceInventoryRepository),
            c.resolve(HeartbeatRepository),
            c.resolve(AgentStatusRepository),
            c.resolve(PolicyRepository),
            c.resolve(AgentTagRepository),
            c.resolve(UserInventoryService),
            c.resolve(SoftwareInventoryService),
            c.resolve(SessionService),
            c.resolve(ApplicationActivityService),
        ),
    )
    return database

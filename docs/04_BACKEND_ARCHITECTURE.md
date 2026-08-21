# 04 — Backend Architecture

This document describes the **Platform API** in `platform/`. The UI static server in `backend/` is a separate, smaller FastAPI app documented in [05_FRONTEND_ARCHITECTURE.md](05_FRONTEND_ARCHITECTURE.md).

Package name in `pyproject.toml`: `risklens-backend` version `0.1.0`. Python `>=3.13`.

## FastAPI structure

Entry: `platform/app/main.py` → `create_app()` → `app = create_app()`.

```
platform/app/
  main.py                 create_app, middleware
  openapi.py              OpenAPI metadata
  api/
    router.py             build_api_router()
    errors.py             exception handlers
    v1/                   HTTP handlers
  config/settings.py      RISKLENS_* settings
  core/errors.py          AppError
  dependencies/container.py  DI
  domain/                 entities, commands, errors
  infrastructure/
    memory/               in-memory adapters
    postgres/             SQLAlchemy, Alembic, repos
  logging/                JSON logging + request_id context
  middleware/             request id, logging, timing
  models/                 empty stub — do not put entities here
  repositories/           ports (ABCs)
  schemas/                Pydantic DTOs
  services/               application services
  startup/lifespan.py     wiring + DB check
```

Router assembly: `platform/app/api/router.py`.

**Reserved prefixes not mounted:** activity, browser, network, alerts, incidents, rules, timeline, reports, ai, groups, administration.

Timeline, health, and policy exist as **Fleet sub-routes**, not as those reserved top-level prefixes.

## Domain layer

Framework-free. No FastAPI imports.

### Active packages (entities and commands)

| Package | Role |
|---------|------|
| `domain/agent` | `Agent`, `RegisterAgentCommand`, `DuplicateAgentError` |
| `domain/device` | Inventory upsert + query types |
| `domain/heartbeat` | Heartbeat command, `compute_presence` |
| `domain/user` | User accounts, list query |
| `domain/software` | Installed applications, deterministic `software_id()` |
| `domain/sessions` | `ComputerSession`, merge/immutability, list query |
| `domain/applications` | Application activity, `activity_id()` |
| `domain/fleet` | `FleetRecord`, `compute_fleet_status`, policy constants, `NOT_IMPLEMENTED` |

### Empty placeholder packages (`__init__.py` only)

Sprint 13.5: `activity`, `ai`, `alerts`, `browser`, `groups`, `reports`, `rules`, `timeline`

Sprint 13.6: `administration`, `incidents`, `network`

Architecture tests assert these packages contain no `def`/`class`.

There is **no** `domain/processes`, `domain/services` (OS services), `domain/policy`, or `domain/health` package. Policy and health types live under `domain/fleet`.

## Services

| Class | File | Methods |
|-------|------|---------|
| `AgentRegistrationService` | `services/agent_service.py` | `register` |
| `HeartbeatService` | `services/heartbeat_service.py` | `record` |
| `DeviceInventoryService` | `services/device_service.py` | `upsert` → `(inventory, created)` |
| `DeviceQueryService` | `services/device_query_service.py` | `get`, `list` |
| `UserInventoryService` | `services/user_service.py` | `upsert`, `get`, `list` |
| `SoftwareInventoryService` | `services/software_service.py` | `upsert`, `get`, `list` |
| `SessionService` | `services/session_service.py` | `upsert`, `get`, `list`, `close_open_for_agent` |
| `ApplicationActivityService` | `services/application_activity_service.py` | `upsert`, `get`, `list` |
| `FleetProjection` | `services/fleet_service.py` | `on_registered`, `on_heartbeat`, `on_inventory` |
| `FleetService` | `services/fleet_service.py` | `list`, `get`, `users`, `software`, `sessions`, `applications`, `health`, `policy`, `require_agent` |

`session_lifecycle_log.py` is a logging helper (`banner`, `error`, `fatal`, `dump_query`), not a service class.

Unknown-agent checks happen in each mutating service by calling `AgentRepository.exists`.

## Repositories

Ports in `platform/app/repositories/`. Every port has:

- `infrastructure/postgres/*_repository.py`
- `infrastructure/memory/memory_*_repository.py`

| Port | Postgres | Memory |
|------|----------|--------|
| `AgentRepository` | `PostgresAgentRepository` | `MemoryAgentRepository` |
| `DeviceInventoryRepository` | `PostgresDeviceInventoryRepository` | `MemoryDeviceInventoryRepository` |
| `DeviceQueryRepository` | `PostgresDeviceQueryRepository` | `MemoryDeviceQueryRepository` |
| `HeartbeatRepository` | `PostgresHeartbeatRepository` | `MemoryHeartbeatRepository` |
| `UserRepository` | `PostgresUserRepository` | `MemoryUserRepository` |
| `SoftwareRepository` | `PostgresSoftwareRepository` | `MemorySoftwareRepository` |
| `SessionRepository` | `PostgresSessionRepository` | `MemorySessionRepository` |
| `ApplicationActivityRepository` | `PostgresApplicationActivityRepository` | `MemoryApplicationActivityRepository` |
| `AgentStatusRepository` | `PostgresAgentStatusRepository` | `MemoryAgentStatusRepository` |
| `PolicyRepository` | `PostgresPolicyRepository` | `MemoryPolicyRepository` |
| `AgentTagRepository` | `PostgresAgentTagRepository` | `MemoryAgentTagRepository` |

`SessionRepository` extra methods:

- `upsert_many`
- `get`, `exists`, `list`
- `activate_current` — transactional close-other-actives + insert. **Implemented. Not called by `SessionService`.**
- `close_active_for_agent` — direct SQL UPDATE of the single active row. **Implemented. `SessionService.close_open_for_agent` uses `upsert_many` instead.**

## Database

Engine + session factory: `infrastructure/postgres/database.py`.

- PostgreSQL: connection pool from settings
- SQLite URLs (tests / offline schema): `StaticPool`
- `check_connection()` on startup for postgres backend

ORM models: `infrastructure/postgres/models.py`. Custom `GUID` type (UUID on PostgreSQL, `CHAR(36)` elsewhere). Naming convention prefixes: `ix_`, `uq_`, `ck_`, `fk_`, `pk_`.

Migrations: Alembic `0001`–`0007`. See [06_DATABASE.md](06_DATABASE.md).

`RISKLENS_REPOSITORY_BACKEND=memory` skips PostgreSQL entirely.

## Models vs schemas vs domain

| Layer | Location | Role |
|-------|----------|------|
| ORM | `infrastructure/postgres/models.py` | Tables |
| Domain | `domain/*/entity.py` | Frozen dataclasses, merge rules |
| HTTP | `schemas/*.py` | Pydantic request/response, `extra="forbid"` on ingest bodies |

`platform/app/models/__init__.py` is an empty reserved stub. Nothing imports `app.models`.

## Dependency injection

`platform/app/dependencies/container.py` — simple registry (`register` / `resolve`).

`startup/lifespan.py` `_register_services`:

1. Choose postgres or memory adapters
2. For postgres: `Database.check_connection()`
3. Construct repositories
4. Construct services
5. Store `container`, `settings`, `database` on `app.state`

Request handlers resolve services from `request.app.state.container`.

There is no third-party DI framework.

## API

HTTP contracts: [07_API_REFERENCE.md](07_API_REFERENCE.md).

OpenAPI customization: `platform/app/openapi.py`. Interactive docs at `/docs` and `/redoc`.

## Validation

1. **Pydantic** on request bodies (`extra="forbid"` on agent ingest models). String fields typically `strip_whitespace`, `min_length=1`, max lengths 256–512.
2. **Path vs body agent_id** on heartbeat: mismatch → `AgentIDMismatchError` 422 `validation_error`.
3. **Domain `__post_init__`** on entities (UUID types, non-empty username, allowed session statuses).
4. **Query objects** (`ListSessionsQuery`, fleet query, …) reject bad `limit`/`offset`/`sort` → 422 `invalid_query`.
5. **SQL check constraints** as a last line of defense.

## Error handling

Base: `app.core.errors.AppError` with `status_code`, `code`, `message`, `details`.

Handlers (`api/errors.py`) always return:

```json
{
  "error": {
    "code": "...",
    "message": "...",
    "details": ...,
    "request_id": "..."
  }
}
```

| Exception | Status | Code |
|-----------|--------|------|
| `DuplicateAgentError` | 409 | `duplicate_agent` |
| `UnknownAgentError` (several domain copies) | 404 | `unknown_agent` |
| `AgentIDMismatchError` | 422 | `validation_error` |
| `DeviceNotFoundError` | 404 | `not_found` |
| `UserNotFoundError` | 404 | `user_not_found` |
| `SoftwareNotFoundError` | 404 | `software_not_found` |
| `SessionNotFoundError` | 404 | `session_not_found` |
| `UnknownSessionError` | 422 | `unknown_session` |
| `ApplicationActivityNotFoundError` | 404 | `application_activity_not_found` |
| `FleetAgentNotFoundError` | 404 | (fleet not found) |
| `Invalid*QueryError` | 422 | `invalid_query` |
| `MultipleActiveSessionsError` | 500 | `multiple_active_sessions` |
| `RequestValidationError` | 422 | `validation_error` |
| Uncaught `Exception` | 500 | `internal_error` |
| Starlette HTTP | mapped | `bad_request`, `not_found`, … |

`MultipleActiveSessionsError` exists on the sessions domain. **`SessionService` does not raise it.** The REPRO dump logs CRITICAL when two ACTIVE rows exist and continues.

## Background tasks

**Not implemented.** No `BackgroundTasks`, no Celery, no APScheduler, no worker process.

Offline session close runs **synchronously** inside Fleet GET handlers when the agent is computed offline.

## Caching

**Not implemented** except `@lru_cache(maxsize=1)` on `get_settings()`.

No Redis. Heartbeats are append-only; presence is computed per read.

## Transactions

SQLAlchemy session per repository operation. Session upsert:

```sql
INSERT ... ON CONFLICT (session_id) DO UPDATE SET ...
WHERE computer_sessions.status != 'closed'
```

`activate_current` (unused by the service) wraps close-others + insert in one transaction and can abort if two ACTIVE rows remain.

Memory adapters use in-process locks for concurrency tests.

## Logging

`platform/app/logging/setup.py` — JSON logs when `RISKLENS_LOG_JSON=true`.

`logging/context.py` holds `request_id` contextvar.

Middleware:

- `RequestIDMiddleware` — header `X-Request-ID` (configurable)
- `RequestLoggingMiddleware` — method, path, client, status, duration
- `TimingMiddleware` — `X-Response-Time-ms`

Session lifecycle banners go to logger `risklens.sessions.lifecycle` and include REPRO SQL dumps.

Startup prints debug lines to stdout for CORS origins and middleware class names (`main.py`).

## Middleware order

`add_middleware` is a stack (last added runs first on the request):

Request: **CORS → TrustedHost → GZip → RequestID → RequestLogging → Timing → route**

CORS is installed only if `cors_origins` is non-empty. `allow_credentials=False`, methods/headers `*`.

## Lifespan

Startup: configure logging → DI → DB check → log metadata.

Shutdown: `container.close()` → `database.dispose()` → `logging.shutdown()`.

If Postgres is down, **the API process exits** and nothing listens on port 8000. That is why browsers show `ERR_CONNECTION_REFUSED` when Docker/Postgres is stopped.

## Related documents

- API catalog: [07_API_REFERENCE.md](07_API_REFERENCE.md)
- Schema: [06_DATABASE.md](06_DATABASE.md)
- Sessions: [08_AGENT_SESSION_LIFECYCLE.md](08_AGENT_SESSION_LIFECYCLE.md)

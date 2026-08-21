# RiskLens Backend — Layer Architecture

The official product architecture is `docs/ARCHITECTURE.md` (Sprint 13.7 baseline).
This file describes backend layers only.

Sprint 1 delivered the foundation of the RiskLens Platform backend. Later sprints
added agents, inventory, Fleet, and the computer-centric UI. Authentication is
still out of scope.

## Layers

Clean Architecture. Dependency arrows point downward only.

```text
+-------------------+
|      API          |  FastAPI routers, request/response shape
+-------------------+
          |
          v
+-------------------+
|    Services       |  Application use cases (empty in Sprint 1)
+-------------------+
          |
          v
+-------------------+
|  Repositories     |  Persistence ports (empty in Sprint 1)
+-------------------+
          |
          v
+-------------------+
|  Infrastructure   |  Future adapters (Postgres, OpenSearch, Kafka, ...)
+-------------------+
```

Cross-cutting: `core`, `config`, `logging`, `middleware`, `schemas`,
`dependencies`, `startup`. None of them import from `api`.

## Startup flow

```text
uvicorn app.main:app
        |
        v
create_app(settings)
        |
        +-- install middleware (TrustedHost -> GZip -> CORS -> RequestID -> Timing -> Logging)
        +-- register exception handlers
        +-- include API router (/, /health, /version)
        +-- customize OpenAPI
        |
        v
lifespan(startup)
        |
        +-- configure structured logging
        +-- build Container, bind Settings
        +-- log "startup"
        |
        v
      READY
        |
        v
lifespan(shutdown)
        |
        +-- log "shutdown"
        +-- container.close()
        +-- logging.shutdown()
```

## Dependency container

`app.dependencies.container.Container` holds `Settings` and, later, services
and repositories. Routes never import global singletons directly; they resolve
dependencies through `get_settings_dep` (and later `get_service_dep`).

## Request pipeline

For every request:

1. `TrustedHostMiddleware` rejects unknown hosts.
2. `GZipMiddleware` compresses eligible responses.
3. `CORSMiddleware` applies CORS if origins are configured.
4. `RequestIDMiddleware` sets an ID (from the header or a UUID) and stores it
   in a `ContextVar` so structured logs and error envelopes can reference it.
5. `TimingMiddleware` measures duration and sets `X-Response-Time-ms`.
6. `RequestLoggingMiddleware` writes a request/response pair as JSON logs.
7. The router handles the request.
8. Exception handlers translate any error into a consistent envelope.

## Error envelope

Every error goes through `register_exception_handlers` and returns:

```json
{
  "error": {
    "code": "not_found",
    "message": "…",
    "details": null,
    "request_id": "…"
  }
}
```

Handled: `AppError`, `HTTPException` (404, 405, 400, …),
`RequestValidationError` (422), and any uncaught `Exception` (500).

## Future extension points

- Register a `Postgres`/`OpenSearch` adapter with `Container.register_factory`
  during startup, expose it through a repository interface.
- Add `/api/v1` routers under `app/api/v1/` (e.g., agents, events, policies).
- Add health sub-checks (database, message bus) behind a new `readiness`
  endpoint; keep `/health` as pure liveness.
- Add auth middleware between `RequestID` and the route.
- Add background workers by wiring an async task supervisor into the lifespan.

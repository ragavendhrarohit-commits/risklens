# RiskLens Backend

RiskLens Platform backend. FastAPI on Python 3.13 with clean architecture.
Agent registration is persisted to PostgreSQL through SQLAlchemy 2.x, with
schema managed by Alembic.

## Run

```powershell
cd platform
py -3.13 -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -e ".[test]"
copy .env.example .env

# PostgreSQL (any instance works; this is the local dev default)
docker run -d --name risklens-postgres `
  -e POSTGRES_USER=risklens -e POSTGRES_PASSWORD=risklens -e POSTGRES_DB=risklens `
  -p 5432:5432 postgres:17-alpine

alembic upgrade head
uvicorn app.main:app --host 127.0.0.1 --port 8000 --reload
```

To accept agents on the LAN (development only; localhost still works):

```powershell
.\scripts\run_dev.ps1
```

That starts `uvicorn` with `--host 0.0.0.0 --port 8000`. Revert to localhost-only with `--host 127.0.0.1`.

Windows Firewall does not allow inbound TCP 8000 by default. From an elevated PowerShell session:

```powershell
New-NetFirewallRule -DisplayName "RiskLens Dev API 8000" -Direction Inbound -Protocol TCP -LocalPort 8000 -Action Allow -Profile Private
```

Remove the rule after LAN testing:

```powershell
Remove-NetFirewallRule -DisplayName "RiskLens Dev API 8000"
```

To run without a database, use the in-memory adapter:

```powershell
$env:RISKLENS_REPOSITORY_BACKEND="memory"
uvicorn app.main:app --host 127.0.0.1 --port 8000
```

Open:

- <http://127.0.0.1:8000/> — service metadata
- <http://127.0.0.1:8000/health> — liveness
- <http://127.0.0.1:8000/version> — build metadata
- <http://127.0.0.1:8000/docs> — Swagger UI
- <http://127.0.0.1:8000/openapi.json> — OpenAPI schema

## Test

```powershell
pytest
```

The PostgreSQL suites use a separate `risklens_test` database, created
automatically on first run and rebuilt with Alembic. If no database is
reachable they skip rather than fail, so the rest of the suite still runs.
Override the target with `RISKLENS_TEST_DATABASE_URL`.

## Structure

```text
platform/
├── app/
│   ├── api/           HTTP layer (routers, exception handlers)
│   │   └── v1/        Versioned routes: /, /health, /version
│   ├── core/          Domain errors and cross-cutting types
│   ├── config/        Pydantic Settings
│   ├── logging/       JSON logging + request-id context
│   ├── dependencies/  DI container
│   ├── middleware/    Request ID, timing, request logging
│   ├── domain/        Framework-free entities, commands, errors
│   ├── schemas/       Pydantic request/response models
│   ├── services/      Application services
│   ├── repositories/  Persistence ports (AgentRepository)
│   ├── infrastructure/
│   │   ├── memory/    In-memory AgentRepository adapter
│   │   └── postgres/  SQLAlchemy adapter
│   │       ├── database.py        Engine + session factory
│   │       ├── models.py          ORM mapping for the agents table
│   │       ├── agent_repository.py
│   │       └── migrations/        Alembic environment + revisions
│   ├── startup/       Lifespan, container wiring
│   ├── openapi.py     Custom OpenAPI metadata
│   └── main.py        create_app factory + module-level app
├── tests/             pytest suite
├── docs/              Architecture notes
├── scripts/           Reserved for tooling
├── alembic.ini
├── .env.example
└── pyproject.toml
```

## Configuration

All settings come from `RISKLENS_*` environment variables or the `.env` file.
See `.env.example` for the full list and defaults. Invalid values fail at
startup, as does an unreachable database when the postgres backend is selected.

| Setting | Default | Purpose |
| --- | --- | --- |
| `RISKLENS_REPOSITORY_BACKEND` | `postgres` | `postgres` or `memory` |
| `RISKLENS_DATABASE_URL` | local dev DSN | SQLAlchemy connection URL |
| `RISKLENS_DATABASE_POOL_SIZE` | `5` | Persistent pooled connections |
| `RISKLENS_DATABASE_MAX_OVERFLOW` | `10` | Burst connections above the pool |
| `RISKLENS_DATABASE_POOL_TIMEOUT` | `30` | Seconds to wait for a connection |
| `RISKLENS_DATABASE_POOL_RECYCLE` | `1800` | Recycle connections after N seconds |
| `RISKLENS_DATABASE_CONNECT_TIMEOUT` | `10` | TCP connect timeout |
| `RISKLENS_DATABASE_ECHO` | `false` | Log emitted SQL |

## Migrations

See `app/infrastructure/postgres/migrations/README.md`. Alembic resolves its
connection string from the same `Settings` the application uses, so migrations
and the service can never target different databases by accident.

```powershell
alembic upgrade head      # apply
alembic downgrade -1      # roll back one
alembic check             # fail if models.py has drifted from the migrations
```

# 12 — Code Map

Repository root: `C:\Users\rohit\OneDrive\Desktop\Workflow Project\Risklens` (path on the development machine). Git history may be incomplete; treat the tree as source of truth.

## Top-level layout

```
Risklens/
  agent/          Go Windows agent (module risklens/agent)
  backend/        FastAPI static UI server (port 8080)
  frontend/       SPA (html/js/css)
  platform/       FastAPI Platform API (port 8000)
  docs/           Engineering documentation (this package)
  requirements.txt          Legacy UI-server deps (includes unused elasticsearch)
  README.md                 Stale Elasticsearch-oriented quickstart
  .env.example              Stale Elasticsearch env
```

There is **no** root `docker-compose.yml`, **no** root `Makefile`, and **no** monorepo workspace file.

---

## Entry points

| Process | Entry | Typical command |
|---------|-------|-----------------|
| Platform API | `platform/app/main.py` (`app`) | `cd platform` → `python -B -m uvicorn app.main:app --host 0.0.0.0 --port 8000` |
| UI server | `backend/app/main.py` (`app`) | repo root → `uvicorn backend.app.main:app --host 127.0.0.1 --port 8080` |
| Agent | `agent/cmd/risklens-agent/main.go` | `agent/bin/RiskLensAgent.exe -console -config ...` |
| Agent helper | same binary `--activity-foreground` | spawned by activity collector |
| Device verify harness | `agent/scripts/verify_device/main.go` | not product |
| Device restart harness | `agent/scripts/verify_device_restart/main.go` | not product |

---

## `agent/` packages and dependencies

```
cmd/risklens-agent
  → configuration, identity, logging, app, service, version
  → modules: device, registration, deviceupload, heartbeat, users, software, activity

internal/app
  → modules/registry, modules/manager, modules/interfaces

internal/service
  → app; windows: golang.org/x/sys/windows/svc
  → activity as SessionChangeSink

internal/configuration
  → yaml.v3 via storage; watcher polls file

internal/config          LEGACY — tests only, not main

internal/identity        no network

internal/modules/*       → transport/interfaces only (no net/http)

internal/transport/client  ONLY net/http importer among modules

internal/logging         slog + optional eventlog (windows)
```

External Go modules (`go.mod`): `golang.org/x/sys`, `gopkg.in/yaml.v3`.

---

## `platform/` packages and dependencies

```
app/main.py
  → api.build_api_router
  → middleware
  → startup.build_lifespan
  → config.get_settings

app/api/v1/*
  → schemas, services, session_lifecycle_log

app/services/*
  → domain, repositories

app/repositories/*          ports

app/infrastructure/postgres → sqlalchemy, models, alembic
app/infrastructure/memory   → dict adapters

app/domain/*                no fastapi
```

Python deps (`platform/pyproject.toml`): fastapi, uvicorn, pydantic, pydantic-settings, python-dotenv, sqlalchemy, psycopg, alembic. Test extras: pytest, pytest-asyncio, httpx.

---

## `backend/` files

```
backend/app/__init__.py
backend/app/main.py      StaticFiles + FileResponse
backend/app/config.py     metadata_db_path
backend/app/db.py         SQLite identities — unused by routes
```

Depends on root `requirements.txt` (fastapi, uvicorn, elasticsearch unused, pydantic-settings, python-dotenv).

---

## `frontend/` files

```
frontend/index.html
frontend/assets/app.js
frontend/assets/app.css
```

Depends on browser CDNs (Tailwind, Google Fonts) and Platform CORS.

---

## `docs/`

```
docs/README.md
docs/01_PROJECT_OVERVIEW.md … 15_NEXT_SPRINT.md
docs/ARCHITECTURE.md          frozen Sprint 13.7 IA
platform/docs/architecture.md historical Sprint 1 backend notes (stale)
agent/README.md               stale Sprint 1 agent notes
platform/README.md            how to run API/migrations
```

---

## Scripts

| Path | Purpose |
|------|---------|
| `platform/scripts/run_dev.ps1` | LAN bind API |
| `agent/scripts/windows/install.ps1` | SCM install |
| `agent/scripts/windows/uninstall.ps1` | SCM delete |
| `agent/Makefile` | build/test/vet |
| `agent/scripts/verify_device/` | Sprint 7 harness |
| `agent/scripts/verify_device_restart/` | restart comparison harness |

---

## Test trees

```
platform/tests/*.py          36 test modules + conftest.py
agent/internal/**/*_test.go  34 files
```

---

## Data / runtime paths (not all in git)

| Path | Owner |
|------|--------|
| `platform/.env` | API settings |
| `%ProgramData%\RiskLens\agent.yaml` | Agent |
| `%ProgramData%\RiskLens\state\identity.json` | Identity |
| `%ProgramData%\RiskLens\state\activity.json` | Session state |
| `%ProgramData%\RiskLens\logs\agent.log` | Agent logs |
| `backend/data/risklens.db` | Unused SQLite |
| PostgreSQL `risklens` | Platform |

---

## Import rules (enforced by design, not all by tests)

- Agent modules must not import `net/http` except `transport/client`
- Platform domain must not import FastAPI
- Domain entities must not live in `platform/app/models/`
- Frontend must not embed Elasticsearch credentials (none are present)

---

## Dual-backend inconsistency

| Name in comments | Actual live code |
|------------------|------------------|
| “backend schemas” | `platform/app/schemas/` |
| `backend/app/main.py` | UI static only |
| Root README Elasticsearch | Unused |

When adding APIs, change `platform/`, not `backend/`.

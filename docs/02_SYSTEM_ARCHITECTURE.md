# 02 — System Architecture

This document describes every running component and how they connect. File paths are relative to the repository root.

## End-to-end data path

```mermaid
flowchart TD
  subgraph windows [Windows endpoint]
    Login[Windows logon / WTS]
    FG[Foreground window]
    HW[WMI / registry / Win32]
    Agent[RiskLensAgent.exe]
    Disk[identity.json / activity.json / queue]
    Login --> Agent
    FG --> Agent
    HW --> Agent
    Agent --> Disk
  end

  subgraph transport [Transport]
    HTTP[internal/transport/client]
    Agent --> HTTP
  end

  subgraph api [Platform API :8000]
    Routers[FastAPI routers]
    Services[Application services]
    Repos[Repository ports]
    PG[(PostgreSQL)]
    HTTP --> Routers
    Routers --> Services
    Services --> Repos
    Repos --> PG
  end

  subgraph ui [Operator UI]
    Browser[Browser]
    Static[backend FastAPI :8080]
    SPA[frontend/index.html + app.js]
    Browser --> Static
    Static --> SPA
    SPA -->|GET JSON| Routers
  end
```

## Component catalog

### 1. Windows Agent (`agent/`)

**Process:** `RiskLensAgent.exe` (`agent/cmd/risklens-agent/main.go`)

**Responsibilities:**

- Run as a Windows service (`RiskLensAgent`) or in console/debug mode
- Load `%ProgramData%\RiskLens\agent.yaml`
- Persist identity in `identity.json`
- Start modules in a fixed order
- Collect and POST JSON to the Platform API
- On logoff/shutdown, flush the open session through `activity.CloseSession`

**Does not:**

- Serve HTTP
- Query PostgreSQL
- Render UI
- Authenticate operators

Details: [03_AGENT_ARCHITECTURE.md](03_AGENT_ARCHITECTURE.md)

### 2. Transport (`agent/internal/transport/`)

The only agent package allowed to import `net/http`.

| Piece | Role |
|-------|------|
| `interfaces.Transport` | `Do(ctx, Request) (Response, error)` contract |
| `client.Client` | Timeout, JSON encode/decode, max 1 MiB response |
| `middleware` | Headers, Request-ID, logging. **No retry.** |
| `errors` | Typed kinds: timeout, canceled, connection, encode, decode, unexpected_status, … |

Each enabled network module constructs **its own** client. Clients are not shared.

### 3. Platform API (`platform/`)

**Process:** Uvicorn serving `app.main:app`

**Default bind:** `--host 127.0.0.1 --port 8000` (README). LAN bind: `platform/scripts/run_dev.ps1` uses `--host 0.0.0.0 --port 8000`. Settings fields `RISKLENS_HOST` / `RISKLENS_PORT` are **not** applied automatically; Uvicorn CLI controls the bind.

**Layering:**

```
HTTP request
  → middleware (CORS, TrustedHost, GZip, RequestID, RequestLogging, Timing)
  → api/v1 router
  → Pydantic schema
  → service
  → domain merge/validation
  → repository port
  → postgres or memory adapter
```

Startup (`platform/app/startup/lifespan.py`) configures logging, builds the DI `Container`, and **fails fast** if PostgreSQL is unreachable when `RISKLENS_REPOSITORY_BACKEND=postgres`.

Details: [04_BACKEND_ARCHITECTURE.md](04_BACKEND_ARCHITECTURE.md)

### 4. Database (PostgreSQL)

Default DSN from `platform/.env.example`:

`postgresql+psycopg://risklens:risklens@127.0.0.1:5432/risklens`

Schema is owned by Alembic revisions `0001`–`0007`. Tables:

- `agents`
- `device_inventory`
- `agent_heartbeats`
- `user_inventory`
- `software_inventory`
- `policies`, `agent_policy`, `agent_tags`, `agent_status`
- `computer_sessions`, `application_activity`

Details: [06_DATABASE.md](06_DATABASE.md)

### 5. Frontend SPA (`frontend/`)

Three files:

- `frontend/index.html` — shell, nav, all views
- `frontend/assets/app.js` — routing, state, fetch, render
- `frontend/assets/app.css` — presence badges, fleet tabs, tables

Default landing view is **Fleet**. The browser calls `window.RISKLENS_PLATFORM_API` (default `http://127.0.0.1:8000`).

Details: [05_FRONTEND_ARCHITECTURE.md](05_FRONTEND_ARCHITECTURE.md)

### 6. UI static server (`backend/`)

`backend/app/main.py`:

| Route | Behavior |
|-------|----------|
| `GET /` | `FileResponse(frontend/index.html)` |
| `/assets/*` | `StaticFiles(frontend/assets)` |

Lifespan calls `init_metadata_db()` which creates SQLite table `identities`. **No HTTP handler uses that table.**

This process is **not** the Platform API. Opening `http://127.0.0.1:8000/` returns JSON service metadata, not the SPA.

### 7. Operator (User)

The operator uses a browser:

1. Open `http://127.0.0.1:8080`
2. SPA loads Tailwind/fonts from CDNs (requires outbound internet for first paint styling)
3. SPA `GET`s Fleet and inventory from port 8000
4. CORS must include `http://127.0.0.1:8080` (`RISKLENS_CORS_ORIGINS`)

There is **no login**. There are no operator accounts in the Platform API.

## Request classes

### Agent → API (ingest)

| Module | Method | Path |
|--------|--------|------|
| registration | POST | `/api/v1/agents/register` |
| heartbeat | POST | `/api/v1/agents/{agent_id}/heartbeat` |
| deviceupload | POST | `/api/v1/devices` |
| users | POST | `/api/v1/users` |
| software | POST | `/api/v1/software` |
| activity | POST | `/api/v1/sessions` |
| activity | POST | `/api/v1/applications` |

### Browser → API (query)

| UI | Method | Path |
|----|--------|------|
| Fleet list / dashboard cards | GET | `/api/v1/fleet` |
| Endpoint overview | GET | `/api/v1/fleet/{agent_id}` |
| Accounts tab | GET | `/api/v1/fleet/{agent_id}/users` |
| Installed Applications tab | GET | `/api/v1/fleet/{agent_id}/software` |
| Sessions tab | GET | `/api/v1/fleet/{agent_id}/sessions` |
| Application Activity tab | GET | `/api/v1/fleet/{agent_id}/applications` |
| Health tab | GET | `/api/v1/fleet/{agent_id}/health` |
| Policies tab | GET | `/api/v1/fleet/{agent_id}/policy` |
| Account Inventory page | GET | `/api/v1/users` |
| Software Inventory page | GET | `/api/v1/software` |

### Implemented API not wired in the frontend

- `GET /api/v1/devices` and `GET /api/v1/devices/{id}`
- `GET /api/v1/sessions`, `GET /api/v1/sessions/{id}`
- `GET /api/v1/applications`, `GET /api/v1/applications/{id}`
- `GET /api/v1/users/{sid}`, `GET /api/v1/software/{id}`
- `GET /api/v1/fleet/{id}/timeline` (stub)
- `GET /`, `/health`, `/version`, `/docs`

## Presence model (shared by Devices query and Fleet)

Computed at read time from `last_seen` (`platform/app/domain/heartbeat/entity.py`):

| Age of last heartbeat | Device `status` | Fleet `online_status` |
|-----------------------|-----------------|------------------------|
| ≤ 60 seconds | `online` | `online` (healthy if device inventory exists) |
| 61–300 seconds | `stale` | `warning` |
| > 300 seconds or never | `offline` | `offline` or `never_seen` |

When Fleet materializes an **offline** agent, `FleetService` may attempt offline session reconciliation (`SessionService.close_open_for_agent`). See [08_AGENT_SESSION_LIFECYCLE.md](08_AGENT_SESSION_LIFECYCLE.md).

## Trust and security (as implemented)

| Concern | Status |
|---------|--------|
| TLS | **Not implemented** (HTTP URLs in examples) |
| Agent authentication | **Not implemented** (anyone who can POST a registered `agent_id` can ingest) |
| Operator authentication | **Not implemented** |
| CORS | Origin allow-list for the UI origin |
| Trusted hosts | Default `*` |
| Secrets in agent YAML | Backend URL only; no API key field exists |

## Deployment topology used in development

Typical two-laptop setup encoded in `agent/configs/agent.lan.example.yaml`:

- Platform host: `192.168.4.99:8000` (example IP in the LAN example file)
- Agent `networking.backend_url: http://192.168.4.99:8000`
- Local UI still `http://127.0.0.1:8080` on the platform host
- Windows Firewall may block inbound TCP 8000 (`platform/README.md` documents a firewall rule)

Docker is used for PostgreSQL (`risklens-postgres` in operational notes). There is **no** `docker-compose.yml` in this repository.

## Inconsistencies in the architecture

These are real, not hypothetical:

1. **Two FastAPI apps.** UI static files are not mounted on the Platform API. Operators must run both processes.
2. **Root README vs code.** README describes Elasticsearch Monitor/Ingest; the SPA is Fleet-centric and talks to `platform/`.
3. **`backend/` SQLite `identities` table** is unused.
4. **Agent `README.md`** still says Sprint 1 “no networking”.
5. **`docs/ARCHITECTURE.md`** still lists sessions/applications as reserved; they are implemented.
6. **Registration comment** in `agent/internal/modules/registration/request.go` cites `backend/app/schemas/agents.py`; the live schema is `platform/app/schemas/agents.py`.
7. **Settings host/port** do not bind Uvicorn by themselves.

## Related documents

- Agent: [03_AGENT_ARCHITECTURE.md](03_AGENT_ARCHITECTURE.md)
- Backend: [04_BACKEND_ARCHITECTURE.md](04_BACKEND_ARCHITECTURE.md)
- Frontend: [05_FRONTEND_ARCHITECTURE.md](05_FRONTEND_ARCHITECTURE.md)
- Sequences: [13_SEQUENCE_DIAGRAMS.md](13_SEQUENCE_DIAGRAMS.md)

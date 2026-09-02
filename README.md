# RiskLens

Open-source Windows endpoint monitoring platform. A Windows agent collects inventory and activity from managed computers; a Platform API stores the data; a web console lets operators browse fleet, sessions, applications, browser usage, geolocation, and reports.

## What is in this repo

| Folder | What it does | Tech |
|--------|--------------|------|
| `agent/` | Windows agent (service or console mode) | Go |
| `platform/` | REST API, business logic, PostgreSQL persistence | Python 3.13, FastAPI |
| `frontend/` | Operator web UI (Fleet, Reports, Geo Location, etc.) | Vanilla HTML/CSS/JS |
| `backend/` | Static file server for the UI on port **8080** | Python, FastAPI |
| `docs/` | Engineering documentation (architecture, APIs, database) | Markdown |

The browser loads the UI from **:8080** and calls the Platform API on **:8000**.

## Prerequisites

- **Windows** (agent + primary dev target)
- **Python 3.13**
- **Go 1.22+** (only if you work on the agent)
- **PostgreSQL 17** (Docker is fine for local dev)
- **Git**

Optional: Docker Desktop (to run Postgres in one command).

## Quick start (full stack)

Open **two terminals** for the servers, plus a third if you also run the agent.

### 1. Platform API (`:8000`)

```powershell
cd platform
py -3.13 -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -e ".[test]"
copy .env.example .env

# Start PostgreSQL (skip if you already have one)
docker run -d --name risklens-postgres `
  -e POSTGRES_USER=risklens -e POSTGRES_PASSWORD=risklens -e POSTGRES_DB=risklens `
  -p 5432:5432 postgres:17-alpine

alembic upgrade head
.\.venv\Scripts\python.exe -B -m uvicorn app.main:app --host 127.0.0.1 --port 8000
```

API docs: http://127.0.0.1:8000/docs

To run **without PostgreSQL** (in-memory only, data lost on restart):

```powershell
$env:RISKLENS_REPOSITORY_BACKEND="memory"
.\.venv\Scripts\python.exe -B -m uvicorn app.main:app --host 127.0.0.1 --port 8000
```

### 2. UI server (`:8080`)

From the **repo root**:

```powershell
py -3.13 -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
uvicorn backend.app.main:app --reload --host 127.0.0.1 --port 8080
```

Open: http://127.0.0.1:8080

### 3. Windows agent (optional)

```powershell
cd agent
go build -o bin\RiskLensAgent.exe .\cmd\risklens-agent

# Console mode (development)
.\bin\RiskLensAgent.exe -console -config configs\agent.example.yaml
```

Copy `configs/agent.example.yaml` to `%ProgramData%\RiskLens\agent.yaml` and set `platform.base_url` to your API (default `http://127.0.0.1:8000`). See `agent/README.md` for Windows service install.

## Verify it works

1. UI loads at http://127.0.0.1:8080
2. API health: http://127.0.0.1:8000/health
3. Fleet page shows registered agents after the agent runs and heartbeats

## Where to change things

Use this map when customizing for your environment or adding features.

| You want to… | Start here |
|--------------|------------|
| Change UI layout, tabs, tables, charts | `frontend/index.html`, `frontend/assets/app.js`, `frontend/assets/app.css` |
| Add or change API endpoints | `platform/app/api/v1/`, then `platform/app/services/` |
| Change database schema | `platform/app/infrastructure/postgres/models.py` + new Alembic migration in `platform/app/infrastructure/postgres/migrations/versions/` |
| Change business rules / aggregation | `platform/app/domain/`, `platform/app/services/` |
| Change what the agent collects | `agent/internal/modules/` (e.g. `activity`, `heartbeat`, `health`, `users`, `software`) |
| Change agent config or intervals | `agent/configs/agent.example.yaml`, `agent/internal/configuration/` |
| Change API ↔ UI contract (request/response shapes) | `platform/app/schemas/` and matching code in `frontend/assets/app.js` |

### Typical dev workflow

1. Edit code in the area above.
2. **Platform**: API auto-reloads if you started uvicorn with `--reload`.
3. **UI**: refresh the browser; hard-refresh (Ctrl+F5) if CSS/JS looks cached.
4. **Agent**: rebuild with `go build`, restart the agent process.
5. **Database change**: `cd platform` → `alembic revision --autogenerate -m "describe change"` → `alembic upgrade head`.
6. Run tests before pushing:

```powershell
# Platform
cd platform
pytest

# Agent
cd agent
go test ./...
```

## Configuration

| Component | Config file | Notes |
|-----------|-------------|-------|
| Platform API | `platform/.env` | Copy from `platform/.env.example`. Key vars: `RISKLENS_DATABASE_URL`, `RISKLENS_CORS_ORIGINS` |
| Agent | `%ProgramData%\RiskLens\agent.yaml` | Copy from `agent/configs/agent.example.yaml` |
| UI → API URL | `frontend/assets/app.js` | Default `http://127.0.0.1:8000` via `window.RISKLENS_PLATFORM_API` |

Full env reference: `docs/10_CONFIGURATION.md`

## Documentation

Start here for deeper reading:

1. [docs/01_PROJECT_OVERVIEW.md](docs/01_PROJECT_OVERVIEW.md) — product and current status
2. [docs/12_CODE_MAP.md](docs/12_CODE_MAP.md) — full repo layout and entry points
3. [docs/07_API_REFERENCE.md](docs/07_API_REFERENCE.md) — every HTTP endpoint
4. [docs/05_FRONTEND_ARCHITECTURE.md](docs/05_FRONTEND_ARCHITECTURE.md) — SPA routing and state
5. [docs/03_AGENT_ARCHITECTURE.md](docs/03_AGENT_ARCHITECTURE.md) — agent modules and lifecycle

Subsystem READMEs:

- [platform/README.md](platform/README.md) — backend setup, migrations, tests
- [agent/README.md](agent/README.md) — agent build and Windows service install

## Project layout

```
Risklens/
├── agent/           # Go Windows agent
├── backend/         # UI static server (:8080)
├── frontend/        # Web console
├── platform/        # Platform API (:8000)
├── docs/            # Engineering docs
├── LICENSE          # MIT
└── README.md        # This file
```

## License

MIT — see [LICENSE](LICENSE). You are free to use, modify, and distribute this project. Keep the license notice in copies you share.

## Contributing

1. Fork or clone the repo.
2. Create a branch for your change.
3. Run tests (`pytest` in `platform/`, `go test ./...` in `agent/`).
4. Open a pull request with a short description of what changed and why.

For large changes (new Fleet tabs, new collectors, schema changes), read `docs/14_CURRENT_STATUS.md` first so you know what is already implemented vs planned.

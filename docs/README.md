# RiskLens Engineering Documentation

This directory is the permanent engineering documentation for RiskLens. It is generated from the current source tree, database schema, APIs, frontend, tests, and configuration. It must be updated when those artifacts change.

The frozen product information architecture (Sprint 13.7, before Sprint 14) remains in [ARCHITECTURE.md](ARCHITECTURE.md). That document describes the intended product model. This package describes **what the code actually does**.

## How to read these documents

A new engineer should read in this order:

1. [01_PROJECT_OVERVIEW.md](01_PROJECT_OVERVIEW.md) — product, stack, current status
2. [02_SYSTEM_ARCHITECTURE.md](02_SYSTEM_ARCHITECTURE.md) — end-to-end component map
3. [12_CODE_MAP.md](12_CODE_MAP.md) — repository layout and entry points
4. Then the subsystem that matches the work: agent, backend, frontend, database, or API

## Document index

| Document | Contents |
|----------|----------|
| [01_PROJECT_OVERVIEW.md](01_PROJECT_OVERVIEW.md) | What RiskLens is, goals, stack, philosophy, status |
| [02_SYSTEM_ARCHITECTURE.md](02_SYSTEM_ARCHITECTURE.md) | End-to-end architecture and component responsibilities |
| [03_AGENT_ARCHITECTURE.md](03_AGENT_ARCHITECTURE.md) | Windows agent packages, modules, lifecycle, transport |
| [04_BACKEND_ARCHITECTURE.md](04_BACKEND_ARCHITECTURE.md) | Platform FastAPI layers, DI, errors, persistence |
| [05_FRONTEND_ARCHITECTURE.md](05_FRONTEND_ARCHITECTURE.md) | SPA structure, routing, state, API integration |
| [06_DATABASE.md](06_DATABASE.md) | Every table, column, index, constraint, read/write path |
| [07_API_REFERENCE.md](07_API_REFERENCE.md) | Every HTTP endpoint, schemas, errors, callers |
| [08_AGENT_SESSION_LIFECYCLE.md](08_AGENT_SESSION_LIFECYCLE.md) | Login, heartbeat, logout, shutdown, offline, remaining issues |
| [09_INVENTORY.md](09_INVENTORY.md) | Inventory and collector status: implemented / stub / future |
| [10_CONFIGURATION.md](10_CONFIGURATION.md) | Config files, env vars, flags, defaults |
| [11_TESTING.md](11_TESTING.md) | Go tests, Python tests, strategy, gaps |
| [12_CODE_MAP.md](12_CODE_MAP.md) | Full repository map, packages, dependencies, entry points |
| [13_SEQUENCE_DIAGRAMS.md](13_SEQUENCE_DIAGRAMS.md) | Mermaid sequences for core flows |
| [14_CURRENT_STATUS.md](14_CURRENT_STATUS.md) | Implemented, partial, stub, debt, bugs, investigations |
| [15_NEXT_SPRINT.md](15_NEXT_SPRINT.md) | Next logical sprint from the current code |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Frozen Sprint 13.7 product information architecture |
| [PIPELINE_ARCHITECTURE.md](PIPELINE_ARCHITECTURE.md) | Generic agent collection pipeline (Collector → Snapshot → Fingerprint → Pipeline → Uploader) |
| [DEVICE_PIPELINE.md](DEVICE_PIPELINE.md) | Device payload ownership and collection vs upload health |
| [OV-1.1_DESIGN.md](OV-1.1_DESIGN.md) | Recurring collection design (OV-1.1); Phase A scheduler implemented |

Backend-layer notes that predate this package still exist at [`platform/docs/architecture.md`](../platform/docs/architecture.md). Treat them as historical; several Sprint 1 statements in that file are stale.

## Runtime map (two HTTP processes)

| Process | Bind | Code | Purpose |
|---------|------|------|---------|
| Platform API | `8000` | `platform/app/main.py` | Agent ingest + Fleet/query APIs + PostgreSQL |
| UI server | `8080` | `backend/app/main.py` | Serves `frontend/index.html` and `/assets` only |

The browser loads the UI from port **8080** and calls the Platform API on port **8000**.

## Conventions used in this package

- **Implemented** means there is a collector, API, persistence, and a live UI path.
- **Partially implemented** means some of those layers exist and others do not.
- **Stub** means a UI tab, API route, or domain package exists but returns `"Not implemented yet"` or has no logic.
- **Not implemented** means no collector, no API, no table, and no domain package (or only an empty `__init__.py`).
- Nothing in these documents is invented. If the code does not do it, the document says so.

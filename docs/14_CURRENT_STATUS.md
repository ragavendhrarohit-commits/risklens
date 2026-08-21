# 14 — Current Status

Baseline label in `docs/ARCHITECTURE.md`: **Sprint 13.7, frozen before Sprint 14**.

This file is the implementation checklist from source, not a sprint plan.

---

## Implemented

- Windows agent service/console runner, identity, YAML config, JSON logs, Event Log lifecycle
- Transport client (no retry) used by all network modules
- Registration, device collect+upload, heartbeat, users, software, activity (sessions + apps)
- WTS/SCM logon/logoff/shutdown flush, activity disk queue
- Platform hexagonal API: domain/services/repos, postgres + memory
- Alembic `0001`–`0007`
- Ingest APIs: agents, heartbeat, devices, users, software, sessions, applications
- Query APIs: devices, users, software, sessions, applications, fleet (+ nested)
- Fleet presence (60s / 300s), default policy assignment, health timestamps + disk/memory **bytes**
- SPA: Fleet list/detail, Accounts, Installed Applications, Sessions, Application Activity, Health, Policies tabs
- Reports → Account Inventory, Software Inventory
- Error envelope, request id, timing header, CORS for :8080
- Architecture tests that freeze placeholders and Fleet IA
- Session immutability for **already closed** rows
- REPRO diagnostic logging on agent + API session paths

---

## Partially implemented

| Item | What exists | What is missing |
|------|-------------|-----------------|
| Dashboard | 4 summary cards from Fleet | Remaining panels |
| Health | Presence + capacity bytes | CPU/memory/disk **usage** |
| Session one-active invariant | Domain comments, repo `activate_current`, tests | Production `SessionService` login write |
| Offline session close | Implemented if exactly one ACTIVE | Refuses when overlap already exists |
| Tags | Table + check constraint + list filter | Write API, UI editor |
| Policy | DB + read API + UI tab | Agent fetch, editor, extra collectors |
| Device inventory | Upload + GET | Frontend Devices page; periodic agent refresh |
| Users/software inventory pages | Live tables | Department/Group/Current User columns |
| Dual servers | UI :8080 + API :8000 | Single process |
| Agent config watch | Reloads YAML | Does not restart modules |
| Activity timeline | Local `session_events.json` | Fleet timeline API |

---

## Stub

- `GET /api/v1/fleet/{id}/timeline`
- Fleet tabs: processes, services, network, browser, timeline, security group
- Nav: Alerts, Incidents, Policies page, Administration, Settings, Groups
- Dashboard placeholder panels; most Reports/Admin cards
- Domain packages listed in [09_INVENTORY.md](09_INVENTORY.md) with empty `__init__.py`
- Health `cpu_usage` / `memory_usage` / `disk_usage` strings
- Global search input (disabled, Sprint 14 message)
- Policy flags `collect_processes` / `collect_services` / `collect_event_logs`
- Agent YAML `security` / `update`
- `platform/app/models/` empty package
- Non-Windows collectors (`*_stub.go`)

---

## Not started

- Operator authentication / RBAC / API keys
- TLS
- Agent authentication to the API
- Groups hierarchy
- Elasticsearch query path (README only)
- Background workers / cache / Redis
- Transport HTTP retry
- Browser/E2E tests, coverage gates
- Unique index one ACTIVE session per agent
- Process/service/network/browser collectors
- Using `RISKLENS_HOST`/`PORT` to bind Uvicorn
- Wiring `GET /api/v1/devices` into the SPA
- Populating `agent_status.ip_address`

---

## Technical debt

1. Two FastAPI applications (`backend/` vs `platform/`).
2. Root `README.md`, root `requirements.txt` elasticsearch, agent `README.md`, `platform/docs/architecture.md`, and parts of `docs/ARCHITECTURE.md` (sessions reserved) are **stale**.
3. Registration Go comment cites `backend/app/schemas/agents.py`.
4. Legacy `agent/internal/config` still tested, unused by `main`.
5. `modules.enabled` and `performance.max_concurrency` unused.
6. Duplicate `UnknownAgentError` classes per domain package.
7. `MultipleActiveSessionsError` unused.
8. `SessionRepository.activate_current` / `close_active_for_agent` unused by `SessionService`.
9. REPRO logs on the production path.
10. SQLite `identities` table created and unused.
11. `#error-banner` unused; dead JS tab `"activity"`.
12. Device module always enabled; other modules default **false** in `Defaults()` but example YAML enables some.
13. No `docker-compose` for Postgres despite depending on it to boot.

---

## Known bugs / behavioural defects (from code + observed)

1. **Overlapping ACTIVE sessions** for one `agent_id` are representable. Login insert does not close other ACTIVE rows. Observed on Fleet for `LAPTOP-8VQ40K6U` (login while previous still ACTIVE; logout times not equal to next login).
2. **Offline reconciliation cannot repair** that state (requires exactly one ACTIVE).
3. **Missed logoff** (agent stopped, flush timeout, console death) leaves ACTIVE rows.
4. **Presence Offline while the user is at the keyboard** if `RiskLensAgent` is not running (no Windows service in one observed MSI setup; last heartbeat matched UI Last Seen).
5. **API `ERR_CONNECTION_REFUSED` on :8000** when Uvicorn exited because Postgres/Docker was down. `/health` does not detect that after the process is gone — the TCP connection simply fails.
6. **Opening `http://0.0.0.0:8000` in a browser** is invalid (`ERR_ADDRESS_INVALID`). Bind address is not a browse URL.
7. Opening `http://127.0.0.1:8000/` is JSON metadata, not the SPA. The page is `:8080`.
8. Fleet does not poll; Online can look stale in the UI until refresh even if heartbeats continue.

---

## Open investigations

**Overlapping session lifecycle (Laptop B).**

- Intent: reproduce Login → Logout → Login → Logout → Login → Shutdown with REPRO logs.
- Do **not** ship a login-path “fix” until the trace shows the first moment two ACTIVE rows exist.
- Agent REPRO code is in source (`repro_log.go`, upload/heartbeat logs) and requires **rebuild/redeploy on Laptop B**.
- Backend REPRO logs require a running Platform on `:8000` with Postgres.
- Production login path remains `_upsert_one` → `upsert_many` (the write that can leave two ACTIVE rows).

---

## Dead code (identified)

| Item | Evidence |
|------|----------|
| `agent/internal/config` | Not imported by `main` |
| `backend/app/db.py` identities | No route uses it |
| elasticsearch extra | No import in platform/backend/frontend |
| `app.models` package | No imports |
| `activate_current` on service path | Only repo tests call it |
| Fleet timeline API | Stub; UI does not call it |
| `renderActivityArchitecture` | No matching HTML tab |

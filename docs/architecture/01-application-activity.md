# Lesson 1 — Application Activity

**Audience:** you, learning RiskLens as a product and as a system.  
**Goal:** you can trace one real event (Chrome becomes the foreground window) from Windows APIs to the Fleet table, and you can explain *why* each layer exists.  
**Code freeze:** this lesson describes the code as it exists. It does not propose edits.

---

# 1. Purpose

## Why does this feature exist?

RiskLens is an endpoint visibility product. Device inventory answers “what computer is this?” Software inventory answers “what is installed?” Application Activity answers a different question:

> **What was this person actually using, on this computer, during this logon, and for how long?**

Without it, Fleet can list Chrome as installed and still have no idea whether the employee spent two hours in Chrome or never opened it.

## What problem does it solve?

It records **foreground-window segments**.

A segment is not “Chrome was running.” Windows can have Chrome running in the background while Excel is in front. RiskLens records the window that had keyboard/mouse focus:

- which executable (`chrome.exe`)
- a short application label (`chrome`)
- the window title (`GitHub - Google Chrome`)
- when that combination started
- when it stopped (or null if it is still in front)
- duration in seconds
- which computer session / user it belonged to

That is the raw material for usage, and later for Browser Activity (parsed from browser window titles) and productivity classification.

## Who uses it?

| Who | How |
|-----|-----|
| Analyst / admin | Fleet → one computer → **Application Activity** tab |
| The Windows agent | Collects and uploads; nobody “uses” this except as a service |
| Platform API | Stores and queries |
| Other features | Browser Activity filters these rows for Chrome/Edge/Firefox. Sessions own the `session_id` foreign key. Global Time Filter restricts `started_at`. |

There is no employee-facing “my activity” screen. This is an operator console.

## How does it interact with other features?

```
Identity (agent_id, install_id)
        │
        ▼
   Sessions  ────────── Computer logon (required parent)
        │
        ▼
Application Activity ── Foreground segments (this lesson)
        │
        ├──► Browser Activity   (view: Chrome/Edge/Firefox + title parse)
        ├──► Global Time Filter (query: started_at_from / started_at_to)
        └──► Fleet UI           (table for one agent)
```

Hard rule: **an application row cannot exist without a session.** The API returns `422 unknown_session` if the agent posts activity for a `session_id` that is not in `computer_sessions`. Collection of sessions and activity lives in the **same agent module** (`activity`), on the same 15-second ticker, so they stay in order: session payload first, then application payload.

What this feature is **not**:

- Not process listing (every running PID).
- Not installed-software inventory (`software` module).
- Not URL collection (no browser extension).
- Not idle-aware productivity (lock/AFK time is still counted as duration).

---

# 2. High-Level Architecture

```
 Interactive Windows desktop
   WTS session + GetForegroundWindow
            │
            ▼
 ┌──────────────────────────────────────┐
 │  RiskLens Agent (Go, Windows)        │
 │                                      │
 │  activity.WindowsCollector           │
 │            │                         │
 │            ▼                         │
 │  activity.step()  (segment machine)  │
 │            │                         │
 │            ▼                         │
 │  activity.json  (one open segment)   │
 │            │                         │
 │            ▼                         │
 │  Client.UploadApplications           │
 │            │  on failure             │
 │            └──► session_upload_queue │
 └──────────────────┬───────────────────┘
                    │  HTTP POST JSON
                    │  /api/v1/applications
                    ▼
 ┌──────────────────────────────────────┐
 │  Platform API (FastAPI / Python)     │
 │                                      │
 │  applications.py  (HTTP)             │
 │            │                         │
 │            ▼                         │
 │  ApplicationActivityService          │
 │            │                         │
 │            ▼                         │
 │  ApplicationActivityRepository       │
 └──────────────────┬───────────────────┘
                    │  INSERT ... ON CONFLICT
                    ▼
              PostgreSQL
         table application_activity
                    │
                    │  HTTP GET
                    │  /api/v1/fleet/{id}/applications
                    ▼
 ┌──────────────────────────────────────┐
 │  Fleet UI (vanilla JS)               │
 │  loadFleetTabData → table HTML       │
 └──────────────────────────────────────┘
```

## Every component

**Windows APIs.** The only place the OS is observed. They answer: who is logged on, and which window is in front.

**Collector.** A Go interface with a Windows implementation. It must never open a socket. Tests substitute a fake collector.

**`step()`.** Pure state machine. Previous `activity.json` + one Observation → closed/open session rows + closed/open app rows + next state. This is the product brain.

**`activity.json`.** Crash-safe memory of “what is currently open.” Without it, a restart would forget the in-progress Chrome segment.

**Upload client.** Builds POST `/api/v1/applications`. Talks only through `transport`, never `net/http` directly.

**Disk queue.** If POST fails, JSON is appended to `session_upload_queue.json` (shared with session payloads) and retried next cycle.

**Platform HTTP layer.** Validates JSON, maps to a command object, returns a stored envelope the agent understands.

**Service.** Checks the agent exists, checks each session exists, assigns ids, clamps duration.

**Repository.** Persistence port. Postgres in production, in-memory in tests.

**PostgreSQL.** Source of truth for Fleet.

**Fleet UI.** Does not talk to the agent. It only GETs the platform.

---

# 3. Runtime Flow

Think in two clocks: **Windows notifications** (logon/logoff) and a **15-second poll**. Application Activity is driven mainly by the poll.

```
User clicks Chrome (or Alt-Tabs to it)
        │
        ▼
Windows makes Chrome the foreground HWND
        │
        ▼  (~every 15s, activity.scheduler)
Module.cycle()
        │
        ▼
WindowsCollector.Collect()
        │
        ├─ WTSEnumerateSessionsW / WTSQuerySessionInformationW
        │     → LiveSession (username, SID, logon time, WTS id)
        │
        └─ GetForegroundWindow → GetWindowTextW
           GetWindowThreadProcessId → QueryFullProcessImageNameW
              → LiveApp (chrome, chrome.exe, "GitHub - Google Chrome")
        │
        ▼
step(previous activity.json, Observation, agent_id, now)
        │
        ├─ same exe + same title  → refresh duration, stopped_at = null
        └─ different exe or title → close previous segment, open new one
        │
        ▼
persistAndUpload()
        │
        ├─ POST /api/v1/sessions     (heartbeat / open / close)
        └─ POST /api/v1/applications (ActivityItem[])
        │
        ▼  on HTTP failure
enqueue session_upload_queue.json and retry next cycle
        │
        ▼  on HTTP 200
ApplicationActivityService.upsert
        │
        ▼
INSERT INTO application_activity ... ON CONFLICT (id) DO UPDATE
        │
        ▼
Operator opens Fleet → computer → Application Activity
        │
        ▼
GET /api/v1/fleet/{agent_id}/applications?limit=200&started_at_from=...&started_at_to=...
        │
        ▼
renderFleetApplicationActivityTable()
```

## Every step, in words

1. **User action.** Focus changes. The agent does not hook every click. It samples.
2. **Scheduler tick.** `activity.interval` (default 15s) calls `Module.cycle`. Logon notification can call `cycle` immediately.
3. **Collect.** OS snapshot only. No HTTP.
4. **Compare.** `sameApp` is case-insensitive executable **and** exact window title. A YouTube title change is a **new row**, not an update of the old one. That is why you see many ~15s Chrome rows.
5. **Build payloads.** Closed segments have `stopped_at`. The current foreground has `stopped_at: null` and `duration_seconds = now - started_at`.
6. **Upload sessions first.** Parent row must exist.
7. **Upload applications.** Upsert by SHA1 `id`.
8. **Query.** Fleet list is filtered by agent and Global Time Filter on `started_at`.
9. **Render.** Open rows recompute duration in the browser from `started_at` to `Date.now()` so the number is not frozen at last upload.

---

# 4. Folder Structure

## Repositories / top-level

```
Risklens/
  agent/          Go Windows service. Collects. Never renders UI.
  platform/       FastAPI. Stores. Serves Fleet JSON.
  frontend/       Static HTML/JS console on port 8080.
  docs/           Human documentation (this course lives under docs/architecture/).
```

The UI process (`backend.app.main` on 8080) is a static file server plus a few UI routes. Application Activity data does **not** live there. The browser calls the Platform API on port 8000.

## Agent folders that matter

```
agent/
  cmd/risklens-agent/main.go     Process entry. Wires modules. --activity-foreground helper.
  configs/                       agent.yaml examples (activity.enabled / interval).
  internal/
    configuration/               Load YAML + env into model.Config.Activity
    identity/                    agent_id + install_id stamped on every POST
    modules/
      interfaces/                Module contract (Start/Stop/Name)
      base/                      Shared module lifecycle helpers
      activity/                  ★ this feature (sessions + apps together)
    transport/                   The only net/http in the agent
      client/                    HTTP JSON client
      interfaces/                Transport interface
      request/ response/ errors/ middleware/
    service/                     Windows SCM; session-change / shutdown callbacks
```

**Why `internal/`?** Go convention: other modules outside this module path cannot import it. The agent is a private product, not a public library.

**Why a dedicated `transport` package?** Collectors must stay unit-testable without sockets. Only `transport/client` imports `net/http`.

**Why sessions and apps in one module?** They share WTS, `activity.json`, the scheduler, the queue, and the “close current app when session closes” rule. Splitting would duplicate that state machine.

## Platform folders that matter

```
platform/app/
  api/v1/applications.py     HTTP ingest + org-wide list + get-by-id
  api/v1/fleet.py            GET /{agent_id}/applications for the console
  api/router.py              Mounts /api/v1/applications and /api/v1/fleet
  domain/applications/       Pure models + errors (no SQL, no FastAPI)
  services/application_activity_service.py   Use-cases
  services/fleet_service.py  Fleet.applications() delegates to the same service
  repositories/              Abstract ports
  infrastructure/
    postgres/                SQLAlchemy models, repository, Alembic 0007
    memory/                  Test double
  schemas/applications.py    Pydantic request/response
  startup/lifespan.py        DI: bind postgres or memory
```

This is **hexagonal / ports-and-adapters**:

- Domain does not know FastAPI or SQLAlchemy.
- Service depends on repository *interfaces*.
- HTTP adapters and Postgres adapters sit on the outside.

## Frontend folders that matter

```
frontend/
  index.html                 Fleet tab button data-tab="applications"
  assets/app.js              State, fetch, table HTML
  assets/global-time-filter.js   Time store (started_at range)
  assets/global-time-picker.js   Header UI for that store
  assets/app.css             Layout only
```

No React. No per-feature JS module. Application Activity is functions inside `app.js`.

---

# 5. File-by-File Explanation

## 5.1 Agent — `collector.go`

**Why it exists.** Defines `Collector` so `Module` does not import Windows syscalls.

**Who calls it.** `Module.cycle` → `m.collector.Collect(ctx)`.

**What it calls.** Implementations: `WindowsCollector` or tests’ fakes. Stub on non-Windows.

```go
type Collector interface {
    Collect(ctx context.Context) (Observation, error)
}
```

**Pattern:** Strategy / dependency inversion.

## 5.2 Agent — `collector_windows.go` (`//go:build windows`)

**Why it exists.** Session-0 isolation. The agent often runs as Local System in session 0. `GetForegroundWindow` in session 0 is **not** the user’s Chrome. This file is the workaround.

**Call chain:**

```
WindowsCollector.Collect
        │
        ├─ interactiveSession()
        │     WTSEnumerateSessionsW
        │     WTSGetActiveConsoleSessionId
        │     WTSQuerySessionInformationW (user, domain, logon FILETIME)
        │     LookupSID
        │
        └─ foregroundApp(wtsSessionID)
              if agent already in that session → readForegroundLocal
              else ImpersonateLoggedOnUser → readForegroundLocal
              else CreateProcessAsUser(RiskLensAgent.exe --activity-foreground)
```

**`readForegroundLocal` Windows APIs:**

| API | DLL | Why |
|-----|-----|-----|
| `GetForegroundWindow` | user32 | HWND with focus |
| `GetWindowTextW` | user32 | Title bar text |
| `GetWindowThreadProcessId` | user32 | PID of that window |
| `OpenProcess` | kernel32 | Handle with `PROCESS_QUERY_LIMITED_INFORMATION` |
| `QueryFullProcessImageNameW` | kernel32 | Full path; then `filepath.Base` → `chrome.exe` |

**`RunForegroundHelper`.** Same binary, different argv. `main.go` sees `--activity-foreground` and exits after printing one JSON `LiveApp` to stdout. Parent reads the pipe. Timeout 5s, then `TerminateProcess`.

**Who calls this file.** `activity.New` constructs `&WindowsCollector{}`. Helper path is `main.run` before normal service startup.

## 5.3 Agent — `collector_stub.go`

Non-Windows compile. `Collect` returns empty Observation. CI on Linux can still compile the module.

## 5.4 Agent — `model.go`

**Why it exists.** Payload shapes and id algorithms. No I/O.

| Type | Role |
|------|------|
| `LiveSession` / `LiveApp` | OS snapshot |
| `Observation` | One poll |
| `SessionItem` / `ActivityItem` | JSON body fields |
| `ApplicationUpload` | POST `/api/v1/applications` envelope |
| `persistedState` | `activity.json` |

**Important functions:**

- `applicationName(exe, title)` — basename without `.exe`, else title, else `"Unknown"`. Product name from version resources is **not** used. That is why the UI shows `chrome` not `Google Chrome`.
- `durationSeconds(start, end)` — wall clock; never negative.
- `activityID(sessionID, executable, startedAt)` — SHA1 of `session + NUL + lower(exe) + NUL + RFC3339 start`. Stable upsert key.
- `sessionID(agentID, wts, logonAt)` — UUID v5 so the same Windows logon maps to the same UUID after agent restart.

## 5.5 Agent — `state.go`

**Why it exists.** The segment machine. If you understand `step`, you understand the product.

**Who calls it.** `Module.cycle` and `FlushClose` / shutdown.

**`sameApp`:** equal-fold executable **and** exact title. Title change = new segment.

**`step` cases (study these):**

1. No live session, no previous → no-op.
2. No live session, previous open → close session + close app.
3. Live session different from previous → close old session/app, open new session, maybe open new app. Close timestamp prefers new logon’s `LogonAt`.
4. Same session, no foreground → close app if one was open.
5. Same session, same app → `refreshOpenApp` (update duration, still open).
6. Same session, different app → close previous app, open new.

**`loadState` / `saveState`.** Write temp file then `Rename` so a crash does not leave half JSON.

## 5.6 Agent — `upload.go`

**Why it exists.** Module lifecycle, HTTP client, scheduler, apply/upload.

**Who calls it.**

- `main.go` `activity.New(...)` then module manager `Start`
- Scheduler goroutine → `cycle`
- Tests via `NewWithDeps`

**What it calls.** Collector, `step`, `Client.UploadSessions` / `UploadApplications`, queue, state file, WTS notify, logoff hook.

**Important types:**

- `Config` — enabled, backend URL, timeout, interval, state dir
- `Client` — thin JSON POST wrapper over Transport
- `Module` — implements `interfaces.Module`
- `scheduler` — ticker; skips a tick if the previous `cycle` still holds `inflight`

**`cycle`:** drain queue → collect → `step` → `applyResult`.

**`persistAndUpload`:** write state if changed → POST sessions → POST apps → on error `enqueueUploads`.

**`FlushClose`:** logoff/shutdown only. Comment in source: **agent restart must not call this** (restart is not logout).

## 5.7 Agent — `queue.go`

**Why it exists.** The API or network can be down. Losing a closed Chrome segment would under-count time.

Shared file: `session_upload_queue.json`. Kinds: `"sessions"` and `"applications"`. Cap **32**. Closed-session payloads are protected when trimming. Open-session heartbeats coalesce (replace last non-closed sessions item).

**`drainQueue`:** FIFO. On first failure, stop (retry next cycle). Do not reorder.

## 5.8 Agent — `lifecycle.go`

WTS constants, `OnWindowsSessionChange`, `OnWindowsShutdown`, `CloseSession` → `FlushClose`. Lock/unlock only write a local timeline event; they do **not** stop the activity clock.

## 5.9 Agent — `notify_windows.go` / `logoff_windows.go`

Hidden message-only window + `WTSRegisterSessionNotification` so logon/logoff is not delayed up to 15s. Console ctrl handler for interactive logoff/shutdown.

## 5.10 Agent — `events.go`

Local `session_events.json` (max 200). Debugging / future timeline. **Not** the Fleet Timeline tab.

## 5.11 Agent — `status.go`

Diagnostic snapshot: collecting / uploading / ready / failed / skipped. Not shown in Fleet Application Activity UI.

## 5.12 Agent — `transport/client`

**Why it exists.** One HTTP stack: JSON encode, timeout, max 1 MiB response, logging middleware.

**Who calls it.** `activity.Client.upload` → `BuildRequest` + `Send` + `DecodeResponse`.

**Call relationship:**

```
activity.Client.upload
    → transport.BuildRequest(POST, "/api/v1/applications", body)
    → transport.Send
        → middleware.Logging
        → client.do  (net/http)
    → DecodeResponse → SuccessResponse{ accepted, inventory_status: "stored" }
```

If `accepted` is false or `inventory_status != "stored"`, the agent treats it as malformed and will retry via the queue.

## 5.13 Agent — `cmd/risklens-agent/main.go`

Creates the activity module from YAML. If `--activity-foreground`, runs helper and exits. Does not know Application Activity business rules.

## 5.14 Platform — `domain/applications/entity.py`

**Why it exists.** Invariants without FastAPI.

- `activity_id()` — same SHA1 idea as Go (must stay compatible).
- `ApplicationActivity` — validated write model.
- `ApplicationActivityRecord` — read model plus `username` / `machine_name` joins.
- `ListApplicationActivityQuery` — limit 1–200, sort field whitelist, `started_at_from <= started_at_to`.

**Pattern:** write model vs read model. Username is not stored on the activity table; it is joined from `computer_sessions` at read time.

## 5.15 Platform — `domain/applications/errors.py`

| Error | HTTP | When |
|-------|------|------|
| `UnknownAgentError` | 404 | POST for unregistered agent |
| `UnknownSessionError` | 422 | `session_id` missing in `computer_sessions` |
| `ApplicationActivityNotFoundError` | 404 | GET by id |
| `InvalidApplicationQueryError` | 422 | Bad sort / inverted time range |

## 5.16 Platform — `schemas/applications.py`

Pydantic: `extra="forbid"`, string length bounds, `duration_seconds >= 0`. `to_command()` is the anti-corruption layer from HTTP JSON into domain.

## 5.17 Platform — `api/v1/applications.py`

Thin adapter. FastAPI Depends → service. Maps records to response models.

## 5.18 Platform — `api/v1/fleet.py` `list_fleet_applications`

Same list response, but `FleetService` first asserts the agent exists (404) then lists **that** agent only. This is what the UI calls.

## 5.19 Platform — `services/application_activity_service.py`

`upsert`: exists(agent) → exists(each session) → build rows → `upsert_many`.  
`list` / `get`: pass-through to repository.

## 5.20 Platform — `services/fleet_service.py` `applications()`

Require agent, then `ApplicationActivityService.list` with `agent_id` plus time filter.

## 5.21 Platform — repositories + postgres + memory

Port: `upsert_many`, `get`, `list`.  
Postgres: `INSERT ... ON CONFLICT (id) DO UPDATE`. Joins agent.machine_name and session.username.  
Memory: same contract for pytest without Docker.

## 5.22 Platform — `infrastructure/postgres/models.py` `ApplicationActivityRow`

ORM mapping of table `application_activity`. `from_domain` / `to_domain`.

## 5.23 Platform — Alembic `0007_session_application_activity.py`

Creates `computer_sessions` and `application_activity` together. Application Activity cannot be migrated without Sessions.

## 5.24 Frontend — `index.html`

Button: `data-tab="applications"` label **Application Activity**.

## 5.25 Frontend — `assets/app.js`

State: `fleet.applicationActivity`.  
Fetch: `loadFleetTabData` always refetches this tab (not cached like Accounts).  
Time: `applicationActivityTimeQuery()` reads Global Time store.  
Render: `renderFleetApplicationActivityTable`.

## 5.26 Frontend — `global-time-filter.js`

Holds `startTime` / `endTime`. Application Activity does not implement its own picker. Changing the header range nulls `applicationActivity` and refetches if that tab is open.

### Call relationships (whole stack)

```
main.go
  → activity.New
      → Module.Start
          → scheduler.loop → Module.cycle
              → WindowsCollector.Collect
              → step
              → persistAndUpload
                  → Client.UploadApplications
                      → transport.Client.Send
                          → FastAPI applications.upload_application_activity
                              → ApplicationActivityService.upsert
                                  → PostgresApplicationActivityRepository.upsert_many

index.html click tab
  → loadFleetTabData
      → GET fleet/.../applications
          → FleetService.applications
              → ApplicationActivityService.list
                  → repository.list
      → renderFleetApplicationActivityTable
```

---

# 6. End-to-End Data Flow

**Event:** User Alt-Tabs from Cursor to Chrome. Title: `GitHub - Google Chrome`.

### 1. Which Windows API?

After the next 15s tick (or sooner if a cycle is already running):

- `GetForegroundWindow` returns Chrome’s HWND.
- `GetWindowTextW` → `GitHub - Google Chrome`
- `GetWindowThreadProcessId` + `QueryFullProcessImageNameW` → `...\chrome.exe`

If the agent is in session 0: `WTSQueryUserToken` → impersonate or `CreateProcessAsUser` helper.

### 2. Which Go function?

`WindowsCollector.Collect` → `foregroundApp` → `readForegroundLocal` (or helper).

### 3. Which struct?

```go
LiveApp{
  ApplicationName: "chrome",
  ExecutableName:  "chrome.exe",
  WindowTitle:     "GitHub - Google Chrome",
}
Observation{ Session: &LiveSession{...}, App: &LiveApp{...} }
```

Previous `persistedState` still has Cursor (`Cursor.exe`, title `Cursor Agents`, `AppID`, `StartedAt`).

### 4. Which queue?

`step` sees same session, different app:

- Closed `ActivityItem` for Cursor (`stopped_at = now`, duration computed)
- Open `ActivityItem` for Chrome (`stopped_at = null`, new SHA1 id)

`persistAndUpload` POSTs immediately. Queue is used **only if** POST fails.

### 5. Which HTTP request?

```
POST {backend_url}/api/v1/applications
User-Agent: RiskLensAgent/{version}
Content-Type: application/json
```

Body (shape):

```json
{
  "agent_id": "e6e55967-ed7a-4eee-904a-789f2d16cbfd",
  "install_id": "...",
  "collected_at": "2026-08-21T08:00:00Z",
  "activities": [
    {
      "id": "<sha1>",
      "session_id": "3538f132-...",
      "application_name": "Cursor",
      "executable_name": "Cursor.exe",
      "window_title": "Cursor Agents",
      "started_at": "2026-08-21T07:18:57Z",
      "stopped_at": "2026-08-21T08:00:00Z",
      "duration_seconds": 2463
    },
    {
      "id": "<sha1>",
      "session_id": "3538f132-...",
      "application_name": "chrome",
      "executable_name": "chrome.exe",
      "window_title": "GitHub - Google Chrome",
      "started_at": "2026-08-21T08:00:00Z",
      "stopped_at": null,
      "duration_seconds": 0
    }
  ]
}
```

Sessions POST may accompany this on the same cycle.

### 6. Which FastAPI endpoint?

`POST /api/v1/applications` → `upload_application_activity`.

### 7. Which service?

`ApplicationActivityService.upsert`.

### 8. Which repository?

`PostgresApplicationActivityRepository.upsert_many`.

### 9. Which SQL? (conceptual)

```sql
INSERT INTO application_activity (
  id, session_id, agent_id, application_name, executable_name, window_title,
  started_at, stopped_at, duration_seconds, collected_at, received_at
) VALUES (...)
ON CONFLICT (id) DO UPDATE SET
  session_id = EXCLUDED.session_id,
  -- all columns except id
  duration_seconds = EXCLUDED.duration_seconds,
  stopped_at = EXCLUDED.stopped_at,
  ...
```

Chrome’s open row is updated every 15s with a larger `duration_seconds` until the title or exe changes.

### 10. Which JSON response?

```json
{
  "accepted": true,
  "agent_id": "...",
  "stored": 2,
  "inventory_status": "stored",
  "server_version": "0.1.0",
  "server_time": "...",
  "received_at": "..."
}
```

### 11. Which frontend function?

Tab open: `loadFleetTabData` → `fetchJson(platformUrl(.../applications, { limit: 200, started_at_from, started_at_to }))`.

### 12. Which UI?

`renderFleetApplicationActivityTable` builds a `<table>`: Application, Executable, Window Title, Started, Stopped, Duration, Session account.

Empty copy: **No application activity during the selected time range.**

---

# 7. Sequence Diagram

```
Windows          Agent collector     step/state      Transport         Platform API      PostgreSQL       Fleet UI
  │                    │                 │                │                  │                │                │
  │  foreground HWND   │                 │                │                  │                │                │
  │───────────────────►│                 │                │                  │                │                │
  │                    │ Observation     │                │                  │                │                │
  │                    │────────────────►│                │                  │                │                │
  │                    │                 │ persist json   │                  │                │                │
  │                    │                 │                │                  │                │                │
  │                    │                 │ POST sessions  │                  │                │                │
  │                    │                 │───────────────►│                  │                │                │
  │                    │                 │                │ POST /sessions   │                │                │
  │                    │                 │                │─────────────────►│                │                │
  │                    │                 │                │                  │ upsert session │                │
  │                    │                 │                │                  │───────────────►│                │
  │                    │                 │                │ 200 stored       │                │                │
  │                    │                 │                │◄─────────────────│                │                │
  │                    │                 │ POST apps      │                  │                │                │
  │                    │                 │───────────────►│                  │                │                │
  │                    │                 │                │ POST /applications               │                │
  │                    │                 │                │─────────────────►│                │                │
  │                    │                 │                │                  │ ON CONFLICT    │                │
  │                    │                 │                │                  │───────────────►│                │
  │                    │                 │                │ 200 stored       │                │                │
  │                    │                 │                │◄─────────────────│                │                │
  │                    │                 │                │                  │                │   GET fleet/apps
  │                    │                 │                │                  │                │◄───────────────│
  │                    │                 │                │                  │ SELECT JOIN    │                │
  │                    │                 │                │                  │───────────────►│                │
  │                    │                 │                │                  │ rows           │                │
  │                    │                 │                │                  │◄───────────────│                │
  │                    │                 │                │                  │ JSON page      │                │
  │                    │                 │                │                  │───────────────────────────────►│
  │                    │                 │                │                  │                │  render table  │
```

If POST applications fails, Transport returns error, `persistAndUpload` enqueues, next `cycle` starts with `drainQueue`. UI is unchanged until a GET succeeds.

---

# 8. Database Design

## Table `application_activity`

Created in Alembic `0007`. Parent: `computer_sessions`. Owner computer: `agents`.

| Column | Type | Null | Why it exists |
|--------|------|------|----------------|
| `id` | `varchar(64)` PK | no | SHA1 upsert key. Same segment can be refreshed every 15s without duplicating rows. |
| `session_id` | UUID FK → `computer_sessions` | no | Computer → Session → Activity. Blocks orphans. |
| `agent_id` | UUID FK → `agents` | no | Fast per-computer queries without always joining session. |
| `application_name` | `varchar(512)` | no | Human label (`chrome`). Check: non-empty. |
| `executable_name` | `varchar(512)` | no | `chrome.exe`. Browser Activity matching uses this. |
| `window_title` | `varchar(512)` | no | Distinguishes tabs/docs. Empty string allowed. |
| `started_at` | timestamptz | no | Segment start. Global Time Filter predicates this. |
| `stopped_at` | timestamptz | yes | Null = still foreground at last upload. |
| `duration_seconds` | int ≥ 0 | no | Stored duration; UI may recompute for open rows. |
| `collected_at` | timestamptz | no | Agent clock when payload was built. |
| `received_at` | timestamptz | no | Server clock. Detects clock skew / delay. |

## Indexes

- `ix_application_activity_session_id_started_at` — activity inside one logon.
- `ix_application_activity_agent_id_started_at` — Fleet tab + time filter.

## Relationships

```
agents (agent_id)
   │ 1
   │
   │ * 
computer_sessions (session_id, agent_id, username, ...)
   │ 1
   │
   │ *
application_activity (id, session_id, agent_id, ...)
```

Username is **not** denormalized onto activity. List queries `OUTER JOIN computer_sessions` for `username` and `agents` for `machine_name`.

## What is missing on purpose

No category, no productivity score, no idle flag, no URL column. Those are other products or later lessons. Duration is wall-clock, not “active input time.”

---

# 9. API Design

Mounted in `platform/app/api/router.py`.

## `POST /api/v1/applications`

**Why.** Agent ingest. The only write path.

**Request:** `ApplicationActivityUploadRequest` (unknown fields forbidden).

**Response 200:** `accepted`, `stored` (row count written), `inventory_status: "stored"`. The word “inventory” is historical; this is not software inventory. The agent **requires** that string.

**Errors:**

| Status | Code | Meaning |
|--------|------|---------|
| 404 | `unknown_agent` | Agent never registered |
| 422 | `unknown_session` | Session POST has not landed / wrong id |
| 422 | validation | Pydantic (bad UUID, empty name, extra fields) |

**Idempotency:** upsert by `id`. Retrying the same payload is safe.

## `GET /api/v1/applications`

**Why.** Org-wide query / debugging. UI does not use this for the computer tab.

Query: `agent_id`, `session_id`, `application_name` (ILIKE contains), `started_at_from`, `started_at_to`, `limit` (1–200, default 50), `offset`, `sort` (default `-started_at`).

**422** `invalid_query` for unknown sort field or inverted time range.

## `GET /api/v1/applications/{activity_id}`

**Why.** Single-row debug. **404** if missing.

## `GET /api/v1/fleet/{agent_id}/applications`

**Why.** Fleet console. 404 if that computer is unknown. Same list model as org GET, default limit from Fleet (50 at API default; UI sends 200). Passes Global Time Filter query params through.

There is **no** `SUM(duration) GROUP BY application_name` endpoint. The table is raw segments.

---

# 10. Frontend Rendering

## Which JavaScript starts?

1. Hash `#/fleet/{agentId}` → `loadFleetDetail`.
2. Overview loads first. Click **Application Activity** → `f.tab = "applications"` → `loadFleetTabData`.
3. `bindApplicationActivityTimeFilter` subscribed at startup: time changes clear cache and refetch if this tab is visible.

## State

```javascript
state.fleet.applicationActivity  // last JSON page or null
state.fleet.extraLoading
state.fleet.extraError
state.fleet.tab === "applications"
```

Switching computers nulls `applicationActivity`. Unlike Accounts, revisiting this tab **always hits the network** so the time filter and live open-row durations stay honest.

## How rendering works

`renderFleetDetail` → if tab applications and not loading/error → `renderFleetApplicationActivityTable(page)`.

That function:

- empty → empty-copy paragraph
- else maps `items` to `<tr>`
- duration via `liveDurationSeconds` (if `stopped_at` set, end−start; else now−start)
- footer: `items.length of page.total`

**No pager.** If `total` is 808 and `limit` is 200, the operator sees 200 newest-by-default-sort rows and a caption “200 of 808”. Older segments exist in Postgres but not on screen.

## How refresh works

- **Tab click / time change / computer switch:** explicit GET.
- **Global Time auto-refresh** (if the picker interval is not Off): store notifies subscribers → same as time change.
- **Open-row duration:** computed at render time, **not** ticked every second (Sessions have a ticker; Application Activity does not). Duration for an open Chrome row updates on the next refetch, not every second.

## How the table updates

There is no DOM diffing. Each successful fetch replaces `innerHTML` of the tab body with a new table string.

---

# 11. Error Handling

| Failure | What happens |
|---------|----------------|
| **Network timeout (agent → API)** | Transport `ErrTimeout` → `ErrTransport`. Upload fails. Payload queued. Next 15s `drainQueue`. Local `activity.json` already has the new open app so the segment is not forgotten. |
| **API down / connection refused** | Same queue path. |
| **Database offline** | FastAPI 5xx. Agent maps server error, queues retry. |
| **Queue full (32)** | Oldest items dropped except closed-session payloads which are retained preferentially. Application payloads can be lost under sustained outage — a real durability limit. |
| **Unknown agent** | 404. Queue retries will keep failing until registration exists. Module snapshot `StateFailed`. |
| **Unknown session** | 422. Typical race: app POST before session POST succeeded. Queue retries after sessions drain first (`enqueue` sessions at `fromIndex` 0, apps at 1). `persistAndUpload` uploads sessions before apps in the live path. |
| **Collector cannot see desktop** | `Collect` error. Cycle may still upload session heartbeat. Snapshot `StatePartial`. Helper timeout → no LiveApp this tick. |
| **Agent restart** | Reloads `activity.json`. Does **not** `FlushClose`. Open Chrome segment continues with same `AppID` / `started_at`. |
| **User logoff / OS shutdown** | `FlushClose` closes session **and** current app with `stopped_at`. Best-effort POST; remainder queued. Timeout ~8s. |
| **User lock screen** | Event recorded locally. Duration **keeps increasing** if the same window is still “foreground” from Windows’ point of view. |
| **Frontend API timeout (10s)** | `fetchJson` abort → `extraError` in the tab. Table not updated. |
| **Time range with no rows** | 200 + `total: 0` + empty copy. Not an error. |
| **Inverted time range** | 422. UI should not send this if the picker is correct. |

---

# 12. Design Decisions

## Why this architecture?

| Decision | Why |
|----------|-----|
| Sample every 15s, don’t hook every focus event | Simpler, fewer events, good enough for “what were they using.” Misses sub-15s switches. |
| Segment on **title** not only exe | Distinguishes GitHub vs YouTube inside Chrome. Cost: many rows. |
| Same module as Sessions | One WTS truth, one file, ordered upload. |
| SHA1 upsert id | Heartbeats update duration in place. |
| Collector isolated from HTTP | Testable; Session-0 helper stays local. |
| Hexagonal backend | pytest with memory repo; postgres in prod. |
| Fleet reads API not agent | Agent may be offline; history is in Postgres. |
| No idle subtraction | Honest wall-clock of “this HWND was foreground,” not “user was productive.” |

## Why not another design?

**ETW / UIAutomation event hooks.** More accurate, more fragile, more privilege, more volume.

**Browser extension for all activity.** Would get URLs; also a second trust surface. Product choice: Application Activity stays OS-level. Browser Activity (later lesson) *parses titles* from these rows.

**Store only “current app.”** Cheap, but you lose history. RiskLens stores history.

**Aggregate in the agent (daily totals only).** Smaller DB, but you cannot show window titles or rebuild Browser Activity.

## Trade-offs

**Advantages:** understandable pipeline; restart-safe open segment; idempotent uploads; Fleet works from DB.

**Disadvantages:** 15s granularity; title churn explodes Chrome rows; lock/idle inflate duration; UI cap 200; `application_name` is not a friendly product name; queue can drop app payloads under prolonged outage.

---

# 13. Enterprise Considerations

How similar products think about “what was in the foreground”:

| Vendor class | Typical approach | vs RiskLens |
|--------------|------------------|-------------|
| **CrowdStrike / Defender / SentinelOne** | Telemetry is process, module, and sometimes UI via sensor; heavy on detections not HR timesheets. Foreground may be a side signal. | RiskLens is explicitly a **usage segment store**, not an EDR event bus. |
| **Elastic Endpoint / Auditbeat** | Process start/stop, optional session data, shipped to a search index. Aggregation is in the SIEM. | RiskLens aggregates little; Postgres rows *are* the product table. |
| **Tanium** | Ask questions at scale; sensors return current state. History is whatever you saved. | RiskLens continuously writes history on a timer. |
| **Productivity SaaS (ActivTrak, Teramind, Microsoft Productivity Score)** | Often hook input + window titles + idle; sometimes screenshots. Strong idle detection. | RiskLens currently **does not** detect idle. That is the biggest product gap vs “time tracking.” |
| **Browser-only DLP** | Extension + native messaging for URLs. | RiskLens Application Activity has no URLs; Browser Activity derives sites from titles. |

**What RiskLens is doing similarly:** endpoint sensor, durable backend, console over historical data, session as the unit of “who was logged on.”

**What could be improved later (not this lesson’s homework):** idle/lock split, paging, rollups, friendlier app names, sub-15s focus, disk-quota for queue.

---

# 14. Code Walkthrough

Read the files in this order when you sit with the repo. Each subsection is “why this function exists,” not a paste of the entire file.

## 14.1 `collector.go`

`Collector.Collect` exists so `cycle` can request an Observation without importing `golang.org/x/sys/windows`. Context cancellation aborts a hung syscall path.

## 14.2 `collector_windows.go`

`WindowsCollector.Collect` — if ctx done, return. Else session, else app. App error with nil app is returned; app error with a partial app still yields Observation (caller may upload session anyway).

`interactiveSession` — enumerate WTS, skip session 0 (services), prefer Active/Connected, prefer console session id. No username → treat as nobody logged on (`nil`, not error).

`foregroundApp` — three attempts because SYSTEM cannot see `WinSta0\Default` the way a user can.

`readForegroundLocal` — HWND 0 or empty title+exe → nil (desktop with no window). `applicationName` strips path and extension.

`readForegroundViaHelper` — duplicate user token to primary, `CreateProcessAsUser` on `winsta0\default`, inherit stdout pipe, JSON decode. This is why `main` special-cases `--activity-foreground` **before** service init: the helper must not start the full agent.

`filetimeToTime` — Windows FILETIME (100ns since 1601) to UTC. Logon time from WTS is this clock.

## 14.3 `model.go`

`activityID` / `sessionID` exist so **restart + upsert** reconstruct the same keys. If SHA1 inputs ever diverge between Go and Python, you would get duplicate rows. Treat that as a contract.

`persistedState` helpers (`alreadyUploadedClosed`, `rememberClosed`, cap 32 ids) exist because Windows can deliver logoff more than once and the agent must not keep POSTing closed sessions forever.

## 14.4 `state.go` `step`

This is the function to study with a notebook.

- Normalize live logon to UTC.
- If logon FILETIME is zero but WTS id matches previous, reuse previous logon (WTS sometimes omits it).
- If still zero logon → ignore this observation (`Next: prev`) rather than mint a garbage session UUID.
- App identity for “same” is exe+title, not HWND (HWND dies when a window is recreated).

`openAppItem` assigns id from session+exe+**this** `startedAt`. New title ⇒ new `startedAt` ⇒ new id.

`refreshOpenApp` keeps `AppID` and `StartedAt`, only duration changes. That is the ON CONFLICT update.

## 14.5 `upload.go`

`New` — if disabled, `client` is nil; `Start` marks skipped and never schedules. If identity missing, failed but does not crash the process (other modules still run).

`Start` — load state, sanitize (if file says session is already closed, drop it), load queue, drain leftover POSTs, install logoff + WTS window, start ticker **immediately** (`s.run` before waiting).

`cycle` — `processMu` so notify-driven `cycleNow` cannot interleave with ticker. `closing` flag lets shutdown own the session.

`applyResult` — if nothing changed, don’t hammer the API. Session switch uses `commitCloseThenOpen` so close is persisted and uploaded before the new session is the local current (avoids `multiple_active_sessions` on the backend).

`persistAndUpload` — drops heartbeats for non-current session ids (safety). Uploads sessions then apps. `uploaded` index into `enqueueUploads` avoids re-POSTing sessions that already succeeded if apps then fail.

`scheduler.run` — `TryLock`: overlapping ticks are skipped, not queued. You never want two Collect+step races on `activity.json`.

## 14.6 `queue.go`

`enqueue` for sessions without a closed row **replaces** the last such item: heartbeats should not fill 32 slots. Application payloads always append (each batch may close different segments).

`drainQueue` holds the lock only to copy the front item, then POSTs without holding the lock (HTTP is slow).

## 14.7 Platform service `upsert`

Loop activities: first time we see a `session_id`, `sessions.exists`. Missing → 422, **no partial commit of earlier rows in that request** (exception before `upsert_many` unless some rows already appended — actually rows are built in memory then one `upsert_many`; if the fifth activity has a bad session, nothing is written). Good.

Empty `item.id` → Python `activity_id()`. Agent normally sends id.

Negative duration clamped; if stopped and duration 0, recompute from timestamps.

## 14.8 Postgres `upsert_many`

Per-row insert (not one multi-values statement). Fine at 15s cadence. `on_conflict_do_update` updates every column except `id`.

`list` — join username/machine_name, filter, count subquery, order, limit. Time filter is **inclusive** on `started_at`. A Chrome segment that started yesterday and is still open **disappears** from “last 15 minutes” because `started_at` is yesterday. That is a product quirk worth remembering.

## 14.9 Frontend `loadFleetTabData`

Applications branch has **no** `&& !f.applicationActivity` guard. Every tab activation refetches. Time query omitted keys if global time API missing (`{}`).

`liveDurationSeconds` for closed rows prefers wall clock from timestamps over stored seconds (handles clock fixes). For open rows ignores stored seconds and uses `Date.now()`.

---

# 15. Knowledge Check

Do not skip ahead. Answer in chat in your own words. Short answers are fine if they are precise.

**Q1.** Application Activity records running processes. True or false? Why?

**Q2.** The same Go module uploads both sessions and application rows. Why is that safer than two modules?

**Q3.** Chrome stays in front but the tab title changes from GitHub to YouTube. How many `application_activity` rows, and why?

**Q4.** Name the three techniques `foregroundApp` uses to read the user’s window when the agent runs as SYSTEM.

**Q5.** What HTTP status does the platform return if application rows arrive before the session exists? What does the agent do next?

**Q6.** Why can `GET /api/v1/fleet/{id}/applications?started_at_from=<15 minutes ago>` hide a Chrome window that is still in the foreground (you can see it on the computer)?

**Q7.** The UI says “200 of 808 activity row(s).” Where did 200 come from? Where is the rest?

**Q8.** Draw (ASCII is fine) the path of one failed POST until it succeeds.

When these are correct, Lesson 2 is **Sessions** (the parent of every activity row).

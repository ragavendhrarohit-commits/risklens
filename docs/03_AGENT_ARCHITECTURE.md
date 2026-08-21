# 03 — Agent Architecture

Source root: `agent/`. Module path: `risklens/agent`. Go version: 1.23.

The agent is a Windows service (or console process) that collects endpoint facts and uploads JSON to the Platform API. It has no UI and no database client.

## Package map

| Package | Path | Responsibility |
|---------|------|----------------|
| Entry | `cmd/risklens-agent` | Flags, config, identity, module assembly, service runner |
| App | `internal/app` | `StartAll` → wait → `StopAll` with shutdown timeout |
| Service | `internal/service` | Windows SCM handler, console runner, session-change sink |
| Configuration (live) | `internal/configuration/*` | YAML load/validate/watch, env overrides |
| Configuration (legacy) | `internal/config` | Sprint 1 `agent:`/`service:` schema. **Not imported by `main`.** |
| Identity | `internal/identity/*` | UUID identity create/load/validate/store |
| Module framework | `internal/modules/interfaces`, `base`, `registry`, `manager` | Register, start, stop |
| Device | `internal/modules/device` | One-shot hardware inventory into memory |
| Schedule | `internal/schedule` | Shared ticker for recurring jobs (Phase A; not yet wired to modules) |
| Pipeline | `internal/pipeline` | Generic collect/compare/upload orchestration (Phase B; not yet wired to modules) |
| Registration | `internal/modules/registration` | `POST /api/v1/agents/register` |
| Device upload | `internal/modules/deviceupload` | `POST /api/v1/devices` |
| Heartbeat | `internal/modules/heartbeat` | Periodic `POST /api/v1/agents/{id}/heartbeat` |
| Users | `internal/modules/users` | Periodic local account enumeration + upload |
| Software | `internal/modules/software` | Periodic installed-app enumeration + upload |
| Activity | `internal/modules/activity` | Sessions, apps, WTS, queue, shutdown flush |
| Transport | `internal/transport/*` | HTTP client, middleware, errors, request/response |
| Logging | `internal/logging` | JSON rotating file, console fanout, Event Log |
| Version | `internal/version` | ldflags `Version`, `Commit`, `BuildTime` |

## Entry point

File: `agent/cmd/risklens-agent/main.go`

### Special argv

If `os.Args[1] == "--activity-foreground"`, run `activity.RunForegroundHelper()` (Windows: print foreground window JSON; non-Windows: exit 1). Used as a helper process from the activity collector.

### Flags (not parsed when running as a detected Windows service)

| Flag | Default | Meaning |
|------|---------|---------|
| `-config` | empty → env → `%ProgramData%\RiskLens\agent.yaml` | YAML path |
| `-console` | false | Force console/debug runner even if already a service |
| `-version` | — | Print version string, exit 0 |

Service mode config path comes only from `RISKLENS_AGENT_CONFIG`.

### Exit codes

| Code | When |
|-----:|------|
| 0 | Clean stop |
| 2 | Flag parse error |
| 10 | Config missing |
| 11 | Config invalid |
| 20 | Logger init failed |
| 21 | Identity failed |
| 30 | Service / SCM failure |
| 40 | Application failure |
| 50 | Shutdown deadline exceeded |

### Console vs service

`useConsole := console \|\| !isService`. Console mode writes JSON to the log file and text to stdout. Windows service uses `svc.Run`. Non-Windows stub uses SIGINT/SIGTERM.

## Application lifecycle

File: `internal/app/app.go`

```
New(shutdownTimeout, logger, modules...)
  register each module
  manager.New

Run(ctx)
  log "Service started"
  manager.StartAll(ctx)
  wait ctx.Done()
  Shutdown(timeout)

Shutdown(ctx)
  log "Service stopping"
  manager.StopAll(ctx)  // reverse start order
  log "Service stopped"
```

Default shutdown timeout if `<= 0`: **15s**. `UseIdentity` / `UseConfiguration` attach read-only providers; App never opens those files itself.

Misconfigured modules are **skipped** in `main.buildModules` (logged). They do not abort process start. Individual module `Start` methods typically `MarkFailed` instead of returning errors to the manager.

## Service / Windows integration

### Windows (`internal/service/windows.go`, `//go:build windows`)

`Handler.Execute` runs `executeLoop` and reports SCM states.

Accepted controls while running:

- Stop
- Shutdown
- PreShutdown
- SessionChange
- Interrogate

**SessionChange:** logs the notification; if a `SessionChangeSink` is set (the activity module), calls `OnWindowsSessionChange(eventType, sessionID)` on a **new goroutine**.

**Shutdown / PreShutdown:** calls `OnWindowsShutdown` with flush timeout `min(8s, handler.Timeout)` **before** requesting stop. This is a user/machine shutdown, not an agent restart.

### Non-Windows stub (`windows_stub.go`)

`IsWindowsService()` is false. SIGINT/SIGTERM → stop. No session notifications.

### Loop (`loop.go`)

1. Report StartPending (30s hint)
2. Start `application.Run` in a goroutine
3. Report Running
4. On stop/shutdown: cancel, wait up to `timeout+1s`
5. Timer expiry → exit 50
6. App error → exit 40 (or 50 if shutdown timeout)

## Interfaces

### Module (`internal/modules/interfaces/module.go`)

```go
Name() string
Version() string
Start(ctx context.Context) error
Stop(ctx context.Context) error
Status() Status
Health() Health
```

Statuses include registered, starting, running, stopping, stopped, failed (see `interfaces` constants).

### Transport (`internal/transport/interfaces`)

Modules call `Transport.Do`. They do not import `net/http`.

### Identity provider

Heartbeat, registration, uploads call `identity.Current() (Identity, bool)`.

### Session change sink

Implemented by the activity module: `OnWindowsSessionChange`, `OnWindowsShutdown`.

## Managers

### Module manager (`internal/modules/manager`)

- Start: registration order
- Stop: reverse of successfully started modules
- Start failure: rollback with 5s stop timeout
- `ErrAlreadyStarted` if `StartAll` called twice

### Configuration manager (`internal/configuration/manager`)

Load → apply env → validate → optional file watcher (1s poll). Subscribers can be notified on reload. **Modules are not restarted** when YAML changes; hot-reload does not reconfigure running collectors.

### Identity manager (`internal/identity/manager`)

`LoadOrCreate`: load and validate, or create UUID v4 `agent_id` and `install_id` (must differ), persist atomically.

## Configuration

Live schema: `internal/configuration/model`. Defaults and env vars: [10_CONFIGURATION.md](10_CONFIGURATION.md).

The YAML store also maps legacy keys `agent.log_level`, `agent.log_file`, `agent.shutdown_timeout`, `service.name`, `service.display_name` into the canonical model.

`cfg.Modules.Enabled` is loaded and validated but **`buildModules` ignores it**. All modules are always constructed; enablement is per-module YAML (`registration.enabled`, `heartbeat.enabled`, …). Exception: the device collector is **hardcoded enabled** in `main`.

`performance.max_concurrency` is validated and **never used**.

`security` and `update` are empty placeholder structs.

## Storage

| File | Typical path | Contents |
|------|----------------|----------|
| Config | `%ProgramData%\RiskLens\agent.yaml` | Canonical YAML |
| Identity | `%ProgramData%\RiskLens\state\identity.json` or `storage.identity_file` | Agent UUID identity |
| Activity state | `{state_dir}/activity.json` (fallback `%TEMP%\RiskLens\activity.json`) | Open session + foreground app |
| Upload queue | `{state_dir}/session_upload_queue.json` | Failed session/app POSTs, max 32 |
| Session events | `{state_dir}/session_events.json` | Local WTS timeline, max 200 events. **Not uploaded.** |
| Logs | `%ProgramData%\RiskLens\logs\agent.log` | JSON logs, 20MB × 5 files |

Identity writes: temp file + rename, mode `0600`, directory `0700`, max 64 KiB.

## Identity

JSON shape:

```json
{
  "schema_version": 1,
  "agent_id": "<uuid v4>",
  "install_id": "<uuid v4>",
  "agent_version": "<version>",
  "created_at": "<RFC3339>",
  "updated_at": "<RFC3339>",
  "status": "active",
  "display_name": "...",
  "machine_name": "...",
  "operating_system": "windows/amd64"
}
```

`CollectInfo` uses hostname (or `"unknown"`), `GOOS/GOARCH`, and display name (config identity display name, else general display name, else hostname).

## Module start order (`main.buildModules`)

1. `device` — always
2. `registration` — if constructed
3. `deviceupload` — only if device module exists
4. `heartbeat`
5. `users`
6. `software`
7. `activity`

Device collection therefore happens before registration and device upload in the start sequence.

## Device inventory collection

Package: `internal/modules/device`

- **Windows:** `WindowsCollector` (Win32 / WMI-style fields: CPU, memory, disks, manufacturer, model, boot time, current user, …)
- **Non-Windows:** stub collector, partial portable fields + error
- Recurring at `device_upload.interval` (default 5m) through `Pipeline.Start`. YAML `device_upload.enabled` gates upload, not collection.
- Device owns comparable payload (`Info.Comparable()`) and transport payload (`Info.ForTransport`). The pipeline collector adapter only maps interfaces.
- Result stays in memory (`device.Info`). Device upload reads it.
- Collection health is independent of upload health. HTTP failure does not fail the collector. Details: [DEVICE_PIPELINE.md](DEVICE_PIPELINE.md).

Fields: `machine_name`, `hostname`, `operating_system`, `os_version`, `windows_build`, `cpu_model`, `cpu_cores`, `memory_bytes`, `disk_size_bytes`, `disk_free_bytes`, `architecture`, `serial_number`, `manufacturer`, `model`, `boot_time`, `current_user`, `agent_version`, `collected_at`.

## How uploads work

Pattern for registration / device / heartbeat / users / software:

1. Read identity
2. Collect (or use cached device info)
3. Build JSON matching Platform Pydantic schemas
4. `Transport.Do`
5. Require `accepted: true` and module-specific status
6. On failure: `MarkFailed` or snapshot error; **no persistent queue** except activity

Heartbeat, users, software, activity use a scheduler with `inflight.TryLock` so ticks do not overlap. Heartbeat fires immediately, then every interval (default 30s).

Success HTTP codes accepted by module clients typically include 200/201/202 depending on the client.

Registration treats **409 Conflict** as already registered (`ErrDuplicate`).

## Activity module (sessions + applications)

This is the largest module. Details of the session state machine: [08_AGENT_SESSION_LIFECYCLE.md](08_AGENT_SESSION_LIFECYCLE.md).

### Collection

Windows collector:

- Enumerate WTS sessions
- Foreground window via Win32; may spawn `RiskLensAgent.exe --activity-foreground` (5s timeout)

Default interval: **15s**. Timeout: **15s**.

### Session ID

UUID v5 over a fixed namespace + `agentID:wtsSessionID:logonAt(RFC3339Nano/RFC3339 as coded)`.

### Application activity ID

SHA1 of `sessionID + NUL + executable + NUL + startedAt`.

### Endpoints

- `POST /api/v1/sessions`
- `POST /api/v1/applications`

### Queue / retry

`session_upload_queue.json`, max **32** items. Closed-session payloads are preserved when trimming. `drainQueue` on start and each cycle; **stops on first failure** and retries later. Failed uploads call `enqueueUploads`.

Transport itself still has **no HTTP retry**.

### Windows event sources

| Source | File | Action |
|--------|------|--------|
| SCM SessionChange | `service/windows.go` | `OnWindowsSessionChange` |
| Hidden WTS window `WM_WTSSESSION_CHANGE` | `notify_windows.go` | `handleSessionChange("wts", …)` |
| `CTRL_LOGOFF_EVENT` / `CTRL_SHUTDOWN_EVENT` | `logoff_windows.go` | `CloseSession` |
| `WM_ENDSESSION` | `notify_windows.go` | `CloseSession("end_session")` |
| SCM Shutdown / PreShutdown | `service/windows.go` | `OnWindowsShutdown` → `CloseSession("shutdown")` |

WTS codes mapped in `lifecycle.go`: logon 0x5, logoff 0x6, lock 0x7, unlock 0x8, terminate 0xB (treated as logoff), console/remote connect/disconnect.

Lock/unlock/disconnect: **timeline only** (`session_events.json`). The session stays active.

**Agent restart must not call `FlushClose`** (code comment). Restart is not logout.

## Heartbeat

Default interval 30s, timeout 10s, enabled via YAML.

Payload:

```json
{ "agent_id": "...", "install_id": "...", "timestamp": "<time>" }
```

Path: `POST /api/v1/agents/{agent_id}/heartbeat`

Logs `REPRO Heartbeat payload` on each beat (diagnostic instrumentation).

No sessions in the heartbeat payload (asserted by tests).

## Users / software collectors

| Module | Default interval | Collector |
|--------|------------------|-----------|
| users | 5m | Windows local accounts; stub errors on `!windows` |
| software | 15m | Windows registry uninstall keys; stub on `!windows`; `DedupKey() = name + "\x00" + version` |

## Concurrency model

| Component | Mechanism |
|-----------|-----------|
| Manager / registry / base status | `sync.Mutex` / `RWMutex` |
| Activity | `processMu` serializes `cycle`/`FlushClose`; `closing atomic.Bool`; RWMutex on state/queue |
| Schedulers | `inflight.TryLock` — skip tick if previous still running |
| SCM session change | extra goroutine |
| WTS notify | dedicated message-pump goroutine |
| Config / identity managers | RWMutex |
| Transport client | documented concurrent-safe |

## Retry logic

| Layer | Retry? |
|-------|--------|
| Transport middleware | **No** (comment: scheduled for a later sprint) |
| Registration / device / heartbeat / users / software | **No persistent retry.** Next scheduler tick retries. |
| Activity | **Yes** — disk queue, drain until first failure |
| Install script service recovery | Windows SCM restart after 60s |

## Offline behaviour

The agent does not detect “offline” as a first-class presence state. If the Platform is unreachable:

- Heartbeat/users/software fail the tick and try again later
- Activity enqueues session/app payloads locally
- Identity and `activity.json` remain on disk
- When the API returns, queued activity uploads drain

The **backend** derives Offline from heartbeat age and may close the single ACTIVE session using `last_seen`. That is a server-side behaviour, not an agent message named “offline”.

## How modules communicate

They do **not** call each other except:

- `deviceupload` reads the in-memory device module result
- `service.Handler` calls activity as `SessionChangeSink`
- All network modules independently read identity

There is no in-process event bus.

## Failure handling

- Logger / identity / config failures **exit the process** with the codes above
- Module start/runtime collection failures **do not** take down the service (failed status + logs)
- Shutdown timeout → exit 50
- Unknown HTTP bodies → module-local malformed-response errors
- Activity `closing` flag blocks normal `cycle` during flush

## Logging

- JSON to rotating file (`maxLogBytes=20MB`, `maxLogFiles=5`)
- UTC timestamps
- Fixed attrs: `service`, `version`, `hostname`, `instance_id`
- Console: extra text handler
- Windows Event Log source `RiskLensAgent` for exact strings: Version, Configuration loaded, Service starting/started/stopping/stopped (IDs 1000–1005, 1099 on write failure)

REPRO logs (`"REPRO "` prefix) are in `activity/repro_log.go`, `activity/upload.go`, and `heartbeat/heartbeat.go`. They are diagnostic, not a product feature.

## Install / uninstall

`agent/scripts/windows/install.ps1` (Administrator):

- Requires `bin\RiskLensAgent.exe`
- Creates ProgramData logs/state
- Copies `configs/agent.example.yaml` → `agent.yaml` if missing
- `sc.exe create RiskLensAgent` delayed-auto, restart after 60s
- Registers Event Log source

`uninstall.ps1` stops and deletes the service; **leaves** ProgramData files.

Verification harnesses (not product): `agent/scripts/verify_device/`, `verify_device_restart/`.

## Dead / unused agent code

| Item | Fact |
|------|------|
| `internal/config` | Not imported by `main` |
| `modules.enabled` | Ignored by `buildModules` |
| `performance.max_concurrency` | No runtime effect |
| `security` / `update` YAML | Empty structs |
| `session_events.json` | Local only |
| Agent README | Claims no networking |

## Related documents

- Session lifecycle: [08_AGENT_SESSION_LIFECYCLE.md](08_AGENT_SESSION_LIFECYCLE.md)
- Config keys: [10_CONFIGURATION.md](10_CONFIGURATION.md)
- Sequences: [13_SEQUENCE_DIAGRAMS.md](13_SEQUENCE_DIAGRAMS.md)

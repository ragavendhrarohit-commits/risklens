# 13 — Sequence Diagrams

All diagrams reflect **current source**, including the login write path that does not call `activate_current`.

---

## Agent startup

```mermaid
sequenceDiagram
  participant SCM as Windows SCM / console
  participant Main as risklens-agent main
  participant Cfg as Configuration manager
  participant Id as Identity manager
  participant Log as Logger
  participant App as App / Module manager
  participant Dev as device
  participant Reg as registration
  participant Up as deviceupload
  participant Hb as heartbeat
  participant API as Platform API

  SCM->>Main: Execute / Run
  Main->>Cfg: Load agent.yaml + env
  Main->>Log: Init JSON file (+ console)
  Main->>Id: LoadOrCreate identity.json
  Main->>App: New(modules in order)
  App->>Dev: Start (collect inventory)
  App->>Reg: Start
  Reg->>API: POST /api/v1/agents/register
  API-->>Reg: 201 registered or 409 duplicate
  App->>Up: Start
  Up->>API: POST /api/v1/devices
  App->>Hb: Start (immediate beat + scheduler)
  Hb->>API: POST /api/v1/agents/{id}/heartbeat
  Note over App: users, software, activity Start similarly
  App->>SCM: Running
```

Misconfigured modules are skipped. Collection/upload failures mark the module failed; the process stays up.

---

## Heartbeat (liveness)

```mermaid
sequenceDiagram
  participant Sch as Heartbeat scheduler
  participant Mod as heartbeat.Module
  participant Tx as Transport client
  participant API as POST /agents/{id}/heartbeat
  participant Svc as HeartbeatService
  participant DB as agent_heartbeats + agent_status

  Sch->>Mod: tick (TryLock)
  Mod->>Mod: identity.Current()
  Mod->>Mod: REPRO Heartbeat payload log
  Mod->>Tx: Do JSON {agent_id, install_id, timestamp}
  Tx->>API: HTTP POST
  API->>Svc: record()
  Svc->>DB: append heartbeat, record_seen
  API-->>Tx: 200 accepted heartbeat_status=ok
```

No HTTP retry. Next tick retries. Inflight skip if previous tick still running.

---

## Inventory upload (device)

```mermaid
sequenceDiagram
  participant Dev as device.Module
  participant Up as deviceupload.Module
  participant API as POST /api/v1/devices
  participant Svc as DeviceInventoryService
  participant FP as FleetProjection
  participant DB as device_inventory / agent_status

  Dev->>Dev: WindowsCollector.Collect
  Dev-->>Up: in-memory Info (collected/partial)
  Up->>API: POST inventory fields
  API->>Svc: upsert
  Svc->>DB: upsert device_inventory
  Svc->>FP: on_inventory
  FP->>DB: last_inventory_at
  API-->>Up: 201 created or 200 stored
```

Users/software follow the same shape with their own POST paths and schedulers (5m / 15m). They have **no** disk queue.

---

## Application upload

```mermaid
sequenceDiagram
  participant Col as activity collector
  participant Mod as activity.Module
  participant Q as session_upload_queue.json
  participant API as POST /api/v1/applications
  participant Svc as ApplicationActivityService
  participant DB as application_activity

  Col->>Mod: Observation{Session, App}
  Mod->>Mod: close previous app segment if window changed
  alt POST succeeds
    Mod->>API: activities[]
    API->>Svc: upsert
    Svc->>DB: require session_id exists
    API-->>Mod: 200 stored
  else transport/HTTP failure
    Mod->>Q: enqueue
  end
  Mod->>Q: drainQueue until first failure
```

Unknown `session_id` → 422 `unknown_session` (not queued specially).

---

## Login

```mermaid
sequenceDiagram
  participant Win as Windows WTS 0x5
  participant Act as activity.Module
  participant Disk as activity.json
  participant API as POST /api/v1/sessions
  participant Svc as SessionService
  participant DB as computer_sessions

  Win->>Act: logon (SCM or WTS window)
  Act->>Act: LOGIN DETECTED + REPRO dumps
  Act->>Act: cycleNow()
  Act->>Disk: persist new session_id
  alt local previous session still open
    Act->>API: sessions[closed previous, active new]
    Note over Svc: mixed path: _upsert_one closed then _activate_login
  else no local previous
    Act->>API: sessions[active new only]
  end
  Svc->>Svc: _activate_login → _upsert_one → upsert_many
  Svc->>DB: INSERT/UPDATE this session_id only
  Note over DB: other ACTIVE rows for this agent are not closed
  Svc->>Svc: REPRO dump AFTER LOGIN
```

---

## Logout

```mermaid
sequenceDiagram
  participant Win as WTS logoff 0x6 / terminate 0xB / CTRL_LOGOFF
  participant Act as activity.Module
  participant API as POST /api/v1/sessions
  participant Svc as SessionService
  participant DB as computer_sessions

  Win->>Act: logoff
  Act->>Act: CloseSession / FlushClose (8s)
  Act->>Act: persistCloseBestEffort
  Act->>API: status=closed, logoff_at=now
  alt payload closed-only
    Svc->>Svc: _close_current_session
  else mixed
    Svc->>Svc: _upsert_one CLOSED
  end
  Svc->>DB: UPDATE WHERE session_id AND status != closed
  Note over DB: closed rows become immutable
```

If the process dies before POST, no logout is stored.

---

## Shutdown

```mermaid
sequenceDiagram
  participant SCM as SERVICE_CONTROL_SHUTDOWN / PRESHUTDOWN
  participant SvcWin as service.Handler
  participant Act as activity.Module
  participant API as POST /api/v1/sessions
  participant Sess as SessionService
  participant DB as computer_sessions

  SCM->>SvcWin: Shutdown
  SvcWin->>Act: OnWindowsShutdown (≤8s)
  Act->>Act: recordEvent shutdown
  Act->>Act: FlushClose
  Act->>API: closed-only payload
  Sess->>Sess: SHUTDOWN path, max logon_at item
  Sess->>DB: _upsert_one current_session_id only
  SvcWin->>SvcWin: then stop App
```

---

## Retry (activity queue)

```mermaid
sequenceDiagram
  participant Cyc as activity cycle / Start
  participant Q as queue file max 32
  participant Tx as Transport
  participant API as Platform

  Cyc->>Q: drainQueue
  loop each item until failure
    Q->>Tx: POST sessions or applications
    Tx->>API: HTTP
    alt success
      API-->>Tx: accepted
      Tx-->>Q: pop item
    else error
      Tx-->>Q: stop drain, keep remaining
    end
  end
```

Registration/device/heartbeat/users/software: **no** this queue.

---

## Offline recovery

```mermaid
sequenceDiagram
  participant UI as Browser
  participant Fleet as FleetService
  participant Pres as compute_fleet_status
  participant Sess as SessionService
  participant DB as computer_sessions

  UI->>Fleet: GET /api/v1/fleet or /sessions
  Fleet->>Pres: last_seen age
  alt age > 300s → offline
    Fleet->>Sess: close_open_for_agent(last_seen)
    alt exactly one ACTIVE and it is latest logon
      Sess->>DB: merge_fallback_close + upsert_many
    else 0 or 2+ ACTIVE
      Sess-->>Fleet: OFFLINE RECONCILIATION REFUSED
    end
  end
  Fleet-->>UI: online_status + session list
```

Agent-side recovery on process start: load `activity.json`, drain queue, **do not** FlushClose, then resume cycles.

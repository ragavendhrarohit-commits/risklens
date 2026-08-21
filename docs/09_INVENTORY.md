# 09 — Inventory and Collectors

RiskLens distinguishes **inventory** (what exists on the computer) from **usage** (what happened during a session). This document lists every collector surface in the UI, agent, API, and database.

Status vocabulary:

- **Implemented** — collector + API + table + live UI path
- **Partial** — some layers only
- **Stub** — UI and/or API return `"Not implemented yet"` / empty domain package
- **Not implemented** — no collector, no API, no table

---

## Device inventory

| Layer | Status |
|-------|--------|
| Agent collector | **Implemented** (`internal/modules/device`, Windows) |
| Upload | **Implemented** `POST /api/v1/devices` once at start |
| Table | **Implemented** `device_inventory` |
| Query API | **Implemented** `GET /api/v1/devices` |
| Fleet UI | **Implemented** (overview columns from fleet join) |
| Dedicated Devices page | **Not implemented** in current SPA (README’s old Devices nav is gone) |

Fields collected: hostname, OS, build, architecture, CPU, cores, memory, disk total/free, manufacturer, model, serial, boot_time, current_user, agent_version.

Non-Windows: stub collector, partial fields + error.

No periodic re-upload; values go stale until the agent restarts.

---

## Installed applications (software inventory)

| Layer | Status |
|-------|--------|
| Agent | **Implemented** registry collector, default 15m |
| API | **Implemented** POST/GET `/api/v1/software` |
| Table | **Implemented** `software_inventory` |
| UI | **Implemented** Reports → Software Inventory; Fleet tab Installed Applications |

Dedup on agent: `name + "\x00" + version`. Backend id is a deterministic hash.

Uninstall deletion: **Not implemented** (old rows remain unless overwritten by same id).

---

## Accounts (user inventory)

| Layer | Status |
|-------|--------|
| Agent | **Implemented** local accounts, default 5m |
| API | **Implemented** POST/GET `/api/v1/users` |
| Table | **Implemented** `user_inventory` PK `sid` |
| UI | **Implemented** Reports → Account Inventory; Fleet Accounts tab |

Department / Group / “Current User” columns on the inventory pages are UI placeholders (`"—"`).

Domain accounts / Azure AD: only whatever the Windows collector returns (`source` default `"Local"`).

---

## Sessions (computer sessions)

Usage, not inventory. **Implemented.** See [08_AGENT_SESSION_LIFECYCLE.md](08_AGENT_SESSION_LIFECYCLE.md).

UI: Fleet detail → Activity → Sessions.

---

## Application activity (runtime)

Usage, not installed-software. **Implemented.** Foreground window segments.

UI: Fleet detail → Activity → Application Activity.

---

## Processes

| Layer | Status |
|-------|--------|
| Domain package | **Not implemented** (no `domain/processes`) |
| Agent collector | **Not implemented** |
| API | **Not implemented** (policy flag `collect_processes` reserved) |
| UI | **Stub** Fleet tab Processes |

---

## Services (Windows services)

| Layer | Status |
|-------|--------|
| Domain | **Not implemented** (do not confuse with `platform/app/services/`) |
| Agent | **Not implemented** |
| API | Policy flag `collect_services` reserved |
| UI | **Stub** Fleet tab Services |

---

## Network

| Layer | Status |
|-------|--------|
| Domain | **Stub** `platform/app/domain/network/` empty |
| Agent | **Not implemented** |
| API prefix | Unmounted `/api/v1/network` |
| UI | **Stub** Adapters / connections / listening ports / public IP = `"Not collected yet"` |

`agent_status.ip_address` column exists; application code does not populate it from the client address.

---

## Policies

| Layer | Status |
|-------|--------|
| Table `policies` + `agent_policy` | **Implemented** |
| Default Windows Policy seed | **Implemented** (id `11111111-1111-4111-8111-111111111111`, 30s heartbeat) |
| Assignment on register | **Implemented** |
| HTTP write/editor | **Not implemented** |
| Agent fetch/apply | **Not implemented** (agent uses local YAML) |
| Company Policies nav page | **Stub** |
| Fleet Policies tab | **Implemented** (read-only) |

Reserved booleans: `collect_processes`, `collect_services`, `collect_event_logs`.

---

## Health

| Layer | Status |
|-------|--------|
| Liveness `GET /health` | **Implemented** (process only) |
| Fleet health endpoint | **Partial** — presence + inventory bytes + timestamps |
| CPU/memory/disk **usage** | **Stub** `"Not implemented yet"` |
| Agent health metrics POST | **Not implemented** |
| Domain `health` package | **Not implemented** (types live on `domain/fleet`) |

---

## Browser activity

| Layer | Status |
|-------|--------|
| Domain | **Stub** `domain/browser/` |
| Agent | **Not implemented** |
| API | Unmounted |
| UI | **Stub** Fleet Activity → Browser |
| Product model | Reserved under Session in ARCHITECTURE.md |

---

## Timeline

| Layer | Status |
|-------|--------|
| Domain | **Stub** `domain/timeline/` |
| API | **Stub** `GET /api/v1/fleet/{id}/timeline` → `{ "status": "Not implemented yet" }` |
| Agent `session_events.json` | **Implemented locally**, explicitly **not** a Fleet payload |
| UI | **Stub** hardcoded placeholder rows |

---

## Event logs

Policy flag `collect_event_logs` only. **Not implemented.** Agent Windows Event Log usage is for **agent service lifecycle**, not endpoint security event collection.

---

## Groups

| Layer | Status |
|-------|--------|
| Domain | **Stub** `domain/groups/` |
| API | Unmounted |
| UI | Administration → Groups placeholder; Fleet Group column `"—"` |

---

## Security product surfaces

Alerts, incidents, detections, response, threat timeline, rules, AI, reports (except inventory links), administration (except Groups link): **Stub** UI + empty domain packages + unmounted routers.

---

## Heartbeat (not inventory)

Liveness signal. **Implemented.** Drives Online/Warning/Offline. See presence thresholds in [02_SYSTEM_ARCHITECTURE.md](02_SYSTEM_ARCHITECTURE.md).

---

## Matrix

| Capability | Agent | API ingest | API query | DB | Fleet UI |
|------------|-------|------------|-----------|-----|----------|
| Device hardware | Yes | POST /devices | GET /devices (unwired) | device_inventory | Overview |
| Accounts | Yes | POST /users | GET /users | user_inventory | Accounts + Reports |
| Installed apps | Yes | POST /software | GET /software | software_inventory | Software + Reports |
| Sessions | Yes | POST /sessions | Fleet nested | computer_sessions | Sessions |
| App usage | Yes | POST /applications | Fleet nested | application_activity | Application Activity |
| Heartbeat | Yes | POST heartbeat | derived | agent_heartbeats + agent_status | Status |
| Policy | No | No | GET fleet/policy | policies | Policies tab |
| Tags | No | No | filter on GET fleet | agent_tags | column, usually empty |
| Processes | No | No | No | No | Stub |
| Services | No | No | No | No | Stub |
| Network | No | No | No | ip_address unused | Stub |
| Browser | No | No | No | No | Stub |
| Timeline | local file only | No | stub GET | No | Stub |

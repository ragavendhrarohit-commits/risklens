# 07 — API Reference

Base URL in development: `http://127.0.0.1:8000`

All application errors use:

```json
{
  "error": {
    "code": "string",
    "message": "string",
    "details": {},
    "request_id": "string"
  }
}
```

Headers commonly present: `X-Request-ID`, `X-Response-Time-ms`.

Pydantic ingest bodies use `extra="forbid"`: unknown fields → 422 `validation_error`.

OpenAPI: `GET /openapi.json`, UI: `/docs`, `/redoc`.

Caller column: **Agent** = Windows agent POST; **Frontend** = `frontend/assets/app.js`; **Probe** = health/tools; **None** = implemented but unused by current callers.

---

## GET `/`

**Files:** `platform/app/api/v1/root.py`

**Response 200** `RootResponse`:

```json
{
  "service": "RiskLens Backend",
  "status": "running",
  "version": "0.1.0"
}
```

Values come from Settings (`app_name`, `app_version`).

**Caller:** Probe. Not the SPA (the SPA is on :8080).

**Errors:** 500 `internal_error` uncaught.

---

## GET `/health`

**Files:** `platform/app/api/v1/health.py`

**Response 200:**

```json
{ "status": "healthy" }
```

Liveness only. Does **not** check PostgreSQL. If this connection is refused, Uvicorn is not listening.

**Caller:** Probe.

---

## GET `/version`

**Files:** `platform/app/api/v1/version.py`

**Response 200:**

```json
{
  "version": "0.1.0",
  "build_time": "unknown",
  "environment": "development"
}
```

**Caller:** Probe.

---

## POST `/api/v1/agents/register`

**Files:** `api/v1/agents.py`, `schemas/agents.py`, `services/agent_service.py`

**Caller:** Agent `internal/modules/registration`

**Request:**

```json
{
  "agent_id": "c2c28db4-9838-45de-85e0-d487bf310091",
  "install_id": "11111111-1111-4111-8111-111111111111",
  "agent_version": "0.1.0",
  "machine_name": "MSI",
  "operating_system": "windows/amd64",
  "display_name": "MSI",
  "created_at": "2026-08-14T04:03:44Z"
}
```

**Validation:** UUID `agent_id`; non-empty strings max 256 for install_id, agent_version, machine_name, operating_system; optional display_name; datetime created_at.

**Response 201:**

```json
{
  "accepted": true,
  "agent_id": "c2c28db4-9838-45de-85e0-d487bf310091",
  "registration_status": "registered",
  "server_version": "0.1.0",
  "server_time": "2026-08-17T03:46:31Z"
}
```

**Errors:**

| Status | Code | When |
|--------|------|------|
| 409 | `duplicate_agent` | Same `agent_id` already registered. Agent treats this as success. |
| 422 | `validation_error` | Schema |

Side effect: `FleetProjection.on_registered` creates `agent_status` + default policy assignment.

---

## POST `/api/v1/agents/{agent_id}/heartbeat`

**Files:** `api/v1/agents.py`, `services/heartbeat_service.py`

**Caller:** Agent heartbeat module

**Request:**

```json
{
  "agent_id": "c2c28db4-9838-45de-85e0-d487bf310091",
  "install_id": "...",
  "timestamp": "2026-08-17T03:46:31Z"
}
```

**Validation:** Path `agent_id` must equal body `agent_id`.

**Response 200:**

```json
{
  "accepted": true,
  "agent_id": "...",
  "heartbeat_status": "ok",
  "server_version": "0.1.0",
  "server_time": "...",
  "received_at": "..."
}
```

**Errors:** 422 path/body mismatch; 404 `unknown_agent`; 422 schema.

**Instrumentation:** `session_lifecycle_log.banner("HEARTBEAT PAYLOAD")`.

**Side effect:** Insert `agent_heartbeats`; update `agent_status.last_seen`.

---

## GET `/api/v1/devices`

**Files:** `api/v1/devices.py`, `services/device_query_service.py`

**Caller:** None in frontend (tests only)

**Query:** `hostname`, `operating_system`, `manufacturer`, `current_user` (max 256), `limit` 1–200 default 50, `offset` ≥ 0, `sort` default `-received_at`

**Response 200:** `{ "items": [ DeviceItemResponse ], "total", "limit", "offset" }`

`DeviceItemResponse` includes inventory fields plus `registration_status`, `status` (`online|stale|offline`), `last_seen`, `heartbeat_age_seconds`.

**Errors:** 422 `invalid_query`

---

## GET `/api/v1/devices/{agent_id}`

**Caller:** None in frontend

**Response 200:** `DeviceItemResponse`

**Errors:** 404 `not_found`; 422 invalid UUID

---

## POST `/api/v1/devices`

**Caller:** Agent deviceupload

**Request (all required unless noted):** `agent_id`, `install_id`, `machine_name`, `hostname`, `operating_system`, `os_version`, `windows_build`, `architecture`, `cpu_model`, `cpu_cores`, `memory_bytes`, `disk_total_bytes`, `disk_free_bytes`, `manufacturer`, `model`, `serial_number`, `boot_time` (nullable), `current_user`, `agent_version`, `collected_at`

**Response 201** first inventory, **200** subsequent:

```json
{
  "accepted": true,
  "agent_id": "...",
  "created": true,
  "inventory_status": "stored",
  "server_version": "...",
  "server_time": "...",
  "received_at": "..."
}
```

**Errors:** 404 `unknown_agent`; 422 schema

---

## POST `/api/v1/users`

**Caller:** Agent users module

**Request:**

```json
{
  "agent_id": "...",
  "install_id": "...",
  "collected_at": "...",
  "users": [
    {
      "username": "rohit",
      "sid": "S-1-5-21-...",
      "display_name": "rohit",
      "account_type": "Local",
      "enabled": true,
      "builtin": false,
      "password_required": true,
      "last_logon": "2026-08-17T03:00:00Z",
      "source": "Local"
    }
  ]
}
```

**Response 200:** `accepted`, `agent_id`, `stored` (count), `inventory_status`, `server_version`, `server_time`, `received_at`

**Errors:** 404 `unknown_agent`; 422 schema

---

## GET `/api/v1/users`

**Caller:** Frontend Account Inventory

**Query:** `username`, `account_type`, `enabled`, `limit`, `offset`, `sort` default `-received_at`

**Response 200:** `{ items: UserItemResponse[], total, limit, offset }`

`UserItemResponse`: `sid`, `agent_id`, `username`, `display_name`, `account_type`, `enabled`, `builtin`, `password_required`, `last_logon`, `source`, `collected_at`, `received_at`, `machine_name`

**Errors:** 422 `invalid_query`

---

## GET `/api/v1/users/{sid}`

**Caller:** None in frontend

**Errors:** 404 `user_not_found`

---

## POST `/api/v1/software`

**Caller:** Agent software module

**Request:** `agent_id`, `install_id`, `collected_at`, `applications[]` with `application_name`, `version`, `publisher`, `install_date`, `install_location`, `estimated_size`, `architecture`, `system_component`, `windows_component`, `registry_path`

**Response 200:** same ingest envelope as users (`stored` count)

**Errors:** 404 `unknown_agent`; 422 schema

---

## GET `/api/v1/software`

**Caller:** Frontend Software Inventory

**Query:** `application_name`, `publisher`, `version`, `architecture`, `limit`, `offset`, `sort`

**Response 200:** items include `id`, names, publisher, install metadata, `machine_name`, timestamps

**Errors:** 422 `invalid_query`

---

## GET `/api/v1/software/{software_id}`

**Caller:** None in frontend

**Errors:** 404 `software_not_found`

---

## POST `/api/v1/sessions`

**Files:** `api/v1/sessions.py`, `services/session_service.py`

**Caller:** Agent activity module

**Request:**

```json
{
  "agent_id": "...",
  "install_id": "...",
  "collected_at": "...",
  "sessions": [
    {
      "session_id": "b5ddd304-dcf5-525b-93c6-f1252b0674a2",
      "username": "MSI\\rohit",
      "account_sid": "S-1-5-21-...",
      "logon_at": "2026-08-17T15:15:26Z",
      "logoff_at": null,
      "status": "active"
    }
  ]
}
```

`status` defaults to `"active"`. Closed payloads set `status: "closed"` and `logoff_at`.

A payload may mix closed + open items (login close-then-open). A payload with only closed items is treated as **shutdown**.

**Response 200:** ingest envelope with `stored` count

**Errors:** 404 `unknown_agent`; 422 schema

**Instrumentation:** INCOMING PAYLOAD, CURRENT ACTIVE SESSIONS, REPRO dumps AFTER LOGIN / AFTER SHUTDOWN.

Service rules: [08_AGENT_SESSION_LIFECYCLE.md](08_AGENT_SESSION_LIFECYCLE.md)

---

## GET `/api/v1/sessions`

**Caller:** None in frontend (Fleet uses nested route)

**Query:** `agent_id`, `status` (alias `status_filter`), `username`, `limit`, `offset`, `sort` default `-logon_at`

**Response 200:** items include `duration_seconds`, `machine_name`

**Errors:** 422 `invalid_query`

---

## GET `/api/v1/sessions/{session_id}`

**Errors:** 404 `session_not_found`

---

## POST `/api/v1/applications`

**Caller:** Agent activity module

**Request:** `agent_id`, `install_id`, `collected_at`, `activities[]` with `id`, `session_id`, `application_name`, `executable_name`, `window_title`, `started_at`, `stopped_at`, `duration_seconds`

**Response 200:** ingest envelope

**Errors:** 404 `unknown_agent`; 422 `unknown_session` if `session_id` missing; 422 schema

---

## GET `/api/v1/applications`

**Caller:** None in frontend (Fleet nested route used instead)

**Query:** `agent_id`, `session_id`, `application_name`, `limit`, `offset`, `sort`

---

## GET `/api/v1/applications/{activity_id}`

**Errors:** 404 `application_activity_not_found`

---

## GET `/api/v1/fleet`

**Caller:** Frontend Fleet + Dashboard cards

**Query:** `q`, `status` (`online|warning|offline|never_seen|healthy`), `tag`, `policy_id`, `limit` default 50 max 200, `offset`, `sort` default `hostname`

**Response 200:**

```json
{
  "summary": {
    "total_agents": 2,
    "online": 0,
    "offline": 2,
    "warning": 0,
    "never_seen": 0,
    "healthy": 0
  },
  "items": [ { "agent_id": "...", "hostname": "MSI", "online_status": "offline", "last_seen": "...", "heartbeat_age_seconds": 2479, "tags": [], "policy_name": "Default Windows Policy", "...": "..." } ],
  "total": 2,
  "limit": 50,
  "offset": 0
}
```

`healthy` means online **and** device inventory exists.

Materializing an offline row may run `close_open_for_agent`.

**Errors:** 422 `invalid_query`

---

## GET `/api/v1/fleet/{agent_id}`

**Caller:** Frontend endpoint overview

**Errors:** 404 fleet agent not found

---

## GET `/api/v1/fleet/{agent_id}/users`

Delegates to `UserInventoryService.list(agent_id=…)`.

**Caller:** Frontend Accounts tab

---

## GET `/api/v1/fleet/{agent_id}/software`

**Caller:** Frontend Installed Applications tab

---

## GET `/api/v1/fleet/{agent_id}/sessions`

**Caller:** Frontend Sessions tab

Triggers offline session close when the agent is offline.

---

## GET `/api/v1/fleet/{agent_id}/applications`

**Caller:** Frontend Application Activity tab

---

## GET `/api/v1/fleet/{agent_id}/health`

**Caller:** Frontend Health tab

**Response:**

```json
{
  "status": "offline",
  "heartbeat_age_seconds": 2479,
  "last_inventory_at": "...",
  "last_communication_at": "...",
  "cpu_usage": "Not implemented yet",
  "memory_usage": "Not implemented yet",
  "disk_usage": "Not implemented yet",
  "memory_bytes": 33736060928,
  "disk_total_bytes": 2015337902080,
  "disk_free_bytes": 1366434844672
}
```

Capacity bytes come from device inventory. Usage percentages are **not implemented**.

---

## GET `/api/v1/fleet/{agent_id}/policy`

**Caller:** Frontend Policies tab

**Response:** `id`, `name`, `heartbeat_interval_seconds`, `collect_devices`, `collect_users`, `collect_software`, `collect_processes`, `collect_services`, `collect_event_logs`, `created_at`

The last three collect flags are reserved (documented on the schema). Read-only.

---

## GET `/api/v1/fleet/{agent_id}/timeline`

**Stub.** Verifies the agent exists, then:

```json
{ "status": "Not implemented yet" }
```

**Caller:** None in frontend (`app.js` does not call this URL; the Timeline tab renders client-side placeholders).

---

## Unmounted prefixes

`platform/app/api/router.py` lists future prefixes that have **no router**:

`/api/v1/activity`, `/browser`, `/network`, `/alerts`, `/incidents`, `/rules`, `/timeline` (top-level), `/reports`, `/ai`, `/groups`, `/administration`

---

## Cross-cutting errors

| Status | Code | Typical cause |
|--------|------|----------------|
| 404 | `not_found` / `unknown_agent` / resource-specific | Missing row |
| 405 | `method_not_allowed` | Wrong verb |
| 409 | `duplicate_agent` | Re-register |
| 422 | `validation_error` / `invalid_query` / `unknown_session` | Body or query |
| 500 | `internal_error` / `multiple_active_sessions` | Uncaught, or unused fatal helper |

---

## Auth

**Not implemented.** No `Authorization` header is required or checked.

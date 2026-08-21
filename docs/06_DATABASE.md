# 06 — Database

Persistence for the Platform API. Default engine: PostgreSQL via SQLAlchemy 2 + Alembic.

DSN default: `postgresql+psycopg://risklens:risklens@127.0.0.1:5432/risklens`

Test DSN (not a Settings field): `RISKLENS_TEST_DATABASE_URL`, default database name `risklens_test`.

ORM: `platform/app/infrastructure/postgres/models.py`  
Migrations: `platform/app/infrastructure/postgres/migrations/versions/`

There is **no** `docker-compose.yml` in this repository. Operationally a container named `risklens-postgres` has been used.

The UI server’s SQLite file `backend/data/risklens.db` table `identities` is **not part of this schema** and is unused.

## ER diagram

```mermaid
erDiagram
  agents ||--o| device_inventory : "agent_id PK/FK"
  agents ||--o{ agent_heartbeats : agent_id
  agents ||--o{ user_inventory : agent_id
  agents ||--o{ software_inventory : agent_id
  agents ||--o| agent_status : agent_id
  agents ||--o| agent_policy : agent_id
  policies ||--o{ agent_policy : policy_id
  agents ||--o{ agent_tags : agent_id
  agents ||--o{ computer_sessions : agent_id
  computer_sessions ||--o{ application_activity : session_id
  agents ||--o{ application_activity : agent_id

  agents {
    uuid agent_id PK
    string install_id
    string agent_version
    string machine_name
    string operating_system
    string display_name
    timestamptz created_at
    timestamptz registered_at
    string status
  }

  device_inventory {
    uuid agent_id PK
    string hostname
    string current_user
    bigint memory_bytes
    timestamptz collected_at
    timestamptz received_at
  }

  agent_heartbeats {
    int id PK
    uuid agent_id FK
    timestamptz heartbeat_time
    timestamptz received_at
    string status
  }

  user_inventory {
    string sid PK
    uuid agent_id FK
    string username
  }

  software_inventory {
    string id PK
    uuid agent_id FK
    string application_name
  }

  policies {
    uuid id PK
    string name UK
    int heartbeat_interval_seconds
  }

  agent_policy {
    uuid agent_id PK
    uuid policy_id FK
  }

  agent_tags {
    uuid agent_id PK
    string tag PK
  }

  agent_status {
    uuid agent_id PK
    timestamptz last_seen
    timestamptz last_inventory_at
    timestamptz last_communication_at
    string ip_address
  }

  computer_sessions {
    uuid session_id PK
    uuid agent_id FK
    string username
    timestamptz logon_at
    timestamptz logoff_at
    string status
  }

  application_activity {
    string id PK
    uuid session_id FK
    uuid agent_id FK
    string application_name
    timestamptz started_at
  }
```

## Migration chain

| Rev | File | Creates |
|-----|------|---------|
| 0001 | `0001_create_agents_table.py` | `agents` |
| 0002 | `0002_create_device_inventory_table.py` | `device_inventory` |
| 0003 | `0003_create_agent_heartbeats_table.py` | `agent_heartbeats` |
| 0004 | `0004_create_user_inventory_table.py` | `user_inventory` |
| 0005 | `0005_create_software_inventory_table.py` | `software_inventory` |
| 0006 | `0006_fleet_management.py` | `policies`, `agent_policy`, `agent_tags`, `agent_status`; seeds Default Windows Policy; backfill |
| 0007 | `0007_session_application_activity.py` | `computer_sessions`, `application_activity` |

GUID type: native UUID on PostgreSQL; `CHAR(36)` on other dialects.

---

## Table: `agents`

**Purpose:** One row per registered Windows agent. Created only by `POST /api/v1/agents/register`.

| Column | Type | Null | Notes |
|--------|------|------|-------|
| `agent_id` | GUID | NO | PK |
| `install_id` | String(256) | NO | |
| `agent_version` | String(64) | NO | |
| `machine_name` | String(256) | NO | |
| `operating_system` | String(256) | NO | |
| `display_name` | String(256) | YES | |
| `created_at` | DateTime(tz) | NO | Agent-reported identity time |
| `registered_at` | DateTime(tz) | NO | Server time |
| `status` | String(32) | NO | Registration status string |

**Indexes:** `ix_agents_install_id`, `ix_agents_machine_name`, `ix_agents_status`, `ix_agents_registered_at`

**Checks:** `install_id` not blank, `machine_name` not blank

**Relationships:** Parent of all other agent-scoped tables (FK `agent_id`).

**Used by:** `AgentRegistrationService`, every upsert that checks `agents.exists`, Fleet joins.

**Read paths:** `AgentRepository.get/exists`; Fleet list join.

**Write paths:** `AgentRepository.add` only (no update API).

**Lifecycle:** Insert on first successful register. Duplicate `agent_id` → 409. No delete API.

---

## Table: `device_inventory`

**Purpose:** Latest hardware/OS snapshot per agent. One row per agent (PK = `agent_id`).

| Column | Type | Null | Default |
|--------|------|------|---------|
| `agent_id` | GUID | NO | PK, FK → `agents` |
| `install_id` | String(256) | NO | |
| `machine_name` | String(256) | NO | |
| `hostname` | String(256) | NO | |
| `operating_system` | String(256) | NO | |
| `os_version` | String(128) | NO | `""` |
| `windows_build` | String(64) | NO | `""` |
| `architecture` | String(64) | NO | `""` |
| `cpu_model` | String(256) | NO | `""` |
| `cpu_cores` | Integer | NO | 0 |
| `memory_bytes` | BigInteger | NO | 0 |
| `disk_total_bytes` | BigInteger | NO | 0 |
| `disk_free_bytes` | BigInteger | NO | 0 |
| `manufacturer` | String(256) | NO | `""` |
| `model` | String(256) | NO | `""` |
| `serial_number` | String(256) | NO | `""` |
| `boot_time` | DateTime(tz) | YES | |
| `current_user` | String(256) | NO | `""` |
| `agent_version` | String(64) | NO | |
| `collected_at` | DateTime(tz) | NO | Agent clock |
| `received_at` | DateTime(tz) | NO | Server clock |

**Indexes:** machine_name, hostname, received_at

**Checks:** non-negative cpu/memory/disk; non-blank install_id/machine_name/hostname

**Used by:** Device upsert, Device query, Fleet projection (`last_inventory_at`), Fleet list columns.

**Read paths:** `GET /api/v1/devices`, `GET /api/v1/fleet`, fleet detail.

**Write paths:** `POST /api/v1/devices` upsert. Also updates `agent_status.last_inventory_at` via `FleetProjection.on_inventory`.

**Lifecycle:** Insert on first inventory (HTTP 201), overwrite on later posts (HTTP 200). Agent currently uploads once at start, not on a timer.

**Frontend:** Fleet uses this via fleet APIs. `GET /api/v1/devices` is **not** called by `app.js`.

---

## Table: `agent_heartbeats`

**Purpose:** Append-only heartbeat history.

| Column | Type | Null | Notes |
|--------|------|------|-------|
| `id` | Integer | NO | PK, autoincrement |
| `agent_id` | GUID | NO | FK → `agents` |
| `heartbeat_time` | DateTime(tz) | NO | Agent `timestamp` |
| `received_at` | DateTime(tz) | NO | Server |
| `status` | String(32) | NO | default `"ok"` |

**Indexes:** `agent_id`, `heartbeat_time`, composite `(agent_id, heartbeat_time)`

**Used by:** `HeartbeatService.record`; presence uses **max(heartbeat_time)** (also copied to `agent_status.last_seen`).

**Read paths:** Device query joins latest heartbeat; Fleet uses `agent_status`.

**Write paths:** `POST /api/v1/agents/{id}/heartbeat` insert + `FleetProjection.on_heartbeat`.

**Lifecycle:** Never updated or deleted in application code.

---

## Table: `user_inventory`

**Purpose:** Local Windows accounts. PK is Windows SID (not per-agent unique across the fleet if the same SID appeared on two machines — the schema is SID-global).

| Column | Type | Null | Default |
|--------|------|------|---------|
| `sid` | String(256) | NO | PK |
| `agent_id` | GUID | NO | FK |
| `username` | String(256) | NO | |
| `display_name` | String(256) | NO | `""` |
| `account_type` | String(64) | NO | `"Local"` |
| `enabled` | Boolean | NO | True |
| `builtin` | Boolean | NO | False |
| `password_required` | Boolean | NO | True |
| `last_logon` | DateTime(tz) | YES | |
| `source` | String(64) | NO | `"Local"` |
| `collected_at` | DateTime(tz) | NO | |
| `received_at` | DateTime(tz) | NO | |

**Indexes:** agent_id, username, account_type, enabled

**Checks:** sid/username not blank

**Used by:** User upsert/list; Fleet Accounts tab.

**Write paths:** `POST /api/v1/users` (`upsert_many`)

**Read paths:** `GET /api/v1/users`, `GET /api/v1/users/{sid}`, `GET /api/v1/fleet/{id}/users`

**Lifecycle:** Upsert by SID. No delete API.

---

## Table: `software_inventory`

**Purpose:** Installed applications. PK `id` is a deterministic hash from the domain (`software_id()`), not an identity column.

| Column | Type | Null | Default |
|--------|------|------|---------|
| `id` | String(64) | NO | PK |
| `agent_id` | GUID | NO | FK |
| `application_name` | String(512) | NO | |
| `version` | String(128) | NO | `""` |
| `publisher` | String(256) | NO | `""` |
| `install_date` | String(32) | NO | `""` |
| `install_location` | String(512) | NO | `""` |
| `estimated_size` | BigInteger | NO | 0 |
| `architecture` | String(16) | NO | `""` |
| `system_component` | Boolean | NO | False |
| `windows_component` | Boolean | NO | False |
| `registry_path` | String(512) | NO | `""` |
| `collected_at` | DateTime(tz) | NO | |
| `received_at` | DateTime(tz) | NO | |

**Indexes:** agent_id, application_name, publisher, architecture

**Checks:** id/application_name not blank; estimated_size ≥ 0

**Write paths:** `POST /api/v1/software`

**Read paths:** `GET /api/v1/software`, `GET /api/v1/software/{id}`, Fleet software tab

**Lifecycle:** Upsert by hashed id. No uninstall-reconciliation delete in application code.

---

## Table: `policies`

**Purpose:** Agent collection policy. Seeded in 0006.

| Column | Type | Notes |
|--------|------|-------|
| `id` | GUID | PK. Default Windows Policy = `11111111-1111-4111-8111-111111111111` |
| `name` | String(256) | Unique `uq_policies_name` |
| `heartbeat_interval_seconds` | Integer | check > 0. Seeded **30** |
| `collect_devices` | Boolean | |
| `collect_users` | Boolean | |
| `collect_software` | Boolean | |
| `collect_processes` | Boolean | Reserved; agent does not read this table |
| `collect_services` | Boolean | Reserved |
| `collect_event_logs` | Boolean | Reserved |
| `created_at` | DateTime(tz) | |

**Checks:** name not blank; heartbeat_interval > 0

**Used by:** Fleet policy tab (read). `FleetProjection.on_registered` assigns the default policy.

**Write paths:** None in the HTTP API. Seed + assignment only.

**Lifecycle:** Static default row. No policy editor.

The **agent does not fetch policies**. Heartbeat interval comes from local YAML, not this table.

---

## Table: `agent_policy`

**Purpose:** Many-to-one assignment of a policy to an agent.

| Column | Type | Notes |
|--------|------|-------|
| `agent_id` | GUID | PK, FK → agents |
| `policy_id` | GUID | FK → policies.id |
| `assigned_at` | DateTime(tz) | |

**Index:** `ix_agent_policy_policy_id`

**Write paths:** `FleetProjection.on_registered`

**Read paths:** Fleet list `policy_name`, `GET /api/v1/fleet/{id}/policy`

---

## Table: `agent_tags`

**Purpose:** Optional tags. Check constraint allows only `Finance`, `HR`, `Production`, `Development`, `Servers`, `Lab`.

| Column | Type | Notes |
|--------|------|-------|
| `agent_id` | GUID | composite PK, FK |
| `tag` | String(64) | composite PK |

**Index:** `ix_agent_tags_tag`

**Write paths:** **None in the HTTP API** (Sprint 13 comment: tags read-only).

**Read paths:** Fleet list `tags` filter/column.

**Lifecycle:** Table exists; unless rows are inserted out of band, tags stay empty.

---

## Table: `agent_status`

**Purpose:** Denormalized last-communication snapshot for Fleet presence.

| Column | Type | Null | Notes |
|--------|------|------|-------|
| `agent_id` | GUID | NO | PK, FK |
| `last_seen` | DateTime(tz) | YES | From heartbeat |
| `last_inventory_at` | DateTime(tz) | YES | From device upsert |
| `last_communication_at` | DateTime(tz) | YES | Last successful ingest |
| `ip_address` | String(64) | YES | Column exists; Fleet `ip_address` is typically null — **no writer sets client IP in application code inspected** |

**Index:** `ix_agent_status_last_seen`

**Write paths:** `FleetProjection.on_registered` / `on_heartbeat` / `on_inventory` (`record_seen`)

**Read paths:** All Fleet GETs; presence computation.

**Lifecycle:** Created at registration; updated on heartbeat and device inventory. Heartbeats for users/software/sessions also go through services that may update `last_communication_at` depending on projection hooks — heartbeat and device inventory are the explicit projection events.

---

## Table: `computer_sessions`

**Purpose:** Windows logon sessions reported by the activity module.

| Column | Type | Notes |
|--------|------|-------|
| `session_id` | GUID | PK (agent-generated UUID v5) |
| `agent_id` | GUID | FK → agents |
| `username` | String(256) | |
| `account_sid` | String(256) | default `""` |
| `logon_at` | DateTime(tz) | |
| `logoff_at` | DateTime(tz) | nullable |
| `status` | String(32) | `'active'` or `'closed'` |
| `collected_at` | DateTime(tz) | |
| `received_at` | DateTime(tz) | |

**Indexes:** `(agent_id, logon_at)`, `(agent_id, status)`

**Checks:** username not blank; status IN (`active`,`closed`)

**Used by:** SessionService, Fleet sessions tab, application_activity FK, offline close.

**Write paths:** `POST /api/v1/sessions` → `upsert_many` ON CONFLICT `session_id` WHERE status != closed. Offline: `close_open_for_agent`.

**Read paths:** `GET /api/v1/sessions`, `GET /api/v1/sessions/{id}`, `GET /api/v1/fleet/{id}/sessions`

**Lifecycle:**

- Insert ACTIVE on first upload of a new `session_id`
- Heartbeat of the same id updates `received_at` / `collected_at` while ACTIVE
- Close sets `logoff_at`, `status=closed`
- Closed rows are **immutable** (SQL WHERE and domain `session_is_immutable`)

There is **no unique constraint** “one ACTIVE per agent_id”. Two ACTIVE rows for one agent are representable and have been observed. `activate_current` would close others in one transaction but is unused by the service.

---

## Table: `application_activity`

**Purpose:** Foreground application segments tied to a session.

| Column | Type | Notes |
|--------|------|-------|
| `id` | String(64) | PK (SHA1 from agent) |
| `session_id` | GUID | FK → `computer_sessions.session_id` |
| `agent_id` | GUID | FK → agents |
| `application_name` | String(512) | |
| `executable_name` | String(512) | default `""` |
| `window_title` | String(512) | default `""` |
| `started_at` | DateTime(tz) | |
| `stopped_at` | DateTime(tz) | nullable |
| `duration_seconds` | Integer | default 0, ≥ 0 |
| `collected_at` | DateTime(tz) | |
| `received_at` | DateTime(tz) | |

**Indexes:** `(session_id, started_at)`, `(agent_id, started_at)`

**Checks:** id/application_name not blank; duration_seconds ≥ 0

**Write paths:** `POST /api/v1/applications` — rejects unknown `session_id` with 422 `unknown_session`

**Read paths:** `GET /api/v1/applications`, `GET /api/v1/applications/{id}`, Fleet applications tab

**Lifecycle:** Upsert by id. Open segments may have null `stopped_at`.

---

## Tables that do not exist

No tables for: processes, OS services, network adapters, browser history, alerts, incidents, groups, operators, API keys, timeline events.

## Memory backend

When `RISKLENS_REPOSITORY_BACKEND=memory`, none of these tables are used. Dicts in `infrastructure/memory/` hold the same records for tests and DB-free runs. Data is lost on process exit.

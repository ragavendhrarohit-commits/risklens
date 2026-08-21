# Session lifecycle investigation — two Active sessions on MSI

**Date:** 2026-08-20.  
**Status:** Investigation only. No source changes.  
**Computer:** MSI (`c2c28db4-9838-45de-85e0-d487bf310091`)  
**Out of scope:** Accounts AC-1, Pipeline, Device.

**Live GET** `GET /api/v1/fleet/c2c28db4-…/sessions` at investigation time:

| Login (UTC) | Login (local, screenshot) | `session_id` | `logoff_at` | `status` | Last `received_at` |
|-------------|---------------------------|--------------|-------------|----------|--------------------|
| 2026-08-17T03:21:54Z | 17 Aug 1:21:54 PM | `db6db62f-2511-533b-8c52-99e712c719c3` | null | active | 2026-08-20T12:58:39Z (still heartbeating) |
| 2026-08-12T02:35:38Z | 12 Aug 12:35:38 PM | `70cb15b6-05d9-5f70-b47a-708b941a396f` | null | active | **2026-08-17T03:17:08Z** (never written again) |

Restarting the agent tonight did **not** create a third session. That is expected: Windows `rohit` is still the 17 Aug logon. The defect is the **12 Aug row still Active**.

---

## Trace

```
Windows logon (WTS)
    ↓
activity collector  (session_id = UUID v5(agentID : wtsSessionID : logonAt))
    ↓
activity.json (one current session locally)
    ↓
POST /api/v1/sessions
    ↓
SessionService.upsert
    → closed items + opened items
    → login/heartbeat: _activate_login → _upsert_one → upsert_many
    ↓
computer_sessions  (PK session_id; ON CONFLICT session_id only)
    ↓
GET /api/v1/fleet/{id}/sessions
    ↓
Fleet Sessions UI
```

---

### Windows login

A session starts at a real Windows logon. `session_id` changes when WTS session id or logon timestamp changes.

The 17 Aug row’s `logon_at` is a new Windows logon (~5 minutes after the 12 Aug row’s last heartbeat). That is a **new** session, not an agent restart.

Agent restart on 20 Aug reused `db6db62f-…` (same WTS logon). Correct.

---

### Agent session collector

**Files:** `agent/internal/modules/activity/state.go` (`step`, lines 195–218), `upload.go` (`cycle` / `applyResult`).

If `activity.json` already holds a **different** open session, `step` emits **closed previous + open new** (`LOGIN_CLOSE_THEN_OPEN` / `commitCloseThenOpen`, `upload.go` 608–623, 642–719).

If local state has **no** current session (`currentID == ""`), the agent logs `OPEN_ONLY_NO_LOCAL_PREVIOUS` (`upload.go` 626–634) and POSTs **only the new active session**. It does not know the 12 Aug `session_id`, so it **cannot** POST a close for it.

That matches this machine:

- 12 Aug session last `received_at` = **03:17:08Z on 17 Aug**
- 17 Aug logon = **03:21:54Z** (~5 minutes later)
- Agent almost certainly had empty local previous (process dead, missing `activity.json`, or never flushed logoff) when the 17 Aug logon was first collected.

Agent restart **must not** `FlushClose` (`upload.go` 459–462). Tonight’s start correctly resumed the 17 Aug session.

---

### POST `/api/v1/sessions`

Body is the collector list. Open-only login = one `status: active` item with a **new** `session_id`. No `sessions[]` entry for 12 Aug.

---

### Backend

**File:** `platform/app/services/session_service.py`

`upsert` (41–110): split closed vs opened. Opened items go through `_activate_login` (267–309).

`_activate_login` detects login vs session heartbeat (`is_login`, lines 288–290), logs `LOGIN DETECTED`, then:

```308:308:platform/app/services/session_service.py
        written = self._upsert_one(command, item, SESSION_STATUS_ACTIVE, received_at)
```

`_upsert_one` (179–265) merges **that** `session_id` and calls `upsert_many([merged])`. It never closes any other row.

`activate_current` on the repository **does** close other ACTIVE rows in one transaction. **`SessionService` never calls it.** Grep: only repository + tests.

---

### Database

**File:** `platform/app/infrastructure/postgres/session_repository.py` `upsert_many` (43–123)

```sql
INSERT INTO computer_sessions (...) VALUES (...)
ON CONFLICT (session_id) DO UPDATE SET ...
WHERE computer_sessions.status != 'closed'
```

Conflict key is **`session_id` only**. A new login **inserts**. The 12 Aug row is a different PK, so it is untouched.

No unique index `UNIQUE (agent_id) WHERE status = 'active'`.

---

### Fleet Sessions UI

**Files:** `fleet_service.py` `sessions` (136–145), `frontend/assets/app.js` `renderFleetSessionsTable`.

GET returns both ACTIVE rows. Duration is computed live from `logon_at` to now when `logoff_at` is null. UI is not inventing a second Active session.

When the agent is **offline**, `FleetService.sessions` / fleet projection call `close_open_for_agent`. That path **refuses** if `len(actives) != 1` (`session_service.py` 136–146). After the 17 Aug insert, two ACTIVE rows already existed, so **offline reconciliation could not close 12 Aug** during the 17–20 Aug outage.

---

## Answers

### 1. Why does the 12 Aug session remain Active?

It was never written as `status=closed` / `logoff_at` set.

Last mutation: heartbeat `received_at=2026-08-17T03:17:08Z`. Every later POST referenced **only** `db6db62f-…`. Merge/upsert by `session_id` ignores other rows. Offline close refused because two ACTIVE rows already existed.

---

### 2. Why wasn’t it closed when the 17 Aug Windows login occurred?

Two layers, one root write-path gap:

**Agent (condition):** With no local previous session, 17 Aug login used `OPEN_ONLY_NO_LOCAL_PREVIOUS`. No close for `70cb15b6-…` in the POST. Expected when the process was not running at the previous logoff.

**Backend (defect):** Product rule: next Windows logon closes a still-ACTIVE previous session. `_activate_login` saw a **new** `session_id` (`_is_login` true) and still only upserted that id. It did not close `70cb15b6-…`.

The 17 Aug login **created** the overlap. Agent restart on 20 Aug did not.

---

### 3. Which component is responsible for closing old sessions?

| Event | Who should close |
|-------|------------------|
| WTS logoff / terminate / CTRL_LOGOFF | Agent `CloseSession` / `FlushClose` — current **local** session only |
| Shutdown / restart | Agent `OnWindowsShutdown` → `FlushClose` — current local session |
| Next Windows logon, previous still ACTIVE in DB | **Backend login path** (and agent if it still has the previous id locally) |
| Agent offline, exactly one ACTIVE | Backend `SessionService.close_open_for_agent` (last resort) |

For MSI 12 Aug → 17 Aug, the **backend login path** is the component that must close the orphan ACTIVE row. The agent cannot name a `session_id` it no longer has.

---

### 4. Is the bug in the agent or the backend?

**Backend.** Production login does not enforce one ACTIVE per agent.

The agent behaving as “open only, no local previous” is a documented limitation, not the reason the 12 Aug row stays Active. The backend is supposed to close leftover ACTIVE rows on a new login. That code exists (`activate_current`) and is **not wired**.

Contributing (not root): missed logoff while the agent was down; offline close refuses once overlap exists.

---

### 5. Under what conditions should a session become Closed?

Intended order (`docs/08_AGENT_SESSION_LIFECYCLE.md`):

1. Real WTS logoff / session terminate  
2. Shutdown / restart flush  
3. Next Windows logon if the previous row is still ACTIVE  
4. Offline `last_seen`, only if there is exactly one ACTIVE session  

Once `status=closed`, logout time and duration are immutable.

A session must **not** close because the RiskLens process restarted.

---

### 6. Is “one Active session per computer” actually enforced in code?

**No, not on the production write path.**

| Mechanism | Used in production? |
|-----------|---------------------|
| Domain comment / tests: `count(active) <= 1` | Intent only |
| `SessionRepository.activate_current` | **No** — tests only |
| `close_active_for_agent` | **No** — unused by `SessionService` |
| `upsert_many` ON CONFLICT `session_id` | Yes — does not close siblings |
| Partial unique index on ACTIVE `agent_id` | **Missing** |
| `MultipleActiveSessionsError` | Unused on login |
| `close_open_for_agent` | Yes, but **refuses** if active count ≠ 1 |

`_activate_login` even logs `current_active_sessions` when more than one already exists and still upserts.

Agent `activity.json` keeps **one** current session locally. That local invariant is not the database invariant.

---

## Exact place the 12 Aug session should have been closed

**On the 17 Aug login POST**, after classifying the incoming row as a login (`incoming.session_id` ≠ current ACTIVE):

**File:** `platform/app/services/session_service.py`  
**Function:** `SessionService._activate_login`  
**Line:** **308**

```267:309:platform/app/services/session_service.py
    def _activate_login(self, command: UpsertSessionsCommand, item, received_at: datetime) -> int:
        ...
        kind = "LOGIN" if is_login else "SESSION_HEARTBEAT"
        ...
        written = self._upsert_one(command, item, SESSION_STATUS_ACTIVE, received_at)
        return written
```

That line should use the already-implemented repository method:

**File:** `platform/app/infrastructure/postgres/session_repository.py`  
**Function:** `PostgresSessionRepository.activate_current`  
**Lines:** **125–231** (close every other ACTIVE for `agent_id` at `incoming.logon_at`, then insert/update the new ACTIVE; abort if more than one ACTIVE remains)

```125:147:platform/app/infrastructure/postgres/session_repository.py
    def activate_current(self, incoming: ComputerSession) -> int:
        ...
            for row in existing_rows:
                if row.session_id == incoming.session_id:
                    continue
                end = incoming.logon_at
                ...
                row.logoff_at = end
                row.status = SESSION_STATUS_CLOSED
```

Heartbeat (`is_login` false) must **not** take this path; only a new `session_id`.

Secondary (cannot repair MSI now that two ACTIVE rows exist):

**File:** `platform/app/services/session_service.py`  
**Function:** `close_open_for_agent`  
**Lines:** **136–146** — refuses when `len(actives) != 1`, so it cannot close 12 Aug after overlap exists.

---

## Root cause

**Login persist is merge-by-`session_id` (`_upsert_one` → `upsert_many`). It does not close other ACTIVE rows for the same `agent_id`.**

`activate_current` implements the product rule and is unused.

On MSI: 12 Aug stayed ACTIVE after a missed logoff; 17 Aug login inserted a second ACTIVE row; offline close then refused; Fleet still shows two Active sessions. Agent restart on 20 Aug is unrelated.

---

## What was not done

- No source changes  
- No session rows closed or deleted  
- No Accounts / Pipeline / Device work  

**Stop.** Waiting for review before any session fix.

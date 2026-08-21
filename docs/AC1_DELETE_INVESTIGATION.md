# AC-1 investigation — deleted local accounts remain in Accounts

**Reproduction:** Windows Local Users and Groups — `CodexSandboxOffline` and `CodexSandboxOnline` deleted. RiskLens Accounts page still shows both after the agent has had time to upload.  
**Guest was not deleted.** Guest still exists in Windows and is expected to remain in Accounts.  
**Date:** 2026-08-20.  
**Status:** Root cause identified. No code was changed.

**Live Windows set after deletion (from this reproduction):**

- Administrator
- ROHIT
- DefaultAccount
- WDAGUtilityAccount
- Guest

Deleted on Windows, still shown in RiskLens:

- CodexSandboxOffline
- CodexSandboxOnline

Guest present on Windows? **YES**  
`CodexSandboxOffline` / `CodexSandboxOnline` present on Windows? **NO**

---

## Verdict

The collector, upload payload, HTTP request, and API ingest all **correctly omit CodexSandboxOffline and CodexSandboxOnline**. Guest is still enumerated because it still exists on Windows.

The two deleted accounts remain because the backend **never deletes** a SID that is absent from a later upload. The previous `CodexSandboxOffline` and `CodexSandboxOnline` rows in `user_inventory` are left untouched. The UI then reads those rows.

**The deletion is lost in the repository write.**

| Item | Value |
|------|--------|
| File | `platform/app/infrastructure/postgres/user_repository.py` |
| Function | `PostgresUserRepository.upsert_many` |
| Lines | 28–60 |
| Why the deleted accounts remain | The method only `INSERT … ON CONFLICT (sid) DO UPDATE` for SIDs **in** the payload. `CodexSandboxOffline` and `CodexSandboxOnline` are not in the payload, so those rows are never written **and never deleted**. |
| Why the logic is this way | The write path was implemented as merge-only inventory (“store/update what the agent sent”). Absence is treated as “no new data”, not “this account is gone”. There is no `DELETE` in the users module. |

Called from:

- `platform/app/services/user_service.py` — `UserInventoryService.upsert` lines 35–63  
  Builds a list **only** from `command.users` and calls `upsert_many`. It never diffs against stored SIDs.

The in-memory twin has the same bug: `platform/app/infrastructure/memory/memory_user_repository.py` `upsert_many` lines 22–26 (`self._rows[account.sid] = account` only).

---

## Where the deleted accounts exist after deletion

```
Windows SAM              CodexSandboxOffline / CodexSandboxOnline?  NO
                         Guest?  YES (still a live local user)
Collector                CodexSandboxOffline / CodexSandboxOnline?  NO
                         Guest?  YES
Upload payload           CodexSandboxOffline / CodexSandboxOnline?  NO
                         Guest?  YES
HTTP POST body           CodexSandboxOffline / CodexSandboxOnline?  NO
                         Guest?  YES
Backend API request      CodexSandboxOffline / CodexSandboxOnline?  NO
                         Guest?  YES
Service command          CodexSandboxOffline / CodexSandboxOnline?  NO
                         Guest?  YES
Repository input list    CodexSandboxOffline / CodexSandboxOnline?  NO
                         Guest?  YES
Repository commit        CodexSandboxOffline / CodexSandboxOnline?  YES  ← rows never deleted
                         Guest?  YES (correct — still posted)
Database                 CodexSandboxOffline / CodexSandboxOnline?  YES
GET /api/v1/users        CodexSandboxOffline / CodexSandboxOnline?  YES
UI                       CodexSandboxOffline / CodexSandboxOnline?  YES
```

The first layer that still has the deleted accounts is **the database after `upsert_many` commits**. Everything upstream of that commit already dropped them. Guest is present at every layer because it was never deleted on Windows.

---

## 1. Windows

**Source:** Windows SAM / Local Users and Groups (not RiskLens code).  
**Function:** n/a  
**Input:** Operator deleted `CodexSandboxOffline` and `CodexSandboxOnline`. Guest was not deleted.  
**Output:** Remaining local users include Administrator, ROHIT, DefaultAccount, WDAGUtilityAccount, Guest.  
**Guest present?** **YES**  
**CodexSandboxOffline present?** **NO**  
**CodexSandboxOnline present?** **NO**

---

## 2. Account collector

**Source file:** `agent/internal/modules/users/collector_windows.go`  
**Function:** `WindowsCollector.Collect` (lines 66–148)

**Input:** Local computer, `NetUserEnum` level 2, `FILTER_NORMAL_ACCOUNT` (lines 78–87).

**Output:** `[]Account` built only from names returned by `NetUserEnum` (loop lines 102–140). After deletion, `NetUserEnum` does not return `CodexSandboxOffline` or `CodexSandboxOnline`. Guest is still returned because it still exists.

**Actual data after the two deletions:**

```
Administrator
ROHIT
DefaultAccount
WDAGUtilityAccount
Guest
```

**Guest present?** **YES**  
**CodexSandboxOffline present?** **NO**  
**CodexSandboxOnline present?** **NO**

The collector is not the bug. It does not hardcode the sandbox accounts. It does not keep a previous snapshot. A deleted SAM user is simply not in the slice. Guest remaining in the slice is correct.

---

## 3. Upload payload

**Source file:** `agent/internal/modules/users/upload.go`  
**Function:** `Module.cycle` (lines 329–408)

**Input:** `accounts, collectErr := m.collector.Collect(ctx)` (line 346).

**Output:** `Inventory` posted as-is (lines 370–376):

```go
req := Inventory{
    AgentID:     id.AgentID,
    InstallID:   id.InstallID,
    CollectedAt: m.now(),
    Users:       accounts,  // collector result, no merge with previous last[]
}
result, err := m.client.Upload(ctx, req)
```

`m.last = accounts` (lines 347–349) is overwritten with the new collect. Previous `CodexSandboxOffline` / `CodexSandboxOnline` entries are discarded in agent memory.

**Actual `users` array after deletion:** the remaining live accounts, including Guest. No `CodexSandboxOffline` or `CodexSandboxOnline` objects.

**Guest present?** **YES**  
**CodexSandboxOffline present?** **NO**  
**CodexSandboxOnline present?** **NO**

If collect returned a non-empty list, the module always POSTs (lines 351–376). After a successful delete on Windows this is a **full live set without the two sandbox accounts**, not a skipped cycle.

---

## 4. HTTP request

**Source file:** `agent/internal/modules/users/upload.go`  
**Function:** `Client.Upload` (lines 122–161)  
**Endpoint:** `POST /api/v1/users` (`EndpointPath`, line 25)

**Input:** `UploadRequest` = `Inventory` (`model.go` lines 21–26). JSON field `users` is the collector slice.

**Output:** HTTP 200 with `accepted: true`, `inventory_status: "stored"`, `stored` = number of rows **upserted** (the remaining live accounts, including Guest). The client does not send deleted SIDs. `SuccessResponse` has no `removed` field.

**Actual POST body (shape):**

```json
{
  "agent_id": "<uuid>",
  "install_id": "<id>",
  "collected_at": "<timestamp>",
  "users": [
    { "username": "Administrator", "sid": "…-500", "account_type": "Administrator", … },
    { "username": "ROHIT", "sid": "…", "account_type": "Local", … },
    { "username": "DefaultAccount", "sid": "…", … },
    { "username": "WDAGUtilityAccount", "sid": "…", … },
    { "username": "Guest", "sid": "…-501", "account_type": "Guest", … }
  ]
}
```

**Guest present?** **YES**  
**CodexSandboxOffline present?** **NO**  
**CodexSandboxOnline present?** **NO**

Transport is a pass-through. It does not re-add the deleted sandbox accounts.

---

## 5. Backend API

**Source file:** `platform/app/api/v1/users.py`  
**Function:** `upload_users` (lines 64–78)

**Input:** `UserInventoryRequest` (`platform/app/schemas/users.py` lines 30–57). `users` is whatever the agent posted. There is no `complete` flag and no deleted-SID field.

**Output:** `payload.to_command()` then `service.upsert(...)`. Response `stored` counts upserted rows only.

**Guest present in the request model?** **YES**  
**CodexSandboxOffline / CodexSandboxOnline present?** **NO**

The API does not invent the deleted accounts. It also does not instruct the service to replace the agent’s account set.

---

## 6. Service

**Source file:** `platform/app/services/user_service.py`  
**Function:** `UserInventoryService.upsert` (lines 35–63)

**Input:** `UpsertUsersCommand.users` = the remaining live accounts, including Guest.

**Output:** A new `list[UserAccount]` built **only** from `command.users` (lines 40–57). Then:

```python
stored = self._users.upsert_many(accounts)
```

There is no load of existing rows for this `agent_id`, no set difference, no delete call.

**Guest present in `accounts` passed to the repository?** **YES**  
**CodexSandboxOffline / CodexSandboxOnline present?** **NO**

The service is part of the defect: it never asks persistence to drop SIDs that disappeared. The deleted rows still exist until the repository write, because the service never issues a delete.

---

## 7. Repository — **defect**

**Source file:** `platform/app/infrastructure/postgres/user_repository.py`  
**Function:** `PostgresUserRepository.upsert_many`  
**Lines:** 28–60

**Input:** Live `UserAccount` values (Administrator, ROHIT, DefaultAccount, WDAGUtilityAccount, Guest). `CodexSandboxOffline` and `CodexSandboxOnline` SIDs are not in this list.

**What the function does:**

1. Line 29–30: if the list is empty, return 0 (not this case).
2. Lines 34–49 (PostgreSQL): for **each posted account**, `INSERT` into `user_inventory` … `ON CONFLICT (sid) DO UPDATE` of every column except `sid`.
3. Lines 51–57 (non-Postgres): get-by-SID, insert or overwrite columns. Same merge behaviour.
4. Line 59: `session.commit()`.
5. Line 60: return `count` = number of posted accounts.

**There is no `DELETE`. There is no `sid NOT IN (posted)` clause. Posted SIDs that already exist are updated. Posted SIDs that are new are inserted. SIDs that exist in the table but are not posted are ignored.**

**Output:** Live rows updated (including Guest). `CodexSandboxOffline` and `CodexSandboxOnline` rows unchanged.

**Guest still exists after this function?** **YES** — correctly; Guest was still posted.  
**CodexSandboxOffline still exists after this function?** **YES** — in the table, as the pre-deletion row.  
**CodexSandboxOnline still exists after this function?** **YES** — in the table, as the pre-deletion row.

**Why the current logic behaves this way:** `upsert_many` was written as “write these accounts by SID primary key”. That is a valid merge. It is not a snapshot replace. The interface documents this explicitly:

`platform/app/repositories/user_repository.py` lines 11–13:

```python
def upsert_many(self, accounts: list[UserAccount]) -> int:
    """Upsert accounts by SID. Returns number of rows written."""
```

The repository never looks at Windows. It only mutates rows it is given. Deleted sandbox accounts are not given, so those rows survive.

---

## 8. Database

**Table:** `user_inventory`  
**Migration:** `platform/app/infrastructure/postgres/migrations/versions/0004_create_user_inventory_table.py`  
**Model:** `UserInventoryRow` in `platform/app/infrastructure/postgres/models.py` (PK `sid`)

**Write paths:** only `upsert_many`. Docs (`docs/06_DATABASE.md`): “Lifecycle: Upsert by SID. No delete API.”

**After the post-deletion upload:**

| username | still a row? | reason |
|----------|--------------|--------|
| Administrator | yes | upserted |
| ROHIT | yes | upserted |
| DefaultAccount | yes | upserted |
| WDAGUtilityAccount | yes | upserted |
| Guest | yes | upserted (still exists on Windows) |
| **CodexSandboxOffline** | **yes** | **not in POST; never deleted** |
| **CodexSandboxOnline** | **yes** | **not in POST; never deleted** |

**Guest present?** **YES** (correct)  
**CodexSandboxOffline present?** **YES** (stale)  
**CodexSandboxOnline present?** **YES** (stale)

`received_at` / `collected_at` on the two sandbox rows stay at the **last upload that still included them**. The remaining live accounts, including Guest, get a newer `received_at`. The UI does not hide stale rows.

---

## 9. Frontend API

**Source file:** `platform/app/api/v1/users.py`  
**Function:** `list_users` (lines 87–113) → `UserInventoryService.list` → `PostgresUserRepository.list` (lines 75–101)

**Input:** `GET /api/v1/users` with optional filters. No “only latest snapshot” filter. No `deleted` column to exclude.

**Output:** Every `user_inventory` row that matches filters, including Guest (live) and the two sandbox accounts (stale).

**Guest present?** **YES**  
**CodexSandboxOffline / CodexSandboxOnline present?** **YES**

---

## 10. UI

**Source file:** `frontend/assets/app.js`  
**Functions:** `loadUsers` (lines 1089–1127), `renderUsersTable` (lines 1047–1075)

**Input:** `GET /api/v1/users` JSON `items`.  
**Output:** One table row per item; username from `item.username` (line 1065). No client-side drop of deleted accounts. No stale-row heuristic.

**Guest present?** **YES** — correct; still on Windows and still returned by the API.  
**CodexSandboxOffline / CodexSandboxOnline present?** **YES** — because the API returned them.

The Accounts page is not caching phantom sandbox users independently of the backend. Refresh still shows them because the database still has them. Guest remaining in the table is not evidence of this bug.

---

## Root cause (stop here)

`CodexSandboxOffline` and `CodexSandboxOnline` are deleted on Windows and omitted from every agent and HTTP artefact. The product then **fails to apply that omission** when writing `user_inventory`.

Exact location:

```28:60:platform/app/infrastructure/postgres/user_repository.py
def upsert_many(self, accounts: list[UserAccount]) -> int:
    ...
    stmt = stmt.on_conflict_do_update(
        index_elements=[UserInventoryRow.sid],
        set_=update_cols,
    )
    session.execute(stmt)
    ...
    session.commit()
    return count
```

Orchestrator that never requests a delete:

```35:63:platform/app/services/user_service.py
def upsert(self, command: UpsertUsersCommand) -> tuple[int, datetime]:
    ...
    accounts = [ UserAccount(...) for item in command.users if ... ]
    stored = self._users.upsert_many(accounts)
```

**Why the deleted accounts remain:** the post-deletion payload does not contain `CodexSandboxOffline` or `CodexSandboxOnline`, and merge-only upsert ignores missing SIDs. The old rows survive `commit()`. GET and the UI display them.

**Why the logic behaves this way:** inventory ingest was implemented as “upsert each posted account by SID”, not “replace this agent’s account set with the posted list”. That is sufficient for create/enable/disable (SID still present). It is insufficient for delete (SID absent). Guest still appearing is expected: its SID is still posted.

This matches the approved AC-1 design (`docs/ACCOUNTS_DELETE_SYNC_PLAN.md`): snapshot replace is missing; this investigation confirms it on a real deletion of `CodexSandboxOffline` and `CodexSandboxOnline`.

**Not the cause:** collector still returning the sandbox accounts, upload skipping the cycle, HTTP re-adding them, frontend hardcoding them, Guest remaining (Guest was not deleted), or a special-case exception in SQL.

---

## What was not done

- No source changes.
- No AC-1 implementation.
- No Pipeline / Device / other-inventory work.
- No change to the root cause or implementation plan.

**Stop.** Waiting for review before a fix.

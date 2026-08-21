# New-account investigation — created local user missing in RiskLens

**Date:** 2026-08-20.  
**Status:** Investigation only. No source changes. Database was not modified.  
**Related:** AC-1 (deleted `CodexSandboxOffline` / `CodexSandboxOnline` still listed). This write-up is about **create**, not delete.

Reproduction machine for Accounts inventory: **LAPTOP-8VQ40K6U**, agent `3e2f1eda-3e1f-4ece-82a8-0197ad6733d7`.  
This Cursor workspace host is **MSI** (agent `c2c28db4-9838-45de-85e0-d487bf310091`, last users upload 2026-08-17). MSI SAM was not used for this reproduction.

The user did not name the new account. Live `GET /api/v1/users` for the laptop has exactly one account that was **not** in the previously documented live Windows set (Administrator, ROHIT, DefaultAccount, WDAGUtilityAccount, Guest):

**`TestUser`** — SID `S-1-5-21-473046328-66222224-1457656888-1010`, `enabled: true`, `account_type: Local`.

---

## 1. Current behaviour

Observed:

1. Deleted accounts `CodexSandboxOffline` and `CodexSandboxOnline` still appear in RiskLens.
2. A newly created Windows local account does not appear in RiskLens.

Live platform read at investigation time (`GET http://127.0.0.1:8000/api/v1/users?limit=200`, 2026-08-20):

| username | machine | `collected_at` (UTC) | Meaning |
|----------|---------|----------------------|---------|
| Administrator | LAPTOP-8VQ40K6U | 12:28:51 | Live; refreshed on last upload |
| ROHIT | LAPTOP-8VQ40K6U | 12:28:51 | Live |
| DefaultAccount | LAPTOP-8VQ40K6U | 12:28:51 | Live |
| Guest | LAPTOP-8VQ40K6U | 12:28:51 | Live |
| WDAGUtilityAccount | LAPTOP-8VQ40K6U | 12:28:51 | Live |
| **TestUser** | LAPTOP-8VQ40K6U | **12:28:51** | **New account is in the backend** |
| CodexSandboxOffline | LAPTOP-8VQ40K6U | **12:00:54** | Stale; not in the 12:28 payload |
| CodexSandboxOnline | LAPTOP-8VQ40K6U | **12:00:54** | Stale; not in the 12:28 payload |

Same rows on `GET /api/v1/fleet/3e2f1eda-3e1f-4ece-82a8-0197ad6733d7/users?limit=200`.  
`GET /api/v1/users?username=TestUser` returns `total: 1`.

**Create ingest succeeded.** `TestUser` was collected, posted, upserted, and is returned by GET.  
**Delete ingest still fails (AC-1).** The 12:28 upload refreshed live SIDs and omitted the sandbox accounts (`collected_at` stayed 12:00:54). Those rows were not deleted.

If the Accounts **UI** still hides `TestUser`, that is not because the repository dropped the insert.

---

## 2. End-to-end trace

Subject account: **TestUser** (only new SID `-1010` on the laptop agent).

### Windows

**Present?** **Yes** (inferred).  
**Evidence:** A new local SAM user received RID **1010** on the same machine SID as ROHIT (`S-1-5-21-473046328-66222224-1457656888-…`). The collector cannot invent that SID. `NetUserEnum` only returns accounts that exist at collect time.  
This host’s `Get-LocalUser` is MSI and does not list `TestUser`; the laptop SAM was not readable from here.  
**Dropped here?** No.

### Collector

**File:** `agent/internal/modules/users/collector_windows.go`  
**Function:** `WindowsCollector.Collect` (lines 66–148)

**Present?** **Yes.**  
**Evidence:** `TestUser` in the later inventory has `account_type: Local`, `builtin: false`, `source: Local`, empty display name — the shape `Collect` emits for a normal local user (lines 129–139). It is in the same `collected_at` batch as Administrator/ROHIT/Guest on that agent. That batch cannot be assembled without the collector returning `TestUser`.  
SID-lookup skip (`continue` at lines 113–116) did **not** fire for this account; a skipped user never gets a SID in the payload or DB.  
**Dropped here?** No.

### POST payload / HTTP request

**File:** `agent/internal/modules/users/upload.go`  
**Functions:** `Module.cycle` (lines 370–376), `Client.Upload` (lines 122–161)  
**Path:** `POST /api/v1/users`

**Present?** **Yes.**  
**Evidence:** All live laptop rows share `collected_at=2026-08-20T12:28:51.645029Z`. `CodexSandbox*` do **not**. The POST at 12:28 contained the live set **including TestUser** and **excluding** the deleted sandbox users. `cycle` assigns `Users: accounts` with no merge from `m.last`.  
Laptop agent log is not on this workstation; the DB timestamps are the payload fingerprint.  
**Dropped here?** No.

### Backend request

**File:** `platform/app/api/v1/users.py`  
**Function:** `upload_users` (lines 64–78)  
**Schema:** `UserInventoryRequest.to_command` (`schemas/users.py` 38–57)

**Present?** **Yes.**  
**Evidence:** A 422 would reject the whole body. A dropped-by-validation account would never appear in GET. `TestUser` is stored with username/SID intact. `extra=forbid` does not strip unknown users from `users[]`.  
**Dropped here?** No.

### Service

**File:** `platform/app/services/user_service.py`  
**Function:** `UserInventoryService.upsert` (lines 35–63)

**Present?** **Yes.**  
**Evidence:** The list comprehension only drops blank SID/username (lines 55–56). `TestUser` / SID `-1010` are non-blank. `upsert_many` was called with this account.  
**Dropped here?** No.

### Repository

**File:** `platform/app/infrastructure/postgres/user_repository.py`  
**Function:** `PostgresUserRepository.upsert_many` (lines 28–60)

**Present?** **Yes.**  
**Evidence:** Merge-only `INSERT … ON CONFLICT (sid) DO UPDATE` **inserts** a SID that did not exist. `TestUser` exists as PK `-1010`. This is the opposite of AC-1 (AC-1 is “missing SID is ignored”). New SIDs are written.  
**Dropped here?** No.

### Database

**Table:** `user_inventory`  
**Present?** **Yes.**  
**Evidence:** Exposed via GET (no direct SQL was run; API is a read of this table). Row:

- `username=TestUser`
- `sid=S-1-5-21-473046328-66222224-1457656888-1010`
- `agent_id=3e2f1eda-3e1f-4ece-82a8-0197ad6733d7`
- `collected_at=2026-08-20T12:28:51.645029Z`
- `received_at=2026-08-20T12:28:51.592163Z`

**Dropped here?** No.

### GET `/api/v1/users`

**File:** `platform/app/api/v1/users.py`  
**Function:** `list_users` (lines 87–113) → `UserInventoryService.list` → `PostgresUserRepository.list`

**Present?** **Yes.**  
**Evidence:** Org list (`limit=200`) includes `TestUser`. Filter `username=TestUser` returns `total: 1`. Fleet list for the laptop agent includes `TestUser`. No list filter excludes `Local` / enabled accounts by default.  
**Dropped here?** No.

### Frontend

**Files:** `frontend/assets/app.js` — `loadUsers` (1089–1127), `renderUsersTable` (1047–1075), `loadFleetTabData` (783–838)

**Present in a fresh GET-backed render?** **Yes** — `renderUsersTable` prints every `payload.items` username (line 1065). Default page size 25, org `total` 13, so `TestUser` fits on page 1. No client filter drops `TestUser`.

**Present on screen if the operator does not refetch?** **No (possible).**

Evidence of stale UI:

| Surface | Behaviour | Can hide TestUser after 12:28 ingest? |
|---------|-----------|----------------------------------------|
| Reports → Account Inventory | `setView("users")` calls `loadUsers()` (lines 91–92). **Refresh** calls `loadUsers()` (line 1151). **No timer.** Leaving the page open does not refetch. | Yes, if the page was left open from before 12:28. |
| Fleet → computer → Accounts | `loadFleetTabData` fetches users **only when `!f.users`** (lines 807–810). Cache is cleared only when **switching computers** (lines 845–847). Revisit the same Accounts tab → **no second GET**. | **Yes.** First open before 12:28 keeps a list without `TestUser` (and still with sandbox rows from GET/AC-1). |

```807:810:frontend/assets/app.js
    if ((tab === "accounts" || tab === "users") && !f.users) {
      f.users = await fetchJson(
        platformUrl(`/api/v1/fleet/${encodeURIComponent(f.selectedId)}/users`, { limit: 200 })
      );
```

Wrong computer: Fleet detail for **MSI** has no `TestUser` (last MSI users upload 2026-08-17). That view cannot show the laptop’s new account.

**Dropped here?** Not in render logic. **Can fail to appear** because of **no refetch** (Fleet cache is the strongest code-level hide).

---

## 3. Exact failure point

**Ingest path (Windows → collector → POST → API → service → repository → database → GET): no failure.**  
`TestUser` is stored and returned. There is no line in `upsert_many` that refuses new SIDs.

**If the new account is still missing in the UI after 12:28:51 UTC, the failure is frontend stale state, not ingest.**

Exact code that prevents a later successful upload from appearing on Fleet Accounts:

- **File:** `frontend/assets/app.js`  
- **Function:** `loadFleetTabData`  
- **Lines:** **807–810** (`&& !f.users`)  
- **Why:** The first Accounts-tab GET is kept for the whole time that computer stays selected. A new account that lands in `user_inventory` afterwards is never requested again.

Exact code that leaves org Account Inventory stale if the view is not re-entered or Refresh is not clicked:

- **File:** `frontend/assets/app.js`  
- **Function:** `loadUsers` / `setView`  
- **Lines:** **91–92** (fetch only when opening the view), **1151** (manual Refresh only). No interval.

**If the missing UI name is not `TestUser`** (a second local user created after `12:28:51`): it is **not** in GET. Then it has not been in a successful POST yet. That wait is `Module.cycle` + `users.interval` (default **5 minutes**) in `upload.go` (`newScheduler`, first tick then ticker). That is delay, not a drop. Collector/repository would include it on the next successful cycle, same as `TestUser`.

---

## 4. Root cause

Two facts, two mechanisms:

1. **Create works.** `upsert_many` inserts new SIDs. Proven by `TestUser` at 12:28:51.
2. **UI can still look empty of the new user** because Fleet Accounts caches the first GET, and neither surface auto-refreshes after an upload.

The 5-minute users interval can also make a brand-new SAM user invisible for up to one cycle; that ended for `TestUser` at 12:28:51.

This is **not** “new accounts cannot be stored”.

---

## 5. Related to AC-1 or a separate bug?

**Separate from AC-1**, observed in the same snapshot.

| | AC-1 (delete) | This investigation (create) |
|--|---------------|-----------------------------|
| Mechanism | Missing SID is never `DELETE`d | New SID **is** `INSERT`ed |
| Proof | Sandbox rows still at `collected_at` 12:00:54 after a 12:28 live upload | `TestUser` row at 12:28:51 |
| File | `postgres/user_repository.py` `upsert_many` 28–60 | Ingest: none. UI hide: `app.js` `loadFleetTabData` 807–810 |
| GET after 12:28 | Still returns sandbox users | Returns `TestUser` |

Seeing both at once is expected: GET returns **union of live upserts and undeleted ghosts**. A cached Fleet tab can add a third effect: **omit later inserts**.

Do not fix create by changing `upsert_many` delete behaviour, and do not treat UI cache as AC-1.

---

## 6. Recommendation (design only — do not implement)

1. **AC-1 remains the delete fix** (authoritative snapshot replace). Unchanged. Needed so sandbox rows leave GET.
2. **Do not change collector or upsert insert path for this create symptom.** `TestUser` shows they already work.
3. **Fleet Accounts:** refetch on every visit to the tab (drop `&& !f.users`), or refetch when the tab is selected. Cache invalidation on computer switch is not enough.
4. **Org Account Inventory:** optional poll or document that Refresh (or re-open the view) is required after an agent cycle.
5. **Operator check:** open Account Inventory, click Refresh, or `GET /api/v1/users?username=TestUser`. If `TestUser` is in JSON, ingest is fine.
6. **If a different new username is missing from GET:** wait one `users.interval` or confirm the laptop agent is running; then re-GET. That is scheduling, not AC-1.

---

## What was not done

- No source changes.
- No database writes.
- No AC-1 implementation.
- No Pipeline / Device / other-inventory work.

**Stop.** Waiting for review.

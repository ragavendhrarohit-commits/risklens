# Accounts inventory — engineering audit

**Feature:** Inventory #2 — Windows local accounts (`users` module).  
**Date:** 2026-08-20.  
**Scope:** complete flow from Windows SAM through UI. No source changes in this task.  
**Out of scope:** Installed Applications, Processes, Services, Network, Policies, Health, Activity.

**Verdict:** Accounts is a working first-generation dump of local SAM users into Postgres and two UI surfaces. It is **not** production-ready as a security inventory feature. Data appearing in the UI does not mean the feature is finished.

Legend used below: **Production Ready** / **Needs Improvement** / **Missing**.

---

## 1. Current architecture

Accounts is **not** on the generic Device Pipeline. It is a standalone module with a private ticker.

| Layer | Package / location | Role |
|-------|--------------------|------|
| Collect | `agent/internal/modules/users/collector_windows.go` | `NetUserEnum` level 2, `FILTER_NORMAL_ACCOUNT` |
| Stub | `collector_stub.go` (`!windows`) | Always errors |
| Schedule + upload | `users/upload.go` | Private `scheduler` + `Client.Upload` |
| Transport | `internal/transport` | `POST /api/v1/users` |
| Ingest | `platform/app/api/v1/users.py` `upload_users` | `UserInventoryService.upsert` |
| Store | `user_inventory` table, PK `sid` | `PostgresUserRepository.upsert_many` |
| Query | `GET /api/v1/users`, `GET /api/v1/users/{sid}`, `GET /api/v1/fleet/{id}/users` | List / get / per-agent |
| UI | `frontend` Account Inventory + Fleet → Accounts | Org-wide list + per-computer tab |

**Not present**

- `pipeline.Collector` / `pipeline.Uploader` adapters
- `fingerprint.Of` / skip-if-unchanged
- `internal/schedule` (copy-pasted ticker in `upload.go`)
- Collection vs upload health split
- Replace-on-snapshot (delete accounts that disappeared)

Module start order (`main.buildModules`): device → registration → deviceupload → heartbeat → **users** → software → activity.

YAML: `users.enabled`, `users.timeout` (≤ 5m), `users.interval` (default **5m**, 1s–1h). `model.Defaults()` sets `users.enabled: false`. Example configs turn it **on**.

---

## 2. Data flow

```
Windows SAM
    ↓
NetUserEnum (USER_INFO_2, FILTER_NORMAL_ACCOUNT)
    ↓
LookupSID(username)     — skip account if this fails
    ↓
[]users.Account
    ↓
every interval (immediate first tick, no jitter)
    ↓
POST /api/v1/users      — always, full list, no fingerprint
    ↓
UserInventoryService.upsert
    ↓
user_inventory upsert by SID only   — no deletes
    ↓
GET /api/v1/users  or  GET /api/v1/fleet/{agent_id}/users
    ↓
Account Inventory page  /  Fleet Accounts tab
```

Each cycle (`Module.cycle`):

1. `collector.Collect`
2. Store `last` in memory
3. Build `Inventory{agent_id, install_id, collected_at: now(), users}`
4. `Client.Upload` — one POST, no retry except the next interval
5. Snapshot state Ready / Partial / Failed (module **Status** stays Running unless identity was missing at Start)

---

## 3. Collector audit

Source: `collector_windows.go`, `model.go`.

### Are all Windows local accounts collected?

**Needs Improvement**

`NetUserEnum` with `FILTER_NORMAL_ACCOUNT` (0x0002) returns SAM **user** accounts (UF_NORMAL_ACCOUNT). Machine/trust accounts are excluded, which is correct for “user accounts”.

Gaps:

| Gap | Why it matters | Severity | Fix |
|-----|----------------|----------|-----|
| `ERROR_MORE_DATA` is accepted but **resume is not looped** | If the API pages, remaining accounts are dropped with **no error**. `MAX_PREFERRED_LENGTH` usually returns one buffer; the footgun remains. | Medium | Loop on `resumeHandle` until status is `NERR_Success`. |
| SID lookup failure **drops the account** (`continue`) | Operator never sees the user. Partial error is logged only if some accounts succeed. | Medium | Include the row without SID, or retry `NetUserGetInfo`; never silent-drop. |
| Domain / Azure AD / Microsoft accounts cached on the box | Product copy says “Windows local account inventory”. Domain users who log on are invisible. | High (if Accounts means “who can use this PC”) / Low (if scope is SAM-only) | Document SAM-only as a hard product rule, **or** add `NetUserEnum`/LSA/AAD collectors later. |
| Local groups are not collected | Cannot tell that `alice` is in Administrators. | High | Collect group membership (or at least Administrators) as metadata. |

Non-Windows: stub returns an error and empty slice. **Production Ready** for “Windows agent only”.

### Is the collection method reliable?

**Needs Improvement**

- `NetUserEnum` level 2 is the right family of API for local SAM users when the agent runs as Local System.
- No dedicated Windows collector tests (`users_test.go` only stubs Collect). Classification, builtin RID, enabled flags, and last-logon conversion are untested on CI unless someone runs Windows build tags.
- `ctx` is honoured between accounts. **Production Ready** for cancellation.

### Are disabled accounts correct?

**Production Ready** (for the UF_ACCOUNTDISABLE bit)

`enabled := flags&UF_ACCOUNTDISABLE == 0` matches the SAM disable flag.

Not collected: **locked out** (`UF_LOCKOUT`), **expired**, **password expired**. A locked account still shows Enabled=true. **Needs Improvement** — Medium. Add lockout/expiry flags.

### Is Last Logon accurate?

**Needs Improvement**

`usri2_last_logon` is converted with `time.Unix(int64(raw.LastLogon), 0)` (seconds since 1970 UTC). That matches the NetUserEnum contract. `0` is omitted (`nil`) = never logged on.

Caveats:

- Not all logon types update this field (some network / service / RunAs logons).
- 32-bit DWORD; not the AD `lastLogonTimestamp`.
- Changing last logon currently forces a full POST every interval **anyway** (no fingerprint). If Accounts later joins the pipeline, last logon **must** be excluded from comparable payload or every interactive logon will upload.

Severity: Medium for “last interactive SAM logon”; High if UI implies “last seen on the network” (see frontend).

### Is Display Name correct?

**Needs Improvement**

Taken from `usri2_full_name`. Empty is common; the UI then shows an em dash, not username. **Low–Medium.** Fallback: display name empty → username.

### Are built-in accounts handled correctly?

**Needs Improvement**

`builtin` is **only** RID **500** (Administrator) and **501** (Guest).

Not flagged: DefaultAccount (503), WDAGUtilityAccount, and other vendor/system local users.

`account_type`:

- RID 500 → `"Administrator"` (the built-in Administrator **account**, not “member of Administrators”)
- RID 501 → `"Guest"`
- else UF_NORMAL_ACCOUNT → `"Local"`

A newly created local user who is in the Administrators group is typed **Local**. That will confuse operators. **High.**

### Missing account metadata

**Missing** (product backlog, not bugs in the current schema)

Password last set / age, expires, locked out, comment, home directory, account expires, bad password count, local group membership, privilege level, whether the account is the current interactive user.

`install_id` is sent on POST and **not stored**.

---

## 4. Pipeline audit

Accounts **does not use** `internal/pipeline` or `internal/fingerprint`.

| Checklist item | Status | Notes |
|----------------|--------|--------|
| Fingerprint correct | **Missing** | No comparable payload, no `Of`, no skip. |
| Timestamp-only changes skip upload | **Missing** | `collected_at` is `time.Now()` every cycle. Last logon changes would also bust a future fingerprint unless excluded in the kind module. |
| New account triggers upload | **Production Ready** (accidentally) | Next 5m tick always POSTs the full list. |
| Deleted account triggers upload | **Needs Improvement** | POST happens; **backend does not delete** the old SID (see §6). UI still shows the deleted user. |
| Enable/disable triggers upload | **Production Ready** (accidentally) | Full list POST includes `enabled`. |

Private scheduler (`upload.go`): immediate first `run`, then `time.Ticker`, `TryLock` skip if overlapping. **Production Ready** as a ticker. **Needs Improvement** vs Device: duplicated scheduler, no jitter, not `internal/schedule`.

Upload on collect error: empty + error → no POST. Partial list + error → POST anyway. **Production Ready** for “send what we have”.

Retry: next interval only. No in-memory pending fingerprint, no disk queue. Same as Device before OV-2.1. **Needs Improvement** — Medium.

Error handling: HTTP failure sets diagnostic `StateFailed` but **does not** `MarkFailed`. Process stays up. **Production Ready** for host isolation. Health stays Healthy (`MarkRunning` never updated). **Needs Improvement** vs Device collection/upload health split.

Uploader (`Client.Upload`): POST JSON, accept 200/201/202 with `accepted` + `inventory_status=stored`, map 404/422/5xx. Shared transport. **Production Ready** as an HTTP adapter. Always called even when the list is unchanged. **Missing** skip-if-unchanged.

---

## 5. Backend audit

| Item | Status | Detail |
|------|--------|--------|
| Ingest API | **Production Ready** | `POST /api/v1/users`, unknown agent → 404, schema → 422, `extra=forbid`. |
| Validation | **Needs Improvement** | Username/SID length 1–256; `account_type` is a free string (no enum). No SID format check. `install_id` accepted and ignored. |
| Query API | **Production Ready** | List filters: username (ilike), account_type (ilike), enabled. Sort allowlist. Limit 1–200. |
| `GET /api/v1/users/{sid}` | **Needs Improvement** | Implemented; **SPA never calls it**. SID in the path is fine (`S-1-5-…`). |
| Fleet API | **Production Ready** | `GET /api/v1/fleet/{id}/users` filters by `agent_id` after `require_agent`. |
| Docs vs code | **Needs Improvement** | `docs/07_API_REFERENCE.md` says GET default sort `-received_at`. Code default is `username`. |

Upsert behaviour: insert or update **each posted SID**. Does **not** replace the agent’s account set. Duplicate SIDs in one POST overwrite in loop order. **Needs Improvement** — see database.

No ingest authentication beyond “agent_id exists”. Platform-wide, not Accounts-only. **Needs Improvement** — High for any internet-exposed API.

---

## 6. Database audit

Table `user_inventory` (migration `0004_create_user_inventory_table.py`).

| Topic | Status | Why it is a problem | Severity | Fix |
|-------|--------|---------------------|----------|-----|
| Primary key is `sid` only | **Needs Improvement** | Two agents cannot own the same SID. Machine-local SIDs are usually unique, but cloned machines (same machine SID) or any well-known SID would **overwrite another computer’s row**, including `agent_id`. | High | PK `(agent_id, sid)`. |
| No delete on snapshot | **Missing** | Deleted local users remain forever. Enable/disable is updated; **removal is not**. Operators will see ghost accounts. | **Critical** | Replace snapshot: upsert posted SIDs, delete rows for that `agent_id` whose SID is absent. |
| Duplicate rows for one SID | **Production Ready** | PK prevents two rows with the same SID. | | |
| Same username, different SID | **Production Ready** | Allowed (rename vs new account). | | |
| Indexes | **Production Ready** | `agent_id`, `username`, `account_type`, `enabled`. | | |
| FK to `agents.agent_id` | **Production Ready** | | | |
| `upsert_many` is per-row | **Needs Improvement** | One statement per account; fine for tens of users, weak for hundreds. | Low | Bulk insert + one delete. |

`install_id` is not a column. `collected_at` is agent clock; `received_at` is server clock.

---

## 7. Frontend audit

Two surfaces.

### Fleet → Computer → Accounts

**Needs Improvement**

- Loading/error: shared `extraLoading` / `extraError`. **Production Ready** at tab level.
- Empty: “No accounts reported for this agent.” **Production Ready**.
- Columns: username, display name, type, enabled, last logon. SID / builtin / password-required omitted (acceptable for a summary tab).
- Fetch `limit=200` (`MAX_LIMIT`). Footer “N of total”. If `total > 200`, extra accounts are **invisible** with no pager. **Medium.**
- Cached in `f.users` until agent switch — no pull-to-refresh on the tab. **Low** (OV-1.2).
- `display_name` empty → placeholder, not username. **Low.**

### Reports → Account Inventory

**Needs Improvement**

| Item | Status | Notes |
|------|--------|--------|
| Loading / empty / error | **Production Ready** | Status line + table body + pagination disable while loading. |
| Pagination | **Production Ready** | Page size 25, prev/next. |
| Sort | **Production Ready** | username, machine_name, enabled, last_logon (toggle desc). |
| Filters | **Production Ready** | username, account_type, enabled. |
| Account type column | **Missing** | Filter exists; table has **no** account type column. |
| Current User / Department / Group | **Missing** | Always `"—"`. Documented as placeholders in `09_INVENTORY.md`. Still shipped as if they were real columns. **High** UX honesty. |
| Last logon header | **Needs Improvement** | Labelled **“Last Seen”** — operators will confuse this with heartbeat/fleet online. **High.** |
| Display name | **Missing** from this table | Org view has no display-name column. |
| Row click / SID detail | **Missing** | `data-sid` is unused; `GET /users/{sid}` unused. |
| Responsive | **Production Ready** | `overflow-x-auto` on the panel. |
| Auto refresh | **Missing** | Manual Refresh only. |

---

## 8. Performance audit

| Path | Status | Notes |
|------|--------|--------|
| Collect | **Production Ready** for typical PCs | Tens of SAM users; NetUserEnum is cheap vs software. |
| Always-on POST every 5m | **Needs Improvement** | Unchanged lists still upload. Fleet-wide chatter. Last logon churn would keep this true even after fingerprint unless excluded. **Medium** at current scale, **High** at large fleet. |
| DB upsert | **Needs Improvement** | N statements per agent per tick; no snapshot delete pass yet (delete pass will add work — do it in one SQL). |
| Org list | **Production Ready** | Indexed filters, limit 25. |
| Fleet tab | **Needs Improvement** | Always asks for 200 rows; no virtualisation (unnecessary at 200). |
| Frontend | **Production Ready** | No huge client-side sort of the full fleet. |

---

## 9. Security considerations

| Topic | Status | Why | Severity | Fix |
|-------|--------|-----|----------|-----|
| Password material | **Production Ready** | `USER_INFO_2` has a password pointer; collector never reads it. | | Keep it that way. |
| Privileged enumeration | **Production Ready** | Agent as SYSTEM can call `NetUserEnum`. | | |
| Ingest auth | **Needs Improvement** | Any client who knows a registered `agent_id` can overwrite that agent’s user rows (and, with SID PK, potentially another agent’s row). | High | Agent attestation / HMAC, and composite PK. |
| PII | **Needs Improvement** | Usernames, display names, last logon stored in clear text. No retention/delete policy for departed staff. | Medium | Retention + snapshot delete (also the ghost-account fix). |
| Builtin admin visibility | **Needs Improvement** | Built-in Administrator is collected (good). Privilege of other local admins is **not**. | High | Group membership. |

---

## 10. Production readiness checklist

| # | Item | Rating |
|---|------|--------|
| 1 | Windows local SAM users collected via NetUserEnum | **Production Ready** (happy path) |
| 2 | Disabled flag from UF_ACCOUNTDISABLE | **Production Ready** |
| 3 | Recurring collect + POST (5m, immediate first tick, no overlap) | **Production Ready** as a loop |
| 4 | Transport + ingest contract (`accepted` / `stored`) | **Production Ready** |
| 5 | Unknown agent rejected | **Production Ready** |
| 6 | Org-wide list with filters, sort, paging | **Production Ready** (for columns that are real) |
| 7 | Fleet per-agent Accounts tab | **Production Ready** (≤ 200 accounts) |
| 8 | Generic Pipeline + fingerprint skip | **Missing** |
| 9 | Upload only when the account **set** changes | **Missing** |
| 10 | Deleted accounts removed from DB/UI | **Missing** |
| 11 | PK scoped to agent | **Needs Improvement** |
| 12 | Account type = privilege / group, not only RID 500/501 | **Needs Improvement** |
| 13 | Builtin set complete | **Needs Improvement** |
| 14 | Last logon labelled honestly | **Needs Improvement** |
| 15 | Placeholder UI columns removed or implemented | **Missing** |
| 16 | Windows collector unit tests | **Missing** |
| 17 | Collection vs upload health | **Missing** |
| 18 | Domain / AAD accounts | **Missing** (scope decision) |
| 19 | Ingest authentication | **Needs Improvement** |
| 20 | NetUserEnum paging + SID-lookup drops | **Needs Improvement** |

**Overall:** not production-ready for a security product’s Accounts inventory. Acceptable only as an internal preview of “local SAM users we have seen at least once”.

---

## 11. Prioritized improvements

Do not implement in this audit. Suggested order after review:

1. **Critical — Snapshot replace**  
   On each successful POST, replace that `agent_id`’s `user_inventory` rows with the posted SID set (upsert + delete missing). Without this, deleted accounts never leave the product.

2. **High — Composite identity**  
   Primary key `(agent_id, sid)`. Update GET-by-SID (or require agent_id). Stops cloned-SID overwrites.

3. **High — Pipeline migration (Accounts only)**  
   Same pattern as Device: comparable payload (sort by SID; exclude `last_logon` and envelope `collected_at`), `Pipeline.Start`, skip unchanged, retry pending, collection health ≠ upload health. Private ticker in `upload.go` goes away.

4. **High — Honest account semantics**  
   - `account_type` / admin: local Administrators group (or a separate `privileged` flag), not only RID 500.  
   - `builtin`: well-known RIDs beyond 500/501.  
   - UI: rename “Last Seen” → “Last logon”; remove or label placeholder Department/Group/Current User columns; show account type on the org table.

5. **Medium — Collector robustness**  
   Page `NetUserEnum`; do not drop users on SID lookup failure; Windows tests for flags, RID classification, last-logon zero.

6. **Medium — Fleet completeness**  
   Paginate Fleet Accounts (or raise and document the 200 cap). Fallback display name → username.

7. **Medium — Ingest auth**  
   Shared with Device; not Accounts-specific, but Accounts writes PII.

8. **Low — Extra SAM fields**  
   Lockout, password required already present; add lockout/expiry/comment when the schema is versioned.

9. **Later — Domain / AAD**  
   Only after SAM local accounts are correct. Do not pretend `source: Local` is “all accounts on the computer”.

---

## Appendix — files reviewed

- `agent/internal/modules/users/*`
- `agent/cmd/risklens-agent/main.go` (module wiring)
- `platform/app/api/v1/users.py`, `api/v1/fleet.py` (users subroute)
- `platform/app/schemas/users.py`, `services/user_service.py`, `services/fleet_service.py`
- `platform/app/infrastructure/postgres/user_repository.py`, `models.py` `UserInventoryRow`
- `platform/app/infrastructure/postgres/migrations/versions/0004_create_user_inventory_table.py`
- `platform/app/infrastructure/memory/memory_user_repository.py`
- `platform/app/domain/user/entity.py`
- `platform/tests/test_users.py`
- `frontend/assets/app.js`, `frontend/index.html`
- `docs/09_INVENTORY.md`, `docs/07_API_REFERENCE.md`, `docs/03_AGENT_ARCHITECTURE.md`

**Stop.** Waiting for review before any Accounts implementation work.

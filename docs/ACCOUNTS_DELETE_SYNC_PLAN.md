# AC-1 — Deleted Account Synchronization

**Feature:** Inventory #2 — Accounts  
**Task:** Design a safe way to remove Windows local accounts from backend inventory after they are deleted on the endpoint.  
**Date:** 2026-08-20.  
**Status:** Design only. Do not implement in this task.

**In scope:** Accounts ingest write path (`POST /api/v1/users` → `UserInventoryService` → `user_inventory`).  
**Out of scope:** Pipeline, Device, Installed Applications, Processes, Services, Network, Policies, Health, Activity, composite primary key, fingerprint / skip-if-unchanged, collector paging, account-type semantics, frontend placeholders.

---

## 1. Current behaviour

The agent already treats each upload as a **full list of local SAM users currently visible** on that computer.

Every `users` interval (default 5 minutes):

1. `WindowsCollector.Collect` enumerates `NetUserEnum` level 2 (`FILTER_NORMAL_ACCOUNT`).
2. The module POSTs `Inventory{agent_id, install_id, collected_at, users}` to `POST /api/v1/users`.
3. The platform validates the agent exists, maps the body to `UserAccount` rows, and calls `UserRepository.upsert_many`.
4. Each posted SID is inserted or updated. Nothing else happens.

There is no Accounts delete API. `GET /api/v1/users` and `GET /api/v1/fleet/{id}/users` read whatever rows remain in `user_inventory`.

Create, enable, and disable are visible after the next successful POST because those SIDs are still in the payload. **Delete is not.** The deleted SID is simply omitted from the next body, and the previous row stays.

---

## 2. Why deleted accounts remain

The write path is **merge-only**.

| Layer | What it does today | What it does not do |
|-------|--------------------|---------------------|
| Collector | Returns the current SAM set (minus SID-lookup failures) | Does not emit tombstones or a deleted-SID list |
| Agent upload | POSTs the current `users` array | Does not tell the server “this is a replace” |
| `UserInventoryService.upsert` | Writes posted accounts | Does not compute the missing set |
| `PostgresUserRepository.upsert_many` | `INSERT … ON CONFLICT (sid) DO UPDATE` | No `DELETE` |
| `MemoryUserRepository.upsert_many` | Overwrites `dict[sid]` | Never removes keys |
| Schema | PK `sid`, index on `agent_id` | No `deleted_at`, no snapshot generation |

So a row is created the first time a SID is seen and is only ever updated. Absence from a later payload is treated as “no new information”, not “this account no longer exists”.

This is the same class of bug as software uninstall, but **this task must not change software**.

Two collector behaviours make a naive “delete everything not in this POST” unsafe:

1. **Partial collect still uploads.** If `Collect` returns some accounts plus an error (SID lookup failure, cancelled context mid-loop), `Module.cycle` still POSTs the partial list (`StatePartial`).
2. **`ERROR_MORE_DATA` is not resumed.** Remaining SAM users can be omitted with no hard failure. That partial list can still be uploaded.

If the backend treated every POST as an authoritative replace, those omitted SIDs would look deleted even though they still exist on Windows.

---

## 3. Target behaviour

After the next **successful, complete** upload from an agent:

- Every SID in that payload is stored (insert or update) for that `agent_id`.
- Every `user_inventory` row for **that same `agent_id`** whose SID is **not** in the payload is **hard-deleted**.
- Other agents’ rows are untouched.
- A failed upload changes nothing (no upsert, no delete).
- A partial / incomplete snapshot updates posted SIDs only; it **must not** delete missing SIDs.
- Org Account Inventory and Fleet Accounts stop showing the deleted user without a frontend change (they already render whatever GET returns).
- `GET /api/v1/users/{sid}` returns 404 once that SID’s row is gone.

Latency: up to one collect interval (default 5 minutes) after the Windows account is deleted, plus upload time. That matches “during the next successful upload”.

This task does **not** add audit history, undelete, or a public DELETE endpoint.

---

## 4. Synchronization strategies

### Strategy A — Authoritative snapshot replace (server-side set sync)

The POST body is the live set. In one database transaction, for `agent_id`:

1. Upsert all posted accounts.
2. `DELETE FROM user_inventory WHERE agent_id = :agent_id AND sid NOT IN (:posted_sids)`.

If the posted SID list is empty, step 2 deletes every row for that agent.

Requires a **completeness gate** so partial payloads do not run step 2 (see §6).

### Strategy B — Agent-supplied tombstone list

The agent remembers the previous SID set, diffs against the new collect, and POSTs `deleted_sids: [...]`. The backend deletes only those SIDs (still scoped by `agent_id`).

The live `users` array is still upserted as today.

### Strategy C — Soft delete / tombstone column

Add `deleted_at` (nullable). Missing SIDs are stamped, not removed. GET filters `deleted_at IS NULL`. Reappearance on a later snapshot clears `deleted_at`.

### Strategy D — Stale TTL purge

A sweeper deletes rows whose `received_at` / `collected_at` is older than N intervals (or N hours).

### Strategy E — Client-only: skip upload unless the set changed, send deletes out of band

Not a complete strategy. The agent has no durable last-SID set today, and this task must not introduce Pipeline / fingerprint work. Without a backend replace, skipped uploads would **delay** delete forever if the agent never POSTs a reduced list.

---

## 5. Advantages and disadvantages

### Strategy A — Snapshot replace

**Advantages**

- Matches the payload the agent already sends (full current list).
- Agent does not need durable previous-SID state. Reinstall, crash, or a new agent still converges on the next complete POST.
- One transaction: the stored set equals the posted set for that agent.
- Smallest durable contract: no new table, no tombstone lifecycle.
- Fleet and org UIs start showing the truth as soon as GET stops returning the row.

**Disadvantages**

- A partial payload must not be treated as complete, or live accounts disappear.
- An empty **complete** payload wipes that agent’s inventory (must be an explicit, tested decision).
- Concurrent POSTs for the same `agent_id` become last-complete-snapshot-wins (the current concurrent test posts disjoint one-SID bodies and would no longer accumulate 20 rows).
- Hard delete is irreversible without backups.
- Does not fix SID-global PK collisions (out of scope). If two agents share a SID, only one row exists; replace is still `WHERE agent_id = :id`.

### Strategy B — Deleted SID list

**Advantages**

- Never deletes a SID the agent did not name.
- Partial collects can still upsert without implying “everyone else is gone”.

**Disadvantages**

- Agent must persist the last successfully uploaded SID set. Memory-only state is lost on restart; the first collect after restart cannot know what to delete.
- After agent reimage / `install_id` change, ghosts remain unless a fallback replace exists — which is Strategy A anyway.
- API grows a second write semantic (`users` plus `deleted_sids`) that can disagree (SID both present and deleted).
- More code on the agent, which this task should keep as thin as possible.

### Strategy C — Soft delete

**Advantages**

- History of “this SID used to exist”.
- Accidental replace is reversible by clearing `deleted_at`.

**Disadvantages**

- Schema migration, GET/list/filter/sort/Fleet changes, unique SID still occupied while “deleted”.
- Target is **remove from inventory**, not hide it. Soft-deleted rows still occupy the PK and still appear if a query forgets the filter.
- Larger change than AC-1 needs.

### Strategy D — TTL

**Advantages**

- No agent contract change.

**Disadvantages**

- Offline or paused agents would lose **all** accounts, which is not a Windows delete.
- Delete is delayed by TTL, not “next successful upload”.
- A partial upload that refreshes only some rows would TTL-delete the rest.
- Unacceptable for a security inventory.

---

## 6. Recommended strategy

**Strategy A, with an explicit completeness gate.**

### Contract

`POST /api/v1/users` remains the only write.

| Payload | Upsert posted SIDs | Delete SIDs missing for this `agent_id` |
|---------|--------------------|-----------------------------------------|
| Complete snapshot (`complete: true`) | Yes | Yes |
| Incomplete snapshot (`complete: false`) | Yes | **No** |
| `complete` omitted (old agents) | Yes | **No** |

Omitted `complete` defaults to **false**. Mixed-version fleets keep today’s merge behaviour until the agent is upgraded. AC-1 takes effect only when the new agent sends `complete: true`.

The agent sets `complete: true` only when `Collect` returns **no error**. Partial collect (`collectErr != nil` with a non-empty list) still POSTs so remaining accounts can update, but with `complete: false`.

Empty `users` plus `complete: true` **is** a wipe of that agent’s rows. That is the correct meaning of “the computer currently has no local users”. Log a warning (`stored=0`, `removed=N`). Windows almost always has at least built-in accounts; a wipe is rare and therefore loud, not forbidden.

Empty `users` plus `complete: false` (or omitted) upserts nothing and deletes nothing. That covers a client that posts `[]` without claiming a full snapshot.

### Why not skip the POST on partial collect instead of a flag?

Skipping upload would also prevent incorrect deletes, with no schema field. It would also delay enable/disable and new-account visibility until a clean collect. The flag keeps merge updates while making replace opt-in and explicit. The field is one boolean on an existing body.

### Why not default `complete` to true?

Current agents already POST partial lists. A default of true would delete live accounts for any unupgraded or partial-uploading agent. Default false is the safe rollout.

### Concurrency

The agent scheduler already `TryLock`s so one cycle runs at a time. Two overlapping complete POSTs for the same `agent_id` (tests, retries, two processes) must still be transactional: the later commit is the live set. That is correct snapshot semantics.

---

## 7. Database impact

**No migration is required** for AC-1. `user_inventory` already has `ix_user_inventory_agent_id`. Hard delete uses the existing table.

### Write algorithm (one transaction)

Inputs: `agent_id`, `accounts` (already filtered to non-empty SID + username), `complete`.

1. If `accounts` is non-empty: existing per-row upsert by `sid` (same conflict target as today).
2. If `complete` is true:
   - If posted SIDs is non-empty:  
     `DELETE FROM user_inventory WHERE agent_id = :agent_id AND sid NOT IN (:sids)`
   - If posted SIDs is empty:  
     `DELETE FROM user_inventory WHERE agent_id = :agent_id`
3. Commit. Return `(stored, removed)`.

If any statement fails, roll back the whole snapshot.

Do **not** delete by SID alone. Always constrain `agent_id`. A SID-only delete would be wrong the moment a future task introduces a composite key, and is already the wrong predicate for “this computer’s inventory”.

### SQLite / non-Postgres (tests)

The repository already has a non-Postgres upsert branch. The delete step must run there too so unit tests and Postgres behaviour match.

### Memory repository

`MemoryUserRepository` must drop keys whose `agent_id` matches and whose SID is not in the complete posted set. Today it never deletes; tests that assume merge-only accumulation must be updated (see §12).

### Composite PK (explicitly not this task)

PK remains `sid`. Snapshot replace still filters by `agent_id`. Known leftover: cloned-machine SID collisions can leave a row owned by another agent; AC-1 will not remove that row when the original agent reports the SID gone, because `agent_id` no longer matches. Document only; do not change the PK here.

### Indexes / volume

Typical SAM size is tens of rows. One indexed `DELETE … agent_id = ?` per successful complete upload is acceptable. Bulk upsert can wait; it is not required to make delete correct.

---

## 8. API impact

### Request (`POST /api/v1/users`)

Add one optional field:

```json
{
  "agent_id": "…",
  "install_id": "…",
  "collected_at": "…",
  "complete": true,
  "users": [ { "username": "…", "sid": "…", "…" : "…" } ]
}
```

- Name: `complete` (boolean).
- Required: no.
- Default: `false`.
- `extra=forbid` stays; old bodies without the field still validate.
- Domain: add `complete: bool` to `UpsertUsersCommand` (default false).

No new routes. No DELETE verb. GET list / GET-by-SID / Fleet users stay unchanged.

### Response

Keep `accepted`, `stored`, `inventory_status=stored`. Add:

- `removed: int` — rows deleted in this request (`0` when `complete` is false).

Existing agents ignore unknown JSON fields if they decode strictly… the Go `SuccessResponse` uses explicit fields and **ignores extra JSON** via encoding/json. Adding `removed` does not break the current client. Optionally plumb `removed` into logs later; not required for correctness.

### Validation

- `complete: true` with a large `users` array: no new limits.
- Duplicate SIDs in one body: last upsert wins; the SID is in the keep-set once. Acceptable.
- Accounts dropped by the service (blank SID/username after strip) are not in the keep-set. They were never stored; they will not cause extra deletes of *other* SIDs. If a previously stored SID is posted with a blank username and stripped, that SID is absent from the keep-set and **will be deleted** on a complete snapshot. That is correct: the service already refuses to store it.

---

## 9. Collector impact

**Do not change enumeration** in this task (`NetUserEnum`, paging, SID lookup, builtin RIDs). Those are separate audit items.

**Do change the upload envelope** produced after collect:

| Collect result | Upload? | `complete` |
|----------------|---------|------------|
| Accounts, no error | Yes | `true` |
| Accounts, error (partial) | Yes (unchanged) | `false` |
| Empty, error | No (unchanged) | — |
| Empty, no error | Yes | `true` (wipe) |

`Inventory` / `UploadRequest` gains `Complete bool \`json:"complete"\``.

Collector tests that only stub `Collect` stay valid. Add module-level tests that a partial collect POSTs `complete: false` and a clean collect POSTs `complete: true`.

Windows collector paging / SID-drop bugs remain. The completeness gate is what stops them from deleting inventory. Fixing those bugs is **not** AC-1; it only makes `complete: true` more common.

---

## 10. Backend impact

| Component | Change |
|-----------|--------|
| `UserInventoryRequest` | Optional `complete: bool = False` |
| `UpsertUsersCommand` | `complete: bool` |
| `UserInventoryResponse` | `removed: int` |
| `UserInventoryService.upsert` | Pass `complete` into the repository; log `stored` and `removed` |
| `UserRepository` | Extend write API, e.g. `replace_snapshot(agent_id, accounts, complete) -> (stored, removed)` or give `upsert_many` a `complete` + `agent_id` argument. Prefer an explicit method so merge-only callers cannot delete by accident. |
| `PostgresUserRepository` | Same upsert as today, then conditional `DELETE` in the **same** `session.commit()` |
| `MemoryUserRepository` | Same semantics |
| `upload_users` | Return `removed` |
| Tests | See §12 |

Do not change Fleet GET, list filters, or frontend. They consume the table.

Do not change Device, Pipeline, software ingest, or other inventory tables.

Service still rejects unknown `agent_id` before any write.

---

## 11. Rollback strategy

AC-1 is reversible without a down-migration because the schema does not change.

**Code rollback**

1. Stop sending `complete: true` (agent) and/or ignore `complete` (platform) so writes become merge-only again.
2. Redeploy previous platform + agent pair if needed.

**Effect of rollback**

- Deletes **stop**. Ghosts can accumulate again.
- Rows already hard-deleted **stay gone**. Rollback cannot resurrect them. Restore from database backup if that is required.
- Old agents that never sent `complete` are unaffected either way.

**Feature flag (optional, not required)**

A platform setting `users_snapshot_replace: bool` (default true once shipped) that forces `complete` to be treated as false. Useful if a bad agent build claims complete snapshots. Prefer shipping the default-false request field first; a flag is extra insurance, not a prerequisite.

**Operational note**

If a complete empty snapshot is posted by mistake, that agent’s accounts disappear immediately. Recovery is the next complete non-empty POST from that agent (re-upsert). There is no undelete of SIDs the agent no longer reports.

---

## 12. Verification plan

Implement these when coding starts. They define the acceptance tests for AC-1.

### Backend / repository

1. **Delete on complete snapshot**  
   Upsert `{alice, bob}` complete. Upsert `{alice}` complete. List for that `agent_id` is only alice. `get(bob)` is missing. `removed == 1`.

2. **Incomplete does not delete**  
   Upsert `{alice, bob}` complete. Upsert `{alice}` with `complete=false`. Both rows remain. `removed == 0`. alice’s fields may update.

3. **Omitted complete does not delete**  
   Same as (2) with the field absent (old agent). Both rows remain.

4. **Empty complete wipes that agent**  
   Upsert `{alice}` complete. Upsert `users: []` complete. Zero rows for that `agent_id`. `removed == 1`.

5. **Empty incomplete is a no-op**  
   Upsert `{alice}` complete. Upsert `[]` with `complete=false`. alice remains.

6. **Agent isolation**  
   Agent A: `{alice}`. Agent B: `{bob}` (different SIDs). A posts `{alice}` complete. bob remains.

7. **Unknown agent**  
   Complete payload for an unregistered `agent_id` still 404; no deletes.

8. **Enable/disable still updates**  
   Same SID, `enabled` flipped, complete snapshot. Row updated, `removed == 0`.

9. **New account still inserts**  
   `{alice}` then `{alice, carol}` complete. Both present.

10. **Postgres and memory**  
    Behaviour identical in both repositories.

### Agent

11. Clean collect → POST body includes `"complete": true`.  
12. Collect returns accounts + error → POST includes `"complete": false`.  
13. Collect returns empty + error → **no** POST (unchanged).

### HTTP / existing tests

14. Update `test_concurrent_upload`: today 20 one-SID complete-default-false POSTs accumulate. After AC-1, if those bodies send `complete: true`, only the last SID remains. Keep the test as concurrent **incomplete** POSTs (still accumulate) **or** as concurrent complete POSTs asserting last-snapshot-wins. Do not leave the old assertion; it would hide a real behaviour change.

15. Existing list/filter/get tests still pass when `complete` is omitted.

### Manual / Windows (when implementing)

16. Create a local user, wait for upload, confirm it in Fleet Accounts / Account Inventory.  
17. Delete the local user, wait for the next **successful** cycle, confirm it is gone from both UIs.  
18. Disable (do not delete) a user; it remains, with `enabled: false`.

Do **not** use Installed Applications, Pipeline, or Device as part of this verification.

---

## 13. Definition of Done

AC-1 is done when all of the following are true:

1. A Windows local account that no longer appears in a **complete** `POST /api/v1/users` is hard-deleted from `user_inventory` for that `agent_id` in the same transaction as the upsert.
2. Incomplete or legacy (no `complete` field) uploads never delete rows.
3. Other agents’ account rows are never removed by this agent’s snapshot.
4. Failed ingest (unknown agent, validation error, transport failure) leaves the previous inventory unchanged.
5. Create / enable / disable continue to work via upsert.
6. `GET /api/v1/users`, `GET /api/v1/users/{sid}`, and `GET /api/v1/fleet/{id}/users` reflect deletions without frontend changes.
7. Postgres and in-memory repositories implement the same semantics.
8. Automated tests cover §12 items 1–15.
9. No Pipeline, Device, software, or other inventory modules are modified.
10. Collector enumeration logic is unchanged except for the `complete` flag on the upload envelope.

**Not required for AC-1:** composite PK, fingerprint, retry queues, collector paging, builtin/admin-group fixes, placeholder UI columns, ingest authentication, delete audit history.

---

## Decision summary

| Question | Decision |
|----------|----------|
| Strategy | Authoritative snapshot replace, scoped by `agent_id` |
| Gate | `complete` boolean on POST; default `false` |
| Empty complete body | Wipe that agent’s rows |
| Partial collect | Still upload; `complete: false` |
| Schema | No migration; hard delete |
| API | Same POST; additive request + `removed` on response |
| Collector enum | Unchanged |
| Pipeline / Device / other inventory | Unchanged |
| Rollback | Revert code; already-deleted rows stay deleted |

**Stop.** Waiting for review before any implementation.

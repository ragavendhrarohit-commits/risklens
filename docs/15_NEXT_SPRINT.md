# 15 — Next Sprint

Recommendation is based **only** on the current tree: frozen IA (`docs/ARCHITECTURE.md` says Sprint 14 is next), reserved UI copy (“Global search will be enabled after Sprint 14”), empty collector domains, and the **unfinished session invariant** on the live login write path.

## Recommendation

**Do not start the reserved Sprint 14 feature set (global search, new collectors) until the session write path matches the documented invariant.**

The next logical engineering sprint is:

### Sprint: Session uniqueness (one ACTIVE session per agent)

This is not named in the frozen IA as “Sprint 14”. It is the gap between **implemented session APIs** and **correct Fleet session history**. Shipping search or process collectors on top of overlapping sessions will encode bad data into more UI surfaces.

Sprint 14 (global search) remains the next **product-IA** sprint after this correctness sprint.

---

## Why this is next

1. Sessions and application activity are already live in agent, API, DB, and Fleet tabs (`test_sprint_13_7_architecture.py` requires those routes).
2. Domain comments and tests describe “new login closes previous ACTIVE at the new logon timestamp”.
3. `SessionService` still logs `repository_method="_upsert_one -> upsert_many"` and never calls `activate_current`.
4. There is no `UNIQUE (agent_id) WHERE status = 'active'`.
5. Offline close **refuses** when two ACTIVE rows exist, so the fallback cannot clean up.
6. REPRO instrumentation is already in the tree specifically to trace this.
7. Frozen Sprint 14 work (search, groups, extra collectors) does not depend on overlapping sessions being unfixed — but operators already use the Sessions tab.

---

## Scope (from existing code, not new product ideas)

**In scope**

- After a REPRO trace exists, make the login write atomic: close other ACTIVE rows for `agent_id` at the new `logon_at`, then insert/activate the new session (the behaviour `activate_current` already implements on the repository).
- Keep shutdown updating **only** `current_session_id`.
- Keep closed rows immutable.
- Add/confirm a DB constraint or transactional abort matching `MultipleActiveSessionsError`.
- Align HTTP/service tests with the invariant; keep observed-timeline regressions.
- Remove or gate REPRO logging after the fix is proven.
- Update `docs/ARCHITECTURE.md` so sessions/applications are no longer labelled reserved.

**Out of scope**

- Global search
- Processes/services/network/browser collectors
- Merging `backend/` into `platform/`
- Auth
- Changing Sprint 12 inventory JSON contracts

---

## Prerequisites

| Prerequisite | Why |
|--------------|-----|
| Platform API running with Postgres | REPRO dumps and real rows |
| Agent on the target laptop rebuilt with current activity/heartbeat REPRO code | Agent-side LOGIN DETECTED / payload kinds |
| Reproduction: Login → Logout → Login → Logout → Login → Shutdown | Evidence before changing write path (current working rule) |
| Do not change public inventory/fleet list contracts | Sprint 12/13 freeze |

If the trace shows the agent already POSTs `LOGIN_CLOSE_THEN_OPEN` and the backend still leaves two ACTIVE rows, the first fix is `SessionService` + SQL. If the trace shows open-only login with a stale ACTIVE in DB, the same backend close-on-login is still required (agent cannot close a `session_id` it no longer has locally).

---

## Risk

| Risk | Mitigation |
|------|------------|
| Closing the wrong session on login | Close only ACTIVE rows for that `agent_id`; cap logoff at new logon; never touch CLOSED |
| Changing timestamps on historical closed rows | `session_is_immutable` + SQL `status != 'closed'` |
| Unique index failing on existing bad data | Detect/repair overlapping ACTIVE rows in a one-off migration or startup check |
| Breaking tests that currently expect `_upsert_one` overlap | Update service-level tests; keep repo tests for `activate_current` |
| Shipping a fix without the Laptop B trace | Violates the current investigation rule — wait for REPRO logs |

Estimated effort: **small-to-medium** (1–3 engineering days) once the trace exists — `activate_current` is already written. Larger if existing production rows must be repaired and if mixed close+open payloads interact badly with the new transaction.

---

## Files involved

**Must change (when fixing)**

- `platform/app/services/session_service.py` — `_activate_login` should call `activate_current` (or equivalent) instead of `_upsert_one` for new logins
- `platform/app/infrastructure/postgres/session_repository.py` — already has `activate_current`; may add unique/partial index in a new Alembic revision `0008_...`
- `platform/app/infrastructure/memory/memory_session_repository.py` — keep parity
- `platform/tests/test_session_lifecycle.py` — service/HTTP tests for one ACTIVE
- `docs/ARCHITECTURE.md`, `docs/08_AGENT_SESSION_LIFECYCLE.md`, `docs/14_CURRENT_STATUS.md`

**Likely unchanged**

- Agent activity module (already can emit close+open from local state)
- Fleet GET contracts
- Session JSON field names (`session_id`, `logon_at`, `logoff_at`, `status`)

**Optional cleanup after green REPRO**

- `agent/internal/modules/activity/repro_log.go`, REPRO logs in `upload.go` / `heartbeat.go`
- `platform/app/services/session_lifecycle_log.py` verbosity
- `MultipleActiveSessionsError` — raise instead of log-only

---

## After that: Sprint 14 (product IA)

From `frontend/index.html` and `docs/ARCHITECTURE.md`:

- Enable global search (currently disabled)
- Still must not invent a parallel nav model
- Collectors remain later sprints unless a smaller slice (for example Fleet timeline backed by `session_events.json`) is explicitly chosen

Merging the UI static server into the Platform API (`GET /` serving `index.html` on :8000) is a reasonable **engineering** follow-up so operators stop confusing `/` JSON with the SPA. It is not required by the IA freeze.

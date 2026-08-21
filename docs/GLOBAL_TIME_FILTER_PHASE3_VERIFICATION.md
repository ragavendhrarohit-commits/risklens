# Global Time Filter — Phase 3 verification

**Date:** 2026-08-21.  
**Scope:** Investigation only. No code was changed.

**Question:** Why does changing the Global Time Picker (Today → Yesterday) not filter Fleet → Application Activity?

**Answer:** The live Platform API process is still the pre-Phase-3 server. It does not accept `started_at_from` / `started_at_to`. FastAPI ignores those query parameters. SQL never filters. The table is rendered from the unfiltered list (58 rows for MSI).

The picker and Phase 1 store are working. The disk source for Phase 3 exists. The running `:8000` process is not that source.

---

## Exact failure point

**Stop: live `GET /api/v1/fleet/{agent_id}/applications` handler.**

Evidence:

1. Live OpenAPI for that path lists only `agent_id`, `limit`, `offset`, `sort`. It does **not** list `started_at_from` or `started_at_to`.
2. The same URL with Today-like bounds and Yesterday-like bounds both returned **total = 58**.
3. Direct SQL on the same agent: Today-like window = **58** rows, Yesterday-like window = **0** rows. If the live handler filtered, Yesterday would be empty.
4. The API process on port 8000 was started at **2026-08-21T04:32:39Z** without `--reload`, before Phase 3 was written. Access logs for UI GETs are `.../applications?limit=200` only. **`started_at_from` never appears in those logs.**

Disk code *does* pass the range into `ListApplicationActivityQuery` and adds `started_at >= from` / `started_at <= to`. That code is not what is serving `:8000`.

---

## Stage-by-stage trace

Computer used for live counts: MSI, `agent_id = e6e55967-ed7a-4eee-904a-789f2d16cbfd`.  
Timezone used for Today/Yesterday examples: `Australia/Sydney` (UTC+10).  
Investigation time: about 2026-08-21 15:09 local / 05:09 UTC.

### 1. Global Time Picker

| Question | Result |
|---|---|
| Is the selected time range present? | **Yes, in the header UI.** |
| What values are being used? | Quick Select writes the Phase 1 store (`setPreset("today")`, `setPreset("yesterday")`, and the other presets). Custom Range writes `setCustomRange`. The header label updates. |
| Sends `started_at_from` / `started_at_to`? | **No.** The picker only updates the store. It does not call the Application Activity API. |

This stage is not the failure. A changing picker with a static table is expected if later stages drop the range.

### 2. Global Time Store

| Question | Result |
|---|---|
| Is the selected time range present? | **Yes.** `getState()` exposes `startTime`, `endTime`, `preset`, `timezone`, `refreshInterval`. Relative presets are resolved at read time. |
| What values are being used? | Default preset is Last 15 Minutes. **Today** (Sydney, 21 Aug 2026) resolves to `startTime = 2026-08-20T14:00:00.000Z`, `endTime = now`. **Yesterday** resolves to `startTime = 2026-08-19T14:00:00.000Z`, `endTime = 2026-08-20T13:59:59.999Z`. |
| Sends HTTP params? | **No.** Store has no I/O. |

This stage is not the failure.

### 3. Fleet Application Activity (frontend)

| Question | Result |
|---|---|
| Is the selected time range present? | **In current `app.js` on disk and on `:8080`: yes.** `applicationActivityTimeQuery()` reads the store. `bindApplicationActivityTimeFilter()` subscribes and refetches only when `view === "fleet-detail"` and `tab === "applications"`. |
| What values are being used? | `started_at_from = range.startTime`, `started_at_to = range.endTime`. |
| Does the request include those params? | **Intended by current source: yes.** **Observed on the live API: no.** Logged UI GETs were `GET /api/v1/fleet/{id}/applications?limit=200` with no time params (04:33 UTC, before Phase 3). No later UI GET with `started_at_from` was logged. |

If the browser tab was left open across Phase 3, it can still be running the old `app.js` (no subscribe, no time query). The picker can still change because it is a different script. That would also produce a static table.

Even after a reload, the live API would still ignore the params (next stage).

Sessions / Browser Activity / Alerts / Reports do not subscribe. That part matches Phase 3 scope.

### 4. HTTP request

| Question | Result |
|---|---|
| Is the range present on the wire? | **Not on any request this API process has logged.** |
| Values? | Observed: `limit=200` only. |
| `started_at_from` / `started_at_to`? | **Absent in access logs.** |

A probe from this investigation *did* add the two params. The live handler still returned 58 rows (see backend).

### 5. Backend endpoint

Live: `GET /api/v1/fleet/{agent_id}/applications`

| Question | Result |
|---|---|
| Does the backend receive the range? | **The running process has no parameters to bind.** OpenAPI: `agent_id`, `limit`, `offset`, `sort`. Extra query keys are ignored. |
| What values are used? | Unfiltered list for the agent, `limit=200`. |

Source on disk *does* declare `started_at_from` and `started_at_to` and passes them to `FleetService.applications`. That is not the process bound to port 8000.

**This is the failure point for live verification.**

### 6. Repository query

| Question | Result |
|---|---|
| Does the live repository use the range? | **No.** The live handler never constructs `ListApplicationActivityQuery` with `started_at_from` / `started_at_to`. |
| Disk source? | Memory and Postgres repositories *do* apply the bounds when those fields are set. |

### 7. Database / SQL

| Question | Result |
|---|---|
| Does live SQL filter on `started_at`? | **No.** The live path never adds those WHERE clauses. |
| Disk source? | `application_activity.started_at >= started_at_from` and `started_at <= started_at_to`. |

Row counts for MSI (`e6e55967-ed7a-4eee-904a-789f2d16cbfd`):

| Window | Rows |
|---|---|
| All rows (before filtering) | **58** |
| Today-like (`started_at` >= 2026-08-20 14:00 UTC, <= 2026-08-21 05:30 UTC) | **58** |
| Yesterday-like (`started_at` >= 2026-08-19 14:00 UTC, <= 2026-08-20 13:59:59.999 UTC) | **0** |
| Earliest `started_at` | 2026-08-21 04:11:25 UTC |
| Latest `started_at` | 2026-08-21 05:09:57 UTC |

Every stored row is on 21 Aug UTC, which is **Today** in Sydney and **not Yesterday**. If SQL filtering ran, Today → Yesterday would go from 58 rows to the empty message.

Live API Today-like and Yesterday-like GETs both returned **58**. That matches “no SQL filter”.

### 8. Response

| Question | Result |
|---|---|
| Filtered? | **No.** `total` stayed 58 with and without time params on the live server. |
| After filtering (SQL, not API) | Today 58 / Yesterday 0. |

### 9. Frontend rendering

| Question | Result |
|---|---|
| Filtered response or old data? | The table is filled from `f.applicationActivity`. That object is whatever the last `GET .../applications` returned. Live responses are the **unfiltered** 58-row page (or the earlier `?limit=200` payload, which is the same set). |
| Empty copy | Current `app.js` would show `No application activity during the selected time range.` if `items` were empty. Yesterday never got an empty payload from the live API, so that message did not appear. |

This is not a stale-render bug in `renderFleetApplicationActivityTable`. The renderer prints the payload it is given. The payload is unfiltered.

---

## What is *not* broken

- Picker UI and Phase 1 store update when the user picks Today / Yesterday / other presets.
- Current `:8080` `app.js` contains `applicationActivityTimeQuery` and the store subscribe (confirmed by fetching `/assets/app.js`).
- Disk backend/repository/SQL for the range exists and unit tests cover it.
- Sessions, Browser Activity, Alerts, and Reports are correctly not wired.

---

## Why Today → Yesterday looks like a no-op

1. Live API ignores time bounds → same 58 rows.
2. Those 58 rows are all “today” in the database; Yesterday *should* be 0, but the API never asks SQL for that.
3. Optional: a tab that was not reloaded after Phase 3 still runs old `app.js`, so it may not even refetch. The picker can still move because it is a separate file.

---

## Stop

No code was changed. The live failure is the **pre-Phase-3 Platform API process on port 8000**, which never applies `started_at` bounds. Restarting that process is an operations step, not a code fix, and was not done here.

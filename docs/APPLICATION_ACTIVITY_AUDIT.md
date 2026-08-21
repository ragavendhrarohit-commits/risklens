# Application Activity — engineering audit

**Date:** 2026-08-20.  
**Status:** Investigation only. No source, agent, or database changes.  
**Question:** Is Application Activity fully functional for employee usage, duration, current foreground, time-per-app, and productivity reporting?

**Verdict:** The **collection → upload → PostgreSQL → Fleet table** path is live and stores real foreground-window segments. It is **not** a production productivity product. There is no time-per-application rollup, no “current foreground” badge, no idle/lock accounting, and no productivity reports.

Live evidence (this environment, `GET /api/v1/fleet/{id}/applications?limit=5&sort=-started_at`):

| Agent | `total` rows | Sample |
|-------|----------------|--------|
| MSI (`c2c28db4-…`) | **808** | `chrome.exe` YouTube titles (14–15s segments); open `chrome` with `stopped_at` null; `Cursor.exe` |
| LAPTOP-8VQ40K6U | **55** | `cmd.exe` open (~3105s); `explorer.exe` closed 14s |

---

## 1. Current architecture

Application Activity is **not** on the generic Device Pipeline. It lives inside the **activity** module (same scheduler as computer sessions).

| Layer | Location | Role |
|-------|----------|------|
| Windows foreground | `agent/internal/modules/activity/collector_windows.go` | `GetForegroundWindow` + process image |
| Helper | same binary `--activity-foreground` | Session-0 / SYSTEM cannot see the user desktop; spawn as the logged-on user |
| Model | `activity/model.go`, `state.go` | `LiveApp` → `ActivityItem` segments |
| Schedule + upload | `activity/upload.go` | Default **15s** ticker; `POST /api/v1/applications` |
| Ingest | `platform/app/api/v1/applications.py` | `ApplicationActivityService.upsert` |
| Store | `application_activity` (migration `0007`) | PK `id` (SHA1); FK `session_id`, `agent_id` |
| Query | `GET /api/v1/applications`, `GET /api/v1/fleet/{id}/applications` | List/filter/sort, max 200 |
| UI | Fleet → computer → **Application Activity** tab | Raw segment table |

Depends on **sessions**: app rows require an existing `computer_sessions.session_id` (`unknown_session` 422).

---

## 2. Current implementation

### Foreground detection

`WindowsCollector.Collect`:

1. `WTSEnumerateSessionsW` / `WTSQuerySessionInformationW` / `WTSGetActiveConsoleSessionId` — interactive logon.
2. `foregroundApp(sessionID)`:
   - Same session as the agent process → `readForegroundLocal()`
   - Else impersonate via `WTSQueryUserToken` + `ImpersonateLoggedOnUser`
   - Else `CreateProcessAsUser` helper (`--activity-foreground`, 5s timeout)

`readForegroundLocal`:

- `GetForegroundWindow`
- `GetWindowTextW`
- `GetWindowThreadProcessId`
- `OpenProcess` + `QueryFullProcessImageNameW`

`application_name` is the executable **basename without extension** (`chrome`, `Cursor`), not the Windows file description / Product Name.

Non-Windows: stub collector; no foreground.

### Switching and duration

`state.step` + `sameApp`:

```64:69:agent/internal/modules/activity/state.go
func sameApp(prev persistedState, live *LiveApp) bool {
	return strings.EqualFold(prev.ExecutableName, live.ExecutableName) &&
		prev.WindowTitle == live.WindowTitle
}
```

A **window-title change** (YouTube video, document tab) starts a **new segment**. Live MSI data matches this: many ~15s Chrome rows with different titles.

- Same exe+title: `refreshOpenApp` — `stopped_at` null, `duration_seconds = now - started_at`
- Different exe or title: previous segment closed (`stopped_at`, duration), new `openAppItem`
- Session close also closes the open app segment (`closeSession`)

`durationSeconds` is wall-clock between `started_at` and `stopped_at` / now. **Idle, lock, and AFK are not subtracted.**

### Upload

Every activity interval (config `activity.interval`, default **15s**), the module POSTs session payload and, when there are activity items, `POST /api/v1/applications`. Failed uploads can queue (same activity queue as sessions).

Backend: unknown agent 404; unknown `session_id` 422; upsert by SHA1 `id` = `session_id + executable + started_at`.

---

## 3. Data flow

```
Interactive Windows session (WTS)
    ↓  every 15s (and WTS logon/logoff/shutdown)
GetForegroundWindow → exe + title
    ↓
activity.json (one current app segment)
    ↓
POST /api/v1/sessions   (session must exist first)
POST /api/v1/applications
    ↓
application_activity upsert by id
    ↓
GET /api/v1/fleet/{agent_id}/applications
    ↓
Fleet Application Activity table (limit 200, cached until computer switch)
```

---

## 4. Database tables

**`application_activity`** (Alembic `0007_session_application_activity.py`)

| Column | Role |
|--------|------|
| `id` | SHA1 PK |
| `session_id` | FK → `computer_sessions` |
| `agent_id` | FK → `agents` |
| `application_name`, `executable_name`, `window_title` | What was in the foreground |
| `started_at`, `stopped_at` | Segment bounds; open = `stopped_at` null |
| `duration_seconds` | ≥ 0 |
| `collected_at`, `received_at` | Agent / server clocks |

Indexes: `(session_id, started_at)`, `(agent_id, started_at)`.

No table for: productivity scores, app categories, daily totals, idle, browser URLs (Browser Activity is unimplemented).

---

## 5. API endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/applications` | Agent ingest |
| GET | `/api/v1/applications` | Org list (`agent_id`, `session_id`, `application_name`, sort, limit ≤ 200) |
| GET | `/api/v1/applications/{id}` | One row |
| GET | `/api/v1/fleet/{agent_id}/applications` | Per-computer list (Fleet tab) |

There is **no** aggregate endpoint (`SUM(duration) GROUP BY application_name`, current-foreground, employee report).

Default sort: `-started_at`.

---

## 6. UI implementation

**Live (real GET data):** Fleet → computer → tab **Application Activity** (`renderFleetApplicationActivityTable`). Columns: Application, Executable, Window Title, Started, Stopped, Duration, Session account. Empty/error via shared tab loading. Open rows: UI computes duration from `started_at` to now (`liveDurationSeconds`); no ticking timer (unlike Sessions).

**Truncation:** fetch `limit=200`. MSI has **808** rows → UI shows “200 of 808” at most; older segments are invisible without paging (Fleet tab has **no pager**).

**Cache:** `loadFleetTabData` fetches only when `!f.applicationActivity` (same pattern as Accounts). Revisit the tab without switching computers → stale list.

**Placeholders (not this feature’s data):**

| Surface | Content |
|---------|---------|
| Fleet → **Activity** | `renderActivityArchitecture()` — “Not collected yet” including **Foreground application**, **Application usage**, **Most used applications**, idle, lock, sleep |
| Fleet → **Timeline** | Fake events (Chrome Started, etc.) |
| Fleet → **Browser Activity** | “Collectors are not implemented” |
| Org Reports | Installed Applications only; no org-wide Application Activity page |

Overview “Current user / logon” placeholders are session/overview, not the Application Activity tab.

---

## Answers to the eleven questions

### 1. Is foreground application detection implemented?

**Yes**, on Windows, for the interactive console/connected session.

### 2. Which Windows APIs are used?

**Session:** `WTSEnumerateSessionsW`, `WTSQuerySessionInformationW`, `WTSGetActiveConsoleSessionId`, `WTSQueryUserToken`, `WTSFreeMemory`.  
**Window:** `GetForegroundWindow`, `GetWindowTextW`, `GetWindowThreadProcessId`.  
**Process:** `OpenProcess`, `QueryFullProcessImageNameW`, `ProcessIdToSessionId`.  
**Identity:** `ImpersonateLoggedOnUser`, `RevertToSelf`, `DuplicateTokenEx`, `CreateProcessAsUser` (helper).  
**SID:** `LookupSID`.

Not used for this feature: ETW, UI Automation, `GetLastInputInfo` (idle), browser DevTools.

### 3. How often is the foreground window sampled?

**Default 15 seconds** (`activity.DefaultInterval` / YAML `activity.interval`). Plus cycle-now on WTS logon/connect. Resolution of a switch is **up to one interval** (typical closed segments in live data are ~14–15s).

### 4. Is duration calculated?

**Yes**, wall-clock per **segment** (`durationSeconds` on the agent; backend can fill from `stopped_at - started_at`). Open segments keep growing until a switch or session close. **Not** productive time (no idle subtract).

### 5. Is application switching detected?

**Yes**, by comparing previous vs current **executable + window title**. Title-only changes count as a switch. Same exe, different title → new row.

### 6. Is activity uploaded?

**Yes.** `Client.UploadApplications` → `POST /api/v1/applications`. Live `total` 808 / 55 proves ingest.

### 7. Is it stored in PostgreSQL?

**Yes**, table `application_activity`, upsert on `id`.

### 8. Does the UI display real data?

**Yes** on Fleet → Application Activity (raw segments). **No** on the Activity monitoring table, Timeline, or any productivity/report view. Those are placeholders or unimplemented.

### 9. Which parts are complete?

- Windows foreground poll (including SYSTEM → user desktop helper)
- Segment model (start/stop/duration)
- Switch detection (exe+title)
- Recurring upload + session FK
- PostgreSQL persistence
- GET list/detail + Fleet nested route
- Fleet segment table with real rows
- Agent unit tests for switch/close; platform `test_application_activity.py` (session required)

### 10. Which parts are placeholders?

- Fleet **Activity** tab (“Not collected yet” for usage/foreground/idle/power)
- Fleet **Timeline** (hard-coded fake events)
- **Browser Activity**
- Org-wide Application Activity report
- “Currently in foreground” indicator
- Time-spent-per-application / most-used ranking
- Productivity scoring or categories
- Employee-level (account) usage report (data is per computer/session only)
- Pagination on the Fleet applications tab
- Auto-refresh of that tab

### 11. What is missing before this is production-ready?

For **manager questions** (which apps, how long, current foreground, time per app, productivity):

1. **Aggregate by application** (exe or Product Name), not by every window title — or keep titles as detail under a rollup.
2. **Current foreground** = latest open row (`stopped_at` IS NULL); show it on Overview/Activity, not only buried in 200 unsorted-looking segments.
3. **Pagination or “current session only”** — 808 rows / cap 200 is not operable.
4. **Idle / lock** — lock is detected for **sessions** (does not close session) but does **not** pause app duration; idle is not collected (`GetLastInputInfo` unused). Time-on-Chrome includes AFK.
5. **Productivity reporting** — no taxonomy, no productive/unproductive, no daily/weekly employee report, no export.
6. **Session invariant** — overlapping Active sessions (separate bug) can attach app rows to the wrong or duplicate session.
7. **Sampling** — 15s misses short switches; title-based splits explode row count.
8. **Privacy** — full window titles (email subjects, YouTube, documents) stored in clear text; no retention policy.
9. **Fleet tab cache** — same `!f.applicationActivity` stale-fetch as Accounts.
10. **Not on Pipeline** — no fingerprint skip; every 15s POST of open segment is expected but chatty at fleet scale.

---

## Missing functionality (vs manager intent)

| Manager ask | Today |
|-------------|--------|
| Which applications an employee uses | Segment list per **computer**, name = exe stem. No HR employee identity. No rollup. |
| How long each application is active | Per **title segment**, ~15s chunks. No `SUM` by app. |
| Currently in the foreground | Infer `stopped_at` null. UI does not say “now”. Activity tab says not collected. |
| Time spent per application | **No API/UI.** Must be built. |
| Productivity reporting | **Does not exist.** |

---

## Risks

| Risk | Severity |
|------|----------|
| Window titles are PII/content; stored forever | High |
| Title-level segments → huge tables, 200-row UI cap hides history | High |
| Duration includes idle/lock | High for productivity claims |
| SYSTEM agent: helper/impersonation failure → empty App, silent gaps | Medium |
| `unknown_session` if session POST lags | Medium |
| Stale Fleet tab cache | Medium |
| Overlapping sessions mis-attribute usage | Medium |
| 15s granularity unsuitable for “active” claims | Medium |

---

## Recommended implementation order

Do **not** implement in this audit. Suggested later:

1. **Honesty in UI** — label open row as current foreground; stop saying “Not collected yet” on Activity for signals that already exist as segments.
2. **Query: current session apps + aggregate by `executable_name`** (`SUM(duration_seconds)`, open row live duration).
3. **Fleet tab:** paginate or default to current session; refetch on tab visit.
4. **Segment key:** switch on **executable** (optional title as attribute), to stop YouTube-title row explosion — product decision.
5. **Idle/lock:** pause or split segments (`GetLastInputInfo`, existing lock events).
6. **Retention** for window titles (or store titles hashed/omitted).
7. **Productivity reports** only after 2–6 (categories, employee mapping, export).
8. Pipeline/fingerprint later; not required for correctness of segments.

---

## Definition of Done (production Application Activity)

**Not done today.** A production bar aligned with the manager questions:

1. Foreground exe detected on Windows interactive sessions (including service-hosted agent). **Met.**
2. Segments stored with start/stop/duration and session binding. **Met.**
3. Operator can see **current** foreground for a computer without guessing. **Not met.**
4. Operator can see **total time per application** for a session/day, not only raw title rows. **Not met.**
5. History is complete (no silent 200-row cap). **Not met.**
6. Idle/lock does not count as app usage (or is labelled). **Not met.**
7. Privacy/retention for window titles is explicit. **Not met.**
8. Productivity report (employee × app × hours) exists or is explicitly out of scope. **Not met.**

**Honest status for leadership:** Application Activity is a **working foreground-window logger** with a Fleet table. It is **not** an employee productivity reporting feature.

---

## Direct answers (checklist)

| # | Question | Answer |
|---|----------|--------|
| 1 | Foreground detection implemented? | **Yes** (Windows) |
| 2 | Windows APIs | WTS + `GetForegroundWindow` / `GetWindowTextW` / `QueryFullProcessImageNameW` (+ token/helper) |
| 3 | Sample interval | **15s** default |
| 4 | Duration calculated? | **Yes**, per segment, wall-clock |
| 5 | Switching detected? | **Yes** (exe **and** window title) |
| 6 | Uploaded? | **Yes** `POST /api/v1/applications` |
| 7 | PostgreSQL? | **Yes** `application_activity` |
| 8 | UI real data? | **Yes** on Application Activity tab; **No** on Activity/Timeline/reports |
| 9 | Complete | Collect, segment, upload, store, GET, raw table |
| 10 | Placeholders | Activity tab usage/foreground, Timeline, Browser, productivity |
| 11 | Missing for production | Rollup, current-app UX, paging, idle, privacy, reports |

---

## What was not done

- No source changes  
- No database changes  
- No agent changes  

**Stop.** Waiting for review.

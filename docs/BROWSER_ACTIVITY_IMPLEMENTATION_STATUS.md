# Browser Activity — implementation status

**Date:** 2026-08-21.  
**Scope:** Investigation only. No code was changed.

**Question:** Why does Fleet → Browser Activity still show “Collectors are not implemented”?

**Answer:** The Fleet UI never loads Browser Activity. The tab is a hardcoded placeholder. That is why the page looks unimplemented even though Phase 1 collection code exists in the repo.

The screenshot of MSI (`c2c28db4-9838-45de-85e0-d487bf310091`) is this branch in `frontend/assets/app.js`:

```javascript
if (f.tab === "browser") {
  body.innerHTML = `<p class="text-mist-400 mb-3">Browser Activity is website usage on this computer during a session. Collectors are not implemented.</p><p>Not implemented yet.</p>`;
  return;
}
```

Application Activity uses `GET /api/v1/fleet/{agent_id}/applications`. Browser Activity has no equivalent Fleet fetch, and `loadFleetTab` never requests `/api/v1/browser`.

Phase 1 collection was written as agent + ingest API only. Architecture listed “Fleet Browser Activity tab (real data)” as a later step. A platform test still asserts the stub:

- `platform/tests/test_sprint_13_7_architecture.py` requires `"Browser Activity is website usage"` in `app.js`
- it requires `"/api/v1/fleet/${encodeURIComponent(f.selectedId)}/browser"` **not** to appear in `app.js`

---

## Exact layer where the page stops

| Layer | Result |
|---|---|
| **Frontend Fleet tab `browser`** | **Stop for the UI.** Hardcoded “Not implemented yet.” No `fetch`. |
| `GET /api/v1/fleet/{agent_id}/browser` | Does not exist. Not mounted on the fleet router. |
| `GET /api/v1/browser` | Exists in source; live process on `:8000` does not serve it (404). |

If the frontend were wired tomorrow, this machine still would not show live rows until the collection path below is deployed. Those are separate from the placeholder text.

---

## End-to-end checkpoints

### 1. Browser extension exists?

**In source: yes. Installed in Chrome/Edge/Firefox on this PC: no.**

| Artifact | Path |
|---|---|
| Chromium MV3 | `agent/browser-extension/chromium/manifest.json` + `background.js` |
| Firefox MV2 | `agent/browser-extension/firefox/` (gecko id `browser-activity@risklens.local`) |

The service worker queries only `{ active: true, lastFocusedWindow: true }` and posts to native host `com.risklens.browser`. Permissions are `tabs`, `nativeMessaging`, `alarms`.

Chrome `HKCU\Software\Google\Chrome\NativeMessagingHosts` has other hosts (`com.anthropic.claude_browser_extension`, `com.microsoft.browsercore`). **`com.risklens.browser` is not registered.** There is no evidence the unpacked RiskLens extension is loaded.

---

### 2. Native Messaging host exists?

**In source: yes. On this machine: no.**

| Piece | Source | Live |
|---|---|---|
| Host process | `RiskLensAgent.exe` enters `RunNativeHost()` when argv is `--browser-native-host`, `chrome-extension://…`, or `*@risklens*` | Running binary is `agent\bin\RiskLensAgent.exe` dated **2026-08-20 14:45**. `findstr` does **not** find `browser_activity` or `com.risklens.browser` in that exe |
| Manifest writer | Module `Start` writes `state/native/com.risklens.browser.*.json` | State dir `C:\Users\rohit\AppData\Local\Temp\risklens-sprint8\data` has `activity.json` only — **no `native/` folder, no `browser_activity.json`, no inbox** |
| Chrome/Edge registry | Written only if `browser_activity.chromium_extension_id` is set | Empty on this host |
| Firefox registry | Written when the module starts enabled | Not present |

Live `C:\ProgramData\RiskLens\agent.yaml` has **no `browser_activity` section**. Code defaults `BrowserActivity.Enabled = false`, so even a rebuilt agent would skip collection until YAML (or env) enables it.

---

### 3. Agent receives browser events?

**Live: no.**

The running process is:

```text
agent\bin\RiskLensAgent.exe -console -config C:\ProgramData\RiskLens\agent.yaml
```

That binary predates Phase 1. Inbox file `browser_native_inbox.jsonl` does not exist. There is no native host child process from Chrome.

---

### 4. Agent uploads browser events?

**Source: yes (`POST /api/v1/browser`). Live: no.**

Uploader reuses the existing HTTP transport. The running agent cannot upload what it does not collect, and the live API does not expose the upload route (see §5).

---

### 5. Backend stores browser events?

**Source: yes. Production database `risklens`: no.**

| Item | Status |
|---|---|
| Domain + service + memory/postgres repos | In `platform/app/` |
| Alembic revision `0008_browser_activity` | In source |
| Table `browser_activity` | **Missing on `risklens`** |
| `alembic_version` on `risklens` | **`0007_session_app_activity`** |
| `application_activity` on `risklens` | Present (**1051** rows) — Application Activity ingest is live |

`0008` was applied to `risklens_test` during pytest. Production was not migrated.

---

### 6. `GET /api/v1/browser` returns data?

**Source: yes (`platform/app/api/v1/browser.py`). Live uvicorn on port 8000: 404.**

Live OpenAPI (`http://127.0.0.1:8000/openapi.json`) has 24 paths. It includes `/api/v1/applications` and `/api/v1/fleet/{agent_id}/applications`. It does **not** include `/api/v1/browser`.

`GET http://127.0.0.1:8000/api/v1/browser?agent_id=c2c28db4-9838-45de-85e0-d487bf310091` → **404**.

The process is `platform\.venv\Scripts\python.exe -m uvicorn app.main:app --host 0.0.0.0 --port 8000` **without `--reload`**. It was started before the Phase 1 router was added, so it is still serving the pre-Phase-1 app.

There is also no `GET /api/v1/fleet/{agent_id}/browser`. Fleet service has no browser method.

---

### 7. Frontend Browser Activity tab still uses the placeholder?

**Yes.** That is the reason for the screenshot.

| Check | Result |
|---|---|
| Tab button exists | `frontend/index.html` `data-tab="browser"` |
| Tab loads data | **No** — `loadFleetTab` has no `browser` branch |
| Tab render | Hardcoded “Collectors are not implemented.” / “Not implemented yet.” |
| Calls `/api/v1/browser` | **No** (repo-wide frontend grep: no matches) |
| Calls `/api/v1/fleet/.../browser` | **No** (test forbids this string) |

---

## Pipeline (what exists vs what this machine runs)

```text
Chrome/Edge/Firefox
    │  source extension on disk; NOT installed
    ▼
Native Messaging  com.risklens.browser
    │  source host in agent; NOT in running exe / registry
    ▼
RiskLens Agent  browser_activity module
    │  source wired in main.go; live YAML disabled; live binary old
    ▼
POST /api/v1/browser
    │  source route; live OpenAPI 404
    ▼
Postgres  browser_activity
    │  source migration 0008; production still 0007
    ▼
GET /api/v1/browser  (ingest list API)
    │  source only; not used by UI
    ▼
Fleet UI tab
    ✖  STOPS HERE for the page you opened
       placeholder HTML, no fetch
```

---

## Why Application Activity works on the same page

Application Activity is a different path that is fully deployed:

1. Agent module `activity` is enabled in live YAML.  
2. Running agent writes `activity.json` and uploads `POST /api/v1/applications`.  
3. Live API serves `/api/v1/fleet/{agent_id}/applications`.  
4. Frontend fetches that URL when the Application Activity tab is selected.

Browser Activity was not wired through those last two steps, and the first three are not live on this host yet.

---

## Phase 1 vs this investigation

Phase 1 implemented collection, native messaging, agent upload, and storage **in the repository**. It did **not** change the Fleet Browser Activity tab (dashboards were out of scope).

The placeholder is therefore expected from the UI contract, not a regression in Application Activity.

**Stop.** No code changes were made in this investigation.

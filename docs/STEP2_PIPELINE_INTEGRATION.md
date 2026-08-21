# Step 2 — Device Inventory one-shot Pipeline integration

**Status:** design only. Do not implement until this document is approved.

**Scope:** first Device wiring to `internal/pipeline`. One collect. One upload. **No recurring scheduler.**

**Out of scope:** fingerprint-only unused helper as the *entire* step (that was `DEVICE_INTEGRATION_PLAN.md` Part 5 Step 2). This document is the next implementation slice after approved Step 1 (`Refresh`). It combines fingerprint + Collector adapter + Uploader adapter + **one** Pipeline tick.

**Do not:** start a later “Step 3” (interval YAML, `Pipeline.Start`, recurring inventory). Do not change backend, frontend, or database.

Contracts: [PIPELINE_CONTRACT.md](PIPELINE_CONTRACT.md), [DEVICE_INTEGRATION_PLAN.md](DEVICE_INTEGRATION_PLAN.md).

---

# Part 1 — Current flow

Traced from the tree **after Step 1**. No `internal/pipeline` import exists under `agent/cmd` or `agent/internal/modules`.

```
main.run
  → buildModules
      → device.New(Enabled: true)          // YAML does not gate collection
      → registration.New
      → deviceupload.New(Enabled from YAML)
      → heartbeat / users / software / activity
  → app.New → service.Run → App.Run
      → manager.StartAll  (registry order)
          → device.Start
          → registration.Start
          → deviceupload.Start
          → …other modules
```

## 1.1 Process construction

| | |
|--|--|
| **Package** | `main` |
| **File** | `agent/cmd/risklens-agent/main.go` |
| **Function** | `main` → `run` → `buildModules` |
| **Responsibility** | Load YAML, `identity.LoadOrCreate`, assemble modules, `service.Run` |

`device.New(device.Config{Enabled: true}, version.Version, logger)` — collection enable is **hardcoded true**.

`deviceupload.New(Config{Enabled: cfg.DeviceUpload.Enabled, BackendURL, Timeout}, idMgr, dev, …)` — upload gated by YAML `device_upload.enabled` (`Defaults()` is **false**). **No interval. No pipeline flag.**

## 1.2 Host start

| | |
|--|--|
| **Package** | `app` |
| **File** | `agent/internal/app/app.go` |
| **Function** | `(*App).Run` |
| **Responsibility** | `manager.StartAll` then block on ctx |

| | |
|--|--|
| **Package** | `manager` |
| **File** | `agent/internal/modules/manager/manager.go` |
| **Function** | `(*Manager).StartAll` |
| **Responsibility** | `module.Start(ctx)` in registry order. Non-nil error rolls back. **`device.Start` and `deviceupload.Start` always return nil.** |

## 1.3 Collect (once)

| | |
|--|--|
| **Package** | `device` |
| **File** | `agent/internal/modules/device/device.go` |
| **Function** | `(*Module).Start` |
| **Responsibility** | Module lifecycle (`MarkStarting` / Running / Failed). Calls `refresh` **once**. Does not upload. Does not schedule. |

| | |
|--|--|
| **Package** | `device` |
| **File** | `agent/internal/modules/device/device.go` |
| **Function** | `(*Module).refresh` (unexported; public `Refresh` wraps it) |
| **Responsibility** | Set `StateCollecting`; `collector.Collect`; `classifyState`; **replace** `Module.snapshot` (`device.Snapshot`: State + Info + LastError). |

| | |
|--|--|
| **Package** | `device` |
| **File** | `agent/internal/modules/device/collector_windows.go` |
| **Function** | `(*WindowsCollector).Collect` |
| **Responsibility** | Fill `device.Info` from Win32/registry. No HTTP. No fingerprint. New `Info` every call (stateless). |

Readers of `Inventory()` / `Info()` after Start: **`deviceupload.Start` only** (plus verify scripts). Registration does not read device inventory.

There is **no** Device ticker. `Refresh` is unused in production.

## 1.4 Upload (once)

| | |
|--|--|
| **Package** | `deviceupload` |
| **File** | `agent/internal/modules/deviceupload/deviceupload.go` |
| **Function** | `(*Module).Start` |
| **Responsibility** | If disabled / identity missing / inventory not Collected|Partial / empty name → skip or fail. Else `RequestFromInventory` + `Client.Upload` **once**. `Stop` is MarkStopped only. **Never retries.** |

| | |
|--|--|
| **Package** | `deviceupload` |
| **File** | `agent/internal/modules/deviceupload/request.go` |
| **Function** | `RequestFromInventory` |
| **Responsibility** | Map `device.Info` → JSON DTO. `DiskSizeBytes` → `disk_total_bytes`. Identity stamped here, not in Collect. |

| | |
|--|--|
| **Package** | `deviceupload` |
| **File** | `agent/internal/modules/deviceupload/client.go` |
| **Function** | `(*Client).Upload` |
| **Responsibility** | `Transport.BuildRequest(POST, "/api/v1/devices", req)` → `Send` → require 200/201 and `accepted && inventory_status=="stored"`. |

| | |
|--|--|
| **Package** | `transport/client` |
| **File** | `agent/internal/transport/client/client.go` |
| **Function** | `BuildRequest`, `Send`, `DecodeResponse` |
| **Responsibility** | Sole `net/http` owner. Unchanged by Device Step 2. |

## 1.5 Backend → Overview (unchanged; not Step 2)

`POST /api/v1/devices` → `upload_device_inventory` → `DeviceInventoryService.upsert` → `device_inventory` PK `agent_id` overwrite → `FleetService.on_inventory` sets `last_inventory_at`.

Overview: `loadFleetDetail` → `GET /api/v1/fleet/{id}` → `renderEndpointOverview`. Not `GET /api/v1/devices`.

## 1.6 What Pipeline is today (unwired)

| | |
|--|--|
| **Package** | `pipeline` |
| **File** | `agent/internal/pipeline/pipeline.go` |
| **Functions** | `New`, `Start`, `Stop`, unexported `tick` |
| **Responsibility** | Generic collect → skip-if-same-fingerprint → upload → remember. `New` **always** constructs `schedule.Scheduler` bound to `tick`. `Start` **always** starts that scheduler (immediate first `tick`, then wait Interval). **`tick` is not exported. There is no `RunOnce`.** |

Device does **not** implement `pipeline.Collector`. Deviceupload does **not** implement `pipeline.Uploader`. There is **no** fingerprint helper.

---

# Part 2 — Target flow

Step 2 production path when `device_upload.enabled` is true:

```
main.buildModules
  device.Start          → MarkRunning; do NOT Refresh (collect deferred)
  registration.Start    → unchanged
  deviceupload.Start
      pipeline.RunOnce     // existing tick, once; scheduler NOT started
          device.Collect     // pipeline.Collector
              Refresh()      // WindowsCollector.Collect + store device.Snapshot
              Fingerprint(Info) excluding CollectedAt
              return pipeline.Snapshot{Kind, SchemaVersion, Payload: Info, …}
          Compare lastSuccess (empty on first process run)
          Uploadable? empty fingerprint / nil payload → no POST
          else Uploader.Upload
              type-assert Payload → device.Info
              RequestFromInventory (same JSON as today)
              Client.Upload → POST /api/v1/devices
          Remember fingerprint on success / pending on failure
      return to StartAll   // “Wait” = do not sleep; do not start Interval loop
  heartbeat…            → unchanged
```

When `device_upload.enabled` is false:

```
device.Start → Refresh once (same as today)
deviceupload.Start → StateSkipped, no Pipeline, no POST
```

That preserves today’s “collect even if upload is off” behaviour.

## Transitions (Pipeline `tick`, already implemented)

`lastSuccess` and `pending` start empty in a new `Pipeline` value (process memory only).

| Condition | Action |
|-----------|--------|
| Collect returns non-`Uploadable` (empty fingerprint or nil Payload) | No POST. HealthError. lastSuccess unchanged. |
| `Uploadable` and `shouldSkip` (lastSuccess equal **and** not pending) | No POST. Remember nothing new. |
| `Uploadable` and not skip (first run, changed fingerprint, or pending) | `Uploader.Upload`. Success → `remember(fp)`. Failure → `markPending`, do **not** overwrite lastSuccess. |

**First process run:** `lastSuccess` empty ⇒ `shouldSkip` is false ⇒ **always POST** if Uploadable. Fingerprint compare is live in code but cannot skip the first upload. Skip is proven in **unit tests** by calling `RunOnce` twice, not in production Step 2 (production calls `RunOnce` once).

**“Wait” in Step 2** means: `deviceupload.Start` returns; the agent continues with heartbeat. It does **not** mean `time.Sleep` or `Scheduler.Start`.

**“Changed? No”** is not observable on a single production `RunOnce`. It must not be deleted; later recurring steps rely on it.

## Collect timing change (intentional)

Today Collect runs in `device.Start`, **before** registration. After Step 2 with upload enabled, Collect runs in `deviceupload.Start` via `Refresh` inside `pipeline.Collector.Collect`.

Nothing between those two `Start` calls reads `device.Info()` in production. Registration does not. Payload field values (except `collected_at`, which is a sample clock) remain the same `device.Info` mapping.

---

# Part 3 — Integration strategy

## How Device Upload becomes a Pipeline Uploader

Do **not** put HTTP inside `internal/pipeline`. Do **not** teach Pipeline about `device.Info`.

Add a small adapter **in package `deviceupload`**:

```
pipeline.Uploader.Upload(ctx, pipeline.Snapshot) error
  → Payload.(device.Info)
  → identity.Current()
  → RequestFromInventory(agentID, installID, info)
  → (*Client).Upload(ctx, req)
  → return err  // nil iff today’s success rules
```

`Client.Upload` and `Request` JSON tags **do not change**. Transport **does not change**.

## Device Collector becomes a Pipeline Collector

Adapter **in package `device`** on `*device.Module` (already has `Refresh`):

```
Kind() string                    → "device"
Collect(ctx) (pipeline.Snapshot, error)
  → Refresh(ctx)
  → fingerprint(Info, partial, kind, schemaVersion)
  → pipeline.Snapshot{
        Kind: "device",
        SchemaVersion: pipeline.SchemaVersionV1,
        CollectedAt: device.Snapshot.CollectedAt,
        Fingerprint: fp,
        Payload: device.Info,          // value, not pointer
        Partial: state == StatePartial,
        CollectError: LastError,
    }
```

`WindowsCollector` stays the OS probe. It still returns `device.Info`. It does not import `pipeline`.

## What changes

| Area | Change |
|------|--------|
| `device` | Fingerprint helper; `Kind`/`Collect` for `pipeline.Collector`; `Start` skips Refresh when collect is deferred to Pipeline |
| `deviceupload` | Uploader adapter; `Start` (enabled) builds `pipeline.Pipeline` and calls `RunOnce` instead of `Client.Upload` directly |
| `pipeline` | **Only** export `RunOnce` that calls existing `tick`. No change to Collector/Uploader/Snapshot/`tick`/`shouldSkip`/`remember` |
| `main` | Pass `CollectOnStart: !cfg.DeviceUpload.Enabled` (or equivalent) so upload-enabled agents collect once inside the tick |

## What does not change

- `RequestFromInventory` field mapping
- `Client.Upload` path, method, success gate
- YAML keys (`enabled`, `timeout` only)
- Scheduler implementation
- `Pipeline.Start` / Interval loop (**must not be called** in Step 2)
- Backend POST schema, Fleet GET, frontend
- Heartbeat / users / software / activity / registration

## Responsibilities before vs after

| Component | Before | After Step 2 |
|-----------|--------|----------------|
| `device.Start` | Collect + health | If upload enabled: lifecycle only. If upload disabled: Collect + health (unchanged) |
| `device.Refresh` | Test/future API | Called from `pipeline.Collector.Collect` |
| `deviceupload.Start` | Read `Inventory()`, POST | Host one `RunOnce`; map Pipeline outcome onto existing `deviceupload.Snapshot` + MarkRunning/Failed |
| `Client` | Called from Start | Called only from Uploader adapter (production enabled path) |
| `Pipeline` | Unused | One `tick` via `RunOnce`; scheduler object exists but **never `Start`ed** |
| Fingerprint | Missing | Computed in device Collect; compared only by Pipeline; **not sent as a JSON field on POST** |

`pipeline.New` today requires `Config.Interval > 0` because it always constructs a Scheduler. Step 2 **must** pass a dummy interval (for example `time.Hour`, jitter 0) solely to satisfy `New`. That interval is unused until a later step calls `Pipeline.Start`. Do not add YAML `interval` in Step 2.

---

# Part 4 — Migration tasks

Each task is independently testable and reversible. **Production cutover is Task 5 only.** Tasks 1–4 leave runtime identical if Task 5 is not merged, or can land first with unused APIs.

### Task 2.1 — Fingerprint helper (unused by Start/upload)

| | |
|--|--|
| **Purpose** | Pure function: comparable `device.Info` → `pipeline.Fingerprint` |
| **Files** | **New** `agent/internal/modules/device/fingerprint.go`, `fingerprint_test.go` (same package, not a new package) |
| **Risk** | Including `CollectedAt` makes every sample “changed” later; JSON-map hash is unstable |
| **Verification** | Same Info, different CollectedAt → equal. DiskFreeBytes / CurrentUser / hostname change → unequal. Partial flag change → unequal. SchemaVersion or Kind in the hash material (per contract). No `time.Now` inside the helper |
| **Rollback** | Delete the two files |
| **Expected runtime** | Unchanged (nothing calls it yet) |

Canonical encoding (fixed field order, not `encoding/json` of a map):

`kind`, `schemaVersion`, `partial`, then Info: MachineName, Hostname, OperatingSystem, OSVersion, WindowsBuild, Architecture, CPUModel, CPUCores, MemoryBytes, DiskSizeBytes, DiskFreeBytes, Manufacturer, Model, SerialNumber, BootTime UTC (empty if zero), CurrentUser, AgentVersion.

Hash: SHA-256 hex (or equivalent stable hex). **Exclude CollectedAt.**

### Task 2.2 — `*device.Module` implements `pipeline.Collector`

| | |
|--|--|
| **Purpose** | `Collect` = `Refresh` + envelope + fingerprint |
| **Files** | `device/device.go`, `device/device_test.go` |
| **Risk** | Name confusion: `device.Collector.Collect` returns `Info`; `pipeline.Collector.Collect` on Module returns `pipeline.Snapshot`. Do not add `Collect` on `WindowsCollector` for pipeline |
| **Verification** | `var _ pipeline.Collector = (*device.Module)(nil)`. Fake OS collector: Collect twice → two envelopes, fingerprints ignore CollectedAt. Failed/empty Refresh → not `Uploadable`. `internal/pipeline` still has zero imports of `device` |
| **Rollback** | Remove `Kind`/`Collect` methods; keep Refresh and fingerprint helper |
| **Expected runtime** | Unchanged (`main` still does not use Pipeline) |

Empty / `StateFailed` with empty Info: return Snapshot with empty Fingerprint and nil or empty Payload so `Uploadable()` is false (matches today’s skip-empty upload).

`StatePartial`: Payload = Info, Partial true, non-empty fingerprint if any comparable field exists — still Uploadable (today Partial **does** POST).

### Task 2.3 — Uploader adapter

| | |
|--|--|
| **Purpose** | `pipeline.Uploader` that POSTs today’s DTO |
| **Files** | **New** `deviceupload/uploader.go`, tests in `deviceupload_test.go` |
| **Risk** | Wrong type assert; calling Collect from Upload (forbidden) |
| **Verification** | Fake Transport: one `pipeline.Snapshot{Payload: full Info}` → exactly one POST `/api/v1/devices`; body bytes equal to `RequestFromInventory` of that Info (JSON). Bad payload type → error, zero Send. Identity missing → error, zero Send |
| **Rollback** | Delete adapter; Start still uses Client |
| **Expected runtime** | Unchanged |

`Upload` MUST NOT skip on fingerprint. Pipeline decides skip.

Keep today’s empty-name guard: if Info has no machine_name and no hostname, return error without Send (today Start skips without POST; Uploader error ⇒ no success remember). Prefer **no HTTP** for that case.

### Task 2.4 — `Pipeline.RunOnce` (invocation only)

| | |
|--|--|
| **Purpose** | Invoke existing `tick` once without `scheduler.Start` |
| **Files** | `agent/internal/pipeline/pipeline.go`, `pipeline_test.go` |
| **Risk** | Accidentally calling `scheduler.Start`; changing `tick` |
| **Verification** | Existing pipeline tests still pass. New: `RunOnce` → first upload; second `RunOnce` same fingerprint → **no** second upload; `Pipeline.Start` was **not** called (scheduler not running / no extra ticks after sleep). Panic recovery still works |
| **Rollback** | Remove `RunOnce`; Device still unused |
| **Expected runtime** | Unchanged until Task 2.5 |

**Allowed body (conceptually):** set running/health the same way a first tick would, then `p.tick(ctx)`, then leave `running` false **or** document that RunOnce does not flip `Start`’s running flag. **Must not** call `p.scheduler.Start`. **Must not** edit `tick` logic.

If review forbids **any** edit to `pipeline.go`, Step 2 is blocked: `tick` is unexported and `Start` always starts the scheduler. Do not duplicate `tick` inside `deviceupload`.

### Task 2.5 — Production one-shot host (runtime change)

| | |
|--|--|
| **Purpose** | Replace enabled `deviceupload.Start` POST with `pipeline.RunOnce`. Single collect via that tick |
| **Files** | `deviceupload/deviceupload.go`, `deviceupload_test.go`, `device/device.go` (Start skip collect), `cmd/risklens-agent/main.go` (CollectOnStart) |
| **Risk** | Double collect (Start Refresh **and** Pipeline Collect). Double POST (old Client.Upload **and** RunOnce). Starting `Pipeline.Start` by mistake. Dummy interval accidentally used via `Start` |
| **Verification** | See Part 9. Fake Transport: enabled + identity + collectable Info → **exactly one** `Send`. Disabled → zero Send, zero `RunOnce`. After `Start` returns, wait 2× dummy interval in test → still one Send |
| **Rollback** | Revert Task 2.5 files; keep adapters. Runtime returns to Part 1 |
| **Expected runtime** | One collect, one POST when upload enabled; same JSON mapping; no Interval loop |

`deviceupload.Start` after RunOnce **should** still map outcomes onto today’s module Status for equivalence:

- Upload success → `StateUploaded`, `MarkRunning` (today)
- Upload failure → `StateFailed`, `MarkFailed` (today; isolation retry is a **later** recurring step)
- Collect not uploadable / empty names → `StateSkipped`, `MarkRunning` (today)
- Identity missing → `StateFailed`, `MarkFailed` (today); do not start RunOnce or fail inside Uploader before Send

Do **not** call `Pipeline.Start` in this task.

---

# Part 5 — Files that change

| File | Current responsibility | Future responsibility | Reason |
|------|------------------------|----------------------|--------|
| `device/fingerprint.go` | Does not exist | Hash comparable Info | Pipeline compare; not HTTP |
| `device/fingerprint_test.go` | Does not exist | Hash tests | Proof CollectedAt excluded |
| `device/device.go` | Start Refresh once; Refresh API | + `pipeline.Collector`; Start collect gated by CollectOnStart | One collect when upload owns the tick |
| `device/device_test.go` | Start / Refresh | + Collector tests; Start skip when CollectOnStart false | Proof |
| `deviceupload/uploader.go` | Does not exist | `pipeline.Uploader` | Map Payload → existing Client |
| `deviceupload/deviceupload.go` | One-shot Client.Upload | Enabled path: `pipeline.New` + `RunOnce` | Wire |
| `deviceupload/deviceupload_test.go` | One-shot Start | One Send; no second Send after wait; skip paths | Proof |
| `cmd/risklens-agent/main.go` | `device.Config{Enabled: true}` | Also `CollectOnStart: !DeviceUpload.Enabled` | Avoid double Collect |
| `pipeline/pipeline.go` | `Start` starts scheduler | + `RunOnce` → `tick` | One-shot without scheduler |
| `pipeline/pipeline_test.go` | Scheduler ticks | + RunOnce tests | Proof scheduler not started |

**Not functionally changed (called as today):** `deviceupload/client.go`, `deviceupload/request.go`, `collector_windows.go`, `transport/**`.

---

# Part 6 — Files that must not change

| Area | Paths | Why |
|------|--------|-----|
| Backend ingest | `platform/app/api/v1/devices.py`, `schemas/devices.py`, `services/device_service.py` | Same POST body; upsert already overwrites |
| Fleet / Overview API | `platform/app/api/v1/fleet.py`, `fleet_service.py` | Overview still one GET |
| Database | `models.py` `DeviceInventoryRow`, migrations | No new columns for fingerprint |
| Frontend | `frontend/assets/app.js` | Still Fleet GET; no poll (OV-1.2) |
| Heartbeat | `internal/modules/heartbeat/**` | Own ticker; presence |
| Users / Accounts | `internal/modules/users/**` | Own loop; later collector |
| Software | `internal/modules/software/**` | Own loop |
| Sessions / activity | `internal/modules/activity/**` | Unrelated |
| Registration | `internal/modules/registration/**` | Does not read device Info |
| Transport | `internal/transport/**` | Shared HTTP; Device only POSTs more times in later steps, not Step 2 |
| Policies | fleet policy routes | Assigned policy, not collect |
| Scheduler | `internal/schedule/**` | Frozen Phase A; Pipeline already uses it internally; Step 2 must not `Start` it |
| Pipeline abstractions | `collector.go`, `snapshot.go`, `uploader.go`, `health.go`, **`tick` / `shouldSkip` / `remember`** | Frozen Phase B contracts. Device adapts. Only `RunOnce` wrapper is in scope |
| Identity format | `internal/identity/**` | Same UUIDs in POST |
| YAML model | `DeviceUpload` enabled/timeout only | Recurring interval is a later step |

---

# Part 7 — Behavioural equivalence (HTTP)

## Old (Part 1), upload enabled, identity present, inventory Collected/Partial with a name

```
device.Start → one WindowsCollector.Collect → store Info
deviceupload.Start → RequestFromInventory(id, Info) → one POST
```

## New (after Task 2.5), same preconditions

```
device.Start → no Collect
deviceupload.Start → RunOnce → one Refresh/Collect → same RequestFromInventory → one POST
```

## Payload identity

`Request` JSON is produced **only** by `RequestFromInventory`. Uploader must call that function with `device.Info` from `Snapshot.Payload`. Fingerprint lives on `pipeline.Snapshot` only. It is **not** a field on `Request` and **must not** appear on the wire.

| JSON field | Source | Fingerprint affect POST? |
|------------|--------|---------------------------|
| `agent_id`, `install_id` | Identity manager | No (Uploader stamps; not in fingerprint) |
| `machine_name` … `agent_version`, disks, user, boot_time | `device.Info` | Compare uses them; POST still the raw fields |
| `collected_at` | `Info.CollectedAt` | **Excluded from fingerprint**; POST still sends it |

`collected_at` may differ by milliseconds because Collect moves from `device.Start` to `deviceupload.Start`. All other mapped fields must match what the same OS sample would have produced in Part 1.

Proof in tests (Task 2.3 / 2.5):

1. Build `RequestFromInventory(id, info)` JSON.
2. Run Uploader / RunOnce with `Payload: info`.
3. `Transport.Send` body **byte-equal** (or canonical JSON equal) to (1).

First-run skip cannot fire (`lastSuccess` empty), so fingerprint **does not omit** the POST that Part 1 would send.

Disabled / identity missing / empty inventory: **zero POST**, same as Part 1.

---

# Part 8 — Risks

| Risk | Mitigation |
|------|------------|
| Duplicate POST | Task 2.5: delete the old `Client.Upload` call from Start; only Uploader sends. Test Send count == 1 |
| Double collect | `CollectOnStart: false` when upload enabled; Pipeline Collect is the only Refresh. Test collector call count == 1 |
| Missed upload | First `RunOnce` with empty lastSuccess must POST if Uploadable. Test Send >= 1 for full Info |
| Fingerprint mismatch (always “changed”) | Exclude CollectedAt; tests in 2.1. Not observable on one production tick |
| Fingerprint mismatch (never “changed”) | Include DiskFreeBytes, CurrentUser, etc.; unit tests. Recurring step will catch in prod |
| Incorrect first upload | Empty lastSuccess; do not pre-seed fingerprint. Test |
| Snapshot corruption / stale merge | Refresh **replaces** `device.Snapshot` (Step 1). Payload is Info value copy |
| Pipeline / scheduler started twice | Do not call `Pipeline.Start`. `RunOnce` must no-op or be safe if called twice in tests; production Start calls it once. Double `deviceupload.Start` is already prevented by manager `ErrAlreadyStarted` |
| Dummy interval starts a loop | Never `scheduler.Start`. Test: sleep then Send still 1 |
| Memory leak | One Pipeline, one snapshot slot, no goroutine if RunOnce is synchronous on the Start goroutine (same as today’s POST). Do not leak a scheduler goroutine |
| Race Info vs Collect | Same mutex as Step 1 `setSnapshot` |
| Shutdown during upload | `RunOnce` uses Start’s `ctx`; Transport already honours ctx. `deviceupload.Stop` still no loop to cancel; in-flight POST same as today |
| Agent restart | Memory fingerprint gone; next process first RunOnce POSTs again (same as today’s always-POST-on-start) |
| Backend unavailable | Uploader returns error; Start still `MarkFailed` (today). No retry until a later step starts the scheduler |
| Type-assert panic | Recover already in `tick`; Uploader should return error on wrong type instead of panicking |
| CollectOnStart default wrong | Zero value `false` would skip collect in existing device tests. **Default CollectOnStart true** (or treat zero as true) so current tests stay valid; `main` sets false only when upload enabled |

---

# Part 9 — Verification

### Automated (required)

- `go test ./internal/modules/device ./internal/modules/deviceupload ./internal/pipeline`
- Fingerprint: CollectedAt ignored; content fields not ignored
- `var _ pipeline.Collector = (*device.Module)(nil)`
- `var _ pipeline.Uploader = (*uploader)(nil)` (or named type)
- Fake Transport: **exactly one** POST `/api/v1/devices` on enabled Start
- Body equal to `RequestFromInventory`
- After Start, `time.Sleep` longer than dummy interval → still one POST
- `Pipeline.Start` not used (grep production Start path; test scheduler not running)
- Disabled upload: zero POST, device.Start still Refresh once
- Existing heartbeat/users/software/registration tests still pass if `main` touched
- `git grep` `pipeline.Start` / `scheduler.Start` in deviceupload must be empty

### Manual (after Task 2.5, lab agent)

1. Agent starts.
2. With upload enabled: **one** `POST /api/v1/devices` (proxy/log). No second POST without restart.
3. Overview reload shows the same hardware fields as before Step 2 (same host).
4. Registration and heartbeat still succeed.
5. Backend and frontend git diff empty.
6. Process restart → one POST again (not a skip).

### Explicit non-goals for this verification

- No 5-minute cadence
- No skip of a second production tick (no second tick)
- No Overview auto-refresh

---

# Part 10 — Definition of done

Step 2 is complete only when:

- ☐ Device uploader implements `pipeline.Uploader` (`deviceupload` adapter → existing `Client`)
- ☐ Device collector implements `pipeline.Collector` (`Refresh` + fingerprint + envelope)
- ☐ Device upload-enabled path uses Pipeline **once** (`RunOnce`, not `Pipeline.Start`)
- ☐ Exactly one upload when Part 1 would have uploaded
- ☐ POST payload identical to `RequestFromInventory` (fingerprint not on the wire)
- ☐ Backend unchanged
- ☐ Frontend unchanged
- ☐ Tests pass (Part 9)
- ☐ Manual verification complete (lab)
- ☐ This document + a short note in `DEVICE_INTEGRATION_PLAN.md` that Step 2 (this slice) is done — **documentation update happens with the implementation PR, not now**

Also:

- ☐ `internal/schedule` not started for Device
- ☐ No new YAML keys
- ☐ `internal/pipeline` still does not import `device`

---

## After approval

Implement **Task 2.1 only**, then stop for review, unless the implementer is explicitly told to land 2.1–2.4 together because they have zero production wiring.

**Do not** implement Task 2.5 until adapters and `RunOnce` are approved.

**Do not** enable recurring inventory in the same change.

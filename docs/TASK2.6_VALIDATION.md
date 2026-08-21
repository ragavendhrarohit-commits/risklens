# Task 2.6 — Production Device Pipeline Migration Validation

**Status:** validation only. No production code was changed in this task.

**Scope:** confirm that the one-shot Device upload path now flows through `PipelineHost` → `Pipeline.RunOnce`, and that observable behaviour is equivalent to the previous direct `Client.Upload` path.

**Out of scope:** recurring inventory, `Pipeline.Start()`, scheduler enablement, backend/frontend/database changes, any implementation.

**Verdict:** pass. See Section 6.

---

## How this validation was performed

| Method | Result |
|--------|--------|
| Source trace of the production call chain | Every hop in Section 1 exists in the current tree |
| Static search for `Pipeline.Start` / `scheduler.Start` on the Device path | No production Device caller |
| Unit tests (`device`, `deviceupload`, `pipeline`, `fingerprint`, `schedule`) | All pass |
| Live agent + live Overview session | **Not executed** in this task |

This is an engineering validation of the completed migration. It is not a live operator run against a running agent and Platform API.

---

## Section 1 — Runtime Flow Verification

Target path (upload enabled):

```
main
  → device.Start()
  → SkipStartCollect
  → deviceupload.Start()
  → PipelineHost
  → Pipeline.RunOnce()
  → Collector Adapter
  → Device.Refresh()
  → Fingerprint.Of()
  → Uploader Adapter
  → Client.Upload()
  → HTTP POST /api/v1/devices
  → Backend
  → Overview
```

Each transition below is confirmed in source.

### 1.1 `main` → `device.Start()`

| | |
|--|--|
| **File** | `agent/cmd/risklens-agent/main.go` `buildModules` |
| **Construction** | `device.New(device.Config{Enabled: true, SkipStartCollect: cfg.DeviceUpload.Enabled}, …)` |
| **Then** | `deviceupload.New(…)` with the same `*device.Module` as inventory source |
| **Start** | `app.Run` → `manager.StartAll` starts modules in registry order |

`device` is appended to the module list **before** `deviceupload`. `StartAll` iterates that list, so `device.Start` always runs before `deviceupload.Start`.

**Confirmed.**

### 1.2 `device.Start()` → `SkipStartCollect`

When `cfg.DeviceUpload.Enabled` is true, `SkipStartCollect` is true.

`device.Start` (`agent/internal/modules/device/device.go`):

1. `MarkStarting`
2. If disabled: mark Running, return (no collect)
3. If `SkipStartCollect`: log `"Device inventory collection deferred to pipeline"`, `MarkRunning`, **return without calling `Refresh`**
4. Otherwise: `refresh` once (legacy collect-on-start when upload is off)

`Refresh()` itself is unchanged. Only `Start` gained an early return.

**Confirmed.** One Windows collect is **not** performed here when upload is enabled.

### 1.3 `deviceupload.Start()` → `PipelineHost`

`deviceupload.Start` (`agent/internal/modules/deviceupload/deviceupload.go`):

1. Skip if `Enabled` is false (no host, no POST)
2. Fail if identity or client is missing
3. `NewPipelineHost(pipeline.Config{Interval, Timeout}, inventory, identity, client, logger)`
4. `host.RunOnce(ctx)`
5. `finishFromPipeline`

`NewPipelineHost` (`host.go`) constructs:

- `device.NewPipelineCollector(refresher)` — Collector Adapter
- `NewUploader(identity, client)` — Uploader Adapter
- `pipeline.New(cfg, collector, uploader, logger)` — Pipeline **not started**

`PipelineHost` has `RunOnce`. It has **no** `Start` method.

**Confirmed.**

### 1.4 `PipelineHost` → `Pipeline.RunOnce()`

```go
func (h *PipelineHost) RunOnce(ctx context.Context) {
    h.pipe.RunOnce(ctx)
}
```

Comment and implementation: MUST NOT start the scheduler.

`deviceupload.Start` calls `host.RunOnce(ctx)` and never `host.Pipeline().Start`.

**Confirmed.**

### 1.5 `Pipeline.RunOnce()` → Collector Adapter

`pipeline.RunOnce` (`agent/internal/pipeline/pipeline.go`):

```
snap, collectErr := p.collector.Collect(ctx)
fp := fingerprint.Of(snap.Kind, snap.SchemaVersion, snap.Partial, snap.Payload)
… shouldSkip …
p.uploader.Upload(ctx, snap)
```

Production collector is `*device.PipelineCollector` (wired in `NewPipelineHost`). Tests assert this type.

**Confirmed.**

### 1.6 Collector Adapter → `Device.Refresh()`

`PipelineCollector.Collect` (`agent/internal/modules/device/pipeline_collector.go`):

```
dev := c.source.Refresh(ctx)
```

Maps `device.Snapshot` → `pipeline.Snapshot`. Fingerprint field is left empty. Payload is `device.Info`.

When upload is enabled this is the **only** production `Refresh` on the start path.

**Confirmed.** Exactly one collect for the upload-enabled start path.

### 1.7 `Fingerprint.Of()`

Immediately after Collect, `pipeline.RunOnce` calls:

```
fingerprint.Of(snap.Kind, snap.SchemaVersion, snap.Partial, snap.Payload)
```

On the first cycle `lastSuccess` is empty, so `shouldSkip` is false. The first successful collect always proceeds to upload. Fingerprint does not change JSON, endpoint, or headers.

**Confirmed.** Hashing is pipeline-owned; collectors do not hash.

### 1.8 Uploader Adapter → `Client.Upload()`

`deviceupload.Uploader.Upload` (`uploader.go`):

1. Payload → `device.Info`
2. `identity.Current()`
3. Empty inventory → `ErrEmptyInventory` (no HTTP)
4. `req := RequestFromInventory(id.AgentID, id.InstallID, info)`
5. `u.client.Upload(ctx, req)`

Same request builder as the pre-pipeline path. `Client.Upload` is unchanged.

**Confirmed.**

### 1.9 `Client.Upload()` → `POST /api/v1/devices`

`deviceupload/client.go`:

```
EndpointPath = "/api/v1/devices"
c.transport.BuildRequest(request.MethodPOST, EndpointPath, req)
c.transport.Send(ctx, txReq)
```

Transport headers (`agent/internal/transport/client/client.go`): `Accept: application/json`, `Content-Type: application/json`, `User-Agent: RiskLensAgent[/version]`. Device upload does not add extra headers.

**Confirmed.** Same method, path, JSON body type, and headers as before.

### 1.10 Backend

`platform/app/api/v1/devices.py` `upload_device_inventory`:

- `DeviceInventoryRequest` → `to_command()`
- `DeviceInventoryService.upsert`
- HTTP 201 if created, 200 if updated
- Body: `accepted=True`, `inventory_status="stored"`, plus agent/server metadata

Agent `Client.Upload` still requires `accepted` and `inventory_status == "stored"` for success.

Task 2.6 did not modify `platform/`.

**Confirmed.** Same ingest contract.

### 1.11 Overview

SPA Overview is `GET /api/v1/fleet/{agent_id}` from `frontend/assets/app.js` `loadFleetDetail`. It does not call `POST` or `GET /api/v1/devices`. Fleet refresh is manual (`#fleet-refresh`). Task 2.6 did not modify `frontend/` or fleet APIs.

After a successful POST, Overview still shows the latest `device_inventory` row joined into the fleet record. Same as before.

**Confirmed** at the code-contract level (not a live browser session).

### 1.12 Upload-disabled path (unchanged observable)

When `device_upload.enabled` is false:

- `SkipStartCollect` is false → `device.Start` still collects once
- `deviceupload.Start` skips → no host, no POST

That matches the pre-2.6 disabled configuration.

---

## Section 2 — Behaviour Equivalence

| Claim | Evidence | Result |
|-------|----------|--------|
| Exactly one collection | `SkipStartCollect` prevents `device.Start` collect; `RunOnce` Collect → `Refresh` once. `TestStartSkipStartCollectDoesNotCollect`, `TestPipelineHostRunOnceOneCollectOnePostWithoutScheduler`, `TestModuleStartUsesPipelineRunOnceNotScheduler` (`probe.calls() == 1`) | **Pass** |
| Exactly one upload / HTTP request | Adapter calls `Client.Upload` once per `RunOnce`; tests assert `transport.callCount() == 1` and still 1 after 80ms | **Pass** |
| Same JSON body | `Uploader` uses `RequestFromInventory`. `TestModuleStartSuccessfulUpload` marshals that helper vs the captured transport body and requires byte equality | **Pass** |
| Same endpoint | `EndpointPath` remains `"/api/v1/devices"`; tests require `MethodPOST` and that path | **Pass** |
| Same headers | Unchanged `Client.Upload` + shared transport `BuildRequest` / `Send` | **Pass** |
| Same backend response handling | Unchanged `Client.Upload` status mapping (201/200 stored, 404 unknown agent, 422 validation) | **Pass** |
| Same Overview behaviour | Unchanged `GET /api/v1/fleet/{id}`; ingest still upserts `device_inventory` | **Pass** (code) |

First-cycle fingerprint skip cannot suppress the POST: `lastSuccess` is empty until the first successful upload.

Disabled-upload behaviour remains: one local collect, zero POSTs.

---

## Section 3 — Scheduler Verification

### 3.1 `Pipeline.Start()` is never called (Device production)

Search of `agent/internal/modules/deviceupload` and `agent/cmd`:

- No `pipe.Start`
- No `Pipeline().Start`
- No `host.Start` (type has no `Start`)
- Production call is `host.RunOnce(ctx)` only

`Pipeline.Start` exists on `pipeline.Pipeline` and is used **only** by `agent/internal/pipeline/pipeline_test.go`.

**Proved.**

### 3.2 Device pipeline `Scheduler.Start()` is never called

`pipeline.New` still **constructs** a `schedule.Scheduler` (`schedule.New(..., p.RunOnce)`). Construction does not spawn a goroutine.

`schedule.Scheduler.Start` is the only method that does `go s.loop(ctx)`. It is invoked only from `Pipeline.Start`.

Device production never calls `Pipeline.Start`, therefore Device never calls `scheduler.Start`.

YAML `device_upload` has `enabled` and `timeout` only. There is no Device inventory interval in configuration. `PipelineInterval` on `deviceupload.Config` is a constructor dummy (default 1 hour) so `pipeline.New` can succeed. It is never used to tick.

**Proved.**

### 3.3 No recurring Device goroutine / periodic Device execution

`Pipeline.RunOnce` comment and body: does not start the scheduler, does not spawn a goroutine, does not wait for Interval.

Tests with `Interval: 25ms` then `Sleep(80ms)`:

- `TestPipelineHostDoesNotExecute` — construction: 0 collects, 0 POSTs, health `Disabled`
- `TestPipelineHostRunOnceOneCollectOnePostWithoutScheduler` — after `RunOnce`: still 1 / 1
- `TestModuleStartUsesPipelineRunOnceNotScheduler` — after module `Start`: still 1 / 1
- `TestRunOnceNeverStartsScheduler` (pipeline package)

If the Device scheduler were running, a 25ms interval would produce additional collects/POSTs within 80ms. Counts stay at 1.

**Proved** for the Device pipeline.

### 3.4 Other module schedulers (not Device)

Heartbeat, users, software, and activity still start **their own** package-local schedulers. Those pre-existed Task 2.6 and are not the generic Device `internal/schedule` pipeline. They do not constitute recurring Device inventory.

---

## Section 4 — Regression Audit

Task 2.6 production files (from the completed implementation, not this validation):

| File | Role in 2.6 |
|------|-------------|
| `agent/cmd/risklens-agent/main.go` | `SkipStartCollect` when upload enabled |
| `agent/internal/modules/device/device.go` | `Start` early return only |
| `agent/internal/modules/deviceupload/deviceupload.go` | `Start` → `PipelineHost.RunOnce` |
| `agent/internal/modules/deviceupload/host.go` | `RunOnce` entry |

| Area | Unchanged? | Basis |
|------|------------|--------|
| Registration | Yes | `registration` package not in the 2.6 set; still `POST /api/v1/agents/register`; still starts before upload |
| Heartbeat | Yes | Separate module + scheduler; 2.6 does not import or start it |
| Sessions | Yes | Activity/session upload module not modified |
| Fleet | Yes | `GET /api/v1/fleet` and `fleet_service.py` not in the 2.6 set |
| Overview | Yes | SPA still `GET /api/v1/fleet/{id}` + manual refresh; no auto-poll of inventory |
| Backend APIs | Yes | `platform/app/api/v1/devices.py` ingest unchanged; no new routes |
| Database | Yes | `device_inventory` upsert unchanged; no migration in 2.6 |
| Transport | Yes | Shared `txclient`; Device still POST JSON through `Client.Upload` |
| Identity | Yes | `identity.Current()` still required before upload; file store unchanged |
| Configuration | Yes | `DeviceUpload` remains `{Enabled, Timeout}`; no interval key |
| Users | Yes | `users` module / `POST /api/v1/users` not modified |
| Installed Applications | Yes | `software` module not modified |
| Policies | Yes | Fleet policy tab / backend policy APIs not modified |
| `Device.Refresh()` body | Yes | Same `refresh` implementation; only who calls it changed |
| `RequestFromInventory` / `Client.Upload` | Yes | Uploader delegates; no rewrite |
| Fingerprint algorithm | Yes | `fingerprint.Of` unchanged; newly used by pipeline on this path |
| `internal/schedule` | Yes | Still unused by Device production (`Start` not called) |

---

## Section 5 — Rollback Verification

Rollback is **wiring only**. Do not delete `internal/pipeline`, adapters, `PipelineHost`, or fingerprint. Do not change backend, frontend, or database.

### Restore the previous upload path

**1. `agent/cmd/risklens-agent/main.go`**

Change Device construction back to collect-on-start:

```go
dev, err := device.New(device.Config{
    Enabled: true,
}, version.Version, base)
```

Remove `SkipStartCollect: cfg.DeviceUpload.Enabled`.

**2. `agent/internal/modules/deviceupload/deviceupload.go` `Start`**

After identity and client checks, restore the direct upload:

- Read `m.inventory.Inventory()` / `Info()`
- Skip empty inventory (same skip rules as today)
- `req := RequestFromInventory(id.AgentID, id.InstallID, info)`
- `m.client.Upload(ctx, req)`
- Map success/failure onto `UploadStatus` as before

Do **not** call `NewPipelineHost` or `RunOnce`.

Leave `PipelineHost`, `PipelineCollector`, and `Uploader` in the tree unused.

### What rollback does not require

- No YAML change
- No identity file change
- No database rollback
- No backend/frontend revert
- No removal of `internal/pipeline` or `internal/schedule`
- No fingerprint package removal

### Verification after rollback

Expect the old runtime again: `device.Start` → `Refresh` once; `deviceupload.Start` → one `POST /api/v1/devices`. No `Pipeline.RunOnce` on the production path.

**Rollback instructions are complete** for production wiring.

---

## Section 6 — Production Readiness

### Test results (this validation)

```
ok  risklens/agent/internal/modules/deviceupload
ok  risklens/agent/internal/modules/device
ok  risklens/agent/internal/pipeline
ok  risklens/agent/internal/fingerprint
ok  risklens/agent/internal/schedule
```

### Non-blocking residuals

These do not affect runtime behaviour:

1. Stale comments still describe adapters as unused by production (`pipeline_collector.go`, `uploader.go`) and `device.Start` as always collecting (`device.go`). Comments only.
2. This task did not run a live agent against a live Platform API and browser Overview. Equivalence for Overview/backend is from unchanged APIs plus agent unit tests, not an operator session.
3. Git history was not available on this working copy (`master` has no commits), so “unchanged” for backend/frontend is from inspecting those trees, not `git diff`.

None of these block the one-shot migration.

### Unresolved for OV-1.1 (explicitly not this task)

- Recurring Device inventory is **not** enabled
- `Pipeline.Start()` is **not** called
- No Device collection interval in YAML
- Overview still does not auto-refresh

Those are the next engineering task, not defects in Task 2.6.

### Decision

The Device Pipeline **one-shot** migration is production ready. Observable start-up behaviour matches the previous path: one collect, one `POST /api/v1/devices`, same JSON, same headers, same backend upsert, same Overview read path. The only difference is that upload now flows through the generic Pipeline via `RunOnce`.

The Device Pipeline migration is complete.

Recurring inventory (OV-1.1) is now the next engineering task.

# Collection pipeline contracts

Permanent engineering specification for every recurring collector in RiskLens.

This document is **normative**. Phase B and every later collector must obey it. [PIPELINE_ARCHITECTURE.md](PIPELINE_ARCHITECTURE.md) is the overview. [OV-1.1_DESIGN.md](OV-1.1_DESIGN.md) is the implementation plan. If they disagree, **this contract wins** after approval.

**Status:** contracts only. No pipeline code exists. Phase A (`agent/internal/schedule`) is implemented and unwired. Do not implement Phase B until this document is approved.

Collectors that must follow these contracts:

- Device Inventory (Overview)
- Accounts
- Installed Applications
- Processes
- Services
- Network
- Policies
- Health

The same architecture must also support:

- Browser Activity
- Application Activity
- Other recurring collectors
- Jobs that only need a ticker (for example heartbeat), using the Scheduler without a Collector

Language: **MUST / MUST NOT / SHOULD / MAY** as requirements.

Failure isolation (§12) is a permanent host rule: the agent is a host for independent collectors, not one large collector.

This document does not define Go interfaces. Named operations below are contract operations, not source to paste.

---

## 1. Pipeline overview

```
Collector
    ↓
Snapshot
    ↓
Fingerprint
    ↓
Pipeline
    ↓
Uploader
    ↓
Transport
    ↓
Backend
    ↓
Database
    ↓
Frontend
```

Time is orthogonal: each Pipeline instance **MUST** use `schedule.Scheduler` (Phase A) to fire ticks. The Scheduler is not a data layer. It does not appear as a box between Collector and Snapshot.

### Layer responsibilities

| Layer | Responsibility | Knows | Must not know |
|-------|----------------|-------|---------------|
| **Collector** | Read the local OS (or an in-memory source). Produce one Snapshot, including Fingerprint. | Windows / payload type for its kind | HTTP, backend URLs, SQL, other kinds, retry, schedule |
| **Snapshot** | Immutable envelope for one sample of one kind | Kind, time, fingerprint, payload | Transport, scheduler, database |
| **Fingerprint** | Deterministic identity of comparable content | Payload fields for that kind | Network, clocks as “now”, Pipeline |
| **Pipeline** | On each tick: collect, compare, decide, upload or skip, remember success, wait for next tick | Collector, Uploader, Scheduler, last success fingerprint | `device.Info`, accounts, services, any concrete payload |
| **Uploader** | Send one Snapshot through Transport. Return success or failure | Snapshot payload → HTTP DTO, Transport | Scheduling, collection, fingerprinting, retry policy |
| **Transport** | BuildRequest / Send / DecodeResponse | HTTP client | Collectors, fingerprints, SQL |
| **Backend** | Validate and accept ingest | Agent identity, request schema | Agent scheduler |
| **Database** | Persist latest (or history, if a later feature adds it) | Tables | Agent internals |
| **Frontend** | Display stored facts | Fleet GET / tab APIs | Agent pipeline |

One **Pipeline instance** exists per enabled kind. Device and Processes are two instances, not one shared loop.

The Pipeline package (`agent/internal/pipeline`, not yet created) **MUST NOT** import `device`, `users`, `software`, `activity`, or any concrete payload type.

---

## 2. Collector contract

A Collector gathers one kind of local fact and returns a Snapshot. It is the only layer allowed to talk to Windows (or to derive a Snapshot from another in-memory source, e.g. Health reading the last device Snapshot).

### Responsibilities

A Collector **MUST**:

1. Implement operation **Collect(ctx) → (Snapshot, error)**.
2. Identify itself with a stable **Kind** string (`device`, `users`, `software`, `processes`, `services`, `network`, `policies`, `health`, `browser_activity`, `application_activity`, …). Kind **MUST** be unique across the agent.
3. Honour `ctx`. If cancelled, Collect **MUST** return promptly. It **MUST NOT** start new OS work after cancellation is observed.
4. Prefer a **partial Snapshot plus error** over a totally empty failure when at least one useful field was obtained.
5. Compute the Snapshot’s Fingerprint before returning (or call a pure helper next to the payload type). The Pipeline **MUST NOT** hash payload fields.
6. Set `CollectedAt` to the sample time in UTC.
7. Leave unavailable fields empty or zero according to that kind’s payload rules. It **MUST NOT** invent values (no fake hostname, no fake RAM).

A Collector **MUST NOT**:

- Upload data
- Schedule itself (no ticker, no `time.Sleep` as a substitute for the Scheduler)
- Know backend APIs, URLs, HTTP status codes, or request DTOs
- Know the database
- Retry uploads
- Store transport logic (`net/http`, Transport client)
- Compare to a previous Snapshot (change detection is Pipeline)
- Start or stop other collectors
- Block indefinitely without watching `ctx`

### Inputs

| Input | Meaning |
|-------|---------|
| `ctx` | Cancellation and deadline for this collect only |
| Kind configuration | Enabled flag is **not** the Collector’s job (see Configuration). Collect **MAY** read kind-specific collect options (which volumes, timeout for WMI, etc.) injected at construction |
| Clock | Injected or `time.Now` for `CollectedAt` only |

Collect **MUST NOT** take a previous Snapshot as input for “diff against last upload”. That is Pipeline state.

### Outputs

| Output | Meaning |
|--------|---------|
| Snapshot | Complete envelope: Kind, CollectedAt, Fingerprint, Payload, Partial flag, optional collect error string |
| error | Total failure (no usable Snapshot) **or** accompanying partial result |

If Payload is empty and no field was collected, Collect **MUST** return an error and the Pipeline **MUST NOT** upload.

### Lifecycle

1. Constructed when the agent module for that kind is built (`main` / module `New`).
2. Does not run until the Pipeline’s Scheduler fires Collect.
3. Lives until module Stop. Stop **MUST NOT** be required for a clean Collect if `ctx` is already cancelled.
4. Construction **MAY** fail if dependencies are missing; a failed construct **MUST NOT** start a Pipeline for that kind.

### Thread safety

- Collect **MAY** be called only from that kind’s Pipeline tick (Scheduler already forbids overlap **for that instance**).
- Collect **MUST** still be safe if a test or a future reader calls it concurrently, or it **MUST** document “not concurrent” and the Pipeline **MUST** be the only caller.
- If the Collector publishes latest payload to other modules (today: `device.Info()`), that publish **MUST** use a mutex. Readers **MUST NOT** mutate the published value.

### Error handling

| Collect result | Pipeline obligation |
|----------------|---------------------|
| Error, empty Snapshot | No upload. Do not update last successful fingerprint. Stay running. Next tick collects again. |
| Error, partial Snapshot | Treat as a Snapshot. Upload if fingerprint/pending requires it. Record Partial. |
| Success, empty comparable payload | No upload (same as empty). |
| `ctx` cancelled | Return without treating cancellation as a payload failure to persist. |

The Collector **MUST NOT** mark the whole agent failed. Process liveness is the module manager’s concern. A kind **MAY** set its own module health to Degraded on repeated collect errors; it **MUST NOT** stop other kinds.

### Configuration

| Concern | Owner |
|---------|--------|
| Kind enabled / disabled | Module / `main` (do not construct or Start the Pipeline) |
| Collect interval and jitter | Pipeline / Scheduler config for that kind |
| Collect timeout | **SHOULD** be derived from `ctx` deadline set by the Pipeline tick, or a kind-specific collect timeout injected into the Collector |
| Payload options (e.g. system drive only) | Kind config, read by Collector only |

### Requirements checklist

- Deterministic payload for the same OS state (aside from excluded fingerprint fields).
- No network I/O.
- No knowledge of other Kinds.
- Tests **MUST** inject a fake Collector; Windows **MUST NOT** be required to unit-test the Pipeline.

---

## 3. Snapshot contract

A Snapshot is the unit of collection. It is **not** module diagnostic state (`device.Snapshot` / `deviceupload.Snapshot` today). Those names are status machines. Pipeline Snapshots are data.

### Conceptual contents

| Field | Required | In Fingerprint? |
|-------|----------|-----------------|
| Kind | Yes | Yes |
| SchemaVersion | Yes | Yes |
| CollectedAt | Yes | **No** |
| Fingerprint | Yes | n/a (it *is* the fingerprint) |
| Payload | Yes (may be empty only on total failure, which is not uploaded) | Payload **content** is hashed; the envelope does not re-hash itself |
| Partial | Yes | Yes (complete vs partial is a real difference) |
| CollectError | No | **No** (message text must not change identity) |

SchemaVersion is the Snapshot envelope version (start at `1`). A change to envelope fields bumps it. Payload-type versioning **MAY** live inside Payload; if it affects comparable content it **MUST** affect the Fingerprint.

### Immutable

After Collect returns, the Snapshot **MUST NOT** be mutated. Pipeline, Uploader, and tests **MUST** treat it as read-only. If a copy is required, copy explicitly.

### Serializable

A Snapshot **MUST** be representable as JSON for logs, tests, and (later) disk queues. Payload **MUST** be JSON-serializable for that kind’s HTTP DTO. Serialization **MUST NOT** be required to perform Fingerprint (hash **MUST** use a canonical encoding defined by the Collector, not `encoding/json` of a map with unstable key order).

### Versionable

Envelope SchemaVersion **MUST** exist so collectors can evolve without silent compare bugs. If SchemaVersion changes, Fingerprint **MUST** change, which **MUST** cause an upload.

### Deterministic

Two Collects of the same comparable OS state **MUST** produce the same Fingerprint. They **MUST NOT** be required to produce the same CollectedAt or the same CollectError string.

### No transport logic

Snapshot **MUST NOT** contain HTTP status, request IDs, backend URLs, or retry counts.

### No scheduler logic

Snapshot **MUST NOT** contain interval, jitter, next-run time, or tick count.

### No backend knowledge

Snapshot **MUST NOT** contain SQL, table names, or server-side IDs except those that originated on the endpoint (e.g. agent_id **MAY** be added by the Uploader, not by Collect, unless the Collector is given identity solely to stamp Payload — **SHOULD** prefer Uploader stamping identity).

Recommended: Collector Payload is OS facts only. Uploader adds `agent_id` / `install_id`.

### Can be fingerprinted

Every uploadable Snapshot **MUST** carry a Fingerprint computed before Collect returns.

### Can be compared

Pipeline compares **only** Fingerprint (and the pending-upload flag). It **MUST NOT** deep-equal Payloads.

---

## 4. Fingerprint contract

Fingerprint is the comparable identity of a Snapshot’s content.

### Deterministic

Same comparable fields → same Fingerprint, always, on every agent build that uses that Collector version.

### Stable ordering

Lists (accounts, applications, processes, services) **MUST** be sorted by a documented key before hashing. Maps **MUST NOT** be hashed with randomized iteration. Use a defined field order.

### Ignore timestamps

**MUST** exclude:

- `CollectedAt`
- Receive time
- Transport timestamps
- CollectError text
- Any “last seen” clock that is not an OS fact (boot time **is** an OS fact and **MUST** be included for device)

### Ignore transient fields

**MUST** exclude fields that change every sample without operator meaning, if a kind has them (e.g. per-sample CPU percentage if that belongs to Health, not to process identity). Each kind **MUST** document included vs excluded fields.

**MUST** include fields operators treat as inventory change: disk free, current user, hostname, installed app set, process image set, etc.

### Pure function

`Fingerprint(payload, kind, schemaVersion, partial) → value`

- **MUST NOT** read time
- **MUST NOT** read random
- **MUST NOT** perform network calls
- **MUST NOT** write files, logs as part of the hash (logging the result is allowed **after** return)
- **MUST NOT** have side effects

### Ownership

The hash algorithm **MAY** live beside the payload type (e.g. device package). The Pipeline **MUST** only see an opaque string (or equivalent). Pipeline **MUST NOT** import payload structs to hash them.

### Equality

Two Fingerprints are equal iff their opaque values are equal. No prefix compare, no version sniffing inside the Pipeline.

---

## 5. Pipeline contract

The Pipeline coordinates one kind. It is generic orchestration only.

### It MUST

On each Scheduler tick:

1. **Collect** — call Collector.Collect(ctx)
2. **Compare** — compare Snapshot.Fingerprint to last **successfully uploaded** fingerprint
3. **Decide**
   - Unusable / empty collect → skip upload, do not update last success
   - Fingerprint equal **and** not pending → skip upload
   - First run (no last success), Fingerprint different, or pending → upload
4. **Upload** — call Uploader.Upload(ctx, Snapshot) when decided
5. **Remember** — on upload success, store that Fingerprint as last success and clear pending. On upload failure, set pending and **MUST NOT** replace last success
6. **Schedule next execution** — by returning to `schedule.Scheduler` (the Pipeline **MUST NOT** implement its own ticker)

Immediate first tick is the Scheduler’s contract. The first Pipeline tick **MUST** upload if Collect produced a usable Snapshot (empty last success).

### It MUST NOT

- Know Device Inventory, Accounts, Installed Applications, Processes, Services, Network, Policies, Health, Browser Activity, or any other concrete kind
- Import `device.Info` or any payload struct
- Call Win32
- Own `net/http`
- Own a private ticker
- Implement retry backoff (see §8; next-tick pending is the only OV-1.1 retry)
- Persist fingerprints to disk (OV-1.1)
- Stop other Pipeline instances
- Deep-compare Payloads

### Interfaces only

Pipeline **MUST** depend on:

- Collector (Collect)
- Uploader (Upload)
- Scheduler (Start/Stop of that instance)
- Opaque Fingerprint values

Construction **MAY** take Kind only as a string for logs. Kind **MUST NOT** be switched on inside Pipeline logic.

### State (in memory, per instance)

| State | Meaning |
|-------|---------|
| lastSuccessfulFingerprint | Empty until first successful upload |
| pendingUpload | True if last upload failed or never succeeded after a usable collect |

After process restart, both **MUST** be empty → first tick uploads again.

### One instance per kind

`Pipeline(device)` and `Pipeline(processes)` **MUST** be separate objects with separate Schedulers and separate state.

---

## 6. Uploader contract

The Uploader sends one Snapshot to the backend through Transport.

### Responsibilities

- Map Snapshot.Payload (+ identity) to the HTTP JSON DTO for that kind
- Call Transport (BuildRequest, Send, DecodeResponse)
- Return **success** or **failure** to the Pipeline
- Honour `ctx` (timeout / cancel)

### It MUST NOT

- Schedule (no interval, no jitter, no Start/Stop loop)
- Collect from Windows
- Compute or compare Fingerprints
- Decide skip vs upload
- Own retry policy (no inner retry loop, no disk queue in OV-1.1)
- Mutate the Snapshot
- Swallow errors (failure **MUST** be returned)

### Success and failure

| Outcome | Pipeline treats as |
|---------|-------------------|
| HTTP 200 / 201 / 204 with accepted body as defined by that API | Success |
| Transport error (timeout, connection refused, DNS) | Failure |
| HTTP 4xx / 5xx (except a kind-specific documented success, none today for inventory POSTs) | Failure |
| `ctx` cancelled | Failure that **MUST NOT** be remembered as pending across process exit; in-process, pending **MAY** remain if the agent is still running |

Uploader **MUST NOT** interpret “unchanged snapshot” — it always sends if called.

Identity (agent_id, install_id) **SHOULD** be applied here, not in the Collector.

---

## 7. Scheduler contract

**Implemented (Phase A).** Package: `agent/internal/schedule`. Reference implementation: `Scheduler` with `Config.Interval` and `Config.Jitter`.

The Scheduler **MUST** remain the standard time mechanism for every recurring collector and for ticker-only jobs (heartbeat).

### Immediate execution

The first `Func` **MUST** run as soon as `Start` is called. It **MUST NOT** wait for Interval. It **MUST NOT** apply jitter.

### Interval

Base wait between later ticks. **MUST** be `> 0`. Each kind **MUST** be allowed its own Interval.

### Jitter

After the first run, each wait **MUST** be `Interval + offset` with `offset ∈ [-Jitter, +Jitter]`. Jitter `0` **MUST** mean exact Interval. Wait **MUST** be clamped to at least 1ms. First tick **MUST NOT** jitter.

### No overlap

If `Func` is still running when the next wait elapses, that tick **MUST** be skipped. Ticks **MUST NOT** queue.

### Graceful stop

`Stop` **MUST** cancel the loop and wait for the loop goroutine to exit. It **MUST** be idempotent. It **MUST NOT** kill an in-flight `Func`; that `Func` **MUST** see `ctx` cancel and return.

### Context cancellation

Parent context cancel **MUST** stop the loop and **MUST** be visible to an in-flight `Func`.

### Thread safety

`Start` while running **MUST** be a no-op. Start/Stop **MUST** be safe for concurrent callers.

### Isolation

The Scheduler **MUST NOT** know Collector, Snapshot, Fingerprint, Uploader, Transport, or HTTP.

Pipeline **MUST** use this Scheduler. Pipeline **MUST NOT** copy a private ticker.

Heartbeat **MAY** keep its current private ticker until it is migrated; new work **MUST** use `internal/schedule`.

---

## 8. Failure model

| Event | Collector | Pipeline | Uploader | Process / other kinds |
|-------|-----------|----------|----------|------------------------|
| **Collection failure** (empty) | Return error | No upload; last success unchanged; next tick retries collect | Not called | Other kinds continue |
| **Collection partial** | Snapshot + error | Upload if fingerprint/pending requires | Called if decided | Other kinds continue |
| **Upload failure** | Unchanged | `pendingUpload = true`; last success unchanged | Return failure | Other kinds continue |
| **Fingerprint failure** (cannot hash) | Treat as collect failure; **MUST NOT** return a Snapshot without Fingerprint if uploadable | No upload | Not called | Other kinds continue |
| **Cancellation** (`ctx`) | Abort Collect | Abort tick; do not record a new success | Abort Send | Stop of one kind does not cancel others unless the agent is shutting down |
| **Shutdown** | Collect/Upload abort via ctx | Stop Scheduler; in-memory state discarded | Abort | Module manager StopAll reverse order |
| **Restart** | New process | Empty last success and pending; first tick uploads | New client | Same as cold start |
| **Offline backend** | Still collects | Upload fails → pending | Transport error | Heartbeat fails independently |
| **Network timeout** | n/a | Same as upload failure | Transport timeout | Isolated to that Uploader |
| **Retry** | Next collect tick | OV-1.1: next Scheduler tick + pending. **MUST NOT** busy-loop. Faster retry / disk queue is OV-2.1, still **per kind**, still not inside Uploader | No retry loop | |

Poison payloads (HTTP 422 forever): OV-1.1 **MUST** still set pending and retry next tick, and **MUST** log. Quarantine is not in this contract until a later OV item.

A failure in one kind **MUST NOT** fail the agent process and **MUST NOT** stop other Pipelines. Binding isolation rules, per-collector health, panic recovery, and observability are in **§12**.

---

## 9. Concurrency model

### Can multiple collectors run simultaneously?

**Yes.** Each enabled kind has its own Pipeline and its own Scheduler goroutine. Device collect and process collect **MAY** run at the same time.

### How is contention avoided?

- **No global collect lock** in this contract.
- Overlap is prevented **per kind** by that kind’s Scheduler (`inflight`).
- Collectors **MUST NOT** share mutable OS handles unsafely; if two kinds would contend on one API, that is a Collector implementation concern (document and serialize **inside those Collectors**), not a Pipeline-wide mutex.
- Transport clients **SHOULD** be safe for concurrent use across kinds (existing HTTP client). Kinds **MUST NOT** share a Pipeline instance.

A global `performance.max_concurrency` cap is **out of this contract** until a later performance OV. Default: unlimited parallel kinds.

### Can one collector fail without stopping others?

**Yes.** Required. See §8 and **§12**.

### If upload blocks for five minutes, does only that collector wait?

**Only that kind waits.** Its Scheduler skips ticks while `Func` (collect+upload) is in flight. Other kinds’ Schedulers are unaffected. See **§12 rule 3**.

The Pipeline **MUST** pass a `ctx` with a deadline **SHOULD** so a five-minute block is a bug (timeout **SHOULD** be the kind’s upload timeout, already in YAML for users/software/device_upload). Uploader **MUST** honour that deadline.

### Can collectors have different intervals?

**Yes. Required.**

| Example job | Typical interval | Notes |
|-------------|------------------|--------|
| Heartbeat | 30s | Ticker-only; **MAY** use Scheduler without Collector/Pipeline |
| Device Inventory | 5m | Pipeline instance |
| Processes | 1m | Pipeline instance |
| Software | 30m | Pipeline instance (today 15m in YAML; the contract allows any valid duration per kind) |

Intervals **MUST NOT** be a single global `inventory.interval`.

### Can collectors be enabled or disabled independently?

**Yes. Required.** Existing YAML already has `device_upload.enabled`, `users.enabled`, `software.enabled`, `heartbeat.enabled`, `activity.enabled`. A disabled kind **MUST NOT** Start its Pipeline. Other kinds **MUST** run unchanged.

Heartbeat **MAY** stay enabled while Device is disabled, and the reverse.

---

## 10. Extension rules

To add a new collector (example: Services):

1. **Implement Collector** — OS gather + Fingerprint helper + Kind `services`.
2. **Produce Snapshot** — envelope fields in §3. No custom Pipeline type.
3. **Implement Uploader** — DTO + Transport path (e.g. `POST /api/v1/services`).
4. **Register** — construct `Pipeline` with that Collector, Uploader, and a `schedule.Scheduler` in `main` / module `New`. Register the module with the module manager as today.
5. **Configure interval** — kind YAML: `enabled`, `timeout`, `interval` (and optional jitter later). Validate bounds in configuration validation.
6. **Backend / database / frontend** — new ingest and UI as that feature’s own OV. The agent Pipeline **MUST NOT** change.

**No Pipeline source changes** are required for a new kind, except adding the Kind string to docs/logs. A Kind switch inside Pipeline **MUST NOT** appear.

**No Scheduler source changes** are required.

Event-driven work (logon/logoff) **MAY** call Uploader directly or enqueue, and **MAY** still use Scheduler only for a flush beat. That job **MAY** skip Fingerprint compare. It **MUST NOT** fork a second ticker library.

---

## 11. Architecture principles

These rules are permanent.

### Single responsibility

| Component | One job |
|-----------|---------|
| Collector | Gather |
| Snapshot | Represent one sample |
| Fingerprint | Identify comparable content |
| Pipeline | Orchestrate |
| Uploader | Send |
| Scheduler | Time |
| Transport | HTTP |
| Backend | Accept |
| Database | Persist |
| Frontend | Display |

### Dependency inversion

Pipeline **MUST** depend on Collector and Uploader abstractions, not on device or users packages. Concrete Collectors depend on OS APIs. Concrete Uploaders depend on Transport. Direction of dependency: inward toward Pipeline contracts, never Pipeline toward Win32.

### Open/closed

Open for new kinds (add Collector + Uploader + config). Closed for modification of Pipeline and Scheduler when a kind is added.

### Composition over inheritance

A kind **MUST** be a Collector + Uploader + Scheduler + config composed at module init. **MUST NOT** be a subclass of Pipeline.

### No concrete collector dependencies

`internal/pipeline` **MUST NOT** import concrete collector packages. Fingerprint helpers **MUST** live with the payload type.

### Generic orchestration only

Pipeline logic **MUST** be identical for Device and Processes. The only per-kind data is injected collaborators and config.

### Isolation of failure and time

Failure isolation and independent intervals are not optimizations; they are contract. A slow or dead kind **MUST NOT** stall the agent’s other kinds. The agent is a **host for independent collectors**, not one large collector. Full rules: **§12**.

### One-shot is forbidden for new recurring collectors

New recurring collectors **MUST** use this pipeline (or Scheduler-only for presence). They **MUST NOT** copy heartbeat’s private ticker or users’ private scheduler.

Existing users/software/activity/heartbeat **MAY** remain until their feature is production-hardened; when touched, they **MUST** migrate to these contracts.

### Agent does not own the UI clock

Frontend refresh (OV-1.2) is not part of this pipeline. A successful upload **MUST NOT** require the SPA to be open.

---

## 12. Failure isolation contract

Permanent architectural rule for every collector, including:

- Device Inventory
- Accounts
- Installed Applications
- Processes
- Services
- Network
- Policies
- Health
- Browser Activity
- Application Activity
- Heartbeat and any other ticker-only job

Ticker-only jobs (heartbeat) **MUST** obey the same isolation rules even when they do not use Collector/Snapshot/Fingerprint.

### Principle

The RiskLens agent is a **host for multiple independent collectors**. It is **not** one large collector.

Every collector **MUST** behave like an independent service inside the agent process.

A failure in one collector **MUST NEVER** stop the monitoring capabilities of the others.

### Rule 1 — Failure MUST NOT stop other collectors

Failure in one collector **MUST NOT** stop any other collector, **MUST NOT** stop heartbeat, and **MUST NOT** terminate the agent process.

**Allowed example** (Processes upload fails):

| Component | State |
|-----------|--------|
| Processes | Warning (or Error) |
| Heartbeat | Running / Healthy |
| Device Inventory | Running / Healthy |
| Accounts | Running / Healthy |
| Services | Running / Healthy |
| Network | Running / Healthy |
| Agent process | Running |

**Not allowed:**

```
Processes upload fails
  → entire agent stops
  → heartbeat stops
  → Fleet shows Offline
```

An Offline Fleet presence **MUST** mean heartbeat (or equivalent last_seen) stopped, not that an inventory upload failed.

Module `Start` **MUST NOT** return an error to the module manager solely because a collect or upload failed. A kind **MAY** mark itself Failed/Error; it **MUST NOT** cause StartAll rollback or process exit.

Host-level fatals remain allowed only for true host failures (cannot load config, cannot open logger, cannot load identity). Those are not collector failures.

### Rule 2 — Each collector owns its own runtime

Each collector **MUST** own:

| Owned state | Meaning |
|-------------|---------|
| Scheduler | Its own `schedule.Scheduler` instance and interval |
| Context | Tick `ctx` derived for that instance; cancel **MUST NOT** cancel other kinds |
| Retry state | Pending upload / next-tick retry for that kind only |
| Fingerprint state | Last successful fingerprint for that kind only |
| Upload state | In-flight upload, last HTTP result, last error for that kind only |
| Health state | Healthy / Warning / Error / Disabled for that kind only |

No collector **MAY** read or write another collector’s runtime state (scheduler, ctx, pending, fingerprint, upload, health).

Sharing **read-only published snapshots** (e.g. Health reading last device Snapshot) is allowed only as a documented Collector input. That **MUST NOT** include retry flags, scheduler status, or health of the source kind. If the source kind is failed or empty, the consumer **MUST** degrade itself, not stop the host.

Kinds **MUST NOT** share a Pipeline instance or a Scheduler instance.

### Rule 3 — A blocked upload blocks only that collector

If Processes upload hangs for five minutes:

- Heartbeat **MUST** continue
- Device Inventory **MUST** continue
- Accounts **MUST** continue
- Every other kind’s Scheduler **MUST NOT** be delayed

Only the blocked kind’s Scheduler **MAY** skip ticks while its collect/upload `Func` is in flight.

Upload **SHOULD** use the kind’s timeout so a hang becomes a failure instead of an unbounded block. Timeout expiry **MUST** still isolate to that kind.

### Rule 4 — Panic and unexpected error are recovered locally

A collector panic or unexpected error **MUST** be recovered locally.

One collector crashing **MUST NEVER** terminate the RiskLens agent.

After recovery that kind **MUST**:

- Log `Collector Recovered` (see Observability)
- Set its health to Error (or Warning if the contract for that kind says so)
- Remain scheduled for the next tick unless it is Disabled
- Leave every other collector untouched

The host **MUST NOT** `os.Exit`, fail the Windows service, or cancel the process root context because a collector panicked.

Agent shutdown / SCM stop is the only path that stops all collectors together.

### Rule 5 — Collector health is independent

Every collector **MUST** report exactly one of:

| Health | Meaning |
|--------|---------|
| **Healthy** | Last collect/upload cycle succeeded (or skipped as unchanged after a known-good upload) |
| **Warning** | Degraded: partial collect, upload retry pending, or non-fatal repeated errors |
| **Error** | Collect or upload failed and the kind is not producing usable data |
| **Disabled** | Kind is off in configuration; Pipeline not started |

These values are **per collector**. They are not the Windows service state.

**Agent process status** (Running) **MUST** stay Running while the host is up, even if one or more collectors are Warning or Error.

**Overall Agent health** is **derived** from all collector states:

- Disabled collectors **MUST** be excluded from aggregation
- If every enabled collector is Disabled or none are enabled, Agent health **MAY** be Warning (misconfiguration), not a process crash
- Otherwise Agent health **MUST** be the worst enabled collector: Error > Warning > Healthy

Fleet Online/Offline **MUST** continue to come from heartbeat last_seen, not from derived Agent health and not from a single inventory kind.

### Observability

Each collector **MUST** log independently. Log lines **MUST** include the collector Kind so streams can be filtered.

Required events (names are contract; wording **MAY** match existing slog style):

| Event | When |
|-------|------|
| Collector Started | Pipeline/Scheduler started for that kind |
| Collector Finished | A collect+decide+upload (or skip) cycle completed |
| Collection Failed | Collect returned unusable result |
| Upload Started | Uploader called |
| Upload Failed | Uploader returned failure |
| Retry Scheduled | pending set; next tick will retry |
| Collector Recovered | Panic or unexpected error caught; kind still hosted |
| Collector Disabled | Kind not started because configuration disabled it |

A kind **MUST NOT** rely on another kind’s logs to explain its own failures.

### Summary

| MUST | MUST NOT |
|------|----------|
| Isolate scheduler, ctx, retry, fingerprint, upload, health per kind | Share runtime state across kinds |
| Keep agent and heartbeat running when one kind fails | Stop the process or heartbeat because an upload failed |
| Recover panics inside the kind | Let a collector panic kill the host |
| Report per-kind Healthy / Warning / Error / Disabled | Treat one kind’s Error as Fleet Offline |
| Log per-kind lifecycle | Mix kinds into one undifferentiated log stream with no Kind |

---

## Normative references

| Document | Role |
|----------|------|
| This file | Binding contracts |
| [PIPELINE_ARCHITECTURE.md](PIPELINE_ARCHITECTURE.md) | Narrative overview; defer to this file on conflict |
| [OV-1.1_DESIGN.md](OV-1.1_DESIGN.md) | Phased implementation |
| `agent/internal/schedule` | Scheduler implementation (Phase A) |

## Out of scope until a later approved OV

- Recurring device upload (Phase C+)
- Disk fingerprint persist
- Upload retry faster than the kind interval
- Global collect concurrency cap
- Migrating users, software, activity, or heartbeat

After approval, Phase B implements Pipeline/Collector/Uploader **abstractions and tests only**, still without changing Device runtime behaviour, unless a later instruction explicitly wires deviceupload.

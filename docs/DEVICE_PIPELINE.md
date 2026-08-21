# Device Pipeline ownership and health

Normative rules for the Device Inventory pipeline host. Generic orchestration stays in [PIPELINE_CONTRACT.md](PIPELINE_CONTRACT.md) and [PIPELINE_ARCHITECTURE.md](PIPELINE_ARCHITECTURE.md). Hashing stays in [FINGERPRINT_CONTRACT.md](FINGERPRINT_CONTRACT.md).

This document describes **what the Device code does**. Pipeline and Fingerprint packages are not Device-aware.

---

## 1. Payload ownership

Device owns both payload views. Adapters only translate interfaces.

| View | Owner | Method | Contents |
|------|--------|--------|----------|
| **Comparable payload** | `device.Info` | `Comparable()` | OS inventory facts. `CollectedAt` cleared. |
| **Transport payload** | `device.Info` | `ForTransport(sampleTime)` | Facts plus sample time for `POST /api/v1/devices`. |
| **Envelope time** | `device.Snapshot` | `SampleTime()` | Collect timestamp for `pipeline.Snapshot.CollectedAt`. |

`pipeline.Snapshot.Payload` **MUST** be `Snapshot.ComparablePayload()` (`Info.Comparable()`). Fingerprint hashing therefore never sees the sample clock.

`deviceupload.Uploader` **MUST** call `Info.ForTransport(snap.CollectedAt)` then `RequestFromInventory`. It **MUST NOT** special-case Device fields.

### Adapter duties (interface translation only)

**Collector adapter** (`device.PipelineCollector`):

1. `Refresh`
2. Copy `Kind`, `SchemaVersion`, `SampleTime()`, `ComparablePayload()`, Partial, CollectError onto `pipeline.Snapshot`
3. Do not hash, upload, schedule, or filter individual `Info` fields

**Uploader adapter** (`deviceupload.Uploader`):

1. Assert payload is `device.Info`
2. Ask Device for the transport view
3. `Client.Upload`

Neither adapter may decide that `CollectedAt` is excluded from fingerprinting. That rule lives on `Info.Comparable()`.

```
WindowsCollector.Collect
        ↓
device.Refresh  (stores full Info, including CollectedAt)
        ↓
PipelineCollector          Snapshot.Payload = Comparable()
        ↓                  Snapshot.CollectedAt = SampleTime()
fingerprint.Of(payload)
        ↓
Uploader                   Info.ForTransport(envelope time)
        ↓
POST /api/v1/devices
```

---

## 2. Collector health vs upload health

These are separate. HTTP failure is not a collection failure.

| Signal | Where | Healthy means | Warning / degraded means | Failed / unhealthy means |
|--------|--------|---------------|--------------------------|---------------------------|
| **Collection health** | `device.CollectionHealth()` and `device.Health()` | Last Refresh produced a usable inventory | Partial inventory | Collect failed or empty |
| **Upload health** | `deviceupload.UploadHealth()` | Last POST succeeded | Last POST failed (retry next interval) | not used for HTTP errors |
| **Overall Device** | `deviceupload.Health()` | Both healthy | Worst of collection and upload | Collection unhealthy |

Required outcome when collection succeeds and HTTP fails:

- Collector = Healthy
- Upload = Warning (reported as `interfaces.HealthDegraded`)
- Overall Device = Warning (`HealthDegraded`)
- Module **Status stays Running**. Do not `MarkFailed` because HTTP failed.

`pipeline.Health()` remains mixed tick health inside the generic orchestrator and **MUST NOT** be treated as Device collection health. Device does not modify Pipeline to split those values.

The host observes collect/upload through adapter wrappers (`HealthObserver`). Wrappers do not change Pipeline.

---

## 3. What must not change for later collectors

Accounts, Applications, and other kinds **MUST** follow the same split:

- Kind module owns comparable vs transport payload
- Collector adapter only maps snapshot → `pipeline.Snapshot`
- Uploader adapter only maps payload → HTTP DTO
- Collection health independent of upload health

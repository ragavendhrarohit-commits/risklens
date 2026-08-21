# Fingerprint service contract

**Status:** frozen public API. Based on `agent/internal/fingerprint` as implemented after Task 2.2.

This document is **normative**. If [PIPELINE_CONTRACT.md](PIPELINE_CONTRACT.md) disagrees on **who hashes**, this document wins: hashing belongs only to package `fingerprint`.

Language: **MUST / MUST NOT / SHOULD / MAY**.

Do not implement collectors against an assumed API. Depend only on what this file freezes.

---

## 1. Purpose

Fingerprint is the comparable identity of one collector sample. It answers: *has comparable content changed since the last successful upload?*

```
Collector
    ↓
Snapshot          (payload + envelope; no hash)
    ↓
fingerprint.Of    (canonicalise + hash)
    ↓
Fingerprint       (opaque value object)
    ↓
Pipeline          (Equal only; skip or upload)
    ↓
Uploader          (sends Payload; does not hash)
```

### Why collectors never hash

A collector’s job is to read the OS (or a test fixture) and return a `pipeline.Snapshot`. If each kind hashed itself:

- SHA-256 vs another algorithm would be copied into every collector.
- Map key order and JSON number rules would drift between Device, Accounts, and Software.
- Pipeline would have to trust a collector-supplied `Snapshot.Fingerprint` that it cannot verify.

`pipeline.Collector.Collect` **MUST NOT** compute a hash. `Snapshot.Fingerprint` on a Collect result is **not** an input to comparison. Pipeline **MUST** ignore it (`Pipeline.RunOnce` calls `fingerprint.Of`, it does not read `snap.Fingerprint`).

### Why Pipeline never hashes payloads

Pipeline **MUST NOT** know payload structure (`device.Info`, account lists, …). If Pipeline hashed, it would import concrete types or special-case kinds.

Pipeline **MUST** call `fingerprint.Of(kind, schemaVersion, partial, payload)` and then use only `Empty` / `Equal`. It **MUST NOT** deep-equal payloads, inspect `Value` bits, or choose an algorithm.

Package `fingerprint` **MUST NOT** import `pipeline`, `device`, transport, or HTTP.

Current call site (`agent/internal/pipeline/pipeline.go`):

```text
fp := fingerprint.Of(snap.Kind, snap.SchemaVersion, snap.Partial, snap.Payload)
```

`CollectedAt`, `CollectError`, and `snap.Fingerprint` are **not** passed in.

---

## 2. Fingerprint contract

### Fingerprint

Go type (frozen):

```go
type Fingerprint struct {
    Value string
}
```

Package: `risklens/agent/internal/fingerprint`.

Pipeline re-exports this type as `pipeline.Fingerprint` (`type Fingerprint = fingerprint.Fingerprint`). That alias **MUST** remain the same type, not a second definition.

`Value` is opaque. Callers **MUST NOT** parse it (no algorithm prefix, no version sniffing). Pipeline **MUST** treat two fingerprints as the same if and only if `Equal` is true.

The hash algorithm (today: SHA-256, hex-encoded) is an **implementation detail of `Of`**. It is **not** a field on `Fingerprint`. Replacing the algorithm **MUST NOT** change this struct or `Of`’s signature.

### Responsibilities

| Actor | MUST | MUST NOT |
|-------|------|----------|
| **`fingerprint.Of`** | Canonicalise inputs; return a `Fingerprint`; be a pure function | Import collectors; dial network; read time; read random; write files |
| **Collector** | Produce Kind, SchemaVersion, Partial, Payload, CollectedAt | Hash; call `Of` (Pipeline does that); put transport metadata in Payload |
| **Pipeline** | Call `Of`; `shouldSkip` / `remember` using `Equal` | Hash; interpret `Value`; skip based on payload deep-equal |
| **Uploader** | Send `Snapshot.Payload` as the HTTP DTO | Hash; compare fingerprints; omit Payload because fingerprint matched |

### Inputs

Frozen function:

```text
Of(kind string, schemaVersion int, partial bool, payload any) Fingerprint
```

| Parameter | Meaning | In the hash? |
|-----------|---------|----------------|
| `kind` | Collector kind (`device`, `users`, …) | **Yes** (envelope key `"kind"`) |
| `schemaVersion` | Snapshot envelope version (`pipeline.SchemaVersionV1` is `1`) | **Yes** (`"schema_version"`) |
| `partial` | Incomplete but usable sample | **Yes** (`"partial"`) |
| `payload` | Comparable facts only | **Yes** (entire JSON-normalised value) |

**Not inputs** (Pipeline must not pass them; `Of` has no parameters for them):

- `CollectedAt`
- `CollectError`
- `Snapshot.Fingerprint` (pre-set)
- HTTP status, request IDs, backend URL
- Scheduler interval / jitter
- Wall clock (`time.Now`)

**Nil `payload`:** `Of` **MUST** return a zero `Fingerprint` (`Empty() == true`). Pipeline treats that as not uploadable.

**Unmarshallable / non-JSON payload:** `Of` **MUST** return empty (current: `json.Marshal` / `Unmarshal` failure). Pipeline **MUST NOT** upload.

Kind-specific exclusions (for example Device `CollectedAt` on `device.Info`) **MUST** be applied **when the kind module builds comparable payload**, not inside `Of` and not inside a pipeline adapter. Device does this with `Info.Comparable()`. `Of` **MUST** hash the payload it is given. A collector that puts a per-sample clock in Payload **will** change the fingerprint every tick.

OS facts that operators treat as inventory (hostname, disk free, boot time, current user, installed apps, …) **MUST** remain in Payload so they affect the fingerprint.

Device payload ownership: [DEVICE_PIPELINE.md](DEVICE_PIPELINE.md).

### Outputs

- Non-empty `Fingerprint` whose `Value` is a stable encoding of the canonical envelope (today: lowercase hex SHA-256 of canonical bytes).
- Empty `Fingerprint` if `payload == nil` or canonicalisation fails.

`Of` **MUST NOT** return an error. Failure is expressed as `Empty()`.

### Equality

```text
a.Equal(b)  ⇔  a.Value == b.Value
a.Empty()   ⇔  a.Value == ""
```

Pipeline skip rule (unchanged): skip upload iff last success is non-empty **and** not pending **and** `lastSuccess.Equal(fp)`.

**MUST NOT:** prefix compare, locale compare, decode `Value` as JSON, treat empty as equal to a real hash.

### Serialization

JSON of `Fingerprint` **MUST** be a JSON **string** (the `Value`), not `{"Value":"..."}`.

`MarshalJSON` / `UnmarshalJSON` on `fingerprint.Fingerprint` implement that. `null` unmarshals to empty.

This keeps `pipeline.Snapshot` field `"fingerprint"` a string if a snapshot is logged or later queued.

`String()` **MUST** return `Value` for logs. Logs **MUST NOT** be part of hashing.

### Versioning

| Kind of change | Effect |
|----------------|--------|
| Envelope `schemaVersion` argument changes | Fingerprint **MUST** change → one upload |
| Payload shape / facts change | Fingerprint **MUST** change → upload |
| Hash algorithm inside `Of` changes | All fingerprints change → **one** extra upload per kind after deploy; `Of` signature and `Fingerprint` type **MUST NOT** change |
| Adding fields to `Fingerprint` (e.g. `Algorithm`) | **MUST NOT** happen in a way that breaks `Equal` on `Value` alone without a documented major contract revision |

Schema version in `Of` is the **Snapshot envelope** version, not a hash-algorithm version.

### Ownership

| Package | Owns |
|---------|------|
| `agent/internal/fingerprint` | `Fingerprint` type, `Of`, canonical bytes, hash algorithm |
| `agent/internal/pipeline` | When to call `Of`; skip/remember; **not** how to hash |
| Collectors | Payload content and field selection |
| Uploaders | HTTP mapping of Payload |

`fingerprint` **MUST NOT** import `pipeline`. Pipeline **MAY** import `fingerprint`. Collectors **MUST NOT** need to import `fingerprint` for Collect (Pipeline hashes after Collect). Tests of `Of` **MAY** live only in `fingerprint`.

---

## 3. Determinism rules

Permanent:

| Rule | Requirement |
|------|-------------|
| Same comparable inputs | Same `Fingerprint` on every call, process, and agent build that uses that `Of` implementation |
| Different comparable payload | Different `Fingerprint` (collision probability of SHA-256 is accepted; Pipeline does not mitigate) |
| `CollectedAt` | Ignored (not an `Of` argument) |
| `CollectError` | Ignored (not an `Of` argument) |
| Transient envelope fields | Ignored (same) |
| Stable output | Repeating `Of`  N times with the same arguments **MUST** yield `Equal` fingerprints |
| Pure function | No hidden process state |
| No randomness | **MUST NOT** read `math/rand`, UUID, or map iteration as the encoded order (maps are sorted) |
| No network | **MUST NOT** dial, DNS, or HTTP |
| No filesystem | **MUST NOT** read or write files as part of `Of` |
| No clock | **MUST NOT** call `time.Now` (or equivalent) inside `Of` |

Go `map` iteration order **MUST NOT** affect output: keys are sorted (`sort.Strings`) before encoding.

---

## 4. Canonicalization rules

Implementation: `normalize` then `writeCanonical` on a four-key envelope, then SHA-256.

### Envelope (included)

After JSON-normalising `payload`, `Of` builds:

```text
{
  "kind":           <kind string>,
  "partial":        <bool>,
  "payload":        <normalised payload>,
  "schema_version": <int>
}
```

Those four keys **MUST** be encoded with **lexicographic key order** (`kind`, `partial`, `payload`, `schema_version`).

### What is included

| Item | How |
|------|-----|
| **Kind** | String in envelope |
| **Schema version** | Number in envelope |
| **Partial** | Boolean in envelope |
| **Payload** | `json.Marshal` then `json.Unmarshal` to `any`, then canonical encode |

JSON normalisation means: struct field names follow `json` tags; numbers become JSON numbers (`float64` after unmarshal); `[]T` becomes `[]any`; objects become `map[string]any`.

**Maps / objects:** keys **MUST** be sorted ascending (Unicode/byte order of `sort.Strings`). Nested objects **MUST** be sorted the same way.

**Slices / arrays:** order **MUST** be preserved. `Of` **MUST NOT** sort lists. Collectors that emit sets (accounts, applications, processes) **MUST** sort those lists **in Payload** by a documented key before Collect returns. Unstable slice order **WILL** change the fingerprint.

**Primitives:** encoded with `encoding/json` (`true`/`false`, JSON strings, JSON numbers, `null`).

### What is excluded

| Item | Why |
|------|-----|
| `CollectedAt` | Not passed to `Of` |
| `CollectError` | Not passed to `Of` |
| `Snapshot.Fingerprint` | Not passed to `Of` |
| HTTP / transport | Not in Snapshot comparable fields |
| Scheduler config | Not in Snapshot |
| Hash algorithm name | Not in envelope; inside `Of` only |
| `time.Now` | Not called |

Anything **inside Payload** is included. Excluding a payload field is the **kind module’s** construction problem (Device: `Info.Comparable()`), not the adapter’s and not `Of`’s.

### Slice ordering (normative)

```
["C:", "D:"]  ≠  ["D:", "C:"]     // different fingerprints
{"z":1,"a":2} == {"a":2,"z":1}    // same fingerprint
```

---

## 5. Future compatibility

| Change | Allowed without Pipeline/Collector/Uploader API change? |
|--------|--------------------------------------------------------|
| SHA-256 → another hash inside `Of` | **Yes.** `Fingerprint` type and `Of(...)` signature stay. Expect one extra upload per kind. |
| Canonical byte format change | **Yes**, same as algorithm change. Tests of exact hex **MUST NOT** be required by Pipeline. |
| Extra optional fields on `Fingerprint` | **Only** if `Equal` remains `Value == Value` and JSON string form is kept, or a new major contract is written. |
| New `Of` parameters | **MUST NOT** without a new function (`OfV2`) plus this contract revision. Pipeline **MUST NOT** grow hashing parameters ad hoc. |
| Collector starts calling `Of` | **MUST NOT** for production Collect. Tests of Payload **MAY** call `Of` to assert expected skip behaviour. |
| Uploader reads `Fingerprint.Value` | **MUST NOT** |

Pipeline API that **MUST NOT** change for a hash swap:

- `RunOnce` / `Start` / `Stop`
- `shouldSkip` / `remember` using `Equal`
- `Uploader.Upload(ctx, Snapshot)`
- `Collector.Collect` returning `Snapshot`

Only package `fingerprint` owns hashing.

---

## 6. Regression rules

Any future change to `fingerprint` **MUST** preserve:

| Property | Test / rule |
|----------|-------------|
| **Determinism** | Same `Of` arguments → `Equal` fingerprints across many iterations |
| **Equality semantics** | `Equal` is `Value` equality; empty is `Value == ""` |
| **Canonical encoding** | Map key order does not change `Of`; slice order does |
| **Collector independence** | `fingerprint` does not import collector packages |
| **Pipeline independence** | `fingerprint` does not import `pipeline`; Pipeline does not hash |
| **Envelope exclusions** | Changing only `CollectedAt` / `CollectError` / collector-supplied `Snapshot.Fingerprint` does not change `Of`’s result when Kind, SchemaVersion, Partial, Payload are fixed |
| **JSON form** | `Fingerprint` still marshals as a JSON string |

A change that keeps `Equal` working but alters `Value` for the same inputs is an **algorithm replacement** (Section 5): allowed inside `Of`, must be called out in the changelog, and **MUST NOT** be treated as a collector bug.

Pipeline tests **MUST NOT** hard-code hex strings from today’s SHA-256. They **MUST** compare using `fingerprint.Of(...)` or `Equal` against a freshly computed value.

---

## 7. Verification

Required automated coverage (already present unless noted). Future PRs **MUST NOT** delete these properties.

| Case | Expectation | Where |
|------|-------------|--------|
| Identical payloads | `Of` equal | `TestIdenticalInputsProduceIdenticalFingerprints` |
| Many repeated calls | Still equal | `TestOfIsDeterministicAcrossManyCalls` |
| Different payloads | Not equal | `TestPayloadChangeProducesDifferentFingerprint` |
| Map key order | Equal | `TestMapKeyOrderDoesNotAffectFingerprint` |
| Clock not read | Equal after `Sleep` | `TestTimestampsAreNotFingerprintInputs` |
| Schema version change | Not equal | `TestPartialAndSchemaAffectFingerprint` |
| Kind change | Not equal | same |
| Partial flag change | Not equal | same |
| Nil payload | `Empty` | `TestNilPayloadIsEmpty` |
| JSON string round-trip | `Equal` after marshal | `TestFingerprintJSONRoundTripAsString` |
| Pipeline ignores collector `Snapshot.Fingerprint` | One upload for same payload | `TestPipelineIgnoresCollectorSuppliedFingerprint` |
| Pipeline ignores `CollectedAt` / `CollectError` | One upload | `TestPipelineFingerprintIgnoresCollectedAt` |
| Existing pipeline skip/upload tests | Still pass | `go test ./internal/pipeline` |

### Algorithm replacement compatibility

When changing the hash inside `Of`:

1. `Fingerprint` struct and `Of` signature **MUST** be unchanged.
2. `go test ./internal/fingerprint ./internal/pipeline` **MUST** pass without rewriting Pipeline’s public API.
3. Tests **MUST NOT** assert a fixed hex digest of a fixture (that would block replacement).
4. Manual: one agent restart **MAY** POST once per kind because `lastSuccess` memory is empty anyway; after a live algorithm swap **without** restart, the next tick **MUST** upload once then skip.

### Out of scope for this contract

- Device Inventory adapter
- Recurring `Pipeline.Start` for Device
- Backend / frontend
- Per-field Device include/exclude lists (those belong in a Device collector contract when Task 2.3+ lands)

---

## Frozen API surface (checklist)

| Symbol | Frozen? |
|--------|---------|
| `fingerprint.Fingerprint` struct with `Value string` | Yes |
| `Empty`, `Equal`, `String` | Yes |
| JSON as string | Yes |
| `Of(kind string, schemaVersion int, partial bool, payload any) Fingerprint` | Yes |
| SHA-256 hex as current `Value` format | Implementation, not API |
| Package does not import `pipeline` / `device` / transport | Yes |

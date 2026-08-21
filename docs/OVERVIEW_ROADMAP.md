# Overview implementation roadmap

Permanent checklist for **Fleet → Inventory → Overview**.

This document replaces the architecture-first audit priority list. Phases still group work by **product value**. Implementation order is **pipeline first**: build the recurring inventory loop before correcting individual Overview cards.

Do not start Accounts, Installed Applications, Processes, Services, Network, Policies, or Health until every required item in this file is **☑ Completed**.

---

## Status legend

| Mark | Meaning |
|------|---------|
| ☐ | Not Started |
| ◐ | In Progress |
| ☑ | Completed |

Update the mark on each task when work starts and when it is accepted. Do not mark a task complete without the verification steps, acceptance criteria, and **every Definition of Done checkbox**.

---

## Per-task Definition of Done (mandatory)

Every task in this file includes the same nine checkboxes. A task may be marked ☑ **only when all nine are ☑**.

| Layer | Meaning |
|-------|---------|
| Agent | Collector, scheduler, or agent config is done — or inspected and unchanged |
| Transport | Payload, retry, and upload path are done — or inspected and unchanged |
| Backend | Service/domain logic is done — or inspected and unchanged |
| Database | Schema, constraints, and writes are done — or inspected and unchanged |
| API | HTTP contract is done — or inspected and unchanged |
| Frontend | Overview (and any affected Fleet chrome) is done — or inspected and unchanged |
| Tests | Automated coverage exists for the change |
| Documentation | Engineering docs updated (`09_INVENTORY.md`, `10_CONFIGURATION.md`, `07_API_REFERENCE.md`, `05_FRONTEND_ARCHITECTURE.md`, `14_CURRENT_STATUS.md` as applicable) |
| Manual verification | Verification steps in the task were run on a real Windows agent and the UI |

A layer checkbox may be marked ☑ with the note **N/A — no change required** only after that layer has been inspected and confirmed unchanged. Uninspected layers stay ☐.

---

## Development rules

1. Finish **one task** at a time. Do not batch unrelated Overview fixes.
2. Implement in **ID order** (see Implementation order). Do not skip ahead to a later card fix.
3. Architecture work (Phase 4) is forbidden until Phases 1–3 are complete, unless a Phase 1/2/3 task cannot be finished without a small, documented backend change that is strictly required for correctness.
4. Optional cards (Phase 5) stay out of scope until Overview is already correct, live, and usable.
5. Overview is production-ready only when the **Definition of done — Overview production-ready** section at the bottom of this file is true.

---

## Implementation order

The first three tasks are the Inventory foundation. They run before remaining Phase 1 correctness cards.

| Order | ID | Why this position |
|-------|----|-------------------|
| 1 | **OV-1.1 Recurring Inventory Collection** | One-shot collection makes every inventory field go stale. This pipeline is the foundation for Overview and every later Inventory tab. |
| 2 | **OV-1.2 Overview auto refresh** | The UI must show new snapshots without a page reload. |
| 3 | **OV-1.3 Interactive Current User** | Correct the Current User collector now that refresh already exists. |
| 4 | OV-1.4 … OV-1.13 | Remaining Overview correctness |
| 5 | Phase 2 | Remaining live-information work (retry, freshness display) |
| 6 | Phase 3 | User experience |
| 7 | Phase 4 | Backend architecture |
| 8 | Phase 5 | Optional features |

Do not start OV-1.2 until OV-1.1 is accepted. Do not start OV-1.3 until OV-1.2 is accepted.

---

## What Overview is

Overview is the default Fleet endpoint tab. It is not a separate product surface.

- UI: `frontend/assets/app.js` → `renderEndpointOverview`
- Read API: `GET /api/v1/fleet/{agent_id}`
- Ingest: `POST /api/v1/devices` (inventory) and `POST /api/v1/agents/{id}/heartbeat` (presence)

Visible cards today: Hostname, Machine, Current User, Agent ID, Operating System, Version, Policy, Group, Tags, Status, Last Seen, plus the hardware list (OS version, Windows build, architecture, manufacturer, model, serial, CPU model, CPU cores, memory, disk total, disk free, last inventory).

---

## Phase summary

| Phase | Goal | Customer question | Sprint |
|-------|------|-------------------|--------|
| 1 | Inventory pipeline + correct information | “Does the data refresh, and can I trust it?” | Overview-1 |
| 2 | Remaining live information | “Is freshness honest under failure?” | Overview-2 |
| 3 | User experience | “Can I operate this under failure?” | Overview-3 |
| 4 | Backend architecture | “Will this still work at fleet scale?” | Overview-4 |
| 5 | Optional features | “What extra metadata helps?” | Overview-5 |

---

## Progress

| ID | Title | Phase | Status |
|----|-------|-------|--------|
| OV-1.1 | Recurring Inventory Collection | 1 | ☐ |
| OV-1.2 | Overview auto refresh | 1 | ☐ |
| OV-1.3 | Interactive Current User | 1 | ☐ |
| OV-1.4 | Operating System must match the real Windows edition | 1 | ☐ |
| OV-1.5 | Do not display registration GOOS/GOARCH as the operating system | 1 | ☐ |
| OV-1.6 | Disk cards must not imply whole-machine storage | 1 | ☐ |
| OV-1.7 | CPU cores must not imply physical cores | 1 | ☐ |
| OV-1.8 | Unknown numeric inventory must not render as zero | 1 | ☐ |
| OV-1.9 | Policy must not look applied when it is only assigned | 1 | ☐ |
| OV-1.10 | Failed or missing inventory must not look like real hardware | 1 | ☐ |
| OV-1.11 | Partial inventory must not look complete | 1 | ☐ |
| OV-1.12 | Last inventory time must be the collection time | 1 | ☐ |
| OV-1.13 | Status and Last Seen must match heartbeat truth at page load | 1 | ☐ |
| OV-2.1 | Retry inventory upload after transport failure | 2 | ☐ |
| OV-2.2 | Show inventory freshness | 2 | ☐ |
| OV-3.1 | Overview loading, empty, error, and retry | 3 | ☐ |
| OV-3.2 | Time display must be unambiguous | 3 | ☐ |
| OV-3.3 | Byte formatting must be consistent and labeled | 3 | ☐ |
| OV-3.4 | Hide or mark unimplemented Group and Tags | 3 | ☐ |
| OV-3.5 | Card labels must match the data | 3 | ☐ |
| OV-4.1 | Overview GET must not close sessions | 4 | ☐ |
| OV-4.2 | `GET /api/v1/fleet/{id}` must load one agent | 4 | ☐ |
| OV-4.3 | Inventory write validation and consistency checks | 4 | ☐ |
| OV-5.1 | Groups | 5 | ☐ |
| OV-5.2 | Tags write path | 5 | ☐ |
| OV-5.3 | Boot time | 5 | ☐ |
| OV-5.4 | IP address | 5 | ☐ |
| OV-5.5 | Extra presence metadata | 5 | ☐ |

When every Phase 1–4 row is ☑, Overview is production-ready (Phase 5 optional).

---

# Phase 1 — Inventory pipeline and correct information

Build the recurring inventory loop first. Then make Overview show live snapshots. Then correct field-level lies.

Recommended sprint: **Overview-1**.

---

### OV-1.1 Recurring Inventory Collection

**Status:** ☐ Not Started

**Description:** Device inventory is collected and uploaded **once at agent start**, then never again. Hostname, current user, memory, disk free, and later Inventory tabs all inherit that freeze.

This task builds the **Inventory collection pipeline**, not a one-off Overview timer.

**Current behaviour:**

```
Agent starts
→ Collect inventory
→ Upload
→ Never refresh again
```

**Desired behaviour:**

```
Agent starts
→ Collect inventory
→ Upload inventory
→ Wait configured interval
→ Collect inventory again
→ Detect changes
→ Upload updated inventory
→ Repeat until shutdown
```

The pipeline must be **reusable** for Overview, Accounts, Installed Applications, Processes, Services, Network, Policies, and Health. This task wires it for **device / Overview inventory** first. Do not implement those other collectors here. Do design the loop so they can share interval, change detection, and upload scheduling later.

**Why users care:** A commercial inventory product that only snapshots at boot is unused after the first hour. Disk, user, and memory change during the day. Every later Inventory feature would repeat the same one-shot mistake if this pipeline is not built first.

**Root cause:** `device.Start` and `deviceupload.Start` are one-shot. There is no shared inventory scheduler, no change detection, and no interval config for device inventory.

**Files involved:**

- `agent/internal/modules/device/device.go`
- `agent/internal/modules/device/collector_windows.go`
- `agent/internal/modules/deviceupload/deviceupload.go`
- `agent/internal/modules/deviceupload/request.go`
- `agent/cmd/risklens-agent/main.go`
- agent YAML / `10_CONFIGURATION.md` (new interval)
- `platform/app/api/v1/devices.py` (existing upsert; confirm it remains correct for repeats)
- `platform/app/services/device_service.py`

**Estimated complexity:** M–L

**Risk:** Medium. Too-frequent collection on large fleets; change detection must not miss real field updates or flood the API with identical payloads.

**Dependencies:** None. This is the first implementation task.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Start the agent; confirm an initial collect + `POST /api/v1/devices`.
2. Change a live field (disk free, or hostname if practical) without restarting the agent.
3. Wait one configured interval; confirm a second collect runs.
4. If the snapshot changed, confirm an updated POST and `device_inventory` upsert.
5. If the snapshot did not change, confirm no redundant upload (or a documented no-op skip).
6. Shut down the agent; confirm the loop stops.
7. Restart; confirm collect + upload run again immediately (do not wait a full interval before the first snapshot).

**Acceptance criteria:**

- First snapshot uploads at start, without waiting a full interval.
- Collection repeats on a documented, configurable interval until shutdown.
- Unchanged snapshots are not uploaded as duplicates.
- Changed snapshots upsert the existing `device_inventory` row (`agent_id` still unique).
- Agent restart is not required for disk free / memory / hostname updates.
- The loop is structured so later Inventory modules can reuse it (interval + collect + diff + upload). Accounts and other tabs are **not** implemented in this task.
- Interval default is safe for a workstation fleet and documented in `10_CONFIGURATION.md`.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.2 Overview auto refresh

**Status:** ☐ Not Started

**Description:** `loadFleetDetail` fetches once. An analyst who leaves Overview open sees frozen Status, Last Seen, Current User, disk, and memory even after the agent has uploaded a new snapshot or gone offline.

This task refreshes the **open Overview page** so server-side inventory and presence appear without reloading the SPA or re-clicking the endpoint.

**Why users care:** Live investigation pages must move. Recurring collection (OV-1.1) is wasted if the UI still shows the first GET.

**Root cause:** SPA caches `state.fleet.detail` until another agent is selected. No poll/SSE.

**Files involved:**

- `frontend/assets/app.js` (`loadFleetDetail`, `renderEndpointOverview`)
- `GET /api/v1/fleet/{agent_id}` (read-only poll is enough)

**Estimated complexity:** S–M

**Risk:** Low. Poll only while Overview is the active tab. Modest interval (e.g. 15–30s). Document that polling a heavy GET is temporary until OV-4.2.

**Dependencies:** OV-1.1 (new inventory must exist to prove hardware cards update). Heartbeats already exist for Status/Last Seen.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Open Overview on a live agent (Online).
2. Stop the agent; wait past 60s and 300s **without** navigating away; Status and Last Seen update in place.
3. Restart the agent; Status returns to Online without a full page reload.
4. With OV-1.1 running, change free disk (or another live inventory field); leave Overview open; hardware cards update after ingest + poll.
5. Leave Overview (other Fleet tab or other endpoint); confirm polling stops.
6. Return to Overview; confirm a fetch runs again.

**Acceptance criteria:**

- Open Overview tracks heartbeat presence (60s / 300s) without reload.
- Open Overview shows new inventory fields after the next successful ingest + poll.
- Polling stops when the user leaves Overview.
- No overlapping fetches scramble UI state or change the selected agent.
- Agent ID and other stable fields do not flash incorrectly.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.3 Interactive Current User

**Status:** ☐ Not Started

**Description:** Overview “Current User” is taken from `device_inventory.current_user`. The agent fills this with `user.Current()`, `GetUserNameEx`, or environment variables at process start. When the agent runs as LocalSystem (the install-script default), the card shows the **service account**, not the person at the console.

The recurring pipeline (OV-1.1) and Overview refresh (OV-1.2) already exist, so a correct collector will show login/logout changes on the open page.

**Why users care:** This is the first identity field an analyst uses (“who is on this PC?”). A wrong user is an incident-response error.

**Root cause:** Collector identity is the agent process token, not the interactive / WTS console session.

**Files involved:**

- `agent/internal/modules/device/collector_windows.go`
- `agent/internal/modules/device/model.go`
- `agent/internal/modules/deviceupload/request.go`
- `platform/app/infrastructure/postgres/models.py` (`device_inventory.current_user`)
- `frontend/assets/app.js` (`renderEndpointOverview` Current User card)

**Estimated complexity:** M

**Risk:** Medium. Wrong WTS session selection (RDP vs console, fast user switching) can still show the wrong person if the new logic is incomplete.

**Dependencies:** OV-1.1 (so user changes are recollected). OV-1.2 (so Overview shows them live).

**Recommended sprint:** Overview-1

**Verification steps:**

1. Install/run the agent as LocalSystem.
2. Log on as a normal console user.
3. Open Fleet → endpoint → Overview.
4. Confirm Current User is the interactive account (`DOMAIN\user` or `MACHINE\user`), not `SYSTEM` / `NT AUTHORITY\SYSTEM`.
5. Log off / switch user; wait one inventory interval with Overview open; confirm the card updates without restarting the agent.
6. Repeat with an RDP session if available; document which session wins (console vs active RDP) in `09_INVENTORY.md`.

**Acceptance criteria:**

- Current User is the interactive Windows user for a LocalSystem agent.
- If no interactive user is logged on, the card shows a true empty/unavailable state (`—` or an explicit “No interactive user”), never `SYSTEM` presented as a person.
- User changes appear after the next inventory interval + Overview refresh, without agent restart.
- Tests cover service-account vs interactive identity.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.4 Operating System must match the real Windows edition

**Status:** ☐ Not Started

**Description:** Overview “Operating System” uses registry `ProductName`. On Windows 11 this is often still “Windows 10 …”. Build `26200` can therefore appear as Windows 10 Home/Pro.

**Why users care:** Analysts filter and triage by OS family. Calling Windows 11 “Windows 10” is a false inventory fact.

**Root cause:** `ProductName` is not a reliable Windows 11 discriminator. Build number / `CurrentMajorVersionNumber` are.

**Files involved:**

- `agent/internal/modules/device/collector_windows.go`
- `device_inventory.operating_system` / `os_version` / `windows_build`
- `frontend/assets/app.js` Operating System card and OS version row

**Estimated complexity:** S–M

**Risk:** Low. Must not invent edition names the machine does not have (Home vs Pro).

**Dependencies:** OV-1.1 so a corrected OS string is re-uploaded without restart. No need to wait for a Windows upgrade to prove the loop.

**Recommended sprint:** Overview-1

**Verification steps:**

1. On a Windows 11 endpoint, compare Overview OS + build with `winver` / Settings → About.
2. On a real Windows 10 endpoint, confirm it still says Windows 10.
3. Confirm edition (Home/Pro) still matches, not just 10 vs 11.

**Acceptance criteria:**

- Windows 11 machines display Windows 11, not Windows 10.
- Windows 10 machines still display Windows 10.
- Windows build on Overview matches the collected `CurrentBuildNumber`.
- Collector tests cover both families.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.5 Do not display registration GOOS/GOARCH as the operating system

**Status:** ☐ Not Started

**Description:** If `device_inventory` is missing, Fleet falls back to `agents.operating_system`, which is the registration string (typically `windows/amd64`). Overview presents that as “Operating System”.

**Why users care:** `windows/amd64` is a runtime identifier, not an OS. It looks like a broken or unknown Windows SKU.

**Root cause:** `FleetService._to_record` uses registration OS when inventory is absent. Upload may also be skipped when `device_upload.enabled` is false.

**Files involved:**

- `platform/app/services/fleet_service.py`
- `platform/app/domain/fleet/entity.py`
- `agent` registration payload / `agents.operating_system`
- `agent` config `device_upload.enabled`
- `frontend/assets/app.js`

**Estimated complexity:** S

**Risk:** Low. Changing the fallback must not break Fleet list sorting that currently reads `operating_system`.

**Dependencies:** Complements OV-1.10 (missing inventory must be obvious).

**Recommended sprint:** Overview-1

**Verification steps:**

1. Register an agent with inventory upload disabled or blocked.
2. Open Overview.
3. Operating System must not show `windows/amd64`.
4. After a successful inventory POST, OS must show the real edition from OV-1.4.

**Acceptance criteria:**

- Registration GOOS/GOARCH never appears in the Overview Operating System card.
- Missing inventory uses empty/unavailable (`—`) or an explicit “Inventory not received”, not a fake OS name.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.6 Disk cards must not imply whole-machine storage

**Status:** ☐ Not Started

**Description:** “Disk total” and “Disk free” come from `GetDiskFreeSpaceEx` on the **system volume only** (normally `C:`). The labels read as machine storage. A PC with a 1 TB data disk and a 200 GB system disk looks like a 200 GB machine.

**Why users care:** Capacity and free space are used for disk-pressure triage. Wrong totals cause bad conclusions.

**Root cause:** Collector measures one volume. UI labels do not say so.

**Files involved:**

- `agent/internal/modules/device/collector_windows.go`
- `deviceupload/request.go` (`DiskSizeBytes` → `disk_total_bytes`)
- `frontend/assets/app.js` Disk total / Disk free rows

**Estimated complexity:** S for honest labeling. M if collecting all volumes in this task.

**Risk:** Low for relabeling. Medium if expanding to all volumes (new schema, overflow, removable drives).

**Dependencies:** None beyond OV-1.1 (live free-space updates already exist). Prefer **correct labeling** in this task. Multi-volume inventory is not required unless labeling alone still misleads.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Compare Overview disk total/free with Explorer properties of the system drive.
2. On a machine with a second data volume, confirm Overview does not claim that volume’s size as “the disk”.
3. Label text must name the volume or say “system drive”.
4. Confirm free space still updates via OV-1.1 + OV-1.2.

**Acceptance criteria:**

- A user cannot reasonably read the disk figures as “all disks on the PC” unless all disks are actually included.
- System-drive total and free match Windows for that volume (within normal filesystem rounding).
- If only the system drive is collected, the UI says so.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.7 CPU cores must not imply physical cores

**Status:** ☐ Not Started

**Description:** “CPU cores” is `runtime.NumCPU()` (logical processors / SMT). Many analysts read “cores” as physical cores or sockets.

**Why users care:** Hardware inventory used for asset records and capacity planning. Logical vs physical is a real discrepancy on SMT systems.

**Root cause:** Collector and UI use the word “cores” for logical CPUs.

**Files involved:**

- `agent/internal/modules/device/collector_windows.go`
- `frontend/assets/app.js` CPU cores row

**Estimated complexity:** S

**Risk:** Low. Relabel vs collect physical cores: relabel is enough for Phase 1 correctness.

**Dependencies:** None.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Compare the number with Task Manager → Performance → CPU (logical processors).
2. Confirm the label says logical processors (or equivalent), not an unqualified “cores”.

**Acceptance criteria:**

- The number matches Windows logical processors.
- The label does not claim physical cores unless physical cores are actually collected.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.8 Unknown numeric inventory must not render as zero

**Status:** ☐ Not Started

**Description:** Database defaults `cpu_cores`, `memory_bytes`, `disk_total_bytes`, and `disk_free_bytes` to `0`. The UI treats `0` as a real value (`0`, `0 B`). Missing collection therefore looks like a machine with no RAM and no disk.

**Why users care:** `0 B` memory is worse than `—`. It is a false hardware fact.

**Root cause:** Integer columns cannot distinguish unknown from zero; frontend `formatNumber` / `formatBytes` render `0` as data.

**Files involved:**

- `platform/app/infrastructure/postgres/models.py` (`device_inventory`)
- `platform/app/services/device_service.py`
- `frontend/assets/app.js` (`formatNumber`, `formatBytes`, `definitionList`)

**Estimated complexity:** S–M (nullable columns or a sentinel plus UI guards).

**Risk:** Medium. Changing `0` meaning can break existing rows and tests.

**Dependencies:** Coordinate with OV-1.10 so “no inventory” and “inventory with unknown disk” are distinct.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Agent with failed disk/memory collection (or DB zeros from a failed/partial row).
2. Overview must show unavailable, not `0 B` / `0` cores.
3. A real tiny value (if ever legitimate) must still display if it is actually collected.

**Acceptance criteria:**

- Unknown numeric fields render as `—` (or explicit unavailable).
- `0 B` / `0` cores appear only when the collector measured zero, which should not happen for RAM/disk on a real Windows PC.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.9 Policy must not look applied when it is only assigned

**Status:** ☐ Not Started

**Description:** Overview “Policy” is `policies.name` via `agent_policy` (default “Default Windows Policy” at register). The Windows agent does not fetch or enforce that policy. The card reads as if the endpoint is running it.

**Why users care:** Policy is a control statement. Showing a name that is not on the agent is a compliance lie.

**Root cause:** Server-side assignment is joined into Overview with no agent confirmation and no “assigned vs applied” language.

**Files involved:**

- `platform/app/services/fleet_service.py`
- `platform/app/infrastructure/postgres/fleet_repository.py`
- `agent` (no policy fetch today)
- `frontend/assets/app.js` Policy card

**Estimated complexity:** S for honest labeling. L if building real policy apply (out of Phase 1).

**Risk:** Low for labeling. Do not pretend the agent applies policy.

**Dependencies:** Full policy enforcement is a later Inventory feature (Policies tab). This task only stops the misleading Overview card.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Open Overview on a default-registered agent.
2. Policy text must not imply the agent loaded or enforced the named policy unless that is true.
3. Document the meaning in `09_INVENTORY.md`.

**Acceptance criteria:**

- Either the card is labeled as **assigned policy** (not applied), or it is omitted until apply exists.
- No Overview copy states that the endpoint is protected by that policy unless the agent has fetched/applied it.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.10 Failed or missing inventory must not look like real hardware

**Status:** ☐ Not Started

**Description:** Device collect/upload can fail (upload disabled, backend down, empty snapshot). Heartbeats may still succeed. Overview then mixes registration fallbacks with empty hardware and an Online status. Analysts read a live, fully inventoried PC.

**Why users care:** An Online endpoint with blank or fallback hardware looks healthy. The failure is invisible.

**Root cause:** Inventory upload is best-effort. Fleet GET still returns a full card layout. No inventory-failure field is rendered.

**Files involved:**

- `agent/internal/modules/deviceupload/deviceupload.go`
- `agent/internal/modules/device/device.go`
- `platform/app/api/v1/devices.py`
- `platform/app/services/fleet_service.py`
- `frontend/assets/app.js`

**Estimated complexity:** M

**Risk:** Medium. Must not mark Online agents as broken solely because inventory is old (that is OV-2.2 freshness). This task is **never received / last upload failed**.

**Dependencies:** OV-1.5 (OS fallback). OV-2.1 will add dedicated retry; this task only makes failure visible and non-misleading.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Block `POST /api/v1/devices`, keep heartbeats working.
2. Open Overview: Status may be Online; hardware must not look like a successful snapshot.
3. Restore upload, confirm cards populate via OV-1.1 + OV-1.2.

**Acceptance criteria:**

- Missing inventory is an explicit empty or error state on the hardware block, not a grid of `—` that looks like “this PC has no CPU”.
- Registration-only fields are not presented as hardware inventory.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.11 Partial inventory must not look complete

**Status:** ☐ Not Started

**Description:** The device module can finish as Partial (some Win32/registry fields failed) and still upload. Overview shows whatever arrived with no “partial” marker. A missing serial after OEM sanitization is fine; a failed CPU/memory read shown as complete inventory is not.

**Why users care:** Incomplete asset data used as complete inventory.

**Root cause:** Partial snapshots have no quality flag on `device_inventory` or the Fleet DTO.

**Files involved:**

- `agent/internal/modules/device/device.go`
- `agent/internal/modules/deviceupload/request.go`
- `platform` device ingest schema / table
- `frontend/assets/app.js` hardware list

**Estimated complexity:** M

**Risk:** Low–medium. Need a clear rule for “optional empty serial” vs “collection error”.

**Dependencies:** OV-1.8 (zeros vs unknown). OEM-empty serial should remain `—` without a failure banner.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Force a field-level collector error in tests.
2. Overview must indicate the snapshot is incomplete, or omit failed fields as unavailable with a snapshot warning.
3. Sanitized empty serial alone must not flag the whole inventory as failed.

**Acceptance criteria:**

- Partial collection is visible to the user.
- Optional empty BIOS fields do not trigger a false “inventory failed”.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.12 Last inventory time must be the collection time

**Status:** ☐ Not Started

**Description:** Overview “Last inventory” uses `agent_status.last_inventory_at` (server time when POST was received), not `device_inventory.collected_at` (agent collection time). Clock skew or delayed POST makes the card wrong relative to when the data was sampled.

**Why users care:** Analysts use this timestamp to judge whether hardware is current. A receive clock is not “when we looked at the PC”.

**Root cause:** Fleet record prefers status receive time over agent `collected_at`. `collected_at` is not on `FleetItemResponse`.

**Files involved:**

- `platform/app/services/fleet_service.py` (`last_inventory_at`)
- `platform/app/schemas/fleet.py`
- `device_inventory.collected_at`
- `frontend/assets/app.js` Last inventory row

**Estimated complexity:** S

**Risk:** Low. Document timezone handling; display remains local until OV-3.2 labels the zone.

**Dependencies:** None. OV-2.2 adds age text on top of a correct timestamp.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Collect inventory, delay upload if possible, or compare agent log `collected_at` with UI.
2. UI Last inventory must match collection time (UTC stored, local displayed until OV-3.2 labels the zone).

**Acceptance criteria:**

- Last inventory is agent `collected_at`, not HTTP receive time.
- If collection time is missing, show unavailable rather than a receive timestamp silently.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-1.13 Status and Last Seen must match heartbeat truth at page load

**Status:** ☐ Not Started

**Description:** Status is derived from `last_seen` (`ONLINE_MAX_SECONDS=60`, `STALE_MAX_SECONDS=300`). Last Seen is `agent_status.last_seen` (heartbeat). This mapping is the product definition. This task is to **prove and lock** that Overview Status/Last Seen at fetch time match that definition, and that `last_communication_at` is not silently substituted.

Live updates while the page is open are already OV-1.2. This task is first-paint correctness and regression tests.

**Why users care:** Online vs Offline is the first operational signal. A mismatch with the heartbeat table is a false outage or false health.

**Root cause:** Possible confusion between `last_seen`, `last_communication_at`, and `last_inventory_at`. Need regression tests that Overview JSON matches `compute_fleet_status`.

**Files involved:**

- `platform/app/domain/heartbeat/entity.py`
- `platform/app/domain/fleet/entity.py`
- `platform/app/services/fleet_service.py`
- `frontend/assets/app.js` Status / Last Seen cards

**Estimated complexity:** S

**Risk:** Low. Do not change thresholds here unless they produce incorrect presence; threshold product changes would be a separate decision.

**Dependencies:** OV-1.2 already refreshes these cards. This task locks the mapping and tests.

**Recommended sprint:** Overview-1

**Verification steps:**

1. Agent heartbeating: Overview Status = Online, Last Seen ≈ now (reload and open-page both).
2. Stop agent for 61–300s: Warning.
3. Stop agent for >300s: Offline, Last Seen = last heartbeat time.
4. Never-heartbeated agent: Never seen, Last Seen `—`.

**Acceptance criteria:**

- Status buckets match the documented 60s / 300s rules at page load and during OV-1.2 refresh.
- Last Seen is the heartbeat/`last_seen` clock, never inventory receive time.
- Tests cover the four states: online, warning, offline, never_seen.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

# Phase 2 — Remaining live information

Recurring collection and Overview auto refresh are already Phase 1 (OV-1.1, OV-1.2). What remains is failure recovery and honest age display.

Recommended sprint: **Overview-2**. Start only after Phase 1 is complete.

---

### OV-2.1 Retry inventory upload after transport failure

**Status:** ☐ Not Started

**Description:** If `POST /api/v1/devices` fails, the snapshot is not queued. A later successful heartbeat does not replay inventory. After reboot with backend down, collection exists in memory until process exit. OV-1.1’s next interval may eventually retry; a dedicated retry is still required for “collect succeeded, POST failed, next interval not yet due”.

**Why users care:** Brief API outages should not leave endpoints with empty or interval-stale Overview hardware.

**Root cause:** No inventory retry/queue (unlike activity queue).

**Files involved:**

- `agent/internal/modules/deviceupload/deviceupload.go`
- the inventory pipeline introduced in OV-1.1
- agent queue/retry utilities if reused

**Estimated complexity:** M

**Risk:** Medium. Duplicate POSTs are already upsert-safe; avoid unbounded retry storms.

**Dependencies:** OV-1.1.

**Recommended sprint:** Overview-2

**Verification steps:**

1. Collect while API is down; bring API up before the next inventory interval.
2. Inventory appears without restarting the agent and without waiting the full collect interval if a retry timer fires sooner.
3. No duplicate rows (still one `device_inventory` per `agent_id`).

**Acceptance criteria:**

- Transient POST failure does not permanently drop a collected snapshot.
- Retry backoff is bounded and documented.
- Change detection still skips identical payloads after a successful retry.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-2.2 Show inventory freshness

**Status:** ☐ Not Started

**Description:** Analysts should see how old the snapshot is (“2 minutes ago” / “3 days ago”), not only a raw timestamp.

**Why users care:** A correct-looking CPU model from last month is still operationally stale. Age is the honesty layer for live inventory.

**Root cause:** UI shows `formatTime(last_inventory_at)` only. `heartbeat_age_seconds` exists on the DTO and is unused on Overview.

**Files involved:**

- `frontend/assets/app.js`
- optionally Fleet DTO `heartbeat_age_seconds` / inventory age

**Estimated complexity:** S

**Risk:** Low. Relative age must use a stable clock (prefer server-provided age if skew matters).

**Dependencies:** OV-1.12 (correct collection timestamp). OV-1.2 so age updates while the page is open.

**Recommended sprint:** Overview-2

**Verification steps:**

1. Fresh inventory: age is minutes, not days.
2. Stop inventory uploads: age grows on the open Overview.
3. Timestamp and relative age do not contradict each other.

**Acceptance criteria:**

- Overview shows inventory age in human-readable form next to Last inventory.
- Age is computed from collection time (OV-1.12), not page-load time alone.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

# Phase 3 — User experience

The page must be operable: loading, empty, error, retry, formatting, time, units, consistency.

Recommended sprint: **Overview-3**. Do not start until Phases 1–2 data is trustworthy; otherwise UX polish freezes the wrong numbers.

---

### OV-3.1 Overview loading, empty, error, and retry

**Status:** ☐ Not Started

**Description:** Loading (“Loading agent…”) and `detailError` exist. There is no Retry control. Empty hardware vs “select an endpoint” vs API 404 vs backend down are easy to confuse. Failed fetches require navigating away and back.

**Why users care:** A SOC cannot sit on a red error with no next action during an incident.

**Root cause:** `loadFleetDetail` sets error text only.

**Files involved:**

- `frontend/assets/app.js` (`loadFleetDetail`, endpoint detail pane)
- `frontend/index.html` if a retry control is added

**Estimated complexity:** S–M

**Risk:** Low.

**Dependencies:** OV-1.10 empty-vs-error copy should stay consistent with this task. Manual retry is separate from OV-1.2 auto refresh.

**Recommended sprint:** Overview-3

**Verification steps:**

1. Stop platform API; open Overview: error + Retry; Retry after API is back succeeds.
2. Unknown UUID: 404 message, not a generic timeout.
3. No agent selected: “Select an endpoint.”
4. Loading does not leave a previous agent’s cards on screen.

**Acceptance criteria:**

- Loading, empty, error, and success are distinct.
- Retry repeats `GET /api/v1/fleet/{id}` without a full SPA reload.
- Previous endpoint data is not shown as if it belonged to the new selection.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-3.2 Time display must be unambiguous

**Status:** ☐ Not Started

**Description:** `formatTime` uses `Date.toLocaleString()` (browser local TZ) with no zone label. Backend stores UTC. Two analysts in different zones will read different clock faces without knowing why.

**Why users care:** Last Seen is an evidence field. Unlabeled local time causes “the PC was online at 3pm” arguments.

**Root cause:** No timezone in the formatted string; no shared product rule (always local vs always UTC).

**Files involved:**

- `frontend/assets/app.js` (`formatTime`)

**Estimated complexity:** S

**Risk:** Low. Pick one product rule and apply it to Overview times (Last Seen, Last inventory). Wider SPA time consistency can follow the same helper.

**Dependencies:** OV-1.12, OV-1.13.

**Recommended sprint:** Overview-3

**Verification steps:**

1. Known UTC timestamp in API JSON.
2. UI shows a labeled zone (e.g. local + `UTC+10` or explicit UTC).
3. Invalid/missing timestamps still show `—`.

**Acceptance criteria:**

- Overview timestamps cannot be mistaken for a different zone.
- The chosen rule is documented for operators.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-3.3 Byte formatting must be consistent and labeled

**Status:** ☐ Not Started

**Description:** `formatBytes` uses 1024 steps with SI-looking labels (KB, MB, GB). Memory/disk in Windows are often compared as GB in decimal or GiB. Analysts comparing Overview to Task Manager will see small mismatches plus unit confusion.

**Why users care:** “8 GB RAM” vs “7.4 GB” looks like a collection bug.

**Root cause:** Binary math with SI unit names; no “GiB” / source note.

**Files involved:**

- `frontend/assets/app.js` (`formatBytes`)

**Estimated complexity:** S

**Risk:** Low.

**Dependencies:** OV-1.6 (disk meaning), OV-1.8 (zero vs unknown).

**Recommended sprint:** Overview-3

**Verification steps:**

1. Known `memory_bytes` (e.g. 16 GiB) formats to a documented unit.
2. Unknown still `—` (OV-1.8).
3. Disk figures match the labeled volume within the chosen unit system.

**Acceptance criteria:**

- Unit names match the math (KiB/MiB/GiB **or** true SI 1000).
- Overview memory/disk use that system consistently.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-3.4 Hide or mark unimplemented Group and Tags

**Status:** ☐ Not Started

**Description:** Group is hardcoded `""` → `—`. Tags are read from `agent_tags` but nothing writes them, so they are always `—`. That looks like “this endpoint has no group/tags”, not “the product does not support this yet”.

**Why users care:** Empty assignment is an operational fact. An unimplemented feature shown as empty assignment is misleading (executed here so Phase 5 can add the real feature).

**Root cause:** UI cards exist before the product features exist.

**Files involved:**

- `frontend/assets/app.js` (`renderEndpointOverview`)
- `frontend/index.html` if Fleet table Group column is also a placeholder (`—`)

**Estimated complexity:** S

**Risk:** Low.

**Dependencies:** Implementation of Groups/Tags is OV-5.1 / OV-5.2. This task only stops the false empty state.

**Recommended sprint:** Overview-3

**Verification steps:**

1. Overview does not show Group/Tags as empty assignments.
2. Fleet table does not imply a Group feature if Overview hides it (same product language).

**Acceptance criteria:**

- Unimplemented Group/Tags are hidden **or** explicitly labeled unavailable.
- `—` is reserved for “none assigned” only after the feature exists.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-3.5 Card labels must match the data

**Status:** ☐ Not Started

**Description:** “Version” is agent version, not OS version (OS version is in the hardware list). “Machine” vs “Hostname” are two Windows names without explanation. Status uses `presenceLabel` (emoji string) while the Fleet table uses `formatPresenceStatus` (HTML badges).

**Why users care:** Two version fields and two names cause support tickets. Inconsistent status chrome looks unfinished.

**Root cause:** Compact labels; two presence renderers.

**Files involved:**

- `frontend/assets/app.js`

**Estimated complexity:** S

**Risk:** Low.

**Dependencies:** None.

**Recommended sprint:** Overview-3

**Verification steps:**

1. Version card is clearly Agent version.
2. Hostname vs Machine are understandable (or collapsed if product decides one identity).
3. Status presentation matches Fleet list language (Online / Warning / Offline / Never seen).

**Acceptance criteria:**

- No Overview label can be reasonably read as a different field.
- Status visual language is consistent with the Fleet table.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

# Phase 4 — Backend architecture

Only after the product behaves correctly. These items do not change what the analyst should believe; they keep that truth cheap, safe, and scalable.

Recommended sprint: **Overview-4**.

---

### OV-4.1 Overview GET must not close sessions

**Status:** ☐ Not Started

**Description:** `FleetService._to_record` calls `close_open_for_agent` for every **offline** agent while materializing fleet records. `get()` uses `_all_records()`, so opening Overview for one endpoint can close sessions on **other** offline agents. OV-1.2 polling makes this fire on a timer.

**Why users care:** Session history is evidence. A read of Overview must not rewrite Activity data. This is deferred to Phase 4 only because it does not change Overview **cards**; it is still a production blocker for the platform and must be done before Overview is declared done.

**Root cause:** Presence materialization mixed with session mutation.

**Files involved:**

- `platform/app/services/fleet_service.py`
- session service/repository
- tests that currently rely on list/get side effects

**Estimated complexity:** M

**Risk:** High if done wrong (sessions left open or closed twice). High if left undone (silent evidence mutation).

**Dependencies:** None from Overview UI. Do not “fix” by skipping offline presence.

**Recommended sprint:** Overview-4

**Verification steps:**

1. Agent A online (Overview open and polling), agent B offline with an ACTIVE session.
2. GET Overview for A must not close B’s session.
3. Offline close remains available on the **intended** lifecycle path (heartbeat/offline job or Sessions read), not as a hidden GET side effect.
4. Tests prove GET fleet-by-id is read-only for sessions.

**Acceptance criteria:**

- `GET /api/v1/fleet/{id}` does not write session rows.
- Offline session close is an explicit, documented path, not a Fleet join side effect.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-4.2 `GET /api/v1/fleet/{id}` must load one agent

**Status:** ☐ Not Started

**Description:** `FleetService.get` builds the entire fleet in memory (all agents, inventories, heartbeats, tags, policies) then picks one id. Overview poll (OV-1.2) makes this worse.

**Why users care:** At hundreds of endpoints, opening one PC will get slow and can time out. The page will look “down”.

**Root cause:** List assembly reused for detail.

**Files involved:**

- `platform/app/services/fleet_service.py`
- `platform/app/api/v1/fleet.py`
- postgres fleet/device/heartbeat repositories
- `platform/tests/test_fleet.py`

**Estimated complexity:** M

**Risk:** Medium. Detail JSON contract must stay stable for the SPA.

**Dependencies:** OV-4.1 should land first or together so the new single-agent path cannot close foreign sessions.

**Recommended sprint:** Overview-4

**Verification steps:**

1. Detail GET query plan / logs touch one `agent_id`, not a full `agents` list join used for every other row.
2. Existing Overview fields unchanged.
3. Load test or unit test: N extra agents do not change get-by-id cost linearly.

**Acceptance criteria:**

- Detail fetch cost is O(1) agents, not O(fleet).
- Response schema for Overview remains valid.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-4.3 Inventory write validation and consistency checks

**Status:** ☐ Not Started

**Description:** Ingest does not check `disk_free_bytes <= disk_total_bytes`, reasonable core counts, or reject impossible combinations. Bad agent payloads become Overview facts. Recurring upload (OV-1.1) can repeat bad data on an interval.

**Why users care:** Garbage hardware on Overview is a trust failure, even if rare.

**Root cause:** Schema types only; little domain validation.

**Files involved:**

- `platform/app/api/v1/devices.py`
- `platform/app/services/device_service.py`
- device request schema

**Estimated complexity:** S–M

**Risk:** Low. Too-strict checks could reject real OEM oddities; keep checks conservative.

**Dependencies:** OV-1.8 null/unknown semantics.

**Recommended sprint:** Overview-4

**Verification steps:**

1. POST with `disk_free > disk_total` is rejected or coerced with a logged error; Overview never shows the impossible pair.
2. Valid MSI/laptop payloads still 200/201.

**Acceptance criteria:**

- Impossible inventory cannot be the live Overview snapshot.
- Validation errors are visible to the agent/logs, not silent.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

# Phase 5 — Optional features

Only after Phases 1–4. These fields are collected, schemed, or shown as placeholders today, but Overview can be production-ready for a commercial MVP **without** them if Phase 3 has hidden the false empty cards.

Recommended sprint: **Overview-5**.

---

### OV-5.1 Groups

**Status:** ☐ Not Started

**Description:** No group model is wired to Overview. The card is a placeholder.

**Why users care:** SOCs group by site, BU, or criticality. Useful, not required to trust a single endpoint’s hardware.

**Root cause:** Feature not built.

**Files involved:** TBD when designed (`agent_groups` or similar, Fleet DTO, Overview card).

**Estimated complexity:** L

**Risk:** Medium (IA and multi-group membership).

**Dependencies:** OV-3.4. Fleet IA in `docs/ARCHITECTURE.md`.

**Recommended sprint:** Overview-5

**Verification steps:** Assign a group; Overview shows it; unassigned shows none.

**Acceptance criteria:** Group is a real assignment, filterable in Fleet, documented, tested.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-5.2 Tags write path

**Status:** ☐ Not Started

**Description:** `agent_tags` exists and Overview can render tags. Nothing writes them.

**Why users care:** Tags help hunt/filter. Empty tags after a writer exists are meaningful; today they are not.

**Root cause:** No admin/API writer.

**Files involved:**

- `platform` tags repository / models
- new write API
- `frontend/assets/app.js`

**Estimated complexity:** M

**Risk:** Low–medium (tag enum vs free-form).

**Dependencies:** OV-3.4.

**Recommended sprint:** Overview-5

**Verification steps:** Write tags; Overview and Fleet table show them; delete/clear shows none.

**Acceptance criteria:** Tags round-trip from write API to Overview. Constraints documented.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-5.3 Boot time

**Status:** ☐ Not Started

**Description:** Agent collects `boot_time`; it is stored on `device_inventory` and omitted from `FleetItemResponse` / Overview.

**Why users care:** Uptime/reboot evidence. Secondary to identity and presence.

**Root cause:** DTO/UI omission.

**Files involved:**

- `platform/app/schemas/fleet.py`
- `platform/app/services/fleet_service.py`
- `frontend/assets/app.js`

**Estimated complexity:** S

**Risk:** Low. Tick-count vs wall clock must be documented.

**Dependencies:** OV-3.2 for display. Recurring inventory (OV-1.1) so boot time updates after reboot without depending only on process start.

**Recommended sprint:** Overview-5

**Verification steps:** Reboot endpoint; after the next inventory interval, Overview boot time matches Windows last boot (within tolerance).

**Acceptance criteria:** Boot time is visible, timezone-labeled, and sourced from inventory.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-5.4 IP address

**Status:** ☐ Not Started

**Description:** `agent_status.ip_address` exists on the Fleet DTO and is never written. Overview does not show it.

**Why users care:** Network identity during investigations. Complements, does not replace, hostname.

**Root cause:** No collector/heartbeat field populated.

**Files involved:**

- agent heartbeat/inventory
- `platform` heartbeat/status write path
- Overview UI

**Estimated complexity:** M

**Risk:** Medium (VPN, multiple NICs, IPv6 — must not pick a random misleading address).

**Dependencies:** Honest labeling if multiple addresses exist (similar to OV-1.6). Prefer using the OV-1.1 pipeline if IP is collected as inventory rather than heartbeat.

**Recommended sprint:** Overview-5

**Verification steps:** Known interface IP appears; offline/unassigned shows unavailable.

**Acceptance criteria:** Shown IP is a documented selection rule (e.g. primary active non-loopback), not an empty column presented as “no network”.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

### OV-5.5 Extra presence metadata

**Status:** ☐ Not Started

**Description:** API already has `heartbeat_age_seconds`, `last_communication_at`, `registration_status`. Overview does not render them. Optional once Last Seen + Status + inventory age are good.

**Why users care:** Debugging agent install vs heartbeat vs inventory. Advanced, not first-line.

**Root cause:** UI subset of DTO.

**Files involved:**

- `frontend/assets/app.js`
- `platform/app/schemas/fleet.py` (already present)

**Estimated complexity:** S

**Risk:** Low. Do not duplicate Last Seen with a second clock that disagrees (OV-1.13).

**Dependencies:** OV-1.13, OV-2.2, OV-3.2.

**Recommended sprint:** Overview-5

**Verification steps:** Extra fields match API; they do not contradict Status/Last Seen.

**Acceptance criteria:** If shown, each extra field has a one-line definition in the UI or docs so clocks are not confused.

**Definition of Done:**

- ☐ Agent complete
- ☐ Transport complete
- ☐ Backend complete
- ☐ Database complete
- ☐ API complete
- ☐ Frontend complete
- ☐ Tests complete
- ☐ Documentation complete
- ☐ Manual verification complete

---

## Explicitly out of Overview scope

Do not pull these into this roadmap as product features (the OV-1.1 **pipeline** must remain reusable for them):

- Implementing Accounts, Installed Applications, Processes, Services, Network, Policies tab, or Health collectors
- Activity / Security
- Global search
- Platform authentication (fleet-wide concern; not an Overview card)
- Rewriting the SPA in React
- Agent README / unrelated dead code cleanup unless a Phase 1–3 task requires it

`GET /api/v1/devices` remaining unused by Overview is acceptable. Overview’s contract is Fleet GET. Do not add a second read path without a Phase 4/5 need.

---

## Definition of done — Overview production-ready

Overview is production-ready when **all** of the following are true:

1. Every task in Phases 1–4 is ☑, including all nine per-task Definition of Done boxes.
2. Inventory collects at start, then on interval, with change detection, until shutdown (OV-1.1).
3. Open Overview shows new inventory and presence without reload (OV-1.2).
4. On a LocalSystem agent, Current User, OS edition, disk meaning, CPU meaning, Status, Last Seen, and inventory timestamps are **correct** per acceptance criteria.
5. Loading, empty, error, and retry are usable.
6. `GET /api/v1/fleet/{id}` does not mutate sessions and does not scan the whole fleet.
7. Automated tests cover the inventory loop, collector identity, OS family, presence buckets, inventory upsert, and Overview GET read-only behaviour.
8. `docs/09_INVENTORY.md` and `docs/14_CURRENT_STATUS.md` describe Overview as implemented (not partial) for the fields in Phases 1–4.

Phase 5 items are **not** required for that gate. When Phase 5 is also ☑, Overview is complete including optional metadata.

---

## How to execute

1. Set the next ☐ task to ◐.
2. Implement only that task.
3. Tick each Definition of Done layer when that layer is done (or N/A after inspection).
4. Run verification steps; update tests and docs.
5. Mark the task ☑ only when all nine DoD boxes are ☑.
6. Stop and wait for approval before the next ID.

Start with **OV-1.1 Recurring Inventory Collection**. Do not start OV-1.2 until OV-1.1 is accepted.

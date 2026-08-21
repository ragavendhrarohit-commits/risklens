# 11 — Testing

There are two independent test suites. There is **no** repository-wide coverage config (`pytest-cov` / `go test -coverprofile` are not configured).

---

## Strategy

| Suite | Location | Runner | What it proves |
|-------|----------|--------|----------------|
| Platform | `platform/tests/` | `pytest` from `platform/` | HTTP contracts, domain merge, postgres adapters, frontend string/architecture guards |
| Agent | `agent/**/*_test.go` | `go test ./...` | Modules, transport, Windows hooks, activity lifecycle |
| Architecture freeze | `test_sprint_13_*.py` | pytest | Placeholders stay empty; Fleet IA; official arch doc exists |

`platform/tests/conftest.py`:

- Memory backend for most tests (Sprint 1/2 style)
- PostgreSQL fixtures skip if `RISKLENS_TEST_DATABASE_URL` is unreachable (Sprint 3)

Frontend tests do **not** run a browser. They read `frontend/index.html` and `frontend/assets/app.js` as text and assert nav, labels, and fetch URLs.

---

## Python tests (`platform/tests/`) — 36 files

Configured in `platform/pyproject.toml`:

```
testpaths = ["tests"]
addopts = "-ra -q --strict-markers"
asyncio_mode = "auto"
filterwarnings = error (DeprecationWarning ignored)
```

Optional deps: `pytest>=8.3`, `pytest-asyncio>=0.24`, `httpx>=0.27`.

| File | Focus |
|------|--------|
| `test_agent_entity.py` | Agent domain entity |
| `test_agent_service.py` | Registration service / duplicates |
| `test_agents_endpoint.py` | `POST /api/v1/agents/register` |
| `test_application_activity.py` | App activity requires session |
| `test_config.py` | Settings defaults and rejection |
| `test_device_query_endpoint.py` | `GET /api/v1/devices` |
| `test_device_query_service.py` | Query filters/sort |
| `test_device_service.py` | Device upsert service |
| `test_devices_endpoint.py` | `POST /api/v1/devices` |
| `test_fleet.py` | Fleet list/detail, stubs, Sprint 12 regression, OpenAPI |
| `test_frontend_fleet.py` | Fleet nav, software refetch, session display, presence helpers |
| `test_frontend_ia.py` | Computer-centric nav |
| `test_frontend_software.py` | Software page wiring |
| `test_frontend_users.py` | Users page wiring |
| `test_health.py` | `GET /health` |
| `test_heartbeat.py` | Presence thresholds, heartbeat service |
| `test_heartbeat_endpoint.py` | Heartbeat HTTP + devices presence |
| `test_lifespan.py` | Container install/clear |
| `test_logging.py` | JSON formatter, request id |
| `test_memory_agent_repository.py` | Memory agent repo concurrency |
| `test_postgres_agent_repository.py` | Postgres agent repo |
| `test_postgres_device_inventory.py` | Postgres device persist |
| `test_postgres_device_query.py` | Postgres device query |
| `test_postgres_heartbeat.py` | Postgres heartbeat history |
| `test_postgres_migrations.py` | Alembic heads, columns, indexes, idempotent upgrade |
| `test_postgres_registration_endpoint.py` | Registration on Postgres |
| `test_root.py` | `/` and request-id |
| `test_session_lifecycle.py` | Login/logout/shutdown/offline; observed overlap regressions |
| `test_session_offline_close.py` | Offline close vs real logout |
| `test_sessions.py` | Session repo/service/endpoint |
| `test_software.py` | Software dedup/concurrency |
| `test_sprint_13_5_architecture.py` | Reserved domain packages empty |
| `test_sprint_13_6_architecture.py` | Fleet tab groups |
| `test_sprint_13_7_architecture.py` | Software label, fleet session/app routes, arch doc |
| `test_users.py` | User inventory |
| `test_version.py` | Version, OpenAPI, 404 envelope |

### Session regression tests (high value)

`test_session_lifecycle.py` includes names such as:

- `test_new_login_closes_previous_active_at_new_logon`
- `test_consecutive_logins_never_overlap`
- `test_login_then_login_then_shutdown_closes_only_current`
- `test_missed_logout_then_second_login_matches_observed_bug`
- `test_exact_observed_timeline`
- Direct tests of `activate_current` and `close_active_for_agent` on the **repository**, not via `SessionService`

If production login still uses `_upsert_one`, tests that expect “second login closes previous” through the **HTTP/service** path will fail or were written against the intended invariant. Engineers must read which helper each test calls before treating a green run as “overlap is impossible”.

---

## Go tests (`agent/`) — 34 `*_test.go` files

`agent/Makefile`:

```
test: go test ./...
vet:  go vet ./...
build: go build -o bin/RiskLensAgent.exe ./cmd/risklens-agent
```

| File | Focus |
|------|--------|
| `internal/app/app_test.go` | App start/stop, identity injection |
| `internal/config/config_test.go` | **Legacy** YAML loader |
| `internal/configuration/*_test.go` | Live YAML manager/storage/validation/watcher |
| `internal/identity/*_test.go` | Generate, file store, manager, validation |
| `internal/logging/logger_test.go` | JSON logs, rotation |
| `internal/modules/activity/activity_test.go` | Upload/queue/session payloads |
| `internal/modules/activity/lifecycle_test.go` | WTS login/logout/RDP/shutdown |
| `internal/modules/base/module_test.go` | Status marks |
| `internal/modules/device/*_test.go` | Collector and Info model |
| `internal/modules/deviceupload/deviceupload_test.go` | Inventory POST |
| `internal/modules/heartbeat/heartbeat_test.go` | Schedule, no overlap, no sessions in payload |
| `internal/modules/manager/manager_test.go` | Start order, rollback |
| `internal/modules/registration/*_test.go` | Client (no retry this sprint), module, status |
| `internal/modules/registry/registry_test.go` | Duplicates |
| `internal/modules/software/software_test.go` | Dedupe/upload |
| `internal/modules/users/users_test.go` | Collector/upload |
| `internal/service/loop_test.go` | Stop/shutdown loop |
| `internal/service/windows_test.go` | SCM session flush |
| `internal/transport/*_test.go` | Client, errors, middleware, request, response |

`registration/transport_fake_test.go` is a fake helper (Sprint 6); it may contain no `func Test`.

Windows-only tests need a Windows builder (`//go:build windows` collectors).

---

## Coverage

**Not implemented.** No CI coverage gate was found in this repository.

---

## Known gaps

1. No browser/E2E tests (Playwright/Cypress absent).
2. No load tests.
3. Frontend tests cannot catch Fleet data bugs; they catch wiring.
4. Postgres tests skip when Docker/Postgres is down — easy to miss adapter regressions locally.
5. `activate_current` is tested at repo level; production `SessionService` path may not match those tests.
6. REPRO logging is not asserted as a contract.
7. UI static server (`backend/`) has **no tests**.
8. Install/uninstall PowerShell scripts have **no tests**.
9. Agent README still describes Sprint 1; tests have moved far beyond that.
10. No contract tests that compile agent JSON structs against Python Pydantic models in one job.

---

## How to run

Platform (from `platform/`, venv with `[test]` extras):

```text
pytest
```

Agent (from `agent/`, Go 1.23):

```text
go test ./...
go vet ./...
```

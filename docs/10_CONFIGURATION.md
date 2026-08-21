# 10 — Configuration

This document lists every config file, environment variable, CLI flag, and default found in source. Secrets in local `.env` files are not copied here; names and defaults come from `.env.example` and Settings classes.

---

## Platform API (`platform/`)

### Files

| File | Role |
|------|------|
| `platform/.env` | Local overrides (git-ignored in normal practice; exists in the tree) |
| `platform/.env.example` | Template |
| `platform/app/config/settings.py` | Pydantic Settings, prefix `RISKLENS_`, loads `platform/.env` |
| `platform/alembic.ini` | Alembic; DSN from Settings |
| `platform/pyproject.toml` | Package metadata and pytest |
| `platform/scripts/run_dev.ps1` | `uvicorn --host 0.0.0.0 --port 8000` using `platform/.venv` |

Env prefix: **`RISKLENS_`**. Nested settings are flat env vars, not `RISKLENS__DATABASE__URL`.

### Environment variables

| Env var | Settings field | Default |
|---------|----------------|---------|
| `RISKLENS_ENVIRONMENT` | `environment` | `development` (`development`/`testing`/`production`) |
| `RISKLENS_APP_NAME` | `app_name` | `RiskLens Backend` |
| `RISKLENS_APP_VERSION` | `app_version` | `0.1.0` |
| `RISKLENS_BUILD_TIME` | `build_time` | `unknown` |
| `RISKLENS_HOST` | `host` | `127.0.0.1` (**does not bind Uvicorn by itself**) |
| `RISKLENS_PORT` | `port` | `8000` (**does not bind Uvicorn by itself**) |
| `RISKLENS_LOG_LEVEL` | `log_level` | `INFO` |
| `RISKLENS_LOG_JSON` | `log_json` | `true` |
| `RISKLENS_ALLOWED_HOSTS` | `allowed_hosts` | `["*"]` (comma-separated) |
| `RISKLENS_CORS_ORIGINS` | `cors_origins` | `http://127.0.0.1:8080,http://localhost:8080` |
| `RISKLENS_REQUEST_ID_HEADER` | `request_id_header` | `X-Request-ID` |
| `RISKLENS_GZIP_MIN_SIZE` | `gzip_min_size` | `1024` |
| `RISKLENS_REPOSITORY_BACKEND` | `repository_backend` | `postgres` (`memory` allowed) |
| `RISKLENS_DATABASE_URL` | `database_url` | `postgresql+psycopg://risklens:risklens@127.0.0.1:5432/risklens` |
| `RISKLENS_DATABASE_POOL_SIZE` | `database_pool_size` | `5` |
| `RISKLENS_DATABASE_MAX_OVERFLOW` | `database_max_overflow` | `10` |
| `RISKLENS_DATABASE_POOL_TIMEOUT` | `database_pool_timeout` | `30` |
| `RISKLENS_DATABASE_POOL_RECYCLE` | `database_pool_recycle` | `1800` |
| `RISKLENS_DATABASE_ECHO` | `database_echo` | `false` |
| `RISKLENS_DATABASE_CONNECT_TIMEOUT` | `database_connect_timeout` | `10` |
| `RISKLENS_TEST_DATABASE_URL` | **not on Settings** | `postgresql+psycopg://risklens:risklens@127.0.0.1:5432/risklens_test` in `conftest.py` |

`database_url` must start with `postgresql+psycopg://`, `postgresql://`, or `sqlite://`.

`get_settings()` is `@lru_cache(maxsize=1)`.

### Command line (Platform)

From `platform/README.md` and `run_dev.ps1`:

```text
python -B -m uvicorn app.main:app --host 127.0.0.1 --port 8000
python -B -m uvicorn app.main:app --host 0.0.0.0 --port 8000
```

`--reload` is documented for local work. Working directory must be `platform/` so `app.main:app` resolves.

Alembic: run from `platform/` using `alembic.ini`.

---

## UI static server (`backend/`)

### Files

| File | Role |
|------|------|
| Repo root `.env` / `.env.example` | Documented Elasticsearch keys — **not read by `backend/app/config.py`** |
| `backend/app/config.py` | `metadata_db_path` |

### Environment / settings

| Name | Default |
|------|---------|
| `METADATA_DB_PATH` / `metadata_db_path` | `backend/data/risklens.db` |

Pydantic Settings `env_file` is repo-root `.env`. Extra keys ignored.

### Command line

From root `README.md`:

```text
uvicorn backend.app.main:app --reload --host 127.0.0.1 --port 8080
```

---

## Frontend

No config file. Runtime:

```javascript
window.RISKLENS_PLATFORM_API || "http://127.0.0.1:8000"
```

`API_TIMEOUT_MS = 10000` hardcoded in `app.js`.

---

## Windows agent

### Files

| File | Role |
|------|------|
| `%ProgramData%\RiskLens\agent.yaml` | Runtime default |
| `agent/configs/agent.example.yaml` | Example (localhost backend, several modules on) |
| `agent/configs/agent.lan.example.yaml` | Example `backend_url: http://192.168.4.99:8000`, registration/heartbeat/device_upload enabled |
| `agent/testdata/config/*.yaml` | Test fixtures |
| `internal/configuration/model` | Canonical struct + `Defaults()` |

Default path: `model.DefaultPath()` → `%ProgramData%\RiskLens\agent.yaml`.

### Command line flags

Parsed only when **not** a detected Windows service:

| Flag | Default | Meaning |
|------|---------|---------|
| `-config` | empty | YAML path |
| `-console` | false | Console/debug runner |
| `-version` | — | Print version, exit 0 |

Special argv (not a flag parser): `--activity-foreground`.

Service mode: path from `RISKLENS_AGENT_CONFIG` only.

### Canonical YAML keys and code defaults (`model.Defaults()`)

| YAML key | Code default |
|----------|----------------|
| `schema_version` | `1` |
| `general.service_name` | `RiskLensAgent` |
| `general.display_name` | `RiskLens Endpoint Agent` |
| `general.shutdown_timeout` | `15s` |
| `logging.level` | `info` |
| `logging.file` | `C:\ProgramData\RiskLens\logs\agent.log` |
| `storage.state_dir` | `C:\ProgramData\RiskLens\state` |
| `storage.identity_file` | `...\state\identity.json` |
| `identity.display_name` | `""` |
| `networking.backend_url` | `""` |
| `modules.enabled` | `[]` (unused by `buildModules`) |
| `performance.max_concurrency` | `0` (unused) |
| `registration.enabled` | `false` |
| `registration.timeout` | `15s` |
| `device_upload.enabled` | `false` |
| `device_upload.timeout` | `15s` |
| `heartbeat.enabled` | `false` |
| `heartbeat.timeout` | `10s` |
| `heartbeat.interval` | `30s` |
| `users.enabled` | `false` |
| `users.timeout` | `30s` |
| `users.interval` | `5m` |
| `software.enabled` | `false` |
| `software.timeout` | `60s` |
| `software.interval` | `15m` |
| `activity.enabled` | `false` |
| `activity.timeout` | `15s` |
| `activity.interval` | `15s` |
| `security` | empty |
| `update` | empty |

`agent.example.yaml` turns **users, software, activity** on (`true`) and sets `backend_url: http://127.0.0.1:8000`. Registration/device_upload/heartbeat stay false in that example unless changed locally.

Device collector enablement is **hardcoded true** in `main`, not a YAML key.

Heartbeat module interval fallback: `DefaultInterval = 30s` if config interval `<= 0`.

### Environment overrides (agent)

| Env var | Overrides |
|---------|-----------|
| `RISKLENS_AGENT_CONFIG` | Config file path |
| `RISKLENS_AGENT_LOG_LEVEL` | `logging.level` |
| `RISKLENS_AGENT_LOG_FILE` | `logging.file` |
| `RISKLENS_AGENT_SHUTDOWN_TIMEOUT` | `general.shutdown_timeout` |
| `RISKLENS_AGENT_BACKEND_URL` | `networking.backend_url` |
| `RISKLENS_AGENT_REGISTRATION_ENABLED` | bool (`1/true/yes/on` or `0/false/no/off`) |
| `RISKLENS_AGENT_REGISTRATION_TIMEOUT` | duration |
| `RISKLENS_AGENT_DEVICE_UPLOAD_ENABLED` | bool |
| `RISKLENS_AGENT_DEVICE_UPLOAD_TIMEOUT` | duration |
| `RISKLENS_AGENT_HEARTBEAT_ENABLED` | bool |
| `RISKLENS_AGENT_HEARTBEAT_TIMEOUT` | duration |
| `RISKLENS_AGENT_HEARTBEAT_INTERVAL` | duration |
| `RISKLENS_AGENT_USERS_ENABLED` | bool |
| `RISKLENS_AGENT_USERS_TIMEOUT` | duration |
| `RISKLENS_AGENT_USERS_INTERVAL` | duration |
| `RISKLENS_AGENT_SOFTWARE_ENABLED` | bool |
| `RISKLENS_AGENT_SOFTWARE_TIMEOUT` | duration |
| `RISKLENS_AGENT_SOFTWARE_INTERVAL` | duration |
| `RISKLENS_AGENT_ACTIVITY_ENABLED` | bool |
| `RISKLENS_AGENT_ACTIVITY_TIMEOUT` | duration |
| `RISKLENS_AGENT_ACTIVITY_INTERVAL` | duration |

### Build flags

```text
go build -o bin\RiskLensAgent.exe .\cmd\risklens-agent

go build -ldflags "-X risklens/agent/internal/version.Version=0.1.0 -X risklens/agent/internal/version.Commit=local -X risklens/agent/internal/version.BuildTime=..." -o bin\RiskLensAgent.exe .\cmd\risklens-agent
```

### Install scripts

`agent/scripts/windows/install.ps1` / `uninstall.ps1` — Administrator. Service name `RiskLensAgent`, delayed-auto, restart after 60s.

---

## Root `requirements.txt` / `.env.example`

Documented but **unused by platform and current frontend**:

- `ELASTICSEARCH_URL`, `ELASTICSEARCH_API_KEY`, `DATA_MODE=elastic|mock`

Do not configure these expecting the SPA to query Elasticsearch.

---

## Presence constants (not configurable)

`platform/app/domain/heartbeat/entity.py`:

- `ONLINE_MAX_SECONDS = 60`
- `STALE_MAX_SECONDS = 300`

---

## Default policy (not env)

Migration 0006 seeds policy id `11111111-1111-4111-8111-111111111111`, name `Default Windows Policy`, `heartbeat_interval_seconds = 30`.

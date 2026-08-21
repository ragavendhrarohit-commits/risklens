# RiskLens Windows Agent

Sprint 1 Windows Service skeleton. This binary is not a monitoring agent yet. It installs as a Windows service, loads configuration, writes structured logs, stays running, and shuts down cleanly.

There is no networking, no telemetry collection, and no backend communication.

## Layout

```text
agent/
├── cmd/risklens-agent/     process entry
├── internal/app/           start → wait → shutdown
├── internal/service/       Windows SCM + console runner
├── internal/config/        YAML load and validation
├── internal/logging/       slog JSON logs
├── internal/version/       build-time version
├── configs/                example configuration
├── testdata/config/        config fixtures
└── scripts/windows/        install / uninstall
```

## Build

From this directory, with Go 1.22 or later:

```powershell
go build -o bin\RiskLensAgent.exe .\cmd\risklens-agent
```

Optional version stamping:

```powershell
go build -ldflags "-X risklens/agent/internal/version.Version=0.1.0 -X risklens/agent/internal/version.Commit=local -X risklens/agent/internal/version.BuildTime=2026-08-13T00:00:00Z" -o bin\RiskLensAgent.exe .\cmd\risklens-agent
```

The output must be `RiskLensAgent.exe`.

## Configuration

The shipped example is `configs/agent.example.yaml`.

Runtime default path:

`%ProgramData%\RiskLens\agent.yaml`

Copy the example there before installing the service, or let `scripts/windows/install.ps1` copy it if the file does not already exist.

Invalid configuration prevents startup. Exit codes:

| Code | Meaning |
| ---: | --- |
| 0 | Clean stop |
| 10 | Config missing or unreadable |
| 11 | Config invalid |
| 20 | Logger initialization failed |
| 30 | Service / SCM failure |
| 40 | Application failure |
| 50 | Shutdown deadline exceeded |

Environment overrides: `RISKLENS_AGENT_CONFIG`, `RISKLENS_AGENT_LOG_LEVEL`, `RISKLENS_AGENT_LOG_FILE`, `RISKLENS_AGENT_SHUTDOWN_TIMEOUT`.

## Console mode

```powershell
.\bin\RiskLensAgent.exe -console -config configs\agent.example.yaml
```

Ctrl+C runs the same stop path as a Service Control Manager stop.

```powershell
.\bin\RiskLensAgent.exe -version
```

## Install the Windows service

Build first, then run PowerShell as Administrator:

```powershell
.\scripts\windows\install.ps1
sc.exe start RiskLensAgent
sc.exe query RiskLensAgent
sc.exe stop RiskLensAgent
```

Uninstall:

```powershell
.\scripts\windows\uninstall.ps1
```

The service name is `RiskLensAgent`. It starts as Automatic (delayed) under LocalSystem.

## Logs

JSON logs are written to `%ProgramData%\RiskLens\logs\agent.log` unless `log_file` is changed.

Lifecycle records:

- Version
- Configuration loaded
- Service starting
- Service started
- Service stopping
- Service stopped

The same lifecycle messages are also written to the Windows Application Event Log when the `RiskLensAgent` source is registered.

## Test

```powershell
go test ./...
go vet ./...
```

## Out of scope

Heartbeat, collectors, HTTP/TLS, buffers, enrollment, updates, OpenSearch, and Elasticsearch are not part of this sprint.

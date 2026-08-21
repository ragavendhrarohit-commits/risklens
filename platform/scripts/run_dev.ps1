# Development-only server bind.
# Listens on all interfaces so a second laptop on the LAN can reach the API.
# 127.0.0.1:8000 and localhost continue to work.
# This script does not change production settings or API contracts.
#
# Revert to localhost-only:
#   .\.venv\Scripts\python.exe -B -m uvicorn app.main:app --host 127.0.0.1 --port 8000

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

$python = Join-Path (Get-Location) ".venv\Scripts\python.exe"
if (-not (Test-Path $python)) {
    throw "platform\.venv is missing. Create it with: py -3.13 -m venv .venv"
}

& $python -B -m uvicorn app.main:app --host 0.0.0.0 --port 8000

$ErrorActionPreference = "Stop"

$identity = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $identity.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this script as Administrator."
}

$ServiceName = "RiskLensAgent"
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $existing) {
    Write-Host "$ServiceName is not installed."
    exit 0
}

if ($existing.Status -eq "Running") {
    Stop-Service -Name $ServiceName -Force
}

sc.exe delete $ServiceName | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "sc delete failed"
}

Write-Host "Removed $ServiceName."
Write-Host "ProgramData logs and agent.yaml were left in place."

$ErrorActionPreference = "Stop"

$identity = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $identity.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this script as Administrator."
}

$ServiceName = "RiskLensAgent"
$DisplayName = "RiskLens Endpoint Agent"
$AgentRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$Exe = Join-Path $AgentRoot "bin\RiskLensAgent.exe"
$ProgramData = Join-Path $env:ProgramData "RiskLens"
$ConfigDst = Join-Path $ProgramData "agent.yaml"
$Example = Join-Path $AgentRoot "configs\agent.example.yaml"

if (-not (Test-Path $Exe)) {
    throw "Build the agent first: go build -o bin\RiskLensAgent.exe .\cmd\risklens-agent"
}

New-Item -ItemType Directory -Force -Path (Join-Path $ProgramData "logs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $ProgramData "state") | Out-Null

if (-not (Test-Path $ConfigDst)) {
    Copy-Item $Example $ConfigDst
}

$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    if ($existing.Status -eq "Running") {
        Stop-Service -Name $ServiceName -Force
    }
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 1
}

$create = sc.exe create $ServiceName binPath= "`"$Exe`"" start= delayed-auto DisplayName= "$DisplayName"
if ($LASTEXITCODE -ne 0) {
    throw "sc create failed: $create"
}

sc.exe description $ServiceName "Collects endpoint telemetry for the RiskLens platform." | Out-Null
sc.exe failure $ServiceName reset= 86400 actions= restart/60000 | Out-Null

try {
    if (-not [System.Diagnostics.EventLog]::SourceExists($ServiceName)) {
        New-EventLog -LogName Application -Source $ServiceName
    }
} catch {
    Write-Warning "Could not register Event Log source. Lifecycle events may only appear in the JSON log file."
}

Write-Host "Installed $ServiceName."
Write-Host "Config: $ConfigDst"
Write-Host "Start:  sc.exe start $ServiceName"
Write-Host "Stop:   sc.exe stop $ServiceName"

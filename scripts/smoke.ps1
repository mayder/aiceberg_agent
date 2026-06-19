param(
  [int]$SmokeBackendPort,
  [int]$SmokeHealthPort,
  [string]$SmokeWorkDir,
  [string]$SmokeEvidenceFile,
  [string]$SmokeAgentBin,
  [string]$SmokeBackendBin,
  [switch]$SmokeKeep
)

$ProgressPreference = "SilentlyContinue"

function Get-FreePort {
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  $listener.Start()
  $port = $listener.LocalEndpoint.Port
  $listener.Stop()
  return $port
}

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not $SmokeWorkDir) {
  $SmokeWorkDir = Join-Path $env:TEMP ("aiceberg-smoke." + [System.Guid]::NewGuid().ToString("N"))
}
New-Item -ItemType Directory -Force -Path $SmokeWorkDir | Out-Null

if (-not $SmokeBackendPort) { $SmokeBackendPort = Get-FreePort }
if (-not $SmokeHealthPort) { $SmokeHealthPort = Get-FreePort }

$agentBin = Join-Path $SmokeWorkDir "agent.exe"
$backendBin = Join-Path $SmokeWorkDir "smoke-backend.exe"
$logFile = Join-Path $SmokeWorkDir "agent.debug.log"
$oslogFile = Join-Path $SmokeWorkDir "oslog.log"
if (-not $SmokeEvidenceFile) {
  $SmokeEvidenceFile = Join-Path $SmokeWorkDir "smoke-evidence.json"
}

Write-Host "[smoke] workdir=$SmokeWorkDir"
if ($SmokeAgentBin) {
  Write-Host "[smoke] use prebuilt agent binary"
  Copy-Item -Path $SmokeAgentBin -Destination $agentBin -Force
} else {
  Write-Host "[smoke] build agent"
  go build -o $agentBin ./cmd/agent | Out-Null
}

if ($SmokeBackendBin) {
  Write-Host "[smoke] use prebuilt backend binary"
  Copy-Item -Path $SmokeBackendBin -Destination $backendBin -Force
} else {
  Write-Host "[smoke] build backend"
  go build -o $backendBin ./scripts/e2e_backend.go | Out-Null
}

"Jan  1 00:00:01 host app[123]: hello world" | Out-File -FilePath $oslogFile -Encoding ascii

Write-Host "[smoke] start backend"
$env:E2E_BACKEND_PORT = "$SmokeBackendPort"
$env:E2E_CONFIG_MODE = "payload"
$backend = Start-Process -FilePath $backendBin -NoNewWindow -PassThru -RedirectStandardOutput (Join-Path $SmokeWorkDir "backend.log") -RedirectStandardError (Join-Path $SmokeWorkDir "backend.err")
$agent = $null

try {

function Wait-HttpOk($url) {
  for ($i=0; $i -lt 30; $i++) {
    try {
      Invoke-WebRequest -Uri $url -UseBasicParsing | Out-Null
      return $true
    } catch {}
    Start-Sleep -Seconds 1
  }
  return $false
}

if (-not (Wait-HttpOk "http://127.0.0.1:$SmokeBackendPort/__stats")) {
  Write-Host "[smoke] backend not ready"
  exit 1
}

Write-Host "[smoke] start agent"
$env:AGENT_TOKEN = "token-smoke"
$env:AGENT_MODE = "direct"
$env:API_BASE_URL = "http://127.0.0.1:$SmokeBackendPort"
$env:HEALTH_PORT = "$SmokeHealthPort"
$env:PING_INTERVAL = "2"
$env:CONFIG_SYNC_INTERVAL = "5"
$env:OUTBOX_PATH = Join-Path $SmokeWorkDir "outbox.db"
$env:OUTBOX_MAX_MB = "5"
$env:PREFS_PATH = Join-Path $SmokeWorkDir "prefs.json"
$env:AGENT_TOKEN_PATH = Join-Path $SmokeWorkDir "agent.token"
$env:AGENT_STATE_PATH = Join-Path $SmokeWorkDir "bootstrap.ok"
$env:OSLOG_FILES = $oslogFile
$env:OSLOG_INTERVAL = "1"
$env:OSLOG_BATCH_LINES = "10"
$env:OSLOG_MAX_BYTES = "256"
$env:LOG_LEVEL = "debug"
$env:LOG_FILE_PATH = $logFile
$env:LOG_FILE_MAX_MB = "1"
$env:LOG_FILE_MAX_BACKUPS = "2"

$agent = Start-Process -FilePath $agentBin -NoNewWindow -PassThru -RedirectStandardOutput (Join-Path $SmokeWorkDir "agent.log") -RedirectStandardError (Join-Path $SmokeWorkDir "agent.err")

if (-not (Wait-HttpOk "http://127.0.0.1:$SmokeHealthPort/health")) {
  Write-Host "[smoke] agent not ready"
  exit 1
}

$healthResponse = Invoke-WebRequest -Uri "http://127.0.0.1:$SmokeHealthPort/health" -UseBasicParsing
$metricsResponse = Invoke-WebRequest -Uri "http://127.0.0.1:$SmokeHealthPort/metrics" -UseBasicParsing
if (-not $metricsResponse.Content) {
  Write-Host "[smoke] metrics endpoint returned empty body"
  exit 1
}

$stats = Invoke-WebRequest -Uri "http://127.0.0.1:$SmokeBackendPort/__stats" -UseBasicParsing
$data = $stats.Content | ConvertFrom-Json
if (-not (Test-Path $logFile)) {
  Write-Host "[smoke] log file not found"
  exit 1
}

$health = $healthResponse.Content | ConvertFrom-Json
if ($agent -and !$agent.HasExited) { Stop-Process -Id $agent.Id -Force }
if ($backend -and !$backend.HasExited) { Stop-Process -Id $backend.Id -Force }
$logHash = (Get-FileHash -Algorithm SHA256 -Path $logFile).Hash.ToLowerInvariant()
$fixtureHash = (Get-FileHash -Algorithm SHA256 -Path $oslogFile).Hash.ToLowerInvariant()
$evidence = [ordered]@{
  schema = "aiceberg.agent.smoke.v1"
  generated_at = (Get-Date).ToUniversalTime().ToString("o")
  platform = "windows"
  checks = [ordered]@{
    health_endpoint = $true
    metrics_endpoint = $true
    logs_ingested = ($data.ingested.'/v1/logs/raw' -ge 1)
    windows_eventlog_mode = $true
    debug_log_created = (Test-Path $logFile)
  }
  health = [ordered]@{
    status = $health.status
    agent_pipeline_version = $health.agent_pipeline_version
    queue_items = $health.queue_items
    queue_bytes = $health.queue_bytes
    flush_detail = $health.flush_detail
  }
  backend = [ordered]@{
    ingested = $data.ingested
    ping_get = $data.ping_get
    ping_post = $data.ping_post
    bootstraps = $data.bootstraps
    config_gets = $data.config_gets
  }
  artifacts = [ordered]@{
    agent_log_sha256 = $logHash
    oslog_fixture_sha256 = $fixtureHash
  }
}
$evidence | ConvertTo-Json -Depth 8 | Out-File -FilePath $SmokeEvidenceFile -Encoding utf8
Write-Host "[smoke] evidence=$SmokeEvidenceFile"
Write-Host "[smoke] ok"
} finally {
  if ($agent -and !$agent.HasExited) { Stop-Process -Id $agent.Id -Force }
  if ($backend -and !$backend.HasExited) { Stop-Process -Id $backend.Id -Force }
}

if (-not $SmokeKeep) {
  Remove-Item -Recurse -Force $SmokeWorkDir
} else {
  Write-Host "[smoke] preserving workdir: $SmokeWorkDir"
}

param(
  [string]$ServiceName = "AIcebergAgent",
  [string]$BinPath = "C:\Program Files\AIceberg\agent\agent.exe",
  [string]$DataDir = "C:\ProgramData\AIceberg"
)

$ErrorActionPreference = "Stop"

$updateFile = $env:AICEBERG_UPDATE_FILE
if ([string]::IsNullOrWhiteSpace($updateFile)) {
  throw "AICEBERG_UPDATE_FILE não informado."
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$applyScript = Join-Path $scriptDir "aiceberg-agent-apply-update.ps1"
if (-not (Test-Path $applyScript)) {
  throw "Script de aplicação não encontrado: $applyScript"
}

$logPath = Join-Path $DataDir "logs\self-update.log"

$args = @(
  "-NoProfile",
  "-ExecutionPolicy", "Bypass",
  "-File", ('"{0}"' -f $applyScript),
  "-UpdateFile", ('"{0}"' -f $updateFile),
  "-ServiceName", ('"{0}"' -f $ServiceName),
  "-BinPath", ('"{0}"' -f $BinPath),
  "-LogPath", ('"{0}"' -f $logPath),
  "-HealthUrl", ('"{0}"' -f $env:AICEBERG_AGENT_HEALTH_URL)
)

$proc = Start-Process -FilePath "powershell.exe" -ArgumentList $args -WindowStyle Hidden -PassThru
if ($null -eq $proc) {
  throw "Falha ao iniciar processo de update em background."
}

Write-Output ("spawned update pid={0} file={1}" -f $proc.Id, $updateFile)

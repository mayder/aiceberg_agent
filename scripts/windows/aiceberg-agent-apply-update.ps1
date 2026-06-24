param(
  [Parameter(Mandatory = $true)]
  [string]$UpdateFile,
  [string]$ServiceName = "AIcebergAgent",
  [string]$BinPath = "C:\Program Files\AIceberg\agent\agent.exe",
  [string]$LogPath = "C:\ProgramData\AIceberg\logs\self-update.log",
  [string]$HealthUrl = $env:AICEBERG_AGENT_HEALTH_URL,
  [int]$StopTimeoutSec = 60,
  [int]$StartTimeoutSec = 60
)

$ErrorActionPreference = "Stop"

function Write-Log {
  param([string]$Message)
  $ts = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
  Add-Content -Path $LogPath -Value ("[{0}] {1}" -f $ts, $Message)
}

function Wait-ServiceStatus {
  param(
    [string]$Name,
    [string]$ExpectedStatus,
    [int]$TimeoutSec
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  while ((Get-Date) -lt $deadline) {
    $svc = Get-Service -Name $Name -ErrorAction Stop
    if ($svc.Status.ToString() -eq $ExpectedStatus) {
      return $true
    }
    Start-Sleep -Seconds 1
  }
  return $false
}

function Copy-WithRetry {
  param(
    [string]$Source,
    [string]$Destination,
    [int]$Attempts = 10
  )

  for ($i = 1; $i -le $Attempts; $i++) {
    try {
      Copy-Item -LiteralPath $Source -Destination $Destination -Force
      return
    } catch {
      if ($i -eq $Attempts) {
        throw
      }
      Start-Sleep -Seconds 1
    }
  }
}

function Test-HealthUrl {
  param(
    [string]$Url,
    [int]$TimeoutSec
  )

  if ([string]::IsNullOrWhiteSpace($Url)) {
    return $true
  }

  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  while ((Get-Date) -lt $deadline) {
    try {
      $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3
      if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
        return $true
      }
    } catch {
      Start-Sleep -Seconds 1
      continue
    }
    Start-Sleep -Seconds 1
  }
  return $false
}

function Start-ServiceAndWaitHealthy {
  param(
    [string]$Name,
    [string]$Url,
    [int]$TimeoutSec
  )

  sc.exe start $Name | Out-Null
  if (-not (Wait-ServiceStatus -Name $Name -ExpectedStatus "Running" -TimeoutSec $TimeoutSec)) {
    return $false
  }
  return (Test-HealthUrl -Url $Url -TimeoutSec $TimeoutSec)
}

$logDir = Split-Path -Parent $LogPath
if (-not [string]::IsNullOrWhiteSpace($logDir)) {
  New-Item -ItemType Directory -Force -Path $logDir | Out-Null
}

$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("aiceberg-update-" + [guid]::NewGuid().ToString("N"))
$backupPath = $BinPath + ".bak"

try {
  Write-Log ("starting update service={0} file={1}" -f $ServiceName, $UpdateFile)

  if (-not (Test-Path -LiteralPath $UpdateFile)) {
    throw "Pacote de update não encontrado: $UpdateFile"
  }

  New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
  Expand-Archive -LiteralPath $UpdateFile -DestinationPath $tmpDir -Force

  $newBin = Get-ChildItem -Path $tmpDir -Filter "agent.exe" -File -Recurse | Select-Object -First 1
  if ($null -eq $newBin) {
    throw "agent.exe não encontrado dentro do pacote."
  }

  $binDir = Split-Path -Parent $BinPath
  New-Item -ItemType Directory -Force -Path $binDir | Out-Null

  $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($null -eq $svc) {
    throw "Serviço não encontrado: $ServiceName"
  }

  if ($svc.Status -ne "Stopped") {
    Write-Log ("stopping service {0}" -f $ServiceName)
    sc.exe stop $ServiceName | Out-Null
    if (-not (Wait-ServiceStatus -Name $ServiceName -ExpectedStatus "Stopped" -TimeoutSec $StopTimeoutSec)) {
      throw "Timeout ao parar serviço $ServiceName"
    }
  }

  if (Test-Path -LiteralPath $BinPath) {
    Copy-Item -LiteralPath $BinPath -Destination $backupPath -Force
    Write-Log ("backup created {0}" -f $backupPath)
  }

  Copy-WithRetry -Source $newBin.FullName -Destination $BinPath
  Write-Log ("binary replaced path={0}" -f $BinPath)

  if (-not (Start-ServiceAndWaitHealthy -Name $ServiceName -Url $HealthUrl -TimeoutSec $StartTimeoutSec)) {
    throw "Timeout ao iniciar serviço saudável $ServiceName após update"
  }

  Write-Log "update completed successfully"
} catch {
  Write-Log ("update failed: {0}" -f $_.Exception.Message)
  if ((Test-Path -LiteralPath $backupPath) -and (Test-Path -LiteralPath $BinPath)) {
    try {
      Copy-Item -LiteralPath $backupPath -Destination $BinPath -Force
      Write-Log "rollback binary restored"
      if (-not (Start-ServiceAndWaitHealthy -Name $ServiceName -Url $HealthUrl -TimeoutSec $StartTimeoutSec)) {
        Write-Log "rollback service did not become healthy"
      }
    } catch {
      Write-Log ("rollback failed: {0}" -f $_.Exception.Message)
    }
  }
  throw
} finally {
  if (Test-Path -LiteralPath $tmpDir) {
    Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
  }
}

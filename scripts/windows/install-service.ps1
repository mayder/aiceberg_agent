param(
  [string]$BinPath = 'C:\Program Files\AIceberg\agent\agent.exe',
  [string]$ConfigPath = 'C:\ProgramData\AIceberg\config.yml',
  [string]$ServiceName = 'AIcebergAgent',
  [int]$StopTimeoutSec = 30,
  [int]$StartTimeoutSec = 30,
  [int]$CreateTimeoutSec = 20
)

$ErrorActionPreference = "Stop"

function Get-ServiceCim {
  param([string]$Name)
  return Get-CimInstance Win32_Service -Filter "Name='$Name'" -ErrorAction SilentlyContinue
}

function Wait-ServiceState {
  param(
    [string]$Name,
    [string]$Desired,
    [int]$TimeoutSec
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  while ((Get-Date) -lt $deadline) {
    $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $svc) { return $false }
    if ($svc.Status.ToString() -eq $Desired) { return $true }
    Start-Sleep -Seconds 1
  }
  return $false
}

function Wait-ServiceExists {
  param(
    [string]$Name,
    [int]$TimeoutSec
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  while ((Get-Date) -lt $deadline) {
    $svc = Get-ServiceCim -Name $Name
    if ($null -ne $svc) { return $true }
    Start-Sleep -Milliseconds 500
  }
  return $false
}

function Ensure-Service {
  param(
    [string]$Name,
    [string]$BinaryPath,
    [int]$CreateTimeout
  )
  $svc = Get-ServiceCim -Name $Name
  if ($null -eq $svc) {
    & sc.exe create $Name binPath= $BinaryPath start= auto | Out-Null
    if (-not (Wait-ServiceExists -Name $Name -TimeoutSec $CreateTimeout)) {
      New-Service -Name $Name -BinaryPathName $BinaryPath -DisplayName $Name -StartupType Automatic
      if (-not (Wait-ServiceExists -Name $Name -TimeoutSec $CreateTimeout)) {
        throw "Falha ao criar serviço $Name."
      }
    }
  } else {
    & sc.exe config $Name binPath= $BinaryPath start= auto | Out-Null
  }
}

$cmd = '"' + $BinPath + '" -config "' + $ConfigPath + '"'

if (-not [System.Diagnostics.EventLog]::SourceExists($ServiceName)) {
  New-EventLog -LogName Application -Source $ServiceName
}

Ensure-Service -Name $ServiceName -BinaryPath $cmd -CreateTimeout $CreateTimeoutSec

$svcObj = Get-ServiceCim -Name $ServiceName
if ($svcObj -and $svcObj.State -ne "Stopped") {
  Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
  if (-not (Wait-ServiceState -Name $ServiceName -Desired "Stopped" -TimeoutSec $StopTimeoutSec)) {
    $pid = [int]$svcObj.ProcessId
    if ($pid -gt 0) {
      Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
      Start-Sleep -Seconds 1
    }
    if (-not (Wait-ServiceState -Name $ServiceName -Desired "Stopped" -TimeoutSec $StopTimeoutSec)) {
      throw "Serviço $ServiceName não parou após timeout/kill."
    }
  }
}

Start-Service -Name $ServiceName
if (-not (Wait-ServiceState -Name $ServiceName -Desired "Running" -TimeoutSec $StartTimeoutSec)) {
  throw "Serviço $ServiceName não entrou em Running no tempo esperado."
}

Get-Service -Name $ServiceName

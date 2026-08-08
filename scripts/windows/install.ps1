<#
.SYNOPSIS
Instala o AIceberg Agent no Windows com configuração mínima.

Passos executados:
- Cria pastas de binário e dados (Program Files + ProgramData).
- Copia agent.exe para o destino.
- Copia scripts de auto-update para ProgramData.
- Grava o token em agent.token (se fornecido) e define AGENT_TOKEN_PATH.
- Define variáveis de ambiente (API_BASE_URL, AGENT_MODE, HUB_URL/HUB_TOKEN/HUB_LISTEN_ADDR, SKIP_BOOTSTRAP).
- Define AUTO_UPDATE_* para atualização remota via backend.
- Cria e inicia o serviço Windows (usa install-service.ps1).

Exemplo:
  powershell -ExecutionPolicy Bypass -File .\install.ps1 -Token SEU_TOKEN
#>

param(
  [string]$Token,
  [string]$BaseUrl = "https://api.aiceberg.com.br",
  [string]$Mode = "direct",
  [string]$HubUrl = "",
  [string]$HubToken = "",
  [string]$HubListen = "",
  [switch]$SkipBootstrap,
  [string]$BinPath = "C:\Program Files\AIceberg\agent\agent.exe",
  [string]$DataDir = "C:\ProgramData\AIceberg",
  [string]$ServiceName = "AIcebergAgent"
)

function Require-Admin {
  $current = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal $current
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator)) {
    Write-Error "Execute este script em um PowerShell elevado (Run as Administrator)."
    exit 1
  }
}

Require-Admin

$ErrorActionPreference = "Stop"

$binDir = Split-Path $BinPath
$tokenPath = Join-Path $DataDir "agent.token"
$updateDir = Join-Path $DataDir "update"
$updateLauncherDst = Join-Path $updateDir "aiceberg-agent-update-launcher.ps1"
$updateApplyDst = Join-Path $updateDir "aiceberg-agent-apply-update.ps1"
$updateLauncherSrc = Join-Path $PSScriptRoot "aiceberg-agent-update-launcher.ps1"
$updateApplySrc = Join-Path $PSScriptRoot "aiceberg-agent-apply-update.ps1"
$updateStorageDir = Join-Path $DataDir "updates"

Write-Host "Criando diretórios..."
New-Item -ItemType Directory -Force -Path $binDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path $updateDir | Out-Null
New-Item -ItemType Directory -Force -Path $updateStorageDir | Out-Null

$srcBin = Join-Path $PSScriptRoot "agent.exe"
if (-not (Test-Path $srcBin)) {
  Write-Error "agent.exe não encontrado em $PSScriptRoot. Extraia o pacote completo antes de rodar."
  exit 1
}
if (-not (Test-Path $updateLauncherSrc)) {
  Write-Error "aiceberg-agent-update-launcher.ps1 não encontrado em $PSScriptRoot. Extraia o pacote completo antes de rodar."
  exit 1
}
if (-not (Test-Path $updateApplySrc)) {
  Write-Error "aiceberg-agent-apply-update.ps1 não encontrado em $PSScriptRoot. Extraia o pacote completo antes de rodar."
  exit 1
}

Write-Host "Copiando binário para $BinPath"
Copy-Item $srcBin $BinPath -Force
Write-Host "Copiando scripts de auto-update para $updateDir"
Copy-Item $updateLauncherSrc $updateLauncherDst -Force
Copy-Item $updateApplySrc $updateApplyDst -Force

if ($Token) {
  Write-Host "Gravando token em $tokenPath"
  $Token | Out-File -FilePath $tokenPath -NoNewline -Encoding ASCII
}

Write-Host "Definindo variáveis de ambiente (escopo máquina)..."
[Environment]::SetEnvironmentVariable("AGENT_TOKEN_PATH", $tokenPath, "Machine")
[Environment]::SetEnvironmentVariable("API_BASE_URL", $BaseUrl, "Machine")
[Environment]::SetEnvironmentVariable("AGENT_MODE", $Mode, "Machine")
if ($HubUrl)     { [Environment]::SetEnvironmentVariable("HUB_URL", $HubUrl, "Machine") }
if ($HubToken)   { [Environment]::SetEnvironmentVariable("HUB_TOKEN", $HubToken, "Machine") }
if ($HubListen)  { [Environment]::SetEnvironmentVariable("HUB_LISTEN_ADDR", $HubListen, "Machine") }
if ($SkipBootstrap) { [Environment]::SetEnvironmentVariable("SKIP_BOOTSTRAP", "true", "Machine") }
[Environment]::SetEnvironmentVariable("AUTO_UPDATE_ENABLED", "true", "Machine")
[Environment]::SetEnvironmentVariable("AUTO_UPDATE_COMMAND", "& '$updateLauncherDst'", "Machine")
[Environment]::SetEnvironmentVariable("AUTO_UPDATE_DIR", $updateStorageDir, "Machine")
[Environment]::SetEnvironmentVariable("AUTO_UPDATE_TIMEOUT", "300", "Machine")
[Environment]::SetEnvironmentVariable("AUTO_UPDATE_RETRY_INTERVAL", "30", "Machine")
[Environment]::SetEnvironmentVariable("AUTO_UPDATE_TRUST_REQUIRED", "true", "Machine")
[Environment]::SetEnvironmentVariable("AUTO_UPDATE_TRUST_PUBLIC_KEY", "9a74529340757946a091dff57d1d1fa721d4b129d76748b6e1c4533514c54045", "Machine")

Write-Host "Registrando fonte de log no Windows Event Log..."
if (-not [System.Diagnostics.EventLog]::SourceExists($ServiceName)) {
  New-EventLog -LogName Application -Source $ServiceName
}

$installSvc = Join-Path $PSScriptRoot "install-service.ps1"
if (-not (Test-Path $installSvc)) {
  Write-Error "install-service.ps1 não encontrado em $PSScriptRoot."
  exit 1
}

Write-Host "Criando serviço $ServiceName..."
& $installSvc -BinPath $BinPath -ConfigPath "$DataDir\config.yml" -ServiceName $ServiceName

Write-Host "Serviço criado. Verifique com: sc query $ServiceName"
Write-Host "Se precisar ajustar variáveis, reabra o shell ou reinicie o serviço após editar."

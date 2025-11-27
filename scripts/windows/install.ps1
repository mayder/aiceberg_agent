<#
.SYNOPSIS
Instala o AIceberg Agent no Windows com configuração mínima.

Passos executados:
- Cria pastas de binário e dados (Program Files + ProgramData).
- Copia agent.exe para o destino.
- Grava o token em agent.token (se fornecido) e define AGENT_TOKEN_PATH.
- Define variáveis de ambiente (API_BASE_URL, AGENT_MODE, HUB_URL/HUB_TOKEN/HUB_LISTEN_ADDR, SKIP_BOOTSTRAP).
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

Write-Host "Criando diretórios..."
New-Item -ItemType Directory -Force -Path $binDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

$srcBin = Join-Path $PSScriptRoot "agent.exe"
if (-not (Test-Path $srcBin)) {
  Write-Error "agent.exe não encontrado em $PSScriptRoot. Extraia o pacote completo antes de rodar."
  exit 1
}

Write-Host "Copiando binário para $BinPath"
Copy-Item $srcBin $BinPath -Force

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

$installSvc = Join-Path $PSScriptRoot "install-service.ps1"
if (-not (Test-Path $installSvc)) {
  Write-Error "install-service.ps1 não encontrado em $PSScriptRoot."
  exit 1
}

Write-Host "Criando serviço $ServiceName..."
& $installSvc -BinPath $BinPath -ConfigPath "$DataDir\config.yml" | Out-Null

Write-Host "Serviço criado. Verifique com: sc query $ServiceName"
Write-Host "Se precisar ajustar variáveis, reabra o shell ou reinicie o serviço após editar."

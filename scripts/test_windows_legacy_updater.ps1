# Portable negative-path tests: every case must stop before host modifications.
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'windows/prepare-legacy-update-launcher.ps1'
$env:AICEBERG_AGENT_BIN = 'C:\Program Files\AIceberg\agent\agent.exe'
$env:AICEBERG_UPDATE_VERSION = '0.8.59'
$env:AICEBERG_UPDATE_SHA256 = '0' * 64
$env:AICEBERG_UPDATE_FILE = [IO.Path]::GetTempFileName()
function Get-CimInstance($ClassName, $Filter) {
  return [PSCustomObject]@{PathName = '"C:\Program Files\AIceberg\agent\agent.exe"'}
}
function Assert-PreparationStops([string]$Message) {
  $stopped = $false
  try { & $script }
  catch {
    if (-not $_.Exception.Message.Contains($Message)) { throw }
    $stopped = $true
  }
  if (-not $stopped) { throw 'Invalid preparation reached host modification.' }
}
$env:AICEBERG_AGENT_BIN = 'C:\Other\agent.exe'
Assert-PreparationStops 'existing standard AIcebergAgent'
$env:AICEBERG_AGENT_BIN = 'C:\Program Files\AIceberg\agent\agent.exe'
$env:AICEBERG_UPDATE_VERSION = '0.8.58'
Assert-PreparationStops 'approved signed 0.8.59'
$env:AICEBERG_UPDATE_VERSION = '0.8.59'
Assert-PreparationStops 'checksum mismatch'
Add-Type -AssemblyName System.IO.Compression.FileSystem
$file = [IO.File]::Open($env:AICEBERG_UPDATE_FILE, [IO.FileMode]::Create)
$zip = New-Object IO.Compression.ZipArchive($file, [IO.Compression.ZipArchiveMode]::Create)
$entry = $zip.CreateEntry('aiceberg-agent-windows-amd64/agent.exe')
$stream = $entry.Open(); $stream.WriteByte(1); $stream.Dispose(); $zip.Dispose(); $file.Dispose()
$env:AICEBERG_UPDATE_SHA256 = (Get-FileHash -LiteralPath $env:AICEBERG_UPDATE_FILE -Algorithm SHA256).Hash
Assert-PreparationStops 'Official update script missing or invalid'
Write-Output 'Windows legacy updater: 4 negative cases PASS; no service or updater directory modified.'

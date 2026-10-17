# One-host preparation invoked only by the existing signed individual update flow.
# Preserves service identity, executable location, pairing and collection settings.
$ErrorActionPreference = 'Stop'
function Set-RestrictedUpdaterAcl([string]$Path, [bool]$Directory) {
  $security = if ($Directory) { New-Object Security.AccessControl.DirectorySecurity } else { New-Object Security.AccessControl.FileSecurity }
  $security.SetAccessRuleProtection($true, $false)
  $security.SetOwner((New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')))
  foreach ($sid in @('S-1-5-18', 'S-1-5-32-544')) {
    $identity = New-Object Security.Principal.SecurityIdentifier($sid)
    $inherit = if ($Directory) { [Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit' } else { [Security.AccessControl.InheritanceFlags]::None }
    $rule = New-Object Security.AccessControl.FileSystemAccessRule($identity, 'FullControl', $inherit, 'None', 'Allow')
    $security.AddAccessRule($rule)
  }
  Set-Acl -LiteralPath $Path -AclObject $security
}
$bin = 'C:\Program Files\AIceberg\agent\agent.exe'
$service = Get-CimInstance Win32_Service -Filter "Name='AIcebergAgent'"
if ($null -eq $service -or $env:AICEBERG_AGENT_BIN -ine $bin) {
  throw 'Preparation requires the existing standard AIcebergAgent service and executable.'
}
$servicePath = $service.PathName.Trim()
if (-not ($servicePath -ieq $bin -or $servicePath.StartsWith('"' + $bin + '"', [StringComparison]::OrdinalIgnoreCase))) {
  throw 'Service executable differs from the approved agent path.'
}
$package = $env:AICEBERG_UPDATE_FILE
$expectedHash = $env:AICEBERG_UPDATE_SHA256
if ($env:AICEBERG_UPDATE_VERSION -ne '0.8.59' -or $expectedHash -notmatch '^[a-fA-F0-9]{64}$') {
  throw 'Preparation requires the approved signed 0.8.59 update.'
}
if ((Get-FileHash -LiteralPath $package -Algorithm SHA256).Hash -ine $expectedHash) {
  throw 'Approved package checksum mismatch.'
}
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [IO.Compression.ZipFile]::OpenRead($package)
$content = @{}
try {
  foreach ($name in @('aiceberg-agent-update-launcher.ps1', 'aiceberg-agent-apply-update.ps1')) {
    $entry = $zip.GetEntry('aiceberg-agent-windows-amd64/' + $name)
    if ($null -eq $entry -or $entry.Length -le 0 -or $entry.Length -gt 1048576) { throw 'Official update script missing or invalid.' }
    $stream = $entry.Open()
    $buffer = New-Object IO.MemoryStream
    try { $stream.CopyTo($buffer); $content[$name] = $buffer.ToArray() }
    finally { $stream.Dispose(); $buffer.Dispose() }
  }
} finally { $zip.Dispose() }
$dir = 'C:\ProgramData\AIceberg\update'
if (Test-Path -LiteralPath $dir) {
  if ((Get-Item -LiteralPath $dir -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Updater directory is a reparse point.' }
}
$previousAcl = if (Test-Path -LiteralPath $dir) { (Get-Acl -LiteralPath $dir).Sddl } else { $null }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Set-RestrictedUpdaterAcl $dir $true
$backup = Join-Path $dir ('preparation-backup-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $backup | Out-Null
@{previous_directory_acl = $previousAcl; command = [Environment]::GetEnvironmentVariable('AUTO_UPDATE_COMMAND', 'Machine'); enabled = [Environment]::GetEnvironmentVariable('AUTO_UPDATE_ENABLED', 'Machine')} |
  ConvertTo-Json | Set-Content -LiteralPath (Join-Path $backup 'previous-environment.json') -Encoding UTF8
foreach ($name in $content.Keys) {
  $target = Join-Path $dir $name
  if (Test-Path -LiteralPath $target) {
    if ((Get-Item -LiteralPath $target -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Updater script is a reparse point.' }
    Copy-Item -LiteralPath $target -Destination (Join-Path $backup $name)
    Set-RestrictedUpdaterAcl $target $false
  }
  [IO.File]::WriteAllBytes($target, $content[$name])
  Set-RestrictedUpdaterAcl $target $false
}
$launcher = Join-Path $dir 'aiceberg-agent-update-launcher.ps1'
[Environment]::SetEnvironmentVariable('AUTO_UPDATE_COMMAND', "& '$launcher'", 'Machine')
[Environment]::SetEnvironmentVariable('AUTO_UPDATE_ENABLED', 'true', 'Machine')
& $launcher

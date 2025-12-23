param(
  [string]$BinPath = 'C:\Program Files\AIceberg\agent\agent.exe',
  [string]$ConfigPath = 'C:\ProgramData\AIceberg\config.yml'
)

$serviceName = 'AIcebergAgent'
$cmd = '"' + $BinPath + '" -config "' + $ConfigPath + '"'
if (-not [System.Diagnostics.EventLog]::SourceExists($serviceName)) {
  New-EventLog -LogName Application -Source $serviceName
}
sc.exe create $serviceName binPath= $cmd start= auto
sc.exe start $serviceName

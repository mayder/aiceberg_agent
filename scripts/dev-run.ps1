# Dev helper to run the agent with sane defaults on Windows or PowerShell
$root = Resolve-Path (Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "..")
Set-Location $root

if (-not $env:LOG_LEVEL)   { $env:LOG_LEVEL   = "info" }
if (-not $env:API_BASE_URL){ $env:API_BASE_URL= "http://localhost:8080" }
if (-not $env:HEALTH_PORT) { $env:HEALTH_PORT = "8081" }

go run ./cmd/agent -config ./configs/config.example.yml

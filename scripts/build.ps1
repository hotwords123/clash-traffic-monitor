$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot

Push-Location (Join-Path $projectRoot "frontend")
try {
    npm ci
    npm run build
}
finally {
    Pop-Location
}

Push-Location $projectRoot
try {
    $env:GOTELEMETRY = "off"
    go test ./...
    go build -trimpath -ldflags="-H=windowsgui -s -w" -o clash-traffic-monitor.exe ./cmd/clash-traffic-monitor
}
finally {
    Pop-Location
}

Write-Host "Built $projectRoot\clash-traffic-monitor.exe"

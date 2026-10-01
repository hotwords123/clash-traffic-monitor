param(
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $OutputPath) {
    $OutputPath = Join-Path $projectRoot "clash-traffic-monitor.exe"
}
$OutputPath = [IO.Path]::GetFullPath($OutputPath)

function Invoke-BuildCommand {
    param([string]$FilePath, [string[]]$ArgumentList)
    & $FilePath @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath $($ArgumentList -join ' ') failed with exit code $LASTEXITCODE"
    }
}

$previousTelemetry = $env:GOTELEMETRY
try {
    $env:GOTELEMETRY = "off"
    Push-Location (Join-Path $projectRoot "frontend")
    try {
        Invoke-BuildCommand "npm.cmd" @("ci")
        Invoke-BuildCommand "npm.cmd" @("test")
        Invoke-BuildCommand "npm.cmd" @("run", "build")
    }
    finally {
        Pop-Location
    }

    Push-Location $projectRoot
    try {
        Invoke-BuildCommand "go" @("test", "./...")
        Invoke-BuildCommand "go" @("vet", "./...")
        Invoke-BuildCommand "go" @("build", "-trimpath", "-ldflags=-H=windowsgui -s -w", "-o", $OutputPath, "./cmd/clash-traffic-monitor")
    }
    finally {
        Pop-Location
    }
}
finally {
    $env:GOTELEMETRY = $previousTelemetry
}

Write-Host "Built $OutputPath"

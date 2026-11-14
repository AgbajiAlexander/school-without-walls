param (
    [Parameter(Position=0)]
    [ValidateSet("up", "status")]
    [string]$Action = "up"
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($env:DATABASE_URL)) {
    throw "DATABASE_URL must be set in this PowerShell session."
}

$apiDirectory = Join-Path $PSScriptRoot "..\services\api"
Push-Location $apiDirectory
try {
    & go run ./cmd/migrate $Action
    if ($LASTEXITCODE -ne 0) {
        throw "Migration command failed with exit code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}

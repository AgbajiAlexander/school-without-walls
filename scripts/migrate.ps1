# Scripts/migrate.ps1
# Runs or rolls back PostgreSQL migrations using psql or prints instructions

param (
    [Parameter(Position=0)]
    [ValidateSet("up", "down", "status")]
    [string]$Action = "up",

    [Parameter(Position=1)]
    [string]$DatabaseUrl = $env:DATABASE_URL
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($DatabaseUrl)) {
    $DatabaseUrl = "postgres://sww_user:sww_password@localhost:5432/sww_dev?sslmode=disable"
}

$MigrationDir = Join-Path $PSScriptRoot "../services/api/migrations"

Write-Host "Migration action: $Action" -ForegroundColor Cyan
Write-Host "Target DB: $DatabaseUrl" -ForegroundColor Cyan

$psqlCmd = Get-Command psql -ErrorAction SilentlyContinue

if ($null -eq $psqlCmd) {
    Write-Host "psql command not found in PATH." -ForegroundColor Yellow
    Write-Host "Migration files located at: $MigrationDir"
    Write-Host "You can apply migrations using any PostgreSQL client (pgAdmin, DBeaver, or psql):"
    if ($Action -eq "up") {
        Get-ChildItem -Path $MigrationDir -Filter "*.up.sql" | ForEach-Object {
            Write-Host "  -> Execute: $($_.FullName)"
        }
    } else {
        Get-ChildItem -Path $MigrationDir -Filter "*.down.sql" | ForEach-Object {
            Write-Host "  -> Execute: $($_.FullName)"
        }
    }
    exit 0
}

if ($Action -eq "up") {
    $files = Get-ChildItem -Path $MigrationDir -Filter "*.up.sql" | Sort-Object Name
    foreach ($file in $files) {
        Write-Host "Applying $($file.Name)..." -ForegroundColor Green
        & psql $DatabaseUrl -f $file.FullName
    }
} elseif ($Action -eq "down") {
    $files = Get-ChildItem -Path $MigrationDir -Filter "*.down.sql" | Sort-Object Name -Descending
    foreach ($file in $files) {
        Write-Host "Rolling back $($file.Name)..." -ForegroundColor Yellow
        & psql $DatabaseUrl -f $file.FullName
    }
}

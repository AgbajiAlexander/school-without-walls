# Scripts/dev-api.ps1
# Runs the Go backend API server

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($env:DATABASE_URL)) {
    throw "DATABASE_URL must be set in this PowerShell session."
}

Set-Location -Path "$PSScriptRoot/../services/api"

Write-Host "Starting Go API server on http://localhost:8080..." -ForegroundColor Green
go run ./cmd/server

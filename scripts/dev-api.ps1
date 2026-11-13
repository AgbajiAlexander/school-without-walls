# Scripts/dev-api.ps1
# Runs the Go backend API server

$ErrorActionPreference = "Stop"

Set-Location -Path "$PSScriptRoot/../services/api"

Write-Host "Starting Go API server on http://localhost:8080..." -ForegroundColor Green
go run ./cmd/server

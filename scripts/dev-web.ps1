# Scripts/dev-web.ps1
# Runs the Next.js frontend development server

$ErrorActionPreference = "Stop"

Set-Location -Path "$PSScriptRoot/../apps/web"

if (-not (Test-Path -Path "node_modules")) {
    Write-Host "Installing dependencies in apps/web..." -ForegroundColor Cyan
    cmd.exe /c npm install
}

Write-Host "Starting Next.js development server on http://localhost:3000..." -ForegroundColor Green
cmd.exe /c npm run dev

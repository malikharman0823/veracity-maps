<#
.SYNOPSIS
    Starts Veracity Maps on Windows (full single-server app or hot-reload dev mode).
.PARAMETER Dev
    Starts Go backend (:8000) and Vite frontend dev server (:5173) concurrently.
#>
param(
    [switch]$Dev
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BackendDir = Join-Path $RepoRoot "backend"
$FrontendDir = Join-Path $RepoRoot "frontend"

# Detect primary LAN IPv4 for convenient multi-device & Viewscape access
$LanIP = (Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -notlike "127.*" -and $_.IPAddress -notlike "169.254.*" -and $_.PrefixOrigin -ne "WellKnown" } |
    Select-Object -First 1 -ExpandProperty IPAddress)
if (-not $LanIP) { $LanIP = "localhost" }

if ($Dev) {
    Write-Host "Starting Veracity Maps in DEVELOPMENT mode..." -ForegroundColor Cyan
    Write-Host "  Frontend (Vite): http://localhost:5173  (LAN: http://${LanIP}:5173)" -ForegroundColor Yellow
    Write-Host "  Backend  (Go):   http://localhost:8000  (LAN: http://${LanIP}:8000)" -ForegroundColor Yellow

    $frontendProc = Start-Process -FilePath "cmd.exe" -ArgumentList "/c npm.cmd run dev" -WorkingDirectory $FrontendDir -PassThru
    try {
        Push-Location $BackendDir
        go run main.go
    } finally {
        Pop-Location
        if ($frontendProc -and -not $frontendProc.HasExited) {
            Stop-Process -Id $frontendProc.Id -Force -ErrorAction SilentlyContinue
        }
    }
    exit 0
}

$ExePath = Join-Path $BackendDir "veracity-maps.exe"
if (-not (Test-Path $ExePath)) {
    Write-Host "Compiled binary not found. Building backend\veracity-maps.exe..." -ForegroundColor Yellow
    Push-Location $BackendDir
    try {
        go build -o veracity-maps.exe .
        if ($LASTEXITCODE -ne 0) { throw "go build failed." }
    } finally {
        Pop-Location
    }
}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host " Starting Veracity Maps (Full Application)        " -ForegroundColor Cyan
Write-Host "  Local URL:  http://localhost:8000" -ForegroundColor Green
Write-Host "  LAN URL:    http://${LanIP}:8000" -ForegroundColor Green
Write-Host "  Press Ctrl+C to stop." -ForegroundColor DarkGray
Write-Host "==================================================" -ForegroundColor Cyan

Push-Location $BackendDir
try {
    & $ExePath
} finally {
    Pop-Location
}

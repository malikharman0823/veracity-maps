<#
.SYNOPSIS
    Installs and builds Veracity Maps as a full-fledged local/LAN application on Windows.
.DESCRIPTION
    1. Verifies Go, Node.js/npm, and Python prerequisites.
    2. Initializes .env and frontend/.env from examples if missing.
    3. Creates backend/.venv and installs Python GIS dependencies.
    4. Installs frontend npm packages and builds the production SPA (frontend/dist).
    5. Compiles the standalone Go application binary (backend/veracity-maps.exe).
#>
param(
    [switch]$SkipPython,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

function Assert-Command($Name, $Hint) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        Write-Error "Required tool '$Name' was not found on PATH. $Hint"
        exit 1
    }
}

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host " Veracity Maps — Full Application Installer (Win) " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. Check prerequisites
Assert-Command "go" "Install Go 1.22+ from https://go.dev/dl/"
Assert-Command "node" "Install Node.js 20+ LTS from https://nodejs.org/"
Assert-Command "npm" "Install Node.js 20+ LTS from https://nodejs.org/"

$PythonCmd = $null
if (-not $SkipPython) {
    if (Get-Command "python" -ErrorAction SilentlyContinue) { $PythonCmd = "python" }
    elseif (Get-Command "py" -ErrorAction SilentlyContinue) { $PythonCmd = "py" }
    else {
        Write-Warning "Python 3.10+ not found. GIS bridge (GeoPackage/Raster/CAD/PDF/Composite Tiles) will be unavailable until Python is installed."
    }
}

# 2. Initialize environment files
$RootEnv = Join-Path $RepoRoot ".env"
$RootEnvExample = Join-Path $RepoRoot ".env.example"
if (-not (Test-Path $RootEnv) -and (Test-Path $RootEnvExample)) {
    Copy-Item $RootEnvExample $RootEnv
    Write-Host "[OK] Created .env from .env.example" -ForegroundColor Green
} else {
    Write-Host "[OK] Existing .env preserved" -ForegroundColor DarkGray
}

$FrontendEnv = Join-Path $RepoRoot "frontend\.env"
$FrontendEnvExample = Join-Path $RepoRoot "frontend\.env.example"
if (-not (Test-Path $FrontendEnv) -and (Test-Path $FrontendEnvExample)) {
    Copy-Item $FrontendEnvExample $FrontendEnv
    Write-Host "[OK] Created frontend/.env from frontend/.env.example" -ForegroundColor Green
} else {
    Write-Host "[OK] Existing frontend/.env preserved" -ForegroundColor DarkGray
}

# 3. Python GIS bridge virtual environment
if (-not $SkipPython -and $PythonCmd) {
    $VenvDir = Join-Path $RepoRoot "backend\.venv"
    $VenvPython = Join-Path $VenvDir "Scripts\python.exe"
    if (-not (Test-Path $VenvPython)) {
        Write-Host "`n[1/3] Creating Python virtual environment at backend\.venv..." -ForegroundColor Yellow
        & $PythonCmd -m venv $VenvDir
        if ($LASTEXITCODE -ne 0) { throw "Failed to create Python venv." }
    } else {
        Write-Host "`n[1/3] Using existing Python virtual environment at backend\.venv" -ForegroundColor Yellow
    }
    Write-Host "      Installing Python GIS dependencies (backend\requirements.txt)..." -ForegroundColor Gray
    & $VenvPython -m pip install --upgrade pip
    & $VenvPython -m pip install -r (Join-Path $RepoRoot "backend\requirements.txt")
    if ($LASTEXITCODE -ne 0) { throw "Failed to install Python dependencies." }
    Write-Host "[OK] Python GIS bridge ready" -ForegroundColor Green
}

# 4. Frontend install & production build
if (-not $SkipFrontend) {
    Write-Host "`n[2/3] Installing frontend dependencies and building production UI..." -ForegroundColor Yellow
    Push-Location (Join-Path $RepoRoot "frontend")
    try {
        $prevPref = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        cmd.exe /c "npm.cmd install"
        $installCode = $LASTEXITCODE
        if ($installCode -ne 0) { throw "npm install failed with exit code $installCode" }

        # Build with same-origin API so the Go binary serves both UI and API on any host/LAN IP
        $env:VITE_API_URL = "same-origin"
        cmd.exe /c "npm.cmd run build"
        $buildCode = $LASTEXITCODE
        $ErrorActionPreference = $prevPref
        if ($buildCode -ne 0) { throw "npm run build failed with exit code $buildCode" }
    } finally {
        Remove-Item Env:\VITE_API_URL -ErrorAction SilentlyContinue
        Pop-Location
    }
    Write-Host "[OK] Frontend production bundle built at frontend\dist" -ForegroundColor Green
}

# 5. Build standalone Go application binary
Write-Host "`n[3/3] Compiling Go server binary (backend\veracity-maps.exe)..." -ForegroundColor Yellow
Push-Location (Join-Path $RepoRoot "backend")
try {
    go mod download
    if ($LASTEXITCODE -ne 0) { throw "go mod download failed." }
    go build -o veracity-maps.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed." }
} finally {
    Pop-Location
}
Write-Host "[OK] Compiled backend\veracity-maps.exe" -ForegroundColor Green

Write-Host "`n==================================================" -ForegroundColor Cyan
Write-Host " Installation Complete!" -ForegroundColor Green
Write-Host " Run the application with:" -ForegroundColor White
Write-Host "   powershell -ExecutionPolicy Bypass -File scripts\start.ps1" -ForegroundColor Yellow
Write-Host "==================================================" -ForegroundColor Cyan

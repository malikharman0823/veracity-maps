# Veracity Maps - Automated Comprehensive Test Runner (PowerShell)
# Strict error handling
$ErrorActionPreference = "Stop"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host " Veracity Maps - Automated Comprehensive Test Suite Runner" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$BackendDir = Resolve-Path (Join-Path $ScriptDir "..")
$FrontendDir = Resolve-Path (Join-Path $BackendDir "..\frontend")
$TestdataDir = Resolve-Path (Join-Path $BackendDir "..\testdata\maptiler-mvp")

Write-Host "Backend Directory:  $BackendDir"
Write-Host "Frontend Directory: $FrontendDir"
Write-Host "Testdata Directory: $TestdataDir"
Write-Host ""

# 1. Prerequisite Checks
Write-Host "[1/5] Verifying Tooling Prerequisites..." -ForegroundColor Yellow
try {
    $goVer = go version
    Write-Host "  [OK] Go: $goVer" -ForegroundColor Green
} catch {
    Write-Error "Go compiler not found in PATH."
    exit 1
}

try {
    $nodeVer = node --version
    Write-Host "  [OK] Node.js: $nodeVer" -ForegroundColor Green
} catch {
    Write-Error "Node.js not found in PATH."
    exit 1
}

# 2. Go Formatting Checks
Write-Host "[2/5] Running Go Formatting Verification..." -ForegroundColor Yellow
Set-Location $BackendDir
$unformatted = gofmt -l .
if ($unformatted) {
    Write-Host "  [FAIL] Unformatted files found:" -ForegroundColor Red
    $unformatted | ForEach-Object { Write-Host "    $_" -ForegroundColor Red }
    exit 1
} else {
    Write-Host "  [OK] All Go files adhere to standard formatting." -ForegroundColor Green
}

# 3. Go Static Analysis (go vet)
Write-Host "[3/5] Running Go Vet Static Analysis..." -ForegroundColor Yellow
$vetOutput = go vet ./... 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "  [FAIL] go vet failed:" -ForegroundColor Red
    Write-Host $vetOutput -ForegroundColor Red
    exit $LASTEXITCODE
} else {
    Write-Host "  [OK] go vet passed with 0 warnings." -ForegroundColor Green
}

# 4. Go Unit & Integration Tests (including full API matrix)
Write-Host "[4/5] Running Go Backend Test Suite & API Matrix..." -ForegroundColor Yellow
go test -v -count=1 ./tests/...
if ($LASTEXITCODE -ne 0) {
    Write-Host "  [FAIL] Go test suite failed with exit code $LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
} else {
    Write-Host "  [OK] All Go unit and integration tests passed." -ForegroundColor Green
}

# 5. Frontend Production Build Verification
Write-Host "[5/5] Running Frontend Build Verification..." -ForegroundColor Yellow
Set-Location $FrontendDir
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
cmd.exe /c "npm.cmd run build"
$buildExit = $LASTEXITCODE
$ErrorActionPreference = $prevEAP
if ($buildExit -ne 0) {
    Write-Host "  [FAIL] Frontend build failed with code $buildExit" -ForegroundColor Red
    exit $buildExit
} else {
    Write-Host "  [OK] Frontend production build succeeded." -ForegroundColor Green
}

Set-Location $BackendDir
Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host " All Automated Tests & Builds Completed Successfully (0 Errors)" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
exit 0

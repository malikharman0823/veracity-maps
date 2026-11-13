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
Write-Host "[1/8] Verifying Tooling Prerequisites..." -ForegroundColor Yellow
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
Write-Host "[2/8] Running Go Formatting Verification..." -ForegroundColor Yellow
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
Write-Host "[3/8] Running Go Vet Static Analysis..." -ForegroundColor Yellow
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
$vetOutput = go vet ./... 2>&1
$ErrorActionPreference = $prevEAP
if ($LASTEXITCODE -ne 0) {
    Write-Host "  [FAIL] go vet failed:" -ForegroundColor Red
    Write-Host $vetOutput -ForegroundColor Red
    exit $LASTEXITCODE
} else {
    Write-Host "  [OK] go vet passed with 0 warnings." -ForegroundColor Green
}

# 4. Go Unit & Integration Tests (including full API matrix and package tests)
Write-Host "[4/8] Running All Go Backend Tests & API Matrix..." -ForegroundColor Yellow
go test -v -count=1 ./...
if ($LASTEXITCODE -ne 0) {
    Write-Host "  [FAIL] Go test suite failed with exit code $LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
} else {
    Write-Host "  [OK] All Go unit and integration tests passed." -ForegroundColor Green
}

# 5. Frontend Unit and API Contract Tests
Write-Host "[5/8] Running Frontend Unit & API Contract Tests..." -ForegroundColor Yellow
Set-Location $FrontendDir
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
cmd.exe /c "npm.cmd test"
$testExit = $LASTEXITCODE
$ErrorActionPreference = $prevEAP
if ($testExit -ne 0) {
    Write-Host "  [FAIL] Frontend tests failed with code $testExit" -ForegroundColor Red
    exit $testExit
} else {
    Write-Host "  [OK] All frontend unit and API contract tests passed." -ForegroundColor Green
}

# 6. Frontend Production Build Verification
Write-Host "[6/8] Running Frontend Production Build Verification..." -ForegroundColor Yellow
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

# 7. Production Bundle Budget
Write-Host "[7/8] Enforcing Production Bundle Budget..." -ForegroundColor Yellow
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
cmd.exe /c "npm.cmd run test:bundle"
$bundleExit = $LASTEXITCODE
$ErrorActionPreference = $prevEAP
if ($bundleExit -ne 0) {
    Write-Host "  [FAIL] Production bundle budget failed with code $bundleExit" -ForegroundColor Red
    exit $bundleExit
} else {
    Write-Host "  [OK] Production JavaScript chunks are within budget." -ForegroundColor Green
}

# 8. Rendered Browser End-to-End Verification
Write-Host "[8/8] Running Desktop and Mobile Browser Tests..." -ForegroundColor Yellow
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
cmd.exe /c "npm.cmd run test:e2e"
$e2eExit = $LASTEXITCODE
$ErrorActionPreference = $prevEAP
if ($e2eExit -ne 0) {
    Write-Host "  [FAIL] Browser tests failed with code $e2eExit" -ForegroundColor Red
    exit $e2eExit
} else {
    Write-Host "  [OK] Desktop and mobile browser tests passed." -ForegroundColor Green
}

Set-Location $BackendDir
Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host " All Automated Tests & Builds Completed Successfully (0 Errors)" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
exit 0

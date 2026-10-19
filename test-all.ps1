# Root convenience runner delegating to backend\scripts\test-all.ps1
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$TargetScript = Join-Path $ScriptDir "backend\scripts\test-all.ps1"

if (Test-Path $TargetScript) {
    & powershell -ExecutionPolicy Bypass -File $TargetScript
    exit $LASTEXITCODE
} else {
    Write-Error "Could not locate $TargetScript"
    exit 1
}

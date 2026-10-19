# Directory: `backend/scripts`

Located in [`backend/scripts/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/scripts)

This directory contains automation scripts for testing, building, and verifying the Veracity Maps project on Windows / PowerShell environments.

---

## File: `test-all.ps1`

### Purpose
Provides a single, repeatable, automated command that validates the entire backend and frontend stack from scratch. It exits with code `0` on success or non-zero on any failure.

### Execution Phases
1. **[1/5] Prerequisite Verification**: Checks for Go (`go version`) and Node.js (`node --version`).
2. **[2/5] Go Formatting Checks**: Runs `gofmt -l .` across all Go source files. Fails if unformatted files are found.
3. **[3/5] Static Analysis (`go vet`)**: Runs `go vet ./...` across all packages to detect suspicious constructs.
4. **[4/5] Backend Test Suite & API Matrix**: Executes `go test -v -count=1 ./tests/...`, running all unit tests, least-squares solvers, repository concurrency tests, and full API HTTP integration matrices.
5. **[5/5] Frontend Production Build**: Executes `npm.cmd run build` inside `frontend/` using `cmd.exe /c` to verify that all React components, Tailwind styling, and MapLibre assets compile cleanly without bundling errors.

### Usage
From either the workspace root or the `backend/` directory:
```powershell
powershell -ExecutionPolicy Bypass -File backend\scripts\test-all.ps1
```

package gis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ResolvePythonInterpreter locates the project's isolated virtual environment Python
// interpreter (backend/.venv) first, falling back to "python" / "python3" on PATH.
func ResolvePythonInterpreter() string {
	if envPy := strings.TrimSpace(os.Getenv("VERACITY_PYTHON")); envPy != "" {
		if _, err := os.Stat(envPy); err == nil {
			return envPy
		}
	}

	var relCandidates []string
	if runtime.GOOS == "windows" {
		relCandidates = []string{
			filepath.Join(".venv", "Scripts", "python.exe"),
			filepath.Join("backend", ".venv", "Scripts", "python.exe"),
			filepath.Join("..", ".venv", "Scripts", "python.exe"),
			filepath.Join("..", "backend", ".venv", "Scripts", "python.exe"),
		}
	} else {
		relCandidates = []string{
			filepath.Join(".venv", "bin", "python"),
			filepath.Join("backend", ".venv", "bin", "python"),
			filepath.Join("..", ".venv", "bin", "python"),
			filepath.Join("..", "backend", ".venv", "bin", "python"),
		}
	}

	for _, candidate := range relCandidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if abs, absErr := filepath.Abs(candidate); absErr == nil {
				return abs
			}
			return candidate
		}
	}

	if p, err := exec.LookPath("python"); err == nil {
		return p
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	return "python"
}

// ResolveScriptPath resolves a Python helper script inside backend/scripts regardless
// of whether the process working directory is repo root, backend/, or backend/tests/.
func ResolveScriptPath(scriptName string) string {
	candidates := []string{
		filepath.Join("scripts", scriptName),
		filepath.Join("backend", "scripts", scriptName),
		filepath.Join("..", "scripts", scriptName),
		filepath.Join("..", "backend", "scripts", scriptName),
		filepath.Join("..", "..", "backend", "scripts", scriptName),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			if abs, absErr := filepath.Abs(c); absErr == nil {
				return abs
			}
			return c
		}
	}
	return filepath.Join("backend", "scripts", scriptName)
}

// RunPythonJSON executes a Python script in backend/scripts with separate stdout/stderr
// streams so native C-library notices (GDAL/PROJ/ONNX) on stderr never corrupt JSON output.
func RunPythonJSON(timeout time.Duration, scriptName string, args []string, out interface{}) error {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pyExe := ResolvePythonInterpreter()
	scriptPath := ResolveScriptPath(scriptName)
	cmdArgs := append([]string{scriptPath}, args...)

	cmd := exec.CommandContext(ctx, pyExe, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	rawOut := bytes.TrimSpace(stdout.Bytes())

	// Even when exit code is non-zero, scripts often return structured {"success": false, "code": ..., "error": ...}
	if len(rawOut) > 0 {
		// Find last JSON object line if any stdout prefix occurred
		jsonBytes := extractJSONObject(rawOut)
		if out != nil && len(jsonBytes) > 0 {
			if err := json.Unmarshal(jsonBytes, out); err == nil {
				return nil
			}
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("PROCESSING_TIMEOUT: %s timed out after %s", scriptName, timeout)
	}
	if runErr != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = string(rawOut)
		}
		return fmt.Errorf("PYTHON_EXEC_FAILED: %s failed: %s (%v)", scriptName, errMsg, runErr)
	}
	if len(rawOut) == 0 {
		return fmt.Errorf("PYTHON_EMPTY_OUTPUT: %s returned empty output", scriptName)
	}
	if out != nil {
		if err := json.Unmarshal(extractJSONObject(rawOut), out); err != nil {
			return fmt.Errorf("PYTHON_PARSE_ERROR: Failed parsing JSON from %s: %w", scriptName, err)
		}
	}
	return nil
}

func extractJSONObject(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return trimmed
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return trimmed
	}
	// Locate last line starting with '{'
	lines := bytes.Split(trimmed, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) > 0 && (line[0] == '{' || line[0] == '[') {
			return line
		}
	}
	idx := bytes.IndexByte(trimmed, '{')
	if idx >= 0 {
		return trimmed[idx:]
	}
	return trimmed
}

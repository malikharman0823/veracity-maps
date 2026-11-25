#!/usr/bin/env bash
# Veracity Maps — Full Application Installer (macOS / Linux)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKIP_PYTHON=0
SKIP_FRONTEND=0

for arg in "$@"; do
  case "$arg" in
    --skip-python) SKIP_PYTHON=1 ;;
    --skip-frontend) SKIP_FRONTEND=1 ;;
  esac
done

echo "===================================================="
echo " Veracity Maps — Full Application Installer (Unix)  "
echo "===================================================="

command -v go >/dev/null 2>&1 || { echo "Error: Go 1.22+ is required (https://go.dev/dl/)"; exit 1; }
command -v node >/dev/null 2>&1 || { echo "Error: Node.js 20+ LTS is required (https://nodejs.org/)"; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "Error: npm is required (https://nodejs.org/)"; exit 1; }

# 1. Initialize .env files if missing
if [ ! -f "$REPO_ROOT/.env" ] && [ -f "$REPO_ROOT/.env.example" ]; then
  cp "$REPO_ROOT/.env.example" "$REPO_ROOT/.env"
  echo "[OK] Created .env from .env.example"
else
  echo "[OK] Existing .env preserved"
fi

if [ ! -f "$REPO_ROOT/frontend/.env" ] && [ -f "$REPO_ROOT/frontend/.env.example" ]; then
  cp "$REPO_ROOT/frontend/.env.example" "$REPO_ROOT/frontend/.env"
  echo "[OK] Created frontend/.env from frontend/.env.example"
else
  echo "[OK] Existing frontend/.env preserved"
fi

# 2. Python GIS bridge venv
if [ "$SKIP_PYTHON" -eq 0 ]; then
  PYTHON_CMD=""
  if command -v python3 >/dev/null 2>&1; then
    PYTHON_CMD="python3"
  elif command -v python >/dev/null 2>&1; then
    PYTHON_CMD="python"
  fi

  if [ -n "$PYTHON_CMD" ]; then
    VENV_DIR="$REPO_ROOT/backend/.venv"
    if [ ! -x "$VENV_DIR/bin/python" ]; then
      echo ""
      echo "[1/3] Creating Python virtual environment at backend/.venv..."
      "$PYTHON_CMD" -m venv "$VENV_DIR"
    else
      echo ""
      echo "[1/3] Using existing Python virtual environment at backend/.venv"
    fi
    "$VENV_DIR/bin/python" -m pip install --upgrade pip
    "$VENV_DIR/bin/python" -m pip install -r "$REPO_ROOT/backend/requirements.txt"
    echo "[OK] Python GIS bridge ready"
  else
    echo "[WARN] python3 not found; skipping Python GIS bridge setup."
  fi
fi

# 3. Frontend dependencies & production bundle
if [ "$SKIP_FRONTEND" -eq 0 ]; then
  echo ""
  echo "[2/3] Installing frontend dependencies and building production UI..."
  (
    cd "$REPO_ROOT/frontend"
    npm install
    VITE_API_URL=same-origin npm run build
  )
  echo "[OK] Frontend production bundle built at frontend/dist"
fi

# 4. Build standalone Go binary
echo ""
echo "[3/3] Compiling Go server binary (backend/veracity-maps)..."
(
  cd "$REPO_ROOT/backend"
  go mod download
  go build -o veracity-maps .
)
echo "[OK] Compiled backend/veracity-maps"

echo ""
echo "===================================================="
echo " Installation Complete!"
echo " Start the application with:"
echo "   bash scripts/start.sh"
echo "===================================================="

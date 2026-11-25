#!/usr/bin/env bash
# Starts Veracity Maps on macOS / Linux (full single-server app or --dev mode).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$REPO_ROOT/backend"
FRONTEND_DIR="$REPO_ROOT/frontend"
DEV_MODE=0

for arg in "$@"; do
  case "$arg" in
    --dev) DEV_MODE=1 ;;
  esac
done

LAN_IP="$(hostname -I 2>/dev/null | awk '{print $1}' || ipconfig getifaddr en0 2>/dev/null || echo "localhost")"
[ -z "$LAN_IP" ] && LAN_IP="localhost"

if [ "$DEV_MODE" -eq 1 ]; then
  echo "Starting Veracity Maps in DEVELOPMENT mode..."
  echo "  Frontend (Vite): http://localhost:5173  (LAN: http://${LAN_IP}:5173)"
  echo "  Backend  (Go):   http://localhost:8000  (LAN: http://${LAN_IP}:8000)"
  (cd "$FRONTEND_DIR" && npm run dev) &
  FE_PID=$!
  trap 'kill "$FE_PID" 2>/dev/null || true' EXIT INT TERM
  (cd "$BACKEND_DIR" && go run main.go)
  exit 0
fi

BIN_PATH="$BACKEND_DIR/veracity-maps"
if [ ! -x "$BIN_PATH" ]; then
  echo "Compiled binary not found. Building backend/veracity-maps..."
  (cd "$BACKEND_DIR" && go build -o veracity-maps .)
fi

echo "===================================================="
echo " Starting Veracity Maps (Full Application)          "
echo "  Local URL:  http://localhost:8000"
echo "  LAN URL:    http://${LAN_IP}:8000"
echo "  Press Ctrl+C to stop."
echo "===================================================="

cd "$BACKEND_DIR"
exec "$BIN_PATH"

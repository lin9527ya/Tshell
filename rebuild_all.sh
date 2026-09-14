#!/usr/bin/env bash
# rebuild_all.sh - one-shot rebuild: frontend + server + implant templates
# 1. Build frontend via build:embed (vite build + auto-copy dist to server dist)
# 2. Rebuild server
# 3. Run build_templates to refresh all implant template binaries
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== [1/3] Build frontend (build:embed) ==="
cd "$SCRIPT_DIR/Tshell-web"
[ -d node_modules ] || npm install
npm run build:embed 2>&1 | tail -5

echo "=== [2/3] Rebuild server ==="
cd "$SCRIPT_DIR/Tshell-server"
GOPROXY=https://goproxy.cn,direct go build -o server.exe .

echo "=== [3/3] Refresh implant templates ==="
cd "$SCRIPT_DIR/Tshell-client/implant"
if command -v powershell >/dev/null 2>&1; then
  powershell -ExecutionPolicy Bypass -File build_templates.ps1 2>&1 | tail -5
else
  bash build_templates.sh 2>&1 | tail -5
fi

echo ""
echo "=== ALL DONE. Restart server.exe to apply. ==="

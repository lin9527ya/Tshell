# rebuild_all.ps1 - one-shot rebuild: frontend + server + implant templates
# 1. Build frontend via build:embed (vite build + auto-copy dist to Tshell-server/dist)
# 2. Rebuild server.exe
# 3. Run build_templates.ps1 to refresh all implant template binaries
# Usage: powershell -ExecutionPolicy Bypass -File rebuild_all.ps1

$ErrorActionPreference = "Continue"
# 脚本位于项目根目录 E:\Code\Tshell
$Root = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path $MyInvocation.MyCommand.Path }

Write-Host "=== [1/3] Build frontend (build:embed, auto-copy to server dist) ==="
Set-Location (Join-Path $Root "Tshell-web")
if (-not (Test-Path "node_modules")) {
    Write-Host "node_modules missing, running npm install..."
    npm install
}
npm run build:embed 2>&1 | Select-Object -Last 5
if ($LASTEXITCODE -ne 0) { Write-Host "[1/3] frontend build FAILED (continue anyway)" }

Write-Host "=== [2/3] Rebuild server ==="
Set-Location (Join-Path $Root "Tshell-server")
$env:GOPROXY = "https://goproxy.cn,direct"
go build -o server.exe .
if ($LASTEXITCODE -ne 0) { Write-Host "[2/3] server build FAILED"; exit 1 }
Remove-Item Env:GOPROXY -ErrorAction SilentlyContinue

Write-Host "=== [3/3] Refresh implant templates ==="
Set-Location (Join-Path $Root "Tshell-client\implant")
powershell -ExecutionPolicy Bypass -File build_templates.ps1 2>&1 | Select-Object -Last 5

Write-Host ""
Write-Host "=== ALL DONE. Restart server.exe to apply. ==="

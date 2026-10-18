# Tshell 统一构建脚本(Windows PowerShell 版;Linux/macOS/Git Bash 请用 build.sh)
#
# 流水线: [1/3] 前端 build:embed -> [2/3] 服务端 CGO 构建 -> [3/3] implant 模板重建
# (顺序敏感: 前端产物与服务端模板均为 go:embed 依赖,须先于服务端构建完成)
#
# 用法:
#   powershell -ExecutionPolicy Bypass -File build.ps1                # 全量
#   powershell -ExecutionPolicy Bypass -File build.ps1 -SkipTemplates # 跳过模板重建
#   powershell -ExecutionPolicy Bypass -File build.ps1 -FrontendOnly / -ServerOnly / -TemplatesOnly
param(
    [switch]$FrontendOnly,
    [switch]$ServerOnly,
    [switch]$TemplatesOnly,
    [switch]$SkipTemplates,
    [switch]$Help
)

# 控制台 UTF-8 输出(中文正确显示)
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}
$OutputEncoding = [System.Text.Encoding]::UTF8
chcp 65001 | Out-Null

if ($Help) {
    Get-Content $PSCommandPath -TotalCount 12 | Write-Host
    exit 0
}

$ErrorActionPreference = "Stop"
$Root = $PSScriptRoot
if (-not $Root) { $Root = Split-Path $MyInvocation.MyCommand.Path }

function Write-Step($n, $msg) { Write-Host "=== [$n] $msg ===" -ForegroundColor Cyan }
function Write-Ok($msg)       { Write-Host "[ OK ] $msg" -ForegroundColor Green }

# ---------- [1/3] 前端 ----------
function Build-Frontend {
    Write-Step "1/3" "前端构建(build:embed,产物自动写入 Tshell-server/dist)"
    Set-Location (Join-Path $Root "Tshell-web")
    if (-not (Test-Path "node_modules")) { npm ci --legacy-peer-deps }
    npm run build:embed
    if ($LASTEXITCODE -ne 0) { throw "前端构建失败" }
    Write-Ok "前端构建完成"
}

# ---------- [2/3] 服务端 ----------
function Build-Server {
    Write-Step "2/3" "服务端 CGO 构建(go-sqlite3 需要 CGO)"
    # C 编译器探测: PATH 优先,回退 TDM-GCC 默认安装路径
    # (回退时把目录前置到 PATH——cgo 经 PATH 查找 gcc,不依赖 CC 显式路径)
    $gcc = Get-Command gcc -ErrorAction SilentlyContinue
    if ($gcc) { $gccPath = $gcc.Source }
    elseif (Test-Path "C:\TDM-GCC-64\bin\gcc.exe") {
        $gccPath = "C:\TDM-GCC-64\bin\gcc.exe"
        $env:PATH = "C:\TDM-GCC-64\bin;" + $env:PATH
    }
    else { throw "未找到 gcc(go-sqlite3 需要 CGO);请安装 TDM-GCC 或 mingw-w64 后重试" }
    Write-Host "C 编译器: $gccPath"

    Set-Location (Join-Path $Root "Tshell-server")
    # GOMEMLIMIT: 服务端嵌入模板+前端后约 1.4GB,低内存机器不限流会构建卡死(见 DEPLOYMENT.md)
    # 【构建配方】必须用文件模式 .\main.go(实证: Windows+TDM-GCC 下包模式 `.` 产出的
    # cgo exe 无法执行——"not a valid Win32 application";文件模式已验证可用)
    $env:CGO_ENABLED = "1"
    $env:GOPROXY = "https://goproxy.cn,direct"; $env:GOMEMLIMIT = "3GiB"
    # 版本信息注入：/api/version 可查，便于产物溯源
    $Version = (git describe --tags --always --dirty 2>$null); if (-not $Version) { $Version = "dev" }
    $Commit = (git rev-parse --short HEAD 2>$null); if (-not $Commit) { $Commit = "unknown" }
    $BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    go build -ldflags "-s -w -X main.Version=$Version -X main.GitCommit=$Commit -X main.BuildTime=$BuildTime" -trimpath -o tshell-server.exe .\main.go
    if ($LASTEXITCODE -ne 0) { throw "服务端构建失败" }
    Remove-Item Env:CGO_ENABLED, Env:GOPROXY, Env:GOMEMLIMIT -ErrorAction SilentlyContinue
    Write-Ok "服务端构建完成: Tshell-server\tshell-server.exe(重启生效)"
}

# ---------- [3/3] implant 模板 ----------
function Build-Templates {
    Write-Step "3/3" "重建 implant 客户端模板(166 EXE + 24 DLL -> Tshell-server/pkg/api/server/)"
    Set-Location (Join-Path $Root "Tshell-client\implant")
    powershell -ExecutionPolicy Bypass -File build_templates.ps1
    Write-Ok "模板重建完成"
}

# ---------- 编排 ----------
try {
    if ($FrontendOnly)  { Build-Frontend;  return }
    if ($ServerOnly)    { Build-Server;    return }
    if ($TemplatesOnly) { Build-Templates; return }

    Build-Frontend
    Build-Server
    if ($SkipTemplates) {
        Write-Host "跳过模板重建(-SkipTemplates);模板未变时可直接复用现有 Tshell-server/pkg/api/server/" -ForegroundColor Yellow
    } else {
        Build-Templates
    }
    Write-Ok "全部完成。重启服务端进程生效。"
} catch {
    Write-Host "[FAIL] $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

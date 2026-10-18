#!/usr/bin/env bash
# Tshell 统一构建脚本(bash 版;Windows PowerShell 请用 build.ps1)
#
# 流水线: [1/3] 前端 build:embed → [2/3] 服务端 CGO 构建 → [3/3] implant 模板重建
# (顺序敏感: 前端产物与服务端模板均为 go:embed 依赖,须先于服务端构建完成)
#
# 用法:
#   ./build.sh                     # 全量(前端 + 服务端 + 模板)
#   ./build.sh --skip-templates    # 跳过耗时的模板重建(166 EXE + 24 DLL)
#   ./build.sh --frontend-only     # 仅前端
#   ./build.sh --server-only       # 仅服务端
#   ./build.sh --templates-only    # 仅模板
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

FRONTEND_ONLY=0; SERVER_ONLY=0; TEMPLATES_ONLY=0; SKIP_TEMPLATES=0
for arg in "$@"; do
  case "$arg" in
    --frontend-only)  FRONTEND_ONLY=1 ;;
    --server-only)    SERVER_ONLY=1 ;;
    --templates-only) TEMPLATES_ONLY=1 ;;
    --skip-templates) SKIP_TEMPLATES=1 ;;
    -h|--help)        sed -n '2,14p' "$0"; exit 0 ;;
    *)                echo "未知参数: $arg(见 --help)"; exit 1 ;;
  esac
done

log()  { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[ OK ]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[FAIL]\033[0m %s\n' "$*"; exit 1; }

UNAME_S="$(uname -s)"
IS_WIN_BASH=0
case "$UNAME_S" in MINGW*|MSYS*|CYGWIN*) IS_WIN_BASH=1 ;; esac

# ---------- [1/3] 前端 ----------
build_frontend() {
  log "[1/3] 前端构建(build:embed,产物自动写入 Tshell-server/dist)"
  cd "$SCRIPT_DIR/Tshell-web"
  [ -d node_modules ] || npm ci --legacy-peer-deps
  npm run build:embed
  ok "前端构建完成"
}

# ---------- [2/3] 服务端 ----------
build_server() {
  log "[2/3] 服务端 CGO 构建(go-sqlite3 需要 CGO)"
  cd "$SCRIPT_DIR/Tshell-server"

  # C 编译器探测: PATH 优先;Windows Git Bash 下回退 TDM-GCC 默认安装路径
  # (回退时将该目录前置到 PATH——cgo 通过 PATH 查找 gcc,不依赖 CC 显式路径)
  CC_BIN="$(command -v gcc || command -v clang || true)"
  if [ -z "$CC_BIN" ] && [ "$IS_WIN_BASH" = 1 ] && [ -x "/c/TDM-GCC-64/bin/gcc.exe" ]; then
    CC_BIN="/c/TDM-GCC-64/bin/gcc.exe"
    export PATH="/c/TDM-GCC-64/bin:$PATH"
  fi
  [ -n "$CC_BIN" ] || fail "未找到 gcc/clang(go-sqlite3 需要 CGO)。Windows 安装 TDM-GCC/mingw-w64,Linux 安装 build-essential 后重试"

  local out="tshell-server"
  [ "$IS_WIN_BASH" = 1 ] && out="tshell-server.exe"

  # GOMEMLIMIT: 服务端嵌入模板+前端后约 1.4GB,低内存机器不限流会构建卡死(见 DEPLOYMENT.md)
  log "C 编译器: $CC_BIN"
  # 【构建配方】必须用文件模式 ./main.go(本机实证: Windows+TDM-GCC 下包模式 `.`
  # 产出的 cgo exe 无法执行——"not a valid Win32 application";文件模式已验证可用)。
  # CC 传 PATH 中的编译器名而非显式路径,与环境变量解析保持一致。
  CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct GOMEMLIMIT=3GiB \
    # 版本信息注入：/api/version 可查，便于产物溯源
    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
    BUILDTIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    go build -ldflags "-s -w -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILDTIME}" -trimpath -o "$out" ./main.go
  ok "服务端构建完成: Tshell-server/$out(重启生效)"
}

# ---------- [3/3] implant 模板 ----------
build_templates() {
  log "[3/3] 重建 implant 客户端模板(166 EXE + 24 DLL → Tshell-server/pkg/api/server/)"
  cd "$SCRIPT_DIR/Tshell-client/implant"
  if [ "$IS_WIN_BASH" = 1 ]; then
    powershell -ExecutionPolicy Bypass -File build_templates.ps1
  else
    bash build_templates.sh
  fi
  ok "模板重建完成"
}

# ---------- 编排 ----------
if [ "$FRONTEND_ONLY" = 1 ];  then build_frontend;  exit 0; fi
if [ "$SERVER_ONLY" = 1 ];    then build_server;    exit 0; fi
if [ "$TEMPLATES_ONLY" = 1 ]; then build_templates; exit 0; fi

build_frontend
build_server
if [ "$SKIP_TEMPLATES" = 1 ]; then
  log "跳过模板重建(--skip-templates);模板未变时可直接复用现有 Tshell-server/pkg/api/server/"
else
  build_templates
fi

log "全部完成。重启服务端进程生效。"

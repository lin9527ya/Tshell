#!/usr/bin/env bash
# 可复现构建：同一份源码 + 同一工具链 + 同一参数 → **逐字节相同**的产物。
#
# 为什么 C2 需要这个：运维拿到的二进制要么来自自己构建，要么来自别人的交付。
# 没有可复现性，"这个产物确实是我审查过的那份源码编译出来的"就无法验证 ——
# 对 C2 这种本身就是高价值攻击目标的软件，这是供应链上最要紧的一环。
#
# 用法:
#   scripts/repro-build.sh              # 构建两遍并比对，输出报告
#   scripts/repro-build.sh --check      # 同上，不一致时退出码 1（CI 用）
#   scripts/repro-build.sh --target server|implant|all
#
# 环境变量:
#   SOURCE_DATE_EPOCH  固定构建时间（缺省用 HEAD 提交时间；这是可复现性的关键）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="all"
CHECK=0
OUT_DIR="${REPO_ROOT}/dist/repro"

while [ $# -gt 0 ]; do
  case "$1" in
    --check)  CHECK=1; shift ;;
    --target) TARGET="$2"; shift 2 ;;
    --out)    OUT_DIR="$2"; shift 2 ;;
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
    *) echo "未知参数: $1（见 --help）" >&2; exit 1 ;;
  esac
done

log() { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
ok()  { printf '\033[1;32m[ OK ]\033[0m %s\n' "$*"; }
bad() { printf '\033[1;31m[FAIL]\033[0m %s\n' "$*"; }

# ---------- 确定性构建输入 ----------
cd "$REPO_ROOT"
if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
  SOURCE_DATE_EPOCH="$(git log -1 --format=%ct 2>/dev/null || echo 0)"
fi
export SOURCE_DATE_EPOCH
BUILD_TIME="$(date -u -d "@${SOURCE_DATE_EPOCH}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo 1970-01-01T00:00:00Z)"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DIRTY="no"; [ -n "$(git status --porcelain 2>/dev/null)" ] && DIRTY="yes"
GO_VERSION="$(go version)"
GOFLAGS_FIXED="-trimpath -buildvcs=false"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/a" "$OUT_DIR/b"

# 服务端 ldflags：时间来自 SOURCE_DATE_EPOCH（不是"现在"），这是可复现的前提
SERVER_LDFLAGS="-s -w -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILD_TIME}"
# implant 的 ldflags 与 build_templates.sh 一致（无时间戳）
IMPLANT_LDFLAGS="-s -w"

build_once() { # $1=目录 $2=目标
  local dir="$1" tgt="$2"
  case "$tgt" in
    server)
      ( cd "$REPO_ROOT/Tshell-server" && \
        CGO_ENABLED=1 go build $GOFLAGS_FIXED -ldflags "$SERVER_LDFLAGS" -o "$dir/tshell-server" ./main.go )
      ;;
    implant)
      ( cd "$REPO_ROOT/Tshell-client/implant" && \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $GOFLAGS_FIXED \
          -tags "transport_tcp" -ldflags "$IMPLANT_LDFLAGS" -o "$dir/implant-tcp-linux-amd64" ./cmd/single )
      ;;
  esac
}

# ---------- 构建两遍并比对 ----------
ARTIFACTS=()
TARGETS=()
case "$TARGET" in
  all) TARGETS=(implant server) ;;   # implant 先跑：纯 Go，不受 cgo 影响
  server|implant) TARGETS=("$TARGET") ;;
  *) echo "未知目标: $TARGET" >&2; exit 1 ;;
esac

FAILED=0
for tgt in "${TARGETS[@]}"; do
  # 每个目标一个子目录：否则上一轮的产物会被再次比对（清单里也会重复）
  mkdir -p "$OUT_DIR/a/$tgt" "$OUT_DIR/b/$tgt"
  log "构建 $tgt（第 1 遍）…"
  build_once "$OUT_DIR/a/$tgt" "$tgt"
  log "构建 $tgt（第 2 遍，同参数）…"
  build_once "$OUT_DIR/b/$tgt" "$tgt"

  for f in "$OUT_DIR/a/$tgt"/*; do
    name="$(basename "$f")"
    [ -f "$OUT_DIR/b/$tgt/$name" ] || continue
    h1="$(sha256sum "$f" | cut -d' ' -f1)"
    h2="$(sha256sum "$OUT_DIR/b/$tgt/$name" | cut -d' ' -f1)"
    size="$(stat -c%s "$f")"
    if [ "$h1" = "$h2" ]; then
      ok "$name 可复现 (sha256 ${h1:0:16}…, ${size} 字节)"
      ARTIFACTS+=("{\"name\":\"$name\",\"sha256\":\"$h1\",\"size\":$size,\"reproducible\":true,\"target\":\"$tgt\"}")
    else
      bad "$name 两次构建不一致: ${h1:0:16}… vs ${h2:0:16}…"
      ARTIFACTS+=("{\"name\":\"$name\",\"sha256\":\"$h1\",\"sha256_second\":\"$h2\",\"size\":$size,\"reproducible\":false,\"target\":\"$tgt\"}")
      FAILED=1
    fi
  done
done

# ---------- 清单 + SBOM ----------
MANIFEST="$OUT_DIR/build-manifest.json"
{
  echo "{"
  echo "  \"version\": \"$VERSION\","
  echo "  \"commit\": \"$COMMIT\","
  echo "  \"dirty_worktree\": \"$DIRTY\","
  echo "  \"source_date_epoch\": $SOURCE_DATE_EPOCH,"
  echo "  \"build_time\": \"$BUILD_TIME\","
  echo "  \"go_version\": \"$(echo "$GO_VERSION" | sed 's/"/\\"/g')\","
  echo "  \"goflags\": \"$GOFLAGS_FIXED\","
  echo "  \"server_ldflags\": \"$(echo "$SERVER_LDFLAGS" | sed 's/"/\\"/g')\","
  echo "  \"implant_ldflags\": \"$IMPLANT_LDFLAGS\","
  echo "  \"artifacts\": ["
  for i in "${!ARTIFACTS[@]}"; do
    sep=","; [ "$i" -eq $(( ${#ARTIFACTS[@]} - 1 )) ] && sep=""
    echo "    ${ARTIFACTS[$i]}$sep"
  done
  echo "  ]"
  echo "}"
} > "$MANIFEST"
ok "构建清单: $MANIFEST"

# SBOM：直接来自模块图（无外部工具依赖）
( cd "$REPO_ROOT/Tshell-server" && go list -m -json all 2>/dev/null ) > "$OUT_DIR/sbom-server.json" || true
( cd "$REPO_ROOT/Tshell-client/implant" && go list -m -json all 2>/dev/null ) > "$OUT_DIR/sbom-implant.json" || true
ok "SBOM: $OUT_DIR/sbom-{server,implant}.json"

if [ "$FAILED" = 1 ]; then
  bad "存在不可复现的产物（详见 $MANIFEST）"
  [ "$CHECK" = 1 ] && exit 1
else
  ok "全部产物可复现"
fi

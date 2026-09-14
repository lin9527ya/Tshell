#!/usr/bin/env bash
# Tshell e2e 回归脚本：启动服务端 → 添加 TCP 监听器 → 生成客户端 →
# 验证会话密钥唯一化 + 命令执行 → 清理。
#
# 用法: bash scripts/e2e.sh
# 前置: Tshell-server 已构建(go build -o /tmp/e2e/tshell-server . 或指定 BIN=路径)
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$ROOT/Tshell-server"
WORK="$(mktemp -d /tmp/tshell-e2e.XXXXXX)"
PORT=${PORT:-8099}
LISTENER_PORT=8093
VKEY="emd9IjJJxpyAkqhftfA5pJuEygHsn0lc"
SALT="KJgPl1emASxQafzq"
PASS="pass123"
BIN="${BIN:-$WORK/tshell-server}"

log()  { echo -e "\033[1;32m[e2e]\033[0m $*"; }
fail() { echo -e "\033[1;31m[e2e FAIL]\033[0m $*"; cleanup; exit 1; }

cleanup() {
  # 【修复】启动命令用相对路径 config.yaml，原 pkill 模式 "-config $WORK" 永不匹配
  # → 残留服务端占用 8099/8093 导致后续运行登录失败
  pkill -9 -f "tshell-server -config" 2>/dev/null
  pkill -9 -x tshell-server 2>/dev/null
  pkill -9 -f "$WORK/client" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT

log "工作目录: $WORK"
cd "$WORK"

# 1. 准备服务端二进制与配置
if [ ! -f "$BIN" ]; then
  log "构建服务端二进制(耗时较长)..."
  (cd "$SERVER_DIR" && GOMEMLIMIT=3GiB go build -gcflags="all=-l" -o "$WORK/tshell-server" .) || fail "服务端构建失败"
fi
sed -e "s/port: 8089/port: $PORT/" \
    -e "s|path: ./database.db|path: $WORK/database.db|" \
    -e "s|secret_key_file: ./jwt_key.hex|secret_key_file: $WORK/jwt_key.hex|" \
    -e "s/default_password: .*/default_password: Admin@12345/" \
    "$SERVER_DIR/config.yaml" > config.yaml

# 2. 启动服务端(脱离会话,防工具清理)
setsid nohup "$BIN" -config config.yaml -p "$PORT" > server.log 2>&1 < /dev/null &
for i in $(seq 1 30); do
  grep -q "Listening on port" server.log 2>/dev/null && break
  sleep 1
done
grep -q "Listening on port" server.log || fail "服务端未在 $PORT 启动"

# 3. 等数据库初始化完成(默认密码 Admin@12345 由 config 注入)
sleep 2

# 4. 登录 + 添加监听器 + 生成客户端
TOKEN=""
for i in $(seq 1 10); do
  TOKEN=$(curl -s -m 5 -X POST "http://127.0.0.1:$PORT/api/users/login" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"Admin@12345"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('access_token',''))" 2>/dev/null)
  [ -n "$TOKEN" ] && break
  sleep 2
done
[ -n "$TOKEN" ] || fail "登录失败"
log "登录成功"

curl -s -m 10 -X POST "http://127.0.0.1:$PORT/api/listener/add" \
  -H "Content-Type: application/json" -H "Authorization2: Bearer $TOKEN" \
  -d "{\"type\":\"tcp\",\"listenAddress\":\"0.0.0.0:$LISTENER_PORT\",\"connectAddress\":\"127.0.0.1:$LISTENER_PORT\",\"vkey\":\"$VKEY\",\"encryptionSalt\":\"$SALT\"}" > /dev/null

curl -s -m 30 -X POST "http://127.0.0.1:$PORT/api/client/GenServer" \
  -H "Content-Type: application/json" -H "Authorization2: Bearer $TOKEN" \
  -d "{\"osType\":\"linux\",\"archType\":\"amd64\",\"listener\":\"tcp://127.0.0.1:$LISTENER_PORT\",\"pass\":\"$PASS\",\"vkey\":\"$VKEY\",\"encryptSalt\":\"$SALT\"}" -o client
chmod +x client || fail "客户端生成失败"

# 5. 运行客户端, 验证唯一化 + 命令
timeout 40 ./client "$PASS" > client.log 2>&1 &
sleep 12
grep -q "会话密钥唯一化完成" client.log || fail "会话密钥唯一化未生效"
log "✅ 会话密钥唯一化验证通过"

CLUID=$(sqlite3 "$WORK/database.db" "SELECT uid FROM clients LIMIT 1;" 2>/dev/null)
[ -n "$CLUID" ] || fail "客户端未上线"
curl -s -m 10 -X POST "http://127.0.0.1:$PORT/api/client/shell/sendcommand" \
  -H "Content-Type: application/json" -H "Authorization2: Bearer $TOKEN" \
  -d "{\"uid\":\"$CLUID\",\"command\":\"whoami\"}" > /dev/null
sleep 6
if sqlite3 "$WORK/database.db" "SELECT shell_content FROM shell WHERE uid='$CLUID';" 2>/dev/null | grep -qE "whoami"; then
  log "✅ 命令执行链路验证通过"
else
  fail "命令执行链路失败"
fi

log "✅✅ e2e 回归全部通过"

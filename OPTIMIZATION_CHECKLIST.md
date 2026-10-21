# 优化检查清单 (OPTIMIZATION CHECKLIST)

> 项目经历过大面积回滚后，此清单用于快速验证关键优化是否在位。
> 用法：逐条执行右侧 grep 命令，期望值 ≥1（存在）；0 = 被回滚，需恢复。

## 服务端 (Tshell-server)

| # | 优化 | 验证命令（pkg/ 下） | 期望 |
|---|------|---------------------|------|
| 1 | 缓冲池 cap 判定 | `grep -c "if buf := pool.Get().([]byte); cap(buf)" pkg/connection/memory_optimizer.go` | ≥1 |
| 2 | shell 增量追加 | `grep -c "func AppendShellContent" pkg/database/sqlite3.go` | ≥1 |
| 3 | dashboard 指标缓存 | `grep -c "getSystemInfoCached" pkg/api/dashboard.go` | ≥1 |
| 4 | 客户端列表 SQL 分页 | `grep -c "Limit(size" pkg/api/clients.go` | ≥1 |
| 5 | LastActive 异步落库 | `grep -c "QueueLastActiveUpdate" pkg/api/clients.go` | ≥1 |
| 6 | 下载进度节流 | `grep -c "QueueDownloadProgress" pkg/connection/tcp/tcp.go` | ≥1 |
| 7 | 队列 uid 清理 | `grep -c "CleanupClientQueues" pkg/command/cleanup.go` | ≥1 |
| 8 | mux 锁序快照 | `grep -c "streams := make" pkg/proxy/mux/mux.go` | ≥1 |
| 9 | UDP 隧道信号量 | `grep -c "udpDatagramSem" pkg/proxy/udp_tunnel.go` | ≥1 |
| 10 | JWT 密钥轮换 | `grep -c "func rotateKeys" pkg/security/jwt_manager.go` | ≥1 |
| 11 | RBAC 空角色拒绝 | `grep -c "RoleOperator" pkg/security/rbac.go pkg/security/jwt_manager.go` | ≥2 |
| 12 | qqwry 内存加载 | `grep -c "NewWithBuffer" pkg/qqwry/qqwry.go` | ≥1 |
| 13 | 审计跳过静态资源 | `grep -c "isSkippablePath" pkg/audit/middleware.go` | ≥1 |
| 14 | 响应格式统一 | `grep -c "func ResponseWrapper" pkg/errors/response_wrapper.go` | ≥1 |
| 15 | 监听器携带密钥 | `grep -c "vkey string" pkg/api/listener.go` | ≥1 |
| 16 | 密钥规范化 | `grep -c "NormalizeListenerKeyConfig" pkg/encrypt/encrypt.go` | ≥1 |
| 17 | 心跳 ack | `grep -c "BuildHeartbeatAckBytes" pkg/connection/tcp/tcp.go` | ≥1 |
| 18 | metaLen 越界校验 | `grep -c "metaLen 越界" pkg/connection/tcp/tcp.go` | ≥1 |
| 19 | 批量分组接口 | `grep -c "BatchUpdateClientGroup" pkg/api/tasks.go` | ≥1 |
| 20 | 断开连接接口 | `grep -c "func DisconnectClient" pkg/api/clients.go` | ≥1 |
| 21 | 批量删除事务 | `grep -c "session.Begin" pkg/api/clients.go` | ≥1 |
| 22 | 连接池覆盖 kcp/ws/dns | `grep -c "AddConnection" pkg/connection/kcp/kcp.go pkg/connection/websocket/websocket.go pkg/connection/dns/dns.go` | ≥3 |

## 客户端 (Tshell-client/implant)

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 23 | 单二进制 cmd/single | `ls cmd/single/transport_*.go \| wc -l` | 12 |
| 24 | Recv 读超时 | `grep -c "func RecvDeadline" core/app/run.go` | ≥1 |
| 25 | 心跳 ack 丢弃 | `grep -c "func IsHeartbeatAck" core/app/run.go` | ≥1 |
| 26 | dispatch 兜底过滤 | `grep -c "cmdType == 3" core/app/dispatch.go` | ≥1 |
| 27 | transport 原子化 | `grep -c "atomic.Pointer" core/communication/transport.go` | ≥1 |
| 28 | 发送串行化 | `grep -c "sendMutex" cmd/single/transport_tcp.go` | ≥1 |
| 29 | criticalSection 无全局锁 | `grep -c "var mutex sync.Mutex" core/communication/dispatch.go` | 0 |
| 30 | aeadCache 上限 | `grep -c "aeadCacheMaxEntries" core/encrypt/encrypt.go` | ≥1 |
| 31 | CmdSleep 'd' 修复 | `grep -c "if sleep != 'd'" core/services/services.go` | 0 |
| 32 | CmdPs 容错 | `grep -c 'archString := "?"' core/services/services.go` | ≥1 |
| 33 | 重连 quality 锁 | `grep -c "rm.quality.mu.RLock" core/reconnect/manager.go` | ≥1 |
| 34 | 旧 adapter 已删 | `ls cmd/` | 仅 single |

## 快速全量脚本

```bash
# 服务端
cd Tshell-server && go build ./... && go test ./pkg/... && go vet ./pkg/...
# 客户端（single 12 协议 EXE + DLL）
cd Tshell-client/implant && ./build_templates.sh
# 前端
cd Tshell-web && npm run build:embed
```

## B 组（代码质量，2026-08 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 35 | 共享回包处理(三协议去重) | `grep -c "func HandleClientReply" pkg/command/reply_handlers.go` | ≥1 |
| 36 | 三协议无内层 switch | `grep -c "switch replyType" pkg/connection/tcp/tcp.go pkg/connection/kcp/kcp.go pkg/connection/websocket/websocket.go` | 0 |
| 37 | shell 增量追加(共享 handler) | `grep -c "AppendShellContent(uid" pkg/command/reply_handlers.go` | ≥1 |
| 38 | 客户端解密回落共享 | `grep -c "func DecryptCommand" core/communication/transport_utils.go`(客户端) | ≥1 |
| 39 | 客户端心跳/分帧共享 | `grep -c "func FrameResult\|func HeartbeatMsg" core/communication/transport_utils.go`(客户端) | ≥2 |
| 40 | 前端单一事件 WS | `grep -c "events/ws" src/stores/events.ts`（原检查的 EventCenter.vue/DashboardView.vue 已随 vshell 重构移除） | ≤1 |
| 41 | 图标按需 | `grep -c "ElementPlusIconsResolver" vite.config.ts`(前端) | ≥1 |

## C 组（加密体系改造，2026-08 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 42 | MetaInfo 主密钥 GCM(客户端) | `grep -c "func EncryptWithMasterKey" Tshell-client/implant/core/encrypt/encrypt.go` | ≥1 |
| 43 | MetaInfo 主密钥 GCM(服务端) | `grep -c "func DecryptWithMasterKeyRaw" Tshell-server/pkg/encrypt/encrypt.go` | ≥1 |
| 44 | 旧 CFB 加密已移除 | `grep -rc "func Encrypt(data" Tshell-server/pkg/encrypt/encrypt.go Tshell-client/implant/core/encrypt/encrypt.go` | 0 |
| 45 | 客户端无解密回落 | `grep -rc "encrypt.Decrypt(rawData)" Tshell-client/implant/cmd/single/*.go` | 0 |
| 46 | 服务端无解密回落 | `grep -rc "SERVER-FALLBACK" Tshell-server/pkg/encrypt/encrypt.go` | 0 |
| 47 | host 明文 patch | `grep -c "host 明文" Tshell-server/pkg/api/server.go Tshell-server/pkg/delivery/payload.go` | ≥2 |

## D 组（会话密钥唯一化，2026-08 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 48 | 服务端 nonce 重派生 | `grep -c "RotateSessionKeyWithNonce" Tshell-server/pkg/encrypt/encrypt.go` | ≥1 |
| 49 | 服务端 nonce 下发 | `grep -rc "BuildSessionNonceMsg" Tshell-server/pkg/command/heartbeat_ack.go Tshell-server/pkg/connection/tcp/tcp.go Tshell-server/pkg/connection/kcp/kcp.go Tshell-server/pkg/connection/websocket/websocket.go` | ≥4 |
| 50 | 客户端 nonce 重派生 | `grep -c "func (ckm \*ClientKeyManager) SetSessionKeyWithNonce" Tshell-client/implant/core/encrypt/encrypt.go` | ≥1 |
| 51 | 客户端协商消费 | `grep -c "func ConsumeSessionNonce" Tshell-client/implant/core/communication/transport_utils.go` | ≥1 |

## 测试覆盖（2026-08 追加）

| # | 测试 | 命令 |
|---|------|------|
| 52 | nonce 派生一致性（服务端公式） | `go test ./pkg/encrypt/ -run TestNonceDerivationConsistency`（Tshell-server） |
| 53 | nonce 派生一致性（客户端公式） | `go test ./core/encrypt/ -run TestClientSetSessionKeyWithNonce`（Tshell-client/implant） |
| 54 | 共享回包处理（shell 追加/下载/未知类型） | `go test ./pkg/command/ -run TestHandleClientReplyShellAppend`（Tshell-server） |
| 55 | 密钥查找一致性 | `go test ./pkg/encrypt/ -run TestListenerKeyLookupWithRealDB` |
| 56 | 响应包装 | `go test ./pkg/errors/ -run TestResponseWrapper` |

## E 组（轮询协议会话密钥唯一化 + 构建矩阵修复，2026-08 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 57 | 轮询协议 nonce 暂存机制 | `grep -c "func StorePendingNonce" Tshell-server/pkg/encrypt/pending_nonce.go` | ≥1 |
| 58 | HTTP 唯一化（仅首次派生+rotate，轮询不覆盖） | `grep -c "RotateSessionKeyWithNonce" Tshell-server/pkg/api/communication/get.go` | ≥1 |
| 59 | OSS firstBlood 后直发 nonce 对象 | `grep -c "BuildSessionNonceMsgWithKey" Tshell-server/pkg/connection/oss/oss.go` | ≥1 |
| 60 | DNS 唯一化 + pull 优先下发 nonce | `grep -c "TakePendingNonce" Tshell-server/pkg/connection/dns/dns.go` | ≥1 |
| 61 | 客户端 12 transport 全部消费 nonce | `grep -rl "ConsumeSessionNonce" Tshell-client/implant/cmd/single/ | wc -l` | 12 |
| 62 | darwin 桩补齐（9+6 函数） | `GOOS=darwin GOARCH=amd64 go build -tags transport_tcp ./cmd/single`（Tshell-client/implant） | 成功 |
| 63 | 构建矩阵剔除不支持的组合 | `bash build_templates.sh` 结尾 FAIL 数 | 0 |
| 64 | pending 生命周期单测 | `go test ./pkg/encrypt/ -run TestPendingNonceLifecycle` | PASS |
| 65 | nonce 消息帧构造单测 | `go test ./pkg/command/ -run TestBuildSessionNonceMsgWithKey` | PASS |

### 修复的关键 bug
- 轮询协议每次 GET 都执行 `DeriveSessionKeyForListener` → 覆盖唯一化后的密钥 → 客户端唯一化后下一次命令解密失败（GCM tag 校验失败）→ 重连死循环。修复：派生仅首次注册执行。
- HTTP 首次接入时 rotate+store 位置在 `!exsists` 判断之外 → 每次轮询重复旋转下发新 nonce。修复：移到首次注册分支 + `HasPendingNonce` 防重。
- 客户端 5 个 transport（http/https/dns/oss/ws_forward）此前 DecryptCommand 替换未生效（正则未匹配）→ 补接 ConsumeSessionNonce。
- https transport 缺 communication import。

## F 组（前端依赖清理 + API 测试覆盖，2026-08 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 66 | 移除未使用依赖（echarts/vxe-table/vxe-table-plugin-element/vue-schart/js-base64/dayjs） | `npm ls echarts vxe-table vue-schart js-base64`（Tshell-web） | 均 removed |
| 67 | 清理 vite.config.ts 失效 vxe manualChunks | `grep -c "vxe" Tshell-web/vite.config.ts` | 0 |
| 68 | 前端构建 + 服务端集成 | `npm run build:embed`（Tshell-web） | SUCCESS，dist 2.07MB |
| 69 | 客户端分页 API 单测 | `go test ./pkg/api/ -run TestGetClients`（Tshell-server） | PASS（2 项） |
| 70 | 密钥配置接口单测（不泄露明文） | `go test ./pkg/api/ -run TestGetListenerKeyConfig`（Tshell-server） | PASS（2 项） |

### 说明
- 前端产物大小维持 2.07MB（被清理依赖本就未被引用）；收益在 npm 安装体积、供应链面与构建稳定性
- vxe manualChunks 配置在依赖卸载后会直接导致 rollup 构建失败，已同步清理
- `npm run type-check` 因环境 TS/vue-tsc 版本不兼容报 ScriptKind 错误（与改动无关），以 build:embed 通过为准

## G 组（全协议 e2e 验证 + DNS 格式修复，2026-08 追加）

| # | 验证 | 结果 |
|---|------|------|
| 71 | TCP e2e（唯一化 + whoami） | PASS |
| 72 | HTTP e2e（唯一化 + hostname） | PASS |
| 73 | WebSocket e2e（唯一化 + whoami，含 URL 补全修复 ws:// + /ws） | PASS |
| 74 | KCP e2e（唯一化 + id） | PASS |
| 75 | DNS e2e（唯一化 + hostname） | PASS |

### 本轮修复
- **WS 客户端 URL 补全**：config.Host 无 scheme（"host:port"），Connect 时补 ws:// 与 /ws 路径（wsc.Dial 需要完整 URL）
- **DNS 服务端 nonce 编码与命令不一致**：nonce 下发为 base32(base64(密文))，命令为 base32(密文)——客户端统一 b32Decode→DecryptCommand 会解密失败。修复：nonce 先 DecodeBase64 再 base32，与命令格式统一
- **DNS registerClient 幂等**：派生+rotate 同入 HasPendingNonce 保护，防 mi 分块重复调用覆盖密钥
- **HTTP 派生仅首次注册**（D 组延续修复：轮询每次 GET 派生会覆盖唯一化密钥）

### 环境注意（e2e 过程记录）
- 服务端重启**不自动恢复监听器**（需 /api/listener/open，status=1 的记录才会在启动时生效）
- 内存不足（9G 总量）时大二进制（1.37GB embed）go build 卡死：用 `GOMEMLIMIT=2-3GiB go build -gcflags="all=-l"` 可完成；或临时精简模板目录
- bash 工具超时会清理进程组：用 `setsid nohup ... &` 使服务端脱离会话存活
- 轮询协议客户端解码链：base32hex → base64 → 密文 → GCM

## H/I/J 组（安全加固 + 健壮性修复，2026-08 追加）

### H 批:安全严重缺陷(5 项,均已验证)

| # | 修复 | 验证 |
|---|------|------|
| 76 | H1: /api/shell/:uid 强制首消息 JWT+角色(admin/operator),CheckOrigin 同源 | curl 未认证 start → "未认证" |
| 77 | H2: refresh 从 DB 回填 Role claim;空 role 拒绝而非当 admin | auditor refresh → role=auditor(原为空→admin 提权) |
| 78 | H3: 插件 Upload/Download/Delete/Details 文件名白名单 | ../ 上传 → "文件名非法" 400 |
| 79 | H4: Downloads 三接口 Uid 严格 ^[a-f0-9]{32}$ | uid=../../ → "客户端UID格式不正确" |
| 80 | H5: 下线 /api/terminal/connect 本机 shell(410)+ 删除本机终端逻辑 | 路由返回 410 |

### I 批:健壮性高风险(8 项)

| # | 修复 |
|---|------|
| 81 | I1: CleanupClientQueues 接入 SignalClientOffline(原死代码→信号 map 泄漏+handler 空等超时) |
| 82 | I2: monitor/hvnc 队列 Add 锁内非阻塞 send(消除 send-on-closed panic 掐断 C2 连接) |
| 83 | I3: WS/KCP 写失败完整断连(close+注销离线检测+清队列;消除幽灵连接) |
| 84 | I4: KCP 首条消息 30s 读超时(消除 firstBlood 失败 UDP 会话/goroutine 泄漏) |
| 85 | I5: XTerm 任何读错误统一 cleanup(消除 session/录像 FD 泄漏) |
| 86 | I6: OSS StopChan 有缓冲(1)(消除停止信号丢失→轮询 goroutine 泄漏) |
| 87 | I7: WS case 2 补 len(dataBytes)<4 校验(消除短包越界 panic 断连) |
| 88 | I8: WS firstBlood 后 AddConnection/断连 RemoveConnection(入池:受 MaxConnections 限制) |

### J 批:中低风险(9 项)

| # | 修复 |
|---|------|
| 89 | J1: UidFileBrowser 统一经 fileBrowserMu 锁(clients.go delete + ParseDrives append) |
| 90 | J2: 连接池 config 读写锁 + ConnectionInfo 锁内读(IsActive/UID/Protocol/LastActivity)+ GetMemoryStats 字段副本(vet copies lock) |
| 91 | J3: worker_pool Stop 30s 超时兜底(无超时任务不再永久挂起) |
| 92 | J4: 默认密钥强制——handleOpenPort 拒绝默认/空密钥;firstBlood(6 协议)拒绝默认密钥;GenServer 拒绝默认/空密钥 |
| 93 | J5: 隐藏访问默认关闭+密钥清空;弱密钥(<8 字符)自动禁用;每 IP 5 分钟 10 次密钥尝试限流 |
| 94 | J6: XTerm 录像 uid 严格校验 + filepath.Base 二次防御 |
| 95 | J7: 改密强制 ValidatePassword 策略(≥8 位含大小写数字) |
| 96 | J8: RBAC 能力矩阵——auditor 只读,命令/exit/sleep operator+,批删/批量命令/forward/ExecuteBin adminOnly;forward HTTPS 启用证书校验 |
| 97 | J9: 全局请求体 10MB 限制中间件;DeliveryAccessLog 30 天轮转;交付文件名白名单(ServeMux 模式注入防护) |

### 验证结果
- `go build ./...` PASS;`go vet ./pkg/...` 干净(godonut 为既有 unsafe 警告,非本次引入);7 测试包全 PASS
- 攻击模拟:未认证 shell 拒绝、refresh 无提权、插件/Downloads 路径穿越拒绝、GenServer 默认密钥拒绝
- e2e 回归:TCP 唯一化(KeyID→-n2)+ whoami→kali 正常

## K/F 组（客户端 + 前端健壮性与安全加固，2026-08 追加）

### K 组:客户端(implant)10 项

| # | 修复 | 验证 |
|---|------|------|
| 98 | K1: 下载 0 字节 bug——移除错误的本地"续传"(两次 Stat 同一源文件 → Seek(EOF) 发送空数据) | 代码修复,编译通过 |
| 99 | K2: HandleCommand 入口 recover + 9 处命令边界校验(CmdPause 上限 10 分钟/CmdKill/CmdModifyTime/CmdHide/CmdUnhide/CmdFileSave/ExecuteLinuxScript/Bin/Bof/File) | 编译通过 |
| 100 | K3: Recv 消息大小上限 512MB(TCP/KCP/tcp_forward;原 4GB make() 可 OOM) | 编译通过 |
| 101 | K4: 移除 WMI_PERSIST 分支误执行 UNHOOK_NTDLL(复制粘贴遗漏) | 编译通过 |
| 102 | K5: 密钥占位符 fail-closed(未 patch 拒绝启动)+ MetaInfo 生成错误上抛(6 transport) | 编译通过 |
| 103 | K6: keylog 设备文件集合跟踪 + Stop 时关闭文件释放阻塞 Read(消除 fd/goroutine 泄漏) | 编译通过 |
| 104 | K7: socks5 Session 互斥锁 + 重复启动幂等(消除数据竞争与 agent 泄漏) | 编译通过 |
| 105 | K8: 加密失败禁止发送空载荷(原 base64 空串照发 → 服务端静默丢包) | 编译通过 |
| 106 | K9: 截图/监控捕获统一 15s 超时(exec.CommandContext;X 挂起不再阻塞命令循环) | 编译通过 |
| 107 | K10: 持久传输 fail/rawSend 竞态修复(close/nil 纳入发送锁 + locked 变体防死锁) | 编译通过 |

### F 组:前端(web)10 项

| # | 修复 | 验证 |
|---|------|------|
| 108 | F1: InteractiveTerminal XSS——addTerminalLine 统一转义 + 已处理路径标记(v-html 渲染面收敛) | vue-tsc 0 错误 |
| 109 | F2: RBAC——路由守卫消费 meta.permiss(无权→/401);TopHeader 菜单按权限过滤;permiss 增加 auditor 只读键;登录角色映射修正 | vue-tsc 0 错误 |
| 110 | F3: refresh_token 不再持久化 localStorage(XSS 无法窃取长期凭证) | 构建通过 |
| 111 | F4: plugin API `\`n 污染修复——delete_plugin 恢复(原插件删除运行时 TypeError) | vue-tsc 0 错误 |
| 112 | F5: 401 刷新重试仅一次(_retry 标志,消除刷新→401 循环风暴) | 构建通过 |
| 113 | F6: API 404 只提示不跳页;新增 410「功能已下线」分支 | 构建通过 |
| 114 | F7: ClientMonitor/ClientScreenshot 定时器与 CSP 监听器引用保存+卸载清理 | vue-tsc 0 错误 |
| 115 | F8: InteractiveTerminal 卸载标志禁止 onclose 重连;events.ts ping 回 pong 保活 | vue-tsc 0 错误 |
| 116 | F9: tokenValidator 区分网络错误与认证失败——服务抖动不再全员登出 | vue-tsc 0 错误 |
| 117 | F10: nginx CSP 收紧(移除 unsafe-eval、connect-src 仅同源)+ 修正 X-XSS-Protection 值 | 配置验证 |

### 验证结果
- 客户端 `go build ./...` + `go vet` 全绿;模板全量重建 EXE 166 + DLL 24 FAIL=0
- 前端 `vue-tsc` 0 错误;`npm run build:embed` 成功(dist 2.07MB)
- 服务端全量重建成功(嵌入新前端 + 新模板)

## M 组(工程化:测试/CI/依赖健康/文档/e2e 脚本,2026-08 追加)

| # | 项 | 结果 |
|---|-----|------|
| 118 | 前端依赖升级:vite 4→5.4、vitest 0.34→1.6、TS 5.1→5.5、vue-tsc 3→2.2;移除 vite-plugin-vue-setup-extend(peer 锁 vite4) | type-check 0 错误 |
| 119 | 修复被转义损坏的 PerformanceMonitor.vue(116 处 `\"` 错误转义 → 418 类型错误归零) | vue-tsc 通过 |
| 120 | （历史项，已完成）修复 16 个被 vue-tsc 崩溃掩盖的真实类型错误(EventCenter/TopHeader/PluginsView/DashboardView/ClientScreenshot/formatUtils);删除调试页 ApiTest.vue | vue-tsc 0 错误 |
| 121 | vitest 配置重写(独立配置,vitest1 不支持 mergeConfig callback)+ 3 个测试文件 9 用例(终端转义 XSS/状态格式化) | 9 passed |
| 122 | 服务端事件总线单测(订阅/退订/无订阅者) | PASS |
| 123 | e2e 一键回归脚本 scripts/e2e.sh(构建→监听器→客户端→唯一化+命令) | 一次通过 |
| 124 | CI:GitHub Actions 三 job(服务端 build/vet/test;植入体 12 transport;前端 type-check/单测/构建) | 已创建 |
| 125 | 文档:ARCHITECTURE.md(加密体系/协议/机制/测试)+ DEPLOYMENT.md(构建/配置/拓扑/运维/升级) | 已创建 |

### 环境注意
- npm 严格 peer 解析对 unocss `^5.0.0-0` 与 vite5 正式版误报冲突 → `npm install --legacy-peer-deps`(实际兼容)
- vue-tsc 3.x + TS5.1 组合崩溃(ScriptKind),已换 vue-tsc 2.2 + TS5.5
- go mod tidy 在离线环境可能因代理下载失败,不影响构建

| 126 | 修复客户端"管理"跳 /401:子路由 permiss 为数字(2)而 permiss store 为字符串('2'),includes 恒 false → 19 处统一转字符串 | 浏览器实测点击"管理"正常进入 clientManager 终端页 |

## N 组（凭据模块第二批：DPAPI + 浏览器，2026-10 追加）

> 用法同前：改动涉及凭据链路时逐条 grep，期望值 ≥1（0 = 被回滚）。

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 127 | 凭据类型三类化（服务端） | `grep -c "KindDPAPI\|KindBrowser" Tshell-server/pkg/cred/dump.go` | ≥2 |
| 128 | 类型×技术分域校验 | `grep -c "techsByKind" Tshell-server/pkg/cred/dump.go` | ≥1 |
| 129 | 制品形态按类型定（minidump/zip/json） | `grep -c "FormatByKind" Tshell-server/pkg/cred/*.go` | ≥2 |
| 130 | ZIP 制品逐条目校验（CRC + manifest） | `grep -c "func ValidateZip" Tshell-server/pkg/cred/zipcheck.go` | ≥1 |
| 131 | 浏览器制品解析与校验 | `grep -c "func ParseBrowserArtifact" Tshell-server/pkg/cred/browser.go` | ≥1 |
| 132 | 明文口令/Cookie 入库到凭据库 | `grep -c "func ingestBrowserArtifact" Tshell-server/pkg/api/cred_ingest.go` | ≥1 |
| 133 | 凭据库新增 cookie 类型 | `grep -c '"cookie": true' Tshell-server/pkg/api/loot.go` | ≥1 |
| 134 | 制品列表覆盖全部类型（不再写死 lsass） | `grep -c "In(\"source\", cred.Kinds)" Tshell-server/pkg/api/cred.go` | ≥1 |
| 135 | 客户端按 kind 分派 | `grep -c "func CmdCredDump" Tshell-client/implant/core/services/cred.go` | ≥1 |
| 136 | 浏览器密文解密（v10/v20） | `grep -c "prefixV20" Tshell-client/implant/core/services/browser.go` | ≥2 |
| 137 | DPAPI 解封装 | `grep -c "func cryptUnprotect" Tshell-client/implant/core/services/browser_crypto_windows.go` | ≥1 |
| 138 | DPAPI 主密钥采集（ZIP 制品） | `grep -c "func collectDPAPIMasterKeys" Tshell-client/implant/core/services/dpapi_windows.go` | ≥1 |
| 139 | 自写 SQLite 读取器（含 WAL） | `grep -c "func (db \*DB) Table\|func openWAL" Tshell-client/implant/core/sqlite/reader_impl.go Tshell-client/implant/core/sqlite/wal.go` | ≥2 |
| 140 | 明文不落盘（三类制品都在回传后删除） | `grep -c "exfilFile" Tshell-client/implant/core/services/cred.go` | ≥3 |
| 141 | 前端类型/技术联动 | `grep -c "CRED_TECHS" Tshell-web/src/api/cred/index.ts Tshell-web/src/layout/Dialogs/CredentialDumpDialog.vue` | ≥2 |

### 验证结果（2026-10-18，Windows 开发机）
- 服务端 `go build ./... && go vet ./...` 全绿；`go test ./pkg/cred/... ./pkg/api/...` 全绿
  （本机 TDM-GCC 下 `-race` 链接失败：缺 `WaitOnAddress`/`WakeByAddressSingle` 符号，
  属工具链限制，race 门禁由 CI 的 Linux job 保证）
- 植入体 `go test ./...` 全绿；13 种传输标签交叉编译全绿（windows/amd64 + linux/darwin 桩）
- 前端 `npm run type-check` 0 错误、`npx vitest run` 47 passed、`npm run build` 成功
- 中继 `go test ./...` 全绿
- **本机（Windows）测试环境的既有失败**（与本次改动无关，Windows 语义差异）：
  `TestInsecurePermPaths` / `backup.TestCreateProducesVerifiedSnapshot`（0600 权限断言）、
  `cna.TestBuiltinOpen*`（symlink 需开发者模式）、`proxy.TestStartTunnelFailsWhenAgentPortBusy`
  （端口占用判定）、`logx.TestSetOutputFile`（Windows 下文件被占用导致 TempDir 清理失败）。
  这些用例在 Linux/CI 上通过。
- 顺带修掉一个真实竞态：`events_collab_test.go` 在服务端"回 ok 之后再登记在线"的窗口里直接断言，
  本机会稳定失败 —— 改成等待状态收敛（3s 上限）。

### 环境注意（本轮踩坑）
- `go mod tidy` 会把植入体的 `go` 指令顶到 1.26、并把 `golang.org/x/*` 升到要求 Go 1.26 的版本，
  CI（Go 1.25）会直接构建失败。本机没有 go1.26 工具链时用
  `GOTOOLCHAIN=local` + 钉住 `x/sys v0.39.0 / x/text v0.32.0 / x/crypto v0.46.0 /
  x/net v0.48.0 / x/sync v0.19.0 / x/time v0.14.0 / x/mod v0.31.0 / x/tools v0.40.0`。
- 前端 `npm run lint` 在本机不可用：`eslint` 不在 `package.json` 依赖里（只有 `eslint-config-*`），
  `npm ci` 不会装它 —— 属既有问题，需要单独补依赖或在 CI 里显式安装。

## O 组（凭据模块族第三批：提权，2026-10 追加）

> 用法同前：改动涉及提权或命令面时逐条 grep，期望值 ≥1（0 = 被回滚）。

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 142 | 提权命令类型（两侧一致） | `grep -c "GETSYSTEM = 132" Tshell-server/pkg/command/cmdType.go Tshell-client/implant/core/services/types.go` | ≥2 |
| 143 | 前缀表 + 无参精确表 | `grep -c "getsystem" Tshell-server/pkg/sendcommand/sendCommand.go` | ≥2 |
| 144 | dispatch 分支 + 节流豁免 | `grep -c "services.GETSYSTEM" Tshell-client/implant/core/app/dispatch.go` | ≥2 |
| 145 | 非 Windows 显式报错 | `grep -c "仅支持 Windows" Tshell-client/implant/core/commands/privesc_stub.go` | ≥1 |
| 146 | 只派生新进程（不动模拟状态） | `grep -c "CreateProcessAsUserW\|CreateProcessWithTokenW" Tshell-client/implant/core/commands/privesc_windows.go` | ≥2 |
| 147 | 新会话透传启动口令 | `grep -c "os.Args\[1\]" Tshell-client/implant/core/commands/privesc_windows.go` | ≥1 |
| 148 | 权限启用检查 ERROR_NOT_ALL_ASSIGNED | `grep -c "errorNotAllAssigned" Tshell-client/implant/core/commands/privesc_windows.go` | ≥2 |
| 149 | UAC 劫持键 + 立即还原 | `grep -c "uacHijackBase\|restore()" Tshell-client/implant/core/commands/privesc_uac_windows.go` | ≥2 |
| 150 | 删不掉键时退化为清空值 | `grep -c "clearHijackValues" Tshell-client/implant/core/commands/privesc_uac_windows.go` | ≥2 |
| 151 | 独立清理入口（随时可用） | `grep -c "uac_cleanup" Tshell-client/implant/core/commands/privesc_windows.go` | ≥1 |
| 152 | TTP 报告映射 | `grep -c '"getsystem"' Tshell-server/pkg/api/report.go` | ≥1 |
| 153 | 控制台帮助文本 | `grep -c "getsystem" Tshell-web/src/layout/BeaconConsole.vue` | ≥2 |

### 验证结果（2026-10-18，Windows 开发机）
- 服务端 `go vet ./...` + `go run staticcheck -checks=all,-ST*,-U1000 ./...` 全绿；
  `pkg/sendcommand`/`pkg/command`/`pkg/api` 测试全绿（新增 `TestGetSystemCommand`）
- 植入体 `go test ./...` 全绿（Windows 目标）+ 13 传输交叉编译全绿（linux/darwin 桩）
- 新增提权单测 9 项：参数校验 / 幂等 / 权限语义（含 `SeTcbPrivilege` 必须失败）/
  注册表还原与退化 / `RegDeleteKeyW` 不递归这条 quirk / 进程枚举过滤
- **本环境限制（不是缺陷）**：受限令牌下 `LookupPrivilegeValueW` 对所有权限都返回失败，
  因此 `token` 路径的端到端与权限启用断言在本机会跳过并打印原因；
  真实提权成功率需授权 VM 验证（清单见 `docs/PRIVILEGE_ESCALATION.md` §7）

### 本轮踩到并修掉的真问题
- `RegDeleteKeyW` **不递归**：带子键的键返回 `Access is denied`（我最初的注释写反了）。
  已改为自己深度优先清子键，并留 `TestRegDeleteKeyIsNotRecursive` 守住这条语义。
- 注册表还原失败时的**安全兜底**：删不掉键就清空劫持命令行值 —— 留下可用的劫持
  比"报告失败"危险得多（`TestUACCleanupDegradesSafely` 守这条）。

## P 组（阶段三 §3.5：MFA / HSTS / jwt.algorithm，2026-10 追加）

> 用法同前：改动涉及认证面时逐条 grep，期望值 ≥1（0 = 被回滚）。

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 154 | TOTP 核心（RFC 6238） | `grep -c "func VerifyWith\|func CodeWith" Tshell-server/pkg/security/otp/totp.go` | ≥2 |
| 155 | 恢复码独立实现 | `grep -c "func NewRecoveryCodes\|func UseRecoveryCode" Tshell-server/pkg/security/otp/recovery.go` | ≥2 |
| 156 | 二维码编码器（零依赖） | `grep -c "func EncodeQR\|func SVG" Tshell-server/pkg/security/otp/qrcode.go` | ≥2 |
| 157 | 密钥密文落库（非明文） | `grep -c "SealMFASecret\|ErrMFANotEnabled" Tshell-server/pkg/security/mfa.go` | ≥2 |
| 158 | 中间态令牌 **audience 隔离** | `grep -c "AudienceMFAPending" Tshell-server/pkg/security/jwt_manager.go` | ≥3 |
| 159 | 拒绝 pending 当访问令牌 | `grep -c "不能作为访问令牌" Tshell-server/pkg/security/jwt_manager.go` | ≥1 |
| 160 | 登录分流（先判 MFA 再签凭据） | `grep -c "users.MFAEnabled" Tshell-server/pkg/api/users.go` | ≥1 |
| 161 | MFA 端点 | `grep -c "func MFA.*Handler" Tshell-server/pkg/api/mfa.go` | ≥6 |
| 162 | QR 端点只收 otpauth | `grep -c 'HasPrefix(text, "otpauth://")' Tshell-server/pkg/api/mfa.go` | ≥1 |
| 163 | 动态码防重放 | `grep -c "mfaLastSteps" Tshell-server/pkg/api/mfa.go` | ≥3 |
| 164 | HSTS 仅 HTTPS 下发 | `grep -c "Strict-Transport-Security" Tshell-server/pkg/security/middleware.go` | ≥1 |
| 165 | jwt.algorithm 真约束 | `grep -c "func ValidateJWTAlgorithm" Tshell-server/pkg/config/manager.go` | ≥1 |
| 166 | 迁移 v3（users MFA 字段） | `grep -c "migrateUsersMFATable" Tshell-server/pkg/database/migrations.go` | ≥1 |
| 167 | MFA 字段不进 JSON | `grep -c 'json:"-"' Tshell-server/pkg/database/sqlite3.go` | ≥3 |
| 168 | 前端两步登录分支 | `grep -c "mfa_required" Tshell-web/src/views/login/LoginView.vue` | ≥1 |
| 169 | 账号安全页 | `grep -c "MFAView" Tshell-web/src/views/settings/SettingsView.vue` | ≥1 |

### 验证结果（2026-10-18，Windows 开发机）
- 服务端 `go vet ./...` 全绿；新增/相关包测试全绿：
  `pkg/security`（含 otp）、`pkg/api`（含 13 项 MFA 集成测试）、`pkg/config`、`pkg/database`
- 前端 `vue-tsc` 0 错误、vitest **53 passed（11 文件）**、`npm run build` 成功
- 全量 `go test ./...` 的失败项均为**既有 Windows 环境限制**（文件权限 666/600、
  符号链接需特权、端口占用），见 `docs/ROADMAP.md` §5 第 8 条 —— 非本轮引入

### 本轮踩到并修掉的真问题
- **二维码编码器四处"尺寸正常但读不出来"的错误**（自实现，无参考实现对拍）：
  定位图形画成 7×7 实心方块（距离 2 那一圈必须浅色）；格式信息 bit 14 写到 (8,15)
  （应在 (8,14)，那格属数据区）；zigzag 列游标写成 `right = 5` 导致**第 0 列永不被访问**、
  直接越界写穿；比特读取器以"码字比特+补位数"为上限多读 7 位。
  另有两处表转录错误：V7-M 块表写成 4×27（标准 4×31）；v7 起两个合法对齐图形被误跳过。
  对策是加**三道互相独立的守卫**（几何↔表、表自洽、遍历↔布局），
  并强制编码与解码**共用同一个 `reserveFunctionModules`**（两套规则必然漂移）。
- **恢复码被误判成动态码**：`isRecoveryCodeShape` 原本"含数字就当动态码"，
  而恢复码字母表本就含 2-9 —— 用户拿着正确恢复码却被报"动态码错误"。改为按长度判定。
- **防重放状态跨测试/跨重绑残留**：`mfaLastSteps` 是包级 map，关闭/重置 MFA 时未清理，
  重新绑定后新认证器可能一开始就被判"该动态码已被使用"。已加 `resetMFAReplayGuard`。
- **集成测试必须挂 `ErrorHandlerMiddleware`**：`AbortWithError` 只登记错误 + Abort，
  真正把 AppError 翻成 HTTP 状态码的是中间件。测试里不挂它，所有失败路径都会得到
  "200 + 空 body"，把断言全放过去 —— 这是一类会让测试**假装通过**的陷阱。

## Q 组（阶段三 §3.6：per-listener C2 画像，2026-10 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 170 | listener 表 profile 列 | `grep -c "Profile string" Tshell-server/pkg/database/sqlite3.go` | ≥1 |
| 171 | 迁移 v4 | `grep -c "migrateListenerProfileColumn" Tshell-server/pkg/database/migrations.go` | ≥1 |
| 172 | 路由按监听器画像注册 | `grep -c "func registerHTTPRoutes(mux \*http.ServeMux, vkey, salt, profileName string)" Tshell-server/pkg/api/listener.go` | ≥1 |
| 173 | WS 路径不再硬编码 | `grep -c "resolveWSRoute" Tshell-server/pkg/api/listener.go` | ≥2 |
| 174 | 画像回落常量 | `grep -c "DefaultProfileName\|DefaultGetURI" Tshell-server/pkg/c2profile/manager.go` | ≥2 |
| 175 | 生成载荷基准=监听器画像 | `grep -c "listenerProfileForPayload" Tshell-server/pkg/api/server.go` | ≥2 |
| 176 | 前端画像下拉 | `grep -c "c2profileApi.listProfiles" Tshell-web/src/layout/Dialogs/EditListenerDialog.vue` | ≥1 |
| 177 | profile 进入请求体 | `grep -c "profile: formData.profile" Tshell-web/src/views/listener/listener.vue` | ≥2 |

### 验证结果（2026-10-19，Windows 开发机）
- 服务端 `go vet ./...` + staticcheck 全绿；`pkg/api`（含 5 项 per-listener 画像测试）、
  `pkg/c2profile`、`pkg/database`、`pkg/security/*` 全绿
- 前端 `vue-tsc` 0 错误、vitest 53 passed、build 成功

### 本轮踩到并修掉的真问题（§3.6）
- **`profileListenerMismatch` 里按 Name 走捷径，把单测短路成空转**：
  `if p.Name == listenerProfile.Name { return "" }` 让测试里两个同名画像
  （都叫 "default"、但字段已被改）永远"一致"，7 个用例里有 5 个其实是空跑。
  已删掉捷径 —— **逐字段比较才是契约**，同名不同内容（文件被改过）恰恰是要抓的情况。
- **测试进程没有画像注册表**：`AddListener` 现在要求"绑定的画像必须存在"，
  而 api 包测试默认没加载过 profiles/ 目录，于是所有监听器用例都得到
  "C2 画像不存在: default"。加了 `ensureTestProfiles`（用 `t.TempDir()` 写一份
  default.yaml + `InitNoWatch`），测试不再依赖工作目录里有没有那个文件夹。

## O 组（Kerberos 票据 · 文件式，2026-10-19 追加）

> 用法同前：改动涉及凭据链路时逐条 grep，期望值 ≥1（0 = 被回滚）。

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 142 | 票据制品类型与形态登记 | `grep -c "KindTicket" Tshell-server/pkg/cred/dump.go` | ≥3 |
| 143 | 票据 ZIP 逐条目解析（真实夹具） | `grep -c "func ParseTicketArtifact" Tshell-server/pkg/cred/ticket.go` | ≥1 |
| 144 | 制品校验按 kind 分派（票据不走通用 ZIP 校验） | `grep -c "kind == KindTicket" Tshell-server/pkg/cred/artifact.go` | ≥1 |
| 145 | 票据入库到凭据库 | `grep -c "func ingestTicketArtifact" Tshell-server/pkg/api/cred_ingest.go` | ≥1 |
| 146 | loot 新增 ticket 类型 | `grep -c '"ticket": true' Tshell-server/pkg/api/loot.go` | ≥1 |
| 147 | 客户端 ccache 发现（KRB5CCNAME + krb5cc*） | `grep -c "func findCCacheFiles" Tshell-client/implant/core/services/ticket.go` | ≥1 |
| 148 | 票据制品打包（manifest + tickets/） | `grep -c "writeTicketBundle" Tshell-client/implant/core/services/ticket.go` | ≥2 |
| 149 | 客户端分派与类型×技术校验含 ticket | `grep -c "credDumpTickets\|ticket\"" Tshell-client/implant/core/services/cred.go` | ≥2 |
| 150 | 前端类型/技术清单含 ticket | `grep -c "ticket" Tshell-web/src/api/cred/index.ts` | ≥2 |
| 151 | Firefox 接线守卫（防再退化成死代码） | `grep -c "TestBrowserTechsIncludeFirefox" Tshell-server/pkg/cred/cred_test.go` | ≥1 |

## P 组（Chromium Web Data：信用卡 + 自动填充，2026-10-19 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 152 | cards 技术登记（服务端/客户端/前端同源） | `grep -c "cards" Tshell-server/pkg/cred/dump.go Tshell-web/src/api/cred/index.ts Tshell-client/implant/core/services/cred.go` | ≥3 |
| 153 | 客户端只读 Web Data 的采集模式 | `grep -c "collectModeWebData" Tshell-client/implant/core/services/browser_windows.go` | ≥2 |
| 154 | 卡号走真实解密路径（可跨平台单测） | `grep -c "func cardsFromRows" Tshell-client/implant/core/services/browser_rows.go` | ≥1 |
| 155 | 自动填充转换（明文，带上限） | `grep -c "func autofillFromRows" Tshell-client/implant/core/services/browser_rows.go` | ≥1 |
| 156 | 服务端卡/自动填充字段与上限 | `grep -c "MaxBrowserCards\|MaxBrowserAutofill" Tshell-server/pkg/cred/browser.go` | ≥2 |
| 157 | 信用卡入库（type=card） | `grep -c '"browser:card"' Tshell-server/pkg/api/cred_ingest.go` | ≥1 |
| 158 | loot 新增 card 类型 | `grep -c '"card":' Tshell-server/pkg/api/loot.go` | ≥1 |
| 159 | 入库测试（卡入库、自动填充不入库） | `grep -c "TestCardsArtifactIngest" Tshell-server/pkg/api/cred_cards_test.go` | ≥1 |

## Q 组（Windows LSA 票据，2026-10-19 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 160 | LSA 结构体与消息号（布局单测） | `grep -c "TestLSALayout\|TestLSAMessageTypes" Tshell-client/implant/core/services/lsa_types_test.go` | ≥2 |
| 161 | 消息号与 ntsecapi.h 对齐 | `grep -c "kerbQueryTicketCacheEx2Message  int32 = 20\|kerbRetrieveEncodedTicketMessage int32 = 8" Tshell-client/implant/core/services/lsa_types.go` | ≥1 |
| 162 | 当前登录会话 LUID（TOKEN_STATISTICS） | `grep -c "AuthenticationId" Tshell-client/implant/core/services/lsa_types.go Tshell-client/implant/core/services/lsa_windows.go` | ≥2 |
| 163 | 枚举 + 逐张导出 kirbi | `grep -c "func lsaListTickets\|func lsaExportTicket\|func collectLSATickets" Tshell-client/implant/core/services/lsa_windows.go` | ≥3 |
| 164 | LSA 制品打包（manifest.lsa_tickets + kirbi/） | `grep -c "writeLSATicketBundle" Tshell-client/implant/core/services/ticket.go` | ≥2 |
| 165 | 服务端接受 kirbi 条目与清单元数据 | `grep -c "lsa_tickets" Tshell-server/pkg/cred/ticket.go` | ≥2 |
| 166 | 入库区分 kerberos:lsa / kerberos:ccache | `grep -c "kerberos:lsa" Tshell-server/pkg/api/cred_ingest.go` | ≥1 |
| 167 | 非 Windows 显式报错（不静默降级） | `grep -c "LSA 票据采集仅支持 Windows" Tshell-client/implant/core/services/lsa_stub.go` | ≥1 |

## R 组（getsystem 接线修复，2026-10-19 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 168 | 服务端命令号与客户端一致 | `grep -c "GETSYSTEM = 132" Tshell-server/pkg/command/cmdType.go Tshell-client/implant/core/services/types.go` | ≥2 |
| 169 | 命令表登记（带参 & 无参） | `grep -c "getsystem" Tshell-server/pkg/sendcommand/sendCommand.go` | ≥2 |
| 170 | 回归守卫：不得落进 SHELL 兜底 | `grep -c "落进了 SHELL 兜底" Tshell-server/pkg/sendcommand/sendCommand_test.go` | ≥1 |

## S 组（Kerberos 票据动作：list/purge/ptt，2026-10-19 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 171 | 命令号与客户端一致 | `grep -c "KERB_ACTION = 133" Tshell-server/pkg/command/cmdType.go Tshell-client/implant/core/services/types.go` | ≥2 |
| 172 | 服务端命令表登记 | `grep -c "kerberos" Tshell-server/pkg/sendcommand/sendCommand.go` | ≥2 |
| 173 | 客户端分派 | `grep -c "services.KERB_ACTION" Tshell-client/implant/core/app/dispatch.go` | ≥1 |
| 174 | 动作解析（四种 base64 / 用法错误 / 上限） | `grep -c "func TestParseKerberosAction" Tshell-client/implant/core/services/kerberos_action_test.go` | ≥3 |
| 175 | purge 原语 | `grep -c "func lsaPurgeTicket" Tshell-client/implant/core/services/lsa_windows.go` | ≥1 |
| 176 | ptt 原语（结构体+blob 缓冲区） | `grep -c "KerbCredOffset" Tshell-client/implant/core/services/lsa_windows.go` | ≥1 |
| 177 | 非 Windows 显式报错 | `grep -c "仅支持 Windows" Tshell-client/implant/core/services/kerberos_action_stub.go` | ≥1 |
| 178 | 前端帮助 | `grep -c "kerberos ptt" Tshell-web/src/layout/BeaconConsole.vue` | ≥1 |

## T 组（代码签名，2026-10-19 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 179 | 签名配置段与结构体 | `grep -c "SigningConfig" Tshell-server/pkg/config/manager.go Tshell-server/pkg/signing/signing.go` | ≥2 |
| 180 | 启动期校验（pfx/命令） | `grep -c "func Validate" Tshell-server/pkg/signing/signing.go` | ≥1 |
| 181 | 参数模板占位符替换 | `grep -c "{{in}}" Tshell-server/pkg/signing/signing.go` | ≥1 |
| 182 | 交付路径启用签名 | `grep -c "Sign:" Tshell-server/pkg/api/server.go Tshell-server/pkg/api/webdelivery.go` | ≥2 |
| 183 | 签名在 patch/HashPad 之后 | `grep -c "代码签名必须在所有 patch/pad 之后" Tshell-server/pkg/delivery/payload.go` | ≥1 |
| 184 | 假签名器单测（成功/失败/无输出/校验） | `grep -c "func TestSign\|func TestValidate" Tshell-server/pkg/signing/signing_test.go` | ≥5 |
| 185 | 文档 | `grep -c "未验证" docs/CODE_SIGNING.md` | ≥1 |

## U 组（前端类型逃逸清零，2026-10-19 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 186 | 非测试源码中 `as any` 清零（不含注释） | `grep -rn "as any" Tshell-web/src --include=*.vue --include=*.ts | grep -v test | grep -v "//" | wc -l` | 0 |
| 187 | C2Event 根字段类型化 | `grep -c "operator?: string" Tshell-web/src/stores/events.ts` | ≥1 |
| 188 | 审计行类型 | `grep -c "interface AuditLogRow" Tshell-web/src/layout/EventLogPanel.vue` | ≥1 |
| 189 | ClientInfo 补 snake_case | `grep -c "external_ip" Tshell-web/src/api/plugin/index.ts` | ≥1 |

## V 组（注入链 syscall 化 + 镂空 W^X，2026-10-20 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 190 | 远端内存原语层（Nt 优先 / 宿主回退） | `grep -c "type remoteMemory interface" Tshell-client/implant/core/commands/remotemem_windows.go` | 1 |
| 191 | 注入链不再直接调 kernel32 分配/写入/改保护 | `grep -c "VirtualAllocEx.Call\|WriteProcessMemory.Call\|VirtualProtectEx.Call" Tshell-client/implant/core/commands/inject_windows.go` | 0 |
| 192 | 镂空镜像按 RW 预留（不是 RWX） | `grep -c "rm.Alloc(h, info.imageBase, size, pageReadWrite)" Tshell-client/implant/core/commands/hollow_windows.go` | 1 |
| 193 | 镂空写入后收紧可执行节 | `grep -c "func hardenImage" Tshell-client/implant/core/commands/hollow_windows.go` | 1 |
| 194 | trampoline 覆盖读/卸载/PEB 查询 | `grep -c "NtReadVirtualMemory\|NtUnmapViewOfSection\|NtQueryInformationProcess" Tshell-client/implant/core/evasion/syscall.go` | ≥3 |
| 195 | 单测 + 源码级棘轮 | `grep -c "^func Test" Tshell-client/implant/core/commands/remotemem_windows_test.go` | 15 |
| 196 | OPSEC 权威文档同步 | `grep -c "进程镂空 W^X" docs/OPSEC.md` | 1 |
| 197 | 生成载荷的 syscall 模式默认仍是 none | `grep -c "SYSCALL_MODE_PLACEHOLDER_12345\":[[:space:]]*\"none\"" Tshell-server/pkg/delivery/payload.go` | 1 |

## W 组（清理现场 cleanup + 自更新残留，2026-10-20 追加）

| # | 优化 | 验证命令 | 期望 |
|---|------|----------|------|
| 198 | 命令号两侧一致（CLEANUP=134） | `grep -c "CLEANUP = 134" Tshell-server/pkg/command/cmdType.go Tshell-client/implant/core/services/types.go` | ≥1 each |
| 199 | 服务端命令表登记（带参 `cleanup ` + 无参 `cleanup`） | `grep -c "cleanup" Tshell-server/pkg/sendcommand/sendCommand.go` | ≥2 |
| 200 | 客户端 dispatch 分支 | `grep -c "services.CLEANUP" Tshell-client/implant/core/app/dispatch.go` | ≥2 |
| 201 | 清理判据是"对象指向本程序"（三类匹配器都在） | `grep -c "regRunValueNames\|taskNamesForExe\|startupFileMatches" Tshell-client/implant/core/commands/cleanup.go` | ≥3 |
| 202 | Author 字段里的同路径不认（防误删别人的任务） | `grep -c "isAuthorKey" Tshell-client/implant/core/commands/cleanup.go` | ≥2 |
| 203 | 逐条回执 + 汇总计数 | `grep -c "已清理 %d 项" Tshell-client/implant/core/commands/cleanup.go` | 1 |
| 204 | 自更新残留由新映像启动后清理 | `grep -c "go commands.PurgeUpdateResidue()" Tshell-client/implant/core/app/init.go` | 1 |
| 205 | 三平台清理实现齐全 | `grep -c "cleanupFileItems" Tshell-client/implant/core/commands/cleanup_windows.go Tshell-client/implant/core/commands/cleanup_linux.go Tshell-client/implant/core/commands/cleanup_darwin.go` | ≥1 each |
| 206 | 判定逻辑单测 | `grep -c "^func Test" Tshell-client/implant/core/commands/cleanup_test.go` | 15 |
| 207 | 接线棘轮（dispatch + 启动期清理） | `grep -c "TestCleanup" Tshell-client/implant/core/app/dispatch_wiring_test.go` | ≥2 |
| 208 | TTP 报告映射 | `grep -c '"cleanup"' Tshell-server/pkg/api/report.go` | 1 |
| 209 | OPSEC 权威文档同步 | `grep -c "清理现场" docs/OPSEC.md` | ≥2 |

### X 组：P2 调用栈伪造 + 睡眠期模块摘链（2026-10-20）

| # | 检查项 | 命令 | 期望 |
|---|---|---|---|
| 210 | 伪栈参数上限是单一事实来源 | `grep -c "SpoofMaxArgs" Tshell-client/implant/core/evasion/spoof.go` | 2 |
| 211 | 伪栈只在 direct 模式装配（indirect 不套） | `grep -c "ModeDirect" Tshell-client/implant/core/evasion/syscall.go` | 2 |
| 212 | `Syscall()` 走伪栈前必须判 `SpoofReady()` | `grep -c "SpoofReady()" Tshell-client/implant/core/evasion/syscall.go` | 1 |
| 213 | 伪栈 syscall 实现存在且被调用 | `grep -c "syscallSpoofed" Tshell-client/implant/core/evasion/spoof_windows.go` | 2 |
| 214 | ntdll 解析改为扫描 syscall 指令（不得写死偏移） | `grep -c "0x0F && body\[i+1\] == 0x05" Tshell-client/implant/core/evasion/trampoline.go` | 1 |
| 215 | 摘链/挂回都只有一处实现入口 | `grep -c "detachLinks\|attachLinks" Tshell-client/implant/core/evasion/modhide_windows.go` | ≥4 |
| 216 | 睡眠路径真的调了摘链且醒后挂回 | `grep -c "evasion.HideForSleep" Tshell-client/implant/core/app/run.go` | 1 |
| 217 | 摘链默认关（跟随主机层对抗开关组） | `grep -c "HostEvasionEnabled" Tshell-client/implant/core/config/config.go` | 2 |
| 218 | 伪栈单测（平台无关 7 项） | `grep -c "^func Test" Tshell-client/implant/core/evasion/spoof_test.go` | 7 |
| 219 | 伪栈实机单测（含 6 参数往返） | `grep -c "^func Test" Tshell-client/implant/core/evasion/spoof_windows_test.go` | 4 |
| 220 | 摘链单测（合成链表 + 真实 PEB 往返） | `grep -c "^func Test" Tshell-client/implant/core/evasion/modhide_windows_test.go` | 7 |
| 221 | 实机 ntdll 解析回归（名单逐个断言） | `grep -c "^func Test" Tshell-client/implant/core/evasion/syscall_windows_test.go` | 3 |
| 222 | 睡眠期摘链接线棘轮 | `grep -c "^func Test" Tshell-client/implant/core/app/sleepmask_wiring_test.go` | 2 |
| 223 | Ekko 不可做的论证写进文档（不是"没做"） | `grep -c "Ekko" docs/OPSEC.md` | ≥4 |
| 224 | 24H2 偏移变更写进文档 | `grep -c "24H2" docs/OPSEC.md` | ≥4 |
| 225 | 内存侧文档同步伪栈边界 | `grep -c "伪栈" docs/MEMORY_OPSEC.md` | ≥3 |

## Y 组（P3 ETW/AMSI 覆盖面 + unhook + PPID/BlockDLLs，2026-10-20 追加）

| # | 断言 | 命令 | 期望 |
|---|------|------|------|
| 226 | ETW patch 目标从 1 个扩到 5 个导出 | `grep -c "etwPatchTargets" Tshell-client/implant/core/evasion/evasion.go` | 3 |
| 227 | AMSI patch 覆盖两个入口 | `grep -c "amsiPatchTargets" Tshell-client/implant/core/evasion/evasion.go` | 3 |
| 228 | patch 后必须读回校验（防写错地址） | `grep -c "func VerifyPatch" Tshell-client/implant/core/evasion/evasion.go` | 1 |
| 229 | unhook 按 PE 节表换算，不拿 RVA 当文件偏移 | `grep -c "fileOffsetForRVA" Tshell-client/implant/core/evasion/pe_image.go` | ≥2 |
| 230 | unhook 默认覆盖三模块 | `grep -c "DefaultUnhookModules" Tshell-client/implant/core/evasion/unhook_windows.go` | ≥2 |
| 231 | 回写用 RWX（用 RW 会在 VirtualProtect 返回时自杀） | `grep -c "PAGE_EXECUTE_READWRITE" Tshell-client/implant/core/evasion/unhook_windows.go` | 1 |
| 232 | PPID 属性传的是 HANDLE 且活到创建之后 | `grep -c "PROCESS_CREATE_PROCESS" Tshell-client/implant/core/commands/procattr_windows.go` | 1 |
| 233 | BlockDLLs 常量是高 32 位第 12 位 | `grep -c "const mitigationBlockNonMS = 0x0000100000000000" Tshell-client/implant/core/commands/procattr_windows.go` | 1 |
| 234 | 主 spawn 路径都走属性封装 | `grep -c "createProcessWithAttrs" Tshell-client/implant/core/commands/commands_windows.go` | 3 |
| 235 | 镂空回执带上属性诊断 | `grep -c "ca.Describe()" Tshell-client/implant/core/commands/hollow_windows.go` | 1 |
| 236 | PE 解析单测 | `grep -c "^func Test" Tshell-client/implant/core/evasion/pe_image_test.go` | 6 |
| 237 | ETW/AMSI 实机单测 | `grep -c "^func Test" Tshell-client/implant/core/evasion/evasion_windows_test.go` | 4 |
| 238 | unhook 实机单测（种假 hook 后抹掉） | `grep -c "^func Test" Tshell-client/implant/core/evasion/unhook_windows_test.go` | 4 |
| 239 | PPID/BlockDLLs 实机单测（读回内核字段与策略位） | `grep -c "^func Test" Tshell-client/implant/core/commands/procattr_windows_test.go` | 6 |
| 240 | spawn 路径接线棘轮存在 | `grep -c "TestSpawnPathsUseChildAttrs" Tshell-client/implant/core/commands/procattr_windows_test.go` | 2 |
| 241 | 两个"假生效"陷阱写进文档 | `grep -c "ERROR_BAD_LENGTH\|ERROR_INVALID_HANDLE" docs/OPSEC.md` | 1 |

## Z 组（对抗能力逐项开关 + 两个前端面板，2026-10-20 追加）

| 242 | 运行期开关命令号两侧一致 | `grep -c "EVASION = 135" Tshell-client/implant/core/services/types.go Tshell-server/pkg/command/cmdType.go` | 2 |
| 243 | 运行期开关在 dispatch 里真接上 | `grep -c "case services.EVASION:" Tshell-client/implant/core/app/dispatch.go` | 1 |
| 244 | 门控统一入口是 EvasionEnabled（先看运行期覆盖） | `grep -c "func EvasionEnabled" Tshell-client/implant/core/config/opsec.go` | 1 |
| 245 | 每项能力都有经 EvasionEnabled 的 wrapper | `grep -c "EvasionEnabled(Evasion" Tshell-client/implant/core/config/config.go` | 7 |
| 246 | 摘链走细门控（不被 ETW patch 顺带打开） | `grep -c "config.SleepMaskHideEnabled()" Tshell-client/implant/core/app/run.go` | 1 |
| 247 | PPID 与 BlockDLLs 各自独立门控 | `grep -c "PpidSpoofEnabled\|BlockDLLsEnabled" Tshell-client/implant/core/commands/procattr_windows.go` | 2 |
| 248 | 伪栈有独立的"要不要用"判定 | `grep -c "func SpoofActive" Tshell-client/implant/core/evasion/spoof_windows.go` | 1 |
| 249 | unhook 在 ETW/AMSI patch 之前跑 | `grep -c "UnhookOnStartEnabled" Tshell-client/implant/core/evasion/evasion.go` | 1 |
| 250 | 新客户端会检查 5 个新占位符残留（fail-closed） | `grep -c "SleepMaskHidePlaceholder" Tshell-client/implant/core/config/config.go` | 1 |
| 251 | 服务端占位符表含全部 9 项 OPSEC | `grep -c "PLACEHOLDER_12345" Tshell-server/pkg/delivery/payload.go` | 42 |
| 252 | 服务端统一归一化/校验入口 | `grep -c "func resolveOpsec" Tshell-server/pkg/api/opsec_params.go` | 1 |
| 253 | 团队级默认走配置并进白名单 | `grep -c "type OpsecConfig struct" Tshell-server/pkg/config/manager.go` | 1 |
| 254 | 配置更新按 opsec 段路由 | `grep -c '"opsec"' Tshell-server/pkg/config/handlers.go` | 1 |
| 255 | 命令表认得 evasion | `grep -c "evasion" Tshell-server/pkg/sendcommand/sendCommand.go` | 4 |
| 256 | 命令有 MITRE 归属 | `grep -c "T1562" Tshell-server/pkg/api/report.go` | 3 |
| 257 | 生成面板不写"勾了不生效"的组合（非 direct 强制关伪栈） | `grep -c "stackSpoof = false" Tshell-web/src/components/Opsec/opsecPresets.ts` | 2 |
| 258 | 生成面板有依赖置灰原因 | `grep -c "capabilityDisabledReason" Tshell-web/src/components/Opsec/opsecPresets.ts` | 1 |
| 259 | 生成期面板接进单包载荷对话框 | `grep -c "OpsecToggles" Tshell-web/src/layout/Dialogs/Payloads/WindowsExeStagelessDialog.vue` | 2 |
| 260 | 生成期面板接进服务端生成页 | `grep -c "OpsecToggles" Tshell-web/src/views/server/ServerGen.vue` | 2 |
| 261 | 会话内面板接进客户端工具页 | `grep -c "OpsecSwitches" Tshell-web/src/views/client/View/ClientTools.vue` | 3 |
| 262 | 两个面板共享同一份能力表 | `grep -c "OPSEC_CAPABILITIES\|evasionCapabilities" Tshell-web/src/components/Opsec/opsecPresets.ts Tshell-client/implant/core/config/opsec.go` | 9 |
| 263 | 客户端能力表单测 | `grep -c "^func Test" Tshell-client/implant/core/config/opsec_test.go` | 8 |
| 264 | 运行期命令单测 | `grep -c "^func Test" Tshell-client/implant/core/commands/evasion_cmd_windows_test.go` | 9 |
| 265 | 服务端归一化单测 | `grep -c "^func Test" Tshell-server/pkg/api/opsec_params_test.go` | 8 |
| 266 | 占位符替换单测 | `grep -c "^func Test" Tshell-server/pkg/delivery/patch_opsec_switches_test.go` | 3 |
| 267 | 配置校验单测 | `grep -c "^func Test" Tshell-server/pkg/config/opsec_apply_test.go` | 4 |
| 268 | 前端预设/置灰单测 | `grep -c "^  it(" Tshell-web/src/components/Opsec/opsecPresets.test.ts` | 13 |
| 269 | 前端仍禁 isZh 三元与 as any（techdebt 棘轮） | `grep -c "isZh\|as any" Tshell-web/src/techdebt.test.ts` | 2 |

## AA 组（阶段四：启动期能力可撤销，2026-10-20 追加）

| 270 | `evasion off` 走 patch 台账撤销（不是只翻开关） | `grep -c "RestorePatchesByTag" Tshell-client/implant/core/commands/evasion_cmd_windows.go` | 1 |
| 271 | 写入前先记原始字节（否则永久不可撤销） | `grep -c "recordPatch(" Tshell-client/implant/core/evasion/evasion.go` | 1 |
| 272 | 台账实现齐全（记账/还原/按能力/清空） | `grep -c "func recordPatch\|func [Rr]estorePatches" Tshell-client/implant/core/evasion/patch_registry_windows.go` | 4 |
| 273 | 撤销行为有实机单测（逆序/隔离/外部改动跳过/空台账） | `grep -c "^func Test" Tshell-client/implant/core/evasion/patch_registry_windows_test.go` | 8 |
| 274 | 文档说清"etw/amsi 能还原、unhook 不回挂" | `grep -c "写回原始字节" docs/OPSEC.md` | 1 |
| 275 | 状态表文案同步（不谎称"不会撤销"） | `grep -c "unhook 关掉不回挂" Tshell-client/implant/core/config/opsec.go` | 1 |
| 276 | 前端面板文案同步 | `grep -c "不会重新挂上" Tshell-web/src/components/Opsec/OpsecSwitches.vue` | 1 |

## AB 组（P5：命令延迟三症状，2026-10-21 追加）

| # | 断言 | 命令 | 期望 |
|---|---|---|---|
| 277 | 命令节流阈值与睡眠间隔分离（不再拿 WaitTime 当节流） | `grep -c "CommandDebounce = 200 \* time.Millisecond" Tshell-client/implant/core/config/config.go` | 1 |
| 278 | dispatch 用的是防抖变量而不是 WaitTime | `grep -c "debounce := config.CommandDebounce" Tshell-client/implant/core/app/dispatch.go` | 1 |
| 279 | 旧的"拿 WaitTime 节流"必须彻底消失 | `grep -c "elapsedTime < config.WaitTime" Tshell-client/implant/core/app/dispatch.go` | 0 |
| 280 | 交互会话活跃判定有两个平台实现（缺一个就会永久快轮询） | `grep -c "func InteractiveShellActive() bool" Tshell-client/implant/core/commands/conpty_windows.go Tshell-client/implant/core/commands/commands_pty_unix.go` | 2 |
| 281 | 快轮询间隔是 150ms 量级并有回落分支 | `grep -c "const fastPollInterval = 150 \* time.Millisecond" Tshell-client/implant/cmd/single/runloop.go` | 1 |
| 282 | 会话结束会回落（否则永久高频） | `grep -c "fastPollActive = false" Tshell-client/implant/cmd/single/runloop.go` | 1 |
| 283 | 队首连续终端输入合并（保序、不跨类型） | `grep -c "func (c \*ClientCommandQueue) GetCommandCoalescingInput" Tshell-server/pkg/command/commandQueue.go` | 1 |
| 284 | GET 单条分支真的用了合并入口 | `grep -c "GetCommandCoalescingInput(uid, command.INTERACTIVE_SHELL_INPUT)" Tshell-server/pkg/api/communication/get.go` | 1 |
| 285 | sleep 前缀解析 + 「间隔」列同步（旧实现只有 UI 对话框会写这列） | `grep -c "func parseSleepCommand\|func syncClientSleepColumn" Tshell-server/pkg/sendcommand/sendCommand.go` | 2 |
| 286 | 快轮询下不再做毫秒级摘链（睡眠窗口太短无意义） | `grep -c "d >= time.Second" Tshell-client/implant/core/app/run.go` | 1 |
| 287 | 延迟相关单测总数（合并 6 + sleep 同步 2 + 防抖 2 + 快轮询 2） | `grep -ch "^func Test" Tshell-server/pkg/command/commandQueue_coalesce_test.go Tshell-server/pkg/sendcommand/sleep_sync_test.go Tshell-client/implant/core/app/dispatch_debounce_test.go Tshell-client/implant/cmd/single/runloop_fastpoll_test.go` | 6+2+2+2=12 |

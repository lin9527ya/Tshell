# Tshell 架构说明

C2 三端架构:服务端(Go + Gin + SQLite)+ 客户端植入体(Go,单二进制 12 种传输)+ Web 前端(Vue3 + TypeScript + Element Plus)。

## 目录结构

```
Tshell/
├── Tshell-server/           # 服务端(API + 监听器 + 模板托管)
│   ├── main.go              # 路由注册、中间件、JWT/RBAC、启动初始化
│   ├── pkg/api/             # HTTP API 层(客户端管理/监听器/终端/插件/交付...)
│   ├── pkg/connection/      # 协议监听器(TCP/KCP/WebSocket/DNS/OSS)
│   │   ├── pool_manager.go  # 统一连接池(容量/健康检查/空闲清理)
│   │   ├── offline.go       # 统一离线检测器
│   │   └── worker_pool.go   # 任务工作池
│   ├── pkg/command/         # 命令队列/回包处理/事件总线/各功能队列
│   ├── pkg/encrypt/         # 密钥体系(MetaInfo 主密钥 + 会话密钥 + nonce 唯一化)
│   ├── pkg/security/        # JWT/RBAC/隐藏访问/限流
│   ├── pkg/database/        # SQLite(WAL/连接池/维护任务)
│   └── pkg/delivery/        # 载荷投递 Hub
├── Tshell-client/implant/   # 客户端植入体
│   ├── cmd/single/          # 单二进制入口 + 12 个 transport(build tag)
│   ├── core/commands/       # 平台命令实现(文件/进程/截屏/PTY...)
│   ├── core/services/       # 命令分发(上传/下载/文件操作/隧道)
│   ├── core/communication/  # 收发/解密/结果回传
│   ├── core/encrypt/        # 客户端密钥管理器(HKDF 派生 + nonce)
│   └── build_templates.sh   # 全平台模板构建(166 EXE + 24 DLL)
└── Tshell-web/              # 前端
    └── src/                 # views/stores/router/utils/components
```

## 加密体系(统一 AES-GCM；旧加密仅保留用于 FirstBlood 元数据解析，默认禁用)

1. **MetaInfo(FirstBlood 前)**:监听器主密钥(VKey/Salt)作 AES-GCM,密文 `nonce(12)||ct`,随机 nonce 每次不同。
2. **会话密钥**:HKDF-SHA256,`info = "TshellSessionKey" + clientID`,由 VKey/Salt 派生。
3. **会话密钥唯一化(per-session)**:连接建立后服务端生成 16B 随机 nonce,`RotateSessionKeyWithNonce` 重派生(初始密钥降为 PreviousKey);nonce 消息(type=4)用初始密钥加密下发,客户端消费后两端密钥对齐。**同一监听器的不同会话密钥互不相同,提取任一客户端只能解密自己的会话**。
4. **密钥策略**:监听器必须配置自定义 VKey/Salt(默认公开密钥被拒绝:监听器启动、firstBlood、GenServer 三处强制);客户端二进制占位符未 patch 时 fail-closed 拒绝启动。

## 传输协议

- **持久连接**(TCP/KCP/WebSocket):length(4)|base64(密文) 分帧,firstBlood 时下发 nonce;读超时/离线检测回收异常连接。
- **轮询**(HTTP/HTTPS/OSS/DNS):命令队列 + 拉取,nonce 经 pending 机制优先下发;DNS 为 base32hex(base64(密文))。
- **反向隧道**(*_forward):客户端监听,服务端拨号。

## 关键机制

- **命令链路**:服务端队列 → 加密下发 → 客户端 `HandleCommand`(入口 recover + 逐命令边界校验)→ 回包 → `HandleClientReply`(共享处理:shell 追加/下载节流/截图分发)。
- **离线信号**:断线时 `CleanupClientQueues` 广播,等待中的 API handler 提前返回(避免白等超时)。
- **连接池**:MaxConnections 限制 + 健康检查 + 空闲清理,WS 已入池。
- **RBAC**:admin/operator/auditor 三角色,服务端路由层强制(RequireRole),前端路由守卫 + 菜单按 permiss 过滤;高危操作(ExecuteBin/批量删除/forward/载荷生成)限 admin+operator;密钥管理/安全中心/配置/用户管理为 admin 独有;审计日志读接口为 admin+auditor。
- **审计**:操作审计中间件 + 文件审计中间件,审计日志落**文件**(SQLite 落库为待办,见 docs/KNOWN_LIMITATIONS.md #12);交付访问日志 30 天轮转。

## 前端要点

- token:access 15 分钟(localStorage),refresh 仅内存不持久化;401 自动刷新仅重试一次。
- XSS:终端输出统一 HTML 转义后 v-html;nginx CSP 收紧(无 unsafe-eval、connect-src 同源)。
- 事件流:单例 WS + 指数退避重连 + ping/pong 保活。

## 测试与验证

- 服务端:`go test ./...`(19 个包有测试;加密派生一致性、队列竞态、事件总线、API 分页/密钥接口、监听器/隧道/插件/RBAC 策略等)。
- 前端:`npm run type-check` + `npx vitest run`(转义/状态工具用例)。
- e2e:`bash scripts/e2e.sh`(服务端 → 监听器 → 客户端 → 唯一化 + 命令回归)。
- CI:GitHub Actions 双端 build/vet/test + 12 transport 编译 + 前端 type-check/单测/构建。

# Tshell

当前版本:**v1.0.0**

跨平台多协议 C2 框架(基于开源 [Rshell](https://github.com/Rubby2001/Rshell---A-Cross-Platform-C2) 二次开发),面向**合法授权**的渗透测试与红蓝对抗场景。

## 架构

```
Tshell/
├── Tshell-server/    # 服务端:Go + Gin + SQLite(多协议监听/命令调度/加密体系/RBAC/交付/CNA)
├── Tshell-client/    # 植入体:Go 单二进制 12 种传输(build tag),Windows/Linux/macOS
├── Tshell-web/       # 前端:Vue3 + TS + Element Plus,CobaltStrike 风格桌面化 UI
├── docs/             # 补充文档(已知限制/SOCKS5 隧道/KCP 优化)
├── scripts/          # e2e 回归脚本等
└── tools/            # 隐匿构建等辅助工具
```

## 能力概览

- **12 种传输**:TCP / KCP / WebSocket(持久)、HTTP / HTTPS(uTLS JA3 伪装/域前置/请求 padding)、OSS 对象存储、DNS TXT 隧道(轮询),以及 5 类 `*_forward` 反向连接
- **加密体系**:AES-256-GCM 全链路;监听器主密钥 + HKDF 会话密钥;per-session nonce 唯一化;24h 密钥轮换(ACK + 重叠期);默认密钥三处 fail-closed 拒绝
- **植入体命令面**:文件管理(分块上传/下载/编辑/隐藏/时间戳)、进程、截屏/屏幕监控/HVNC、真 PTY 终端(ConPTY/creack-pty)、CMD/WMI 双引擎、ExecuteAssembly/InlineBin/BOF/进程注入/镂空、Token 链、PSEXEC/WMI/WinRM 横向、多形态持久化、键盘记录(中文 IME)、剪贴板/WiFi/服务/注册表、ETW patch/ntdll unhook、Linux hide_process/hide_net(LD_PRELOAD)
- **服务端**:统一命令队列与回包处理、事件总线、连接池、统一离线检测、JWT/RBAC(admin/operator/auditor)、审计、隐藏访问(伪 404)、限流、WebDelivery 投递 Hub(XOR/UA 白名单/一次性/StagerToken)、动态 stager 现场编译、CNA(Aggressor 兼容)脚本引擎、C2 Profile、钉钉/企微/飞书上线通知
- **前端**:会话表/Beacon 控制台/Pivot 拓扑/事件日志/脚本控制台/Payload 与 Attacks 对话框族/密钥管理/审计/安全中心

详细设计见 [ARCHITECTURE.md](./ARCHITECTURE.md)。

## 构建(顺序敏感)

> 预编译客户端模板(约 587MB)**已随仓库提供**,克隆后可直接编译(`go:embed` 依赖);
> 仅当植入体源码改动、需要更新模板时才重跑 `build_templates.sh`。

**一键构建**(推荐):

```bash
./build.sh                          # Linux / macOS / Git Bash:全量(前端 + 服务端 + 模板)
./build.sh --skip-templates         # 模板未变时跳过耗时的模板重建
powershell -ExecutionPolicy Bypass -File build.ps1   # Windows PowerShell 等价入口
```

**分步执行**(与一键脚本等价):

```bash
# 1. 客户端模板(166 EXE + 24 DLL,输出到 Tshell-server/pkg/api/server/)
cd Tshell-client/implant
bash build_templates.sh          # Windows 下可用 build_templates.ps1

# 2. 前端(产物自动嵌入服务端)
cd ../../Tshell-web
npm ci --legacy-peer-deps
npm run build:embed              # 写入 Tshell-server/dist

# 3. 服务端(CGO 必需,go-sqlite3 依赖)
cd ../Tshell-server
cp config.example.yaml config.yaml   # 按需修改(密钥/通知凭证等)
# 注意: 必须文件模式 .\main.go(包模式 `.` 在 Windows+TDM-GCC 下产出的
# cgo exe 可能无法执行);gcc 需在 PATH(如 C:/TDM-GCC-64/bin)
CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go build -ldflags "-s -w" -trimpath -o tshell-server.exe .\main.go
```

前置要求:Go ≥ 1.24、Node ≥ 18、GCC(仅 DLL 模板需要,如 mingw-w64/TDM-GCC)。
完整说明见 [DEPLOYMENT.md](./DEPLOYMENT.md)。

## 使用

```bash
./tshell-server -p 8089          # 默认账号 admin/admin(首启后立即改密)
```

浏览器打开 `http://服务器:8089`。监听器必须配置独立 VKey(≥32B)/加密盐,默认/空密钥会被拒绝;客户端需携带反沙箱 ExecuteKey 上线。

## 开发

- 服务端测试:`cd Tshell-server && go test ./pkg/...`(需 CGO + GCC,如 `CC=C:/TDM-GCC-64/bin/gcc.exe`)
- 前端检查:`cd Tshell-web && npm run type-check && npx vitest run`
- e2e 回归:`bash scripts/e2e.sh`
- 关键优化项与验证命令:[OPTIMIZATION_CHECKLIST.md](./OPTIMIZATION_CHECKLIST.md)
- 已知限制:[docs/KNOWN_LIMITATIONS.md](./docs/KNOWN_LIMITATIONS.md)

## 免责声明

本项目仅为网络安全研究、合法授权测试及教育目的而设计开发。**禁止将本项目用于任何非法用途**,包括但不限于未经授权的系统入侵与网络攻击活动。使用者应确保在完全合法授权的前提下使用本工具,并遵守当地法律法规;开发者不对任何滥用行为负责。下载、使用本项目即表示您已充分阅读并同意本声明所有条款。

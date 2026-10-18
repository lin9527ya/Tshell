# Tshell 已知限制项（Known Limitations）

> 状态约定：✅ 已实现 | ⏳ 待实现（实现时更新）

## 1. inject 隐蔽注入
- 现状：✅ 已实现（阶段三-1）：最小权限句柄 + RW 写入→执行前 RX + 写后校验 + LastError 细节。
- 遗留：间接 syscall（NtCreateThreadEx 直调）未做，CreateRemoteThread 仍可被检测。进一步隐蔽需替换为线程劫持/APC 注入。
- 状态：✅（核心）/ ⏳（深度隐蔽）

## 2. BOF token API
- 现状：✅ 已实现（阶段三-2）：BeaconIsAdmin（TokenElevation）、BeaconUseToken（ImpersonateLoggedOnUser）、BeaconRevertToken（RevertToSelf）。
- 状态：✅

## 3. inline-bin patchExit 对直接 syscall 无效
- 现状：patch 劫持导出函数入口；shellcode 走直接 syscall（绕过 ntdll 导出）时 patch 无效。
- 计划：syscall 级 patch 涉及内核调用表/重定位层，工程量大且稳定性风险高。
- 状态：⏳（接受现有限制，文档化）

## 4. inline-bin 子进程 stdout 句柄继承
- 现状：✅ 已实现（阶段三-4）：CreateFile 传 SECURITY_ATTRIBUTES{bInheritHandle:1}。
- 状态：✅

## 5. keylog 完整 IME/中文输入
- 现状：✅ 已实现（阶段三-5）：ToUnicodeEx + GetKeyboardState + GetKeyboardLayout，支持中文 IME 组合；保留 ASCII 回退。
- 状态：✅

## 6. execute-assembly 单例 CLR
- 现状：✅ 部分实现（阶段三-6）：全局执行互斥消除并发句柄竞争；go-clr 单例 host 限制仍存在（restore 后旧管道写入，输出捕获不受影响）。
- 状态：✅（互斥）/ ⏳（多实例需替换 go-clr）

## 7. 双引擎架构重构
- 现状：✅ 部分实现：currentProc 赋值（中断生效）、去 .bat 落盘（参数传递）、人为延迟移除。
- 遗留：WMI OLE 固定 goroutine + CoInitialize 配对、输出统一 cmdType 36、与 ConPTY 共享会话注册表。
- 状态：✅（核心）/ ⏳（深度收敛）

## 8. 遗留弱加密
- 现状：✅ 已实现（阶段三-8）：`AllowLegacyCommandFallback` 默认 false（新客户端全部使用会话密钥）。
- 遗留：旧加密函数（Encrypt/Decrypt）代码保留用于 FirstBlood 元数据解析；彻底移除需改 FirstBlood 协议。
- 状态：✅（禁用）/ ⏳（代码移除）

## 9. 密钥轮换协议（notifyKeyUpdate）
- 现状：✅ 已实现（2026-10 核对）：服务端 `pkg/encrypt/encrypt.go` 的 `notifyKeyUpdate`
  经 `KeyRotationSender`（`main.go` 注入）下发 `type=5|nonce`，客户端
  `core/communication/transport_utils.go` 消费并回 ACK(121)，`ConfirmKeyRotation`
  完成 Current/Previous/Next 切换（重叠期仍可解旧消息），`derivation_compare_test.go`
  有对应单测。
- 状态：✅

## 10. VKey 密钥口径统一
- 现状：GenerateListenerKeyConfig 返回 base64(32B)，消费方取字符串前 32/16 字符（熵 192bit）；手动设置 VKey 直接按字符串使用。
- 计划：统一"先 base64 解码再取字节"（破坏性，需迁移现有 DB 监听器密钥，部署环境迁移风险高）。
- 状态：⏳（破坏性变更，谨慎评估）

## 11. 前端 GBK 修复映射表
- 现状：encoding.js 硬编码乱码→中文映射表（几十条）。
- 计划：统一为字节级 TextDecoder('gb18030') 流式方案。
- 状态：⏳

## 12. 审计日志落库
- 现状：✅ 已实现（2026-10）：审计**文件 + SQLite 双写**（文件保留人读与归档），
  查询/统计/登录失败/保留策略全部改走 `audit_log_db` 表（索引 + 库侧分页 + 排序）。
  时间范围与排序用数值列 `ts_unix`（SQLite 的 DATETIME 是 NUMERIC 亲和，
  直接比较时间串/时间值不可靠，实测踩过）。
- 遗留：10 万级日志的预聚合统计（当前统计仍按范围聚合查询）与按月分区。
- 状态：✅（落库）/ ⏳（预聚合）

## 12b. 下载任务无"传输结束"报文
- 现状：下载协议只有"首包声明大小 + 分块"，客户端发完最后一块就停，没有结束报文。
  服务端据此判定终态：**已落盘字节 ≥ 声明大小 → completed**；客户端断线或 120s 无分块 → failed。
- 影响：慢链路长时间停顿（>120s 无分块）会被判 failed，而客户端随后仍可能续传
  （句柄已回收，续传分块被忽略）。此时任务状态与磁盘实际内容可能不一致。
- 若要根治：协议加一个下载结束报文（客户端发完主动告知），属协议变更。
- 状态：⏳（协议层），当前判定已能覆盖常规场景且不再让任务永远停在 dispatched。

## 12c. 凭据获取（阶段三 · 模块 1：LSASS）
- 现状：✅ 服务端链路 + 协议 + 客户端 Windows 实现（comsvcs / dbgcore / auto 三种技术）
  已实现；制品经下载通道回传，服务端校验 minidump 后登记为 `loot(type=dump)`，
  全程有 `CRED_ACCESS` 审计与 `tshell_cred_artifacts_total` 指标。
- 遗留：**Windows 端未在真实目标上验证**（本仓库只能交叉编译）；DPAPI / 浏览器 /
  Kerberos / BloodHound / 提权 / 端口扫描等后续模块未实现。
- 详见 docs/CREDENTIAL_ACCESS.md（含"验证状态"与首次验证建议）。
- 状态：✅（链路）/ ⏳（实机验证 + 后续模块）

## 12d. 内存 OPSEC（阶段三 · 第 14 项）
- 现状：✅ sleep mask 修复并扩展到**全部传输**（原来只有轮询型走 mask，且遮蔽的是
  `GetSessionKey()` 返回的拷贝 —— 真身一直明文驻留，等于没保护）；遮蔽范围含会话密钥/
  上一把/主密钥/盐 + 清空 AEAD 缓存；按片遮蔽 + 在途判定，不打断活跃会话。
  ✅ syscall trampoline：RWX → W^X（RW 写入后 VirtualProtect 为 RX），
  `jmp rel32`（±2GB 射程，ASLR 下会溢出成垃圾地址）→ `mov rax,imm64; jmp rax`。
- 遗留：Ekko/Foliage 式"加密自身内存镜像"、调用栈伪造、注入路径全 syscall 化
  —— 均需 Windows 实机验证；Go runtime 无法安全加密自身 .text 段。
- 详见 docs/MEMORY_OPSEC.md。
- 状态：✅（sleep mask / trampoline）/ ⏳（Ekko 级 mask、调用栈伪造、全 syscall）

## 12e. macOS 能力补齐（阶段三 · 第 15 项）
- 现状（本次实现）：
  - **持久化**：launchd —— `launchagent`（~/Library/LaunchAgents，登录用户身份）与
    `launchdaemon`（/Library/LaunchDaemons，需 root）。plist 带 RunAtLoad + KeepAlive +
    Background + 输出丢弃，并以 `os.Args[1:]` 作为启动参数（生成时设了口令的植入体
    不带口令会直接退出，表现为"持久化成功但重启不上线"）。载入按
    `launchctl bootstrap gui/<uid>` → `launchctl load -w` 顺序尝试。
  - **文件隐藏**：`chflags hidden|nohidden`（与 Windows 的隐藏属性语义一致）。
  - **屏幕截图**：系统 `screencapture -x -t png`；屏幕监控采用**拉取式**
    （服务端"获取监控帧"接口按需截一帧），不在客户端常驻推送循环。
  - **交互式 PTY 终端**：把 Linux 的 PTY 实现抽成 `linux || darwin` 共享文件
    （`commands_pty_unix.go`），平台差异只有默认 shell（Linux `/bin/bash`，
    macOS 优先 `/bin/zsh`，缺失回退 bash）。xterm 终端页在 macOS 上完整可用。
- 遗留：
  - InlineBin（内存执行）、令牌操作、进程注入等 Windows 专属能力在 macOS 上仍明确报不支持；
  - macOS 端未在真实设备上验证（本仓库只能交叉编译；单测覆盖 plist 生成、PTY 会话
    生命周期/特殊键/窗口调整、截图与隐藏的实现选择）。
- 状态：✅（持久化/隐藏/截图/监控拉取/交互式 PTY）/ ⏳（实机验证）

## 12f. 端口扫描（阶段三 · 第 13 项子能力）
- 现状：✅ 已实现：控制台命令 `scan <目标> <端口> [超时毫秒]`，如
  `scan 10.0.0.0/24 top`、`scan 10.0.0.5 22,80,443 500`。
  - 目标支持单 IP / CIDR / 主机名 / 逗号混合；端口支持列表、范围、`top`（26 个常用端口）。
  - TCP connect 扫描（跨平台、无需 root），每端口超时默认 800ms（50ms–10s 钳制），
    并发 128；**目标上限 256 台、端口上限 4096**（超限直接拒绝而不是静默截断）。
  - 结果按主机聚合成一行回显（便于 grep），并给出一行汇总。
- 遗留：SYN 半开扫描（需要 raw socket/root）未做；UDP 扫描未做。
- 状态：✅（TCP connect）/ ⏳（SYN/UDP）

## 12g. Schema 迁移与水平扩展（阶段二 §10 / 阶段三 §16）
- 现状（本次实现）：
  - **版本化迁移**：`schema_migrations` 表 + 有序迁移清单（`pkg/database/migrations.go`），
    启动时按序补齐缺口；每个迁移成功后立即记录，失败则中止启动且不记录（可重试，
    要求迁移幂等）。`GET /api/version` 增加 `schema_version` 字段。
    原实现只有"检查列存在再 ALTER"的两个函数，没有版本记录，无法判断一个库到哪一步。
  - **单实例约束写进文档**：`DEPLOYMENT.md` 明确说明为什么必须单副本
    （连接池/在线状态/会话锁/事件流/命令队列均在进程内存，数据库是单文件 SQLite），
    并给出当前架构下唯一诚实的 HA 方案（定时备份 + 冷备）。
- 遗留：
  - 存储抽象（repository 层）与 Postgres 支持未做：当前所有 SQL 直接写在 `pkg/api`/`pkg/database`，
    且含 SQLite 专有语法（PRAGMA、`UPDATE ...` 的 SQLite 方言等）；
  - 共享状态外置（会话/队列/锁）与 leader 选举未做 —— 这两项完成后才能真正多副本。
- 工作区里的 `k8s-deployment.yaml` 是旧草稿（replicas:2 + 两个 HPA），与代码能力不符，
  且未被 Git 跟踪；不要直接套用。
- 状态：✅（版本化迁移 + 单实例文档）/ ⏳（存储抽象、Postgres、共享状态）

## 13. 级联 C2（B9，二级中继）
- 现状：✅ 已实现（传输层中继，`Tshell-relay/`，纯标准库）：
  - TCP/UDP 双向转发，支持**多级级联**（上游指向下一跳）；TLS 终止可选；
    连接数/单 IP 上限、空闲超时、UDP 流表上限与回收、`/healthz` 健康检查。
  - 中继**不解析协议、不持有密钥**：beacon ↔ 服务端的会话加密是端到端的，
    中继被取证也解不开指令内容；传输类型对它透明（TCP/WS/HTTP(S)/KCP 都能承接）。
  - 部署要点：监听器的 connect_address 必须填中继地址（填反会绕过中继）。
- 遗留：DNS 传输（需按域名转发）不纳入中继；中继不做多上游负载均衡。
- 详见 docs/RELAY.md。
- 状态：✅（传输层中继 + 级联）/ ⏳（DNS 中继、多上游）

## 14. 多协议热切换（B10，单二进制多协议）
- 现状：✅ 已实现（运行时传输回退链）。`-tags transport_multi` 构建一个包含全部
  12 种传输的二进制；服务端生成载荷时传 `endpoints`（`scheme://addr,...`），
  patch 进 512 字节定长占位符。运行时按端点顺序尝试，某个连不上时 runloop 轮转到
  下一个端点 —— 下一个端点可以是完全不同的传输（tcp → https → dns）。
  - 客户端：传输构造器注册到运行时表（`registerTransport`），`multiTransport`
    按 scheme 选择实现；`listenerType` 在每次连接时更新为实际传输，服务端据此
    记录会话类型（FirstBlood 后 `ClientListenerType` 由实际监听器更新）。
  - 服务端：`POST /api/client/GenServer` 新增 `endpoints` 字段；校验端点格式/类型
    （≤8 个、总长 ≤512）；**校验端点监听器与主监听器的 VKey/盐一致**（一个二进制
    只能带一份密钥，密钥不一致的 beacon 切过去后握手必失败，现场只表现为"切过去就掉线"）。
  - 模板：`build_templates.sh` 已加入 `multi`（各平台 EXE + DLL），生成到
    `pkg/api/server/multi/`。
- 遗留：多传输的**主动健康探测**（当前只在连接失败时回退，不会在"连接还在但已半死"
  时主动切换）；macOS/Windows 实机验证。
- 状态：✅（运行时回退链）/ ⏳（主动探测、实机验证）

## 15. TLS 证书自动签发（ACME）
- 状态：⏳

## 16. 密钥健康检查（已下线入口）
- 现状：该功能三层都不成立，已摘除入口：服务端 `SendKeyHealthCheck` 把 4 字节 cmdType
  交给面向文本命令的 `SendCommand`，匹配不到任何前缀后被兜底成 `buildCmd(SHELL, ...)`
  —— 客户端收到的是"包着 4 个原始字节的 SHELL 命令"，cmdType 94 根本没发出去；
  客户端也没有 94 的常量与 dispatch 分支；服务端更没有消费回包的处理
  （`KEY_HEALTH_CHECK` 除定义外全仓零引用）。因此点了按钮只会"没反应"。
- 替代：`GET /api/key-management/key-status/:client_id` 已能显示服务端侧的会话密钥状态
  （当前/上一/下一密钥）与轮换参数，密钥管理页保留"强制轮换"与轮换配置。
- 若要恢复"让客户端确认自身密钥状态"：需客户端补常量+case、服务端改用原始命令发送方式、
  并补回包消费与前端展示 —— 属新功能而非修 bug。

# Tshell 已知限制项（Known Limitations）

> 状态约定：✅ 已实现 | ⏳ 待实现（实现时更新）

## 1. inject 隐蔽注入
- 现状：✅ 已实现（阶段三-1）：最小权限句柄 + RW 写入→执行前 RX + 写后校验 + LastError 细节。
- 已实现（2026-10-20，OPSEC P0）：
  - **direct/indirect syscall 可用**（`core/evasion/syscall.go`，`SYSCALL_MODE_PLACEHOLDER`
    取 direct/indirect）。**默认仍是 none**（服务端默认替换为 none）—— 要用必须在生成
    载荷时显式选择，不要按"已经开启"做假设。
  - 注入链的分配/写入/改保护改走 `NtAllocateVirtualMemory` / `NtWriteVirtualMemory` /
    `NtProtectVirtualMemory` / `NtOpenProcess`（`core/commands/remotemem_windows.go`），
    逐项失败回退宿主 API；`NtCreateThreadEx` 已是默认首选，`CreateRemoteThread`
    只在显式 `crt` 或整链回退时出现。
  - 进程镂空的镜像内存不再申请 RWX：先 RW 写入，再把**可执行节**收紧为 RX
    （`hollow_windows.go` 的 `hardenImage`）。非可执行节保持 RW 是 loader 的硬需求
    （ResumeThread 之后它要写 IAT 与全局），理由写在函数注释里。
- 遗留：
  - APC 分发仍用 kernel32 `QueueUserAPC`（未接 `NtQueueApcThread`）；`CreateRemoteThread`
    回退路径仍在。
  - 镂空"需重定位"路径（拿不到 preferred base 且镜像带 `.reloc`）可执行节仍是 RWX，
    回包诊断会显式标 `prot=[...RWX(重定位期)]`，不是静默降级。
  - 以上改动**未在真实目标上验证**（本机只有单测 + 真实 kernel32 往返）。
- 状态：✅（核心 + syscall 接线）/ ⏳（APC 与实机验证）

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

## 12c. 凭据获取（阶段三）
- 现状：✅ **三类制品**的服务端链路 + 协议 + 客户端 Windows 实现已就绪：
  - **LSASS 内存转储**（`kind=lsass`）：comsvcs / dbgcore / auto 三种技术；
    制品经下载通道回传，服务端校验 minidump 后登记为 `loot(type=dump)`。
  - **DPAPI 主密钥集**（`kind=dpapi`，本轮）：逐个解密
    `%APPDATA%\Microsoft\Protect\<SID>` 下的用户主密钥，打包为
    ZIP（`manifest.json` + `keys/<GUID>.json`，masterkey 为 base64 明文）；
    服务端逐条目 CRC 校验 + 要求 manifest，登记 `loot(type=dump, source=dpapi)`。
  - **浏览器凭据与 Cookie**（`kind=browser`，本轮）：Chrome/Edge 的
    `Local State`（DPAPI 解出浏览器主密钥）→ `Login Data` / `Network/Cookies`
    逐条 AES-GCM 解密（v10 / v20 / 老式明文），**在植入体内存中完成**，
    只回传明文 JSON；服务端解析后把口令写入 `loot(type=pwd, source=browser:<浏览器>)`、
    Cookie 写入 `loot(type=cookie)`。明文不落盘，临时文件回传后删除。
  - 三类共用：类型×技术分域校验（`browser` 配 `comsvcs` 之类当场拒绝）、
    `CRED_ACCESS` 审计、`tshell_cred_artifacts_total{kind}` 指标、失败显式（不登记半截制品）。
- 遗留：
  - **Windows 端未在真实目标上验证**（本仓库只能交叉编译）；
  - 已实现：LSASS / DPAPI / 浏览器（Chrome/Edge/Firefox）/ **Kerberos 票据（文件式 ccache）**；
  - 已实现：Chromium `Web Data`（信用卡 + 自动填充，`tech=cards`；卡进 `loot(type=card)`，
    自动填充只留制品）；
  - 已实现：**Windows LSA 票据**（`kind=ticket, tech=lsa`）：枚举 + 导出 kirbi，
    元数据随制品清单入库（2026-10-19，实机未验证）；
  - 已实现：票据动作 `kerberos list|purge [服务名]|ptt <base64>`（2026-10-19，实机未验证）；
  - 未实现：kirbi↔ccache 互转、BloodHound 采集器；
  - 提权模块（getsystem / UAC bypass）已实现，见 §12c 之外的独立说明。
- 详见 docs/CREDENTIAL_ACCESS.md（含 §7 第二批模块、§9 验证状态）。
- 状态：✅（四条链路 + 单测）/ ⏳（实机验证 + LSA 票据/BloodHound/Web Data）

## 12h. 自写 SQLite 只读读取器（浏览器凭据模块的依赖）
- 现状：✅ 已实现（本轮）：`implant/core/sqlite/`，纯 Go、零依赖，只做
  "按表读行"：文件头校验 → `sqlite_master` 定位根页 → 表 b-tree 遍历 →
  记录解码（varint / serial type / NULL / BLOB / TEXT / REAL）→ 溢出页链 +
  **WAL 覆盖**（Chromium 默认 `journal_mode=WAL`，最新数据常只在 `-wal` 里）。
  所有长度字段都按页/库大小钳制，畸形结构返回错误而不 panic 或死循环。
- 为什么不引驱动：植入体是 `CGO_ENABLED=0` 全矩阵交叉编译，且模板体积敏感
  （141 个模板共 373MB）；`go-sqlite3` 需要 cgo，纯 Go 驱动会让每个模板胀大数 MB
  并把模块的 Go 版本要求抬高（实测：引 `modernc.org/sqlite` 后 `go mod tidy`
  直接把 `go` 指令顶到 1.26，CI 的 1.25 立刻构建失败）。
- **遗留（重要）**：溢出页单元格的**页内切分点**尚未逐字节对齐。
  SQLite 没有公开可靠的解析式（file format §1.5 的写法与实测样本不符：
  `scripts/fit_sqlite_overflow.py` 用 4 种页大小 × 9 种载荷实测，没有一个候选式能全覆盖）。
  读取器目前用"溢出链正好用完 + 末页指针为 0"的结构判据反推切分点，
  并在所有可能切分点里穷举验证 —— 链结构与记录列数都能对上，
  但**页内片段与溢出页的衔接处仍可能偏移若干字节**（表现为该条密文解不开，
  不会产生错误的明文）。影响面：只有 >1KB 的字段（浏览器库里主要是大 Cookie、
  少数长口令）会走到这条路径；LSASS/DPAPI 制品与常规浏览器条目不受影响。
- 若要根治：按 SQLite 源码（`btreeInitPage` / `fillInCell`）逐位对齐
  `maxLocal` / `minLocal` / 溢出阈值，并用真实 Chrome 库做逐字节回归。
- 状态：⏳（常规路径 ✅ / 溢出页切分点待精修）

## 12i. 浏览器凭据模块的实机验证
- 现状：✅ 链路与算法已实现并有单测（v10/v20/明文的解密分支、篡改与截断必须报错、
  `Local State` 畸形输入、制品 schema 与服务端对齐、真实库只读冒烟测试）。
- 尚未验证（需要授权 Windows 目标）：
  1. `CryptUnprotectData` 在真实用户上下文里的行为（开发机上浏览器库是空的，
     覆盖不到"有已保存口令"的分支）；
  2. 较新 Chrome 的 **App-Bound Encryption**：若目标启用了 App-Bound，
     仅靠 `CryptUnprotectData` 可能拿不到可用主密钥（需要在目标侧另有手段）；
  3. `Login Data` 被浏览器独占时的副本回退路径；
  4. 数万条 Cookie 的采集与回传耗时。
- 状态：✅（实现 + 单测）/ ⏳（实机验证）

## 12j. 提权（阶段三 · getsystem）
- 现状：✅ 已实现 `getsystem [auto|token|uac|uac_cleanup]`，三条技术路径：
  - `already`：已是 SYSTEM/高完整性时只报告（幂等，可反复执行）；
  - `token`：从已提权进程（winlogon/lsass/services/wininit/smss/explorer）复制
    SYSTEM/高完整性令牌 → `CreateProcessAsUserW` 派生**新会话**；
  - `uac`：`fodhelper.exe`/`computerdefaults.exe` 注册表劫持 → 高完整性新会话 →
    **立即还原注册表**（删不掉时退化为清空劫持值，绝不留下可用劫持）。
  设计上**只派生新进程、不动当前会话的令牌模拟状态**：成功失败都不影响已有 beacon；
  新会话按 `os.Args[1]` 透传启动口令（否则新进程会因校验失败立刻退出，
  表现为"提权成功但没上线"）。
- 单测：参数校验、权限启用语义（含 `ERROR_NOT_ALL_ASSIGNED` 检查）、注册表还原/退化、
  `RegDeleteKeyW` 不递归这条 Windows quirk、进程枚举过滤 —— 见 docs/PRIVILEGE_ESCALATION.md §6。
- 遗留：
  - **未在授权 Windows 目标上验证真实成功率**（开发环境的受限令牌连
    `LookupPrivilegeValueW` 都拒绝，`token` 路径端到端跑不到）；
  - `uac` 路径依赖 `fodhelper`/`computerdefaults` 仍读取 `HKCU\...\ms-settings\shell\open\command`，
    较新 Windows 版本可能已收紧，需现场确认；
  - 除 UAC 之外的本地提权（服务配置错误、AlwaysInstallElevated、令牌特权滥用等）未做。
- 详见 docs/PRIVILEGE_ESCALATION.md（含 VM 验证清单）。
- 状态：✅（实现 + 单测）/ ⏳（实机验证 + 更多提权链）

## 12k. MFA/TOTP 的防重放状态与救援路径（阶段三 §3.5）
- 现状：✅ 两步登录（口令 + 动态码/恢复码）、自助绑定/关闭、管理员重置全部实现并有
  13 项集成测试；TOTP 密钥 AES-256-GCM 密文落库（密钥由 JWT 私钥 HKDF 派生）。
- 已知限制：
  1. **防重放状态在进程内存里**（`pkg/api/mfa.go` 的 `mfaLastSteps`）。单实例正确；
     多副本部署下同一个 30 秒窗口内的码可能在另一个副本上被接受 —— 与 `pkg/collab`
     会话锁同类问题，属 §3.4 水平扩展范畴。进程重启会清空该表（30s 窗口内重启才可能被重放）。
  2. **未做 WebAuthn / "记住此设备"**：每次登录都必须输动态码。
  3. **JWT 私钥变更会让既有 TOTP 密钥解不开**：表现为登录报"动态码校验暂不可用"。
     救援路径是管理员 `POST /users/mfa/reset`。**不静默降级为单因子** ——
     解不开就当"没开启"等于悄悄关掉第二因子（有测试守住这条区分）。
  4. 恢复码用完不自动补发：仍可用动态码登录；重走 `start`+`confirm` 会换一批
     （同时换新密钥与重放记录）。
- 详见 `docs/AUTH_MFA.md`。
- 状态：✅（实现 + 单测 + 集成测试）/ ⏳（多副本下的防重放外置，属 §3.4）

## 12l. per-listener C2 画像（阶段三 §3.6）
- 现状：✅ 已实现。`listener.profile` 列（迁移 v4）+ 路由与 WS 路径按该监听器绑定的画像注册
  + 生成载荷的一致性基准改为该监听器的画像。同机可并存多套特征
  （`TestTwoListenersDifferentProfilesCoexist`）。
- 已知边界：
  1. **运行中不能改画像**：句柄按启动时的路径注册，改库会让"关不掉 + 显示不对"变成
     僵尸监听器。与改地址/协议类型同一规则（`UpdateListener` 返回明确提示）。
  2. **改画像不会通知已上线的载荷**：它们连的是自己的老路径，继续可用；
     但**新生成的载荷必须用新画像**，否则服务端会拒绝生成（这是刻意的 fail-fast）。
  3. 画像文件缺失时**静态回落 default 并告警**，而不是拒绝启动 —— 升级路径上
     不能因为 profiles/ 里少一个文件就让监听器起不来。
  4. 迁移只加列、默认空串（=default），老库升级后行为与升级前完全一致。
- 状态：✅（实现 + 测试）

## 12d. 内存 OPSEC（阶段三 · 第 14 项）- 现状：✅ sleep mask 修复并扩展到**全部传输**（原来只有轮询型走 mask，且遮蔽的是
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
- 现状（本次实现）：
  - **PostgreSQL 后端**：`config.database.type: postgres` + `dsn` 即可切换；方言差异
    （占位符、序列重置、列内省、shell 追加截断、大小写匹配、DDL 时间类型、SQLite 调优/
    旧库修复）收敛在 `pkg/database/dialect.go`，业务代码通过 `ExecSQL`/`QueryStringSQL`
    与 xorm 构建器访问数据库。详见 docs/STORAGE.md。
  - PostgreSQL 集成测试（Docker/CI service）覆盖建表 → 迁移 → CRUD → 原生 SQL →
    序列重置 → 列内省；并借此修掉一个真实的 schema 缺陷（listener 迁移检查的是
    `vkey` 而模型列名是 `v_key`，导致多建一列）。
  - 内置备份在 PostgreSQL 下不启用（503），改用 pg_dump（见 docs/STORAGE.md）。
  - **存储访问层（Store 接口）**：业务代码统一走 `database.DB()`（`pkg/database/store.go`
    的 `Store` 接口），不再直接引用具体引擎；根包 `store_access_test.go` 是回归棘轮
    （业务代码一旦重写 `database.Engine` 即 CI 失败）。见 docs/STORAGE.md 第 4 节。
- 遗留：
  - 领域 repository 尚未抽出：查询语句仍写在各业务包里（只是访问点已收口到 `Store`）；
  - SQLite → PostgreSQL 无内置数据迁移工具（需 pgloader/脚本搬运）；
  - 共享状态外置（会话/队列/锁）与 leader 选举未做 —— 这两项完成后才能真正多副本。
- 工作区里的 `k8s-deployment.yaml` 是旧草稿（replicas:2 + 两个 HPA），与代码能力不符，
  且未被 Git 跟踪；不要直接套用。
- 状态：✅（版本化迁移 + 存储访问层 + Postgres + 单实例文档）/ ⏳（领域 repository、共享状态）

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
- **主动健康探测**（本次实现）：对全部端点做轻量 TCP 探测（60s 周期、3s 超时、
  连续 2 次失败判定不健康）：
  - 轮转时**优先选健康端点**（`app.EndpointSelector` 钩子），全不健康才退回顺序轮转；
  - 活动端点连续探测失败 → 主动断开，让 runloop 立即重连（写成功但读不通的黑洞路径
    只靠读超时发现太慢，探测更快）；
  - 不可 TCP 探测的传输（KCP 是 UDP、DNS 是解析器地址、OSS 是对象存储地址格式）
    视为健康——宁可漏判，不误伤正常端点。
- 遗留：macOS/Windows 实机验证；探测只验证 TCP 可达性，不验证业务协议可用性
  （协议层的可用性仍由真实收发与回执反映）。
- 状态：✅（运行时回退链 + 主动探测）/ ⏳（实机验证）

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

## 17. 清理现场（cleanup）与自更新残留
- 现状：✅ 已实现（2026-10-20，P1）
  - 客户端命令 `cleanup [all|persist|files|uac|wmi]`（cmdType 134），逐条回执
    ✅/⚠️/⏭️ 并给汇总计数；全部 best-effort，只有范围串非法才返回错误。
  - 覆盖：Windows 注册表 Run、计划任务、启动文件夹；Linux crontab 与 shell rc；
    macOS launchd plist（LaunchAgents/LaunchDaemons）；`files` 清 `<exe>.old` 与
    `%TEMP%\~*_update_*.exe|.dll`；`uac`/`wmi` 复用已有的
    `uacCleanupReport`/`wmiPersistRemove`。
  - 自更新残留：新映像启动后 `go commands.PurgeUpdateResidue()` 后台重试删除
    （40×500ms），不需要操作员记得发 cleanup。
- 判据与代价：**按"对象指向本程序路径"识别，不按名字**（名字是编译期 masquerade，
  运行期无法反推）。保守取向是宁可漏删不误删：`Author` 字段里的同路径不认、
  计划任务名在控制台转 GBK 时退化为"值以 `\` 开头且非运行命令行"、
  启动文件夹只在同名或同大小同扩展名时删。
- 遗留：
  - **未实机验证**：注册表/计划任务/cron/launchd 的真实卸载只在单测里验过判定逻辑，
    平台副作用（`reg delete`/`schtasks /delete`/`crontab -`/`launchctl unload`）
    没在真机上跑过；`reg query`/`schtasks` 的中文输出格式是按已知样例写的，
    系统语言/版本差异可能让匹配落空（落空只会少删，不会误删）。
  - 只管本程序留下的东西：不删事件日志、不删 Prefetch、不还原 AMSI/ETW patch
    （进程退出即恢复）。
- 状态：✅（实现 + 单测）/ ⏳（三平台实机验证）

## 18. 内存 OPSEC 的 P2（调用栈伪造 / 模块摘链）
- 现状：✅ 已实现（2026-10-20，P2），全部只在 `windows/amd64` 生效
  - **调用栈伪造**：`evasion.Syscall()` 在 direct 模式下把 `mov rsp,<伪栈>` 插到
    syscall 前，伪栈顶放 `BaseThreadInitThunk+0x14`、链尾 `RtlUserThreadStart+0x21`，
    第 5~8 个参数从真栈复制到伪栈 `+0x28`；醒来 `mov rsp,rbx` 复位。
    桩按 SSN 惰性生成（RW→RX），带缓存与并发去重。
  - **睡眠期模块摘链**：`SleepMask` 期间把本模块从 PEB 三条链表摘掉，醒来挂回；
    门控是 `config.HostEvasionEnabled()`（ETW patch ∥ AMSI bypass ∥ syscall 非 none），
    默认全关 ⇒ 默认不摘链。
  - **Win11 24H2 ntdll 解析修复**：函数体里 syscall 指令在偏移 0x12（老布局 8），
    24H2 上原实现会让**所有** Nt\* 解析失败、syscall 模式静默退化成 kernel32。
- 为什么不是 Ekko：Ekko 要加密自身 `.text/.rdata`，前提是"能停住所有线程"。Go 的
  常驻 sysmon/GC/信号线程会在加密窗口执行被加密的字节（必崩）；`SuspendThread`
  方案会因 mheap/mspan 锁或 GC safepoint 死锁；`runtime.stopTheWorld` 不对外暴露。
  **所以这不是"P2 没做完"，是这条路在 Go 上不成立**（详见 `docs/OPSEC.md` §3）。
- 遗留（如实列出，不当已完成）：
  - **只覆盖 `Syscall()` 一条路径**：`CreateProcessW`/`VirtualAllocEx`/`CreateRemoteThread`
    等直接调用仍长在 Go 线程栈上，回溯会看到 `runtime.asmcgocall` 一类帧。
  - **indirect 模式不套伪栈**：它的返回地址本就落在 ntdll 内，枢轴反而制造
    "返回地址不在模块内"的破绽。
  - **摘链只动 PEB 链表，不动 VAD**：`EnumProcessModules`/`Toolhelp` 看不到，
    但按内存区域扫（pe-sieve 一类）仍能看到文件映射的镜像页。
  - **没有 EDR 实机对抗**：以上全部只在开发机验证（含 Toolhelp 往返与真 syscall），
    没有在开了 CrowdStrike/Defender ATP 的主机上跑过；伪栈能不能骗过具体产品的
    回溯器，只有实测能回答。
  - 摘链依赖固定偏移（`PEB+0x18`、`LDR+0x10/0x20/0x30`、entry `+0x30` DllBase），
    这些在 Win10/11 x64 上稳定，但不是文档化契约，未来版本若变会**失败而非崩溃**
    （`findSelfEntry` 找不到就返回错误，不摘）。
- 状态：✅（实现 + 实机单测）/ ⏳（EDR 对抗验证）

---

## 19. 内存 OPSEC 的 P3（ETW/AMSI 覆盖面 / unhook / PPID / BlockDLLs）

**现状**：四项都已实现并带实机单测（2026-10-20），但都只有"机制正确"级别的证据。

- **已完成**
  - ETW patch 扩到 5 个导出（`EtwEventWrite`/`Ex`/`Full`/`String`/`Transfer`），
    统一返回 `STATUS_SUCCESS`（旧实现是裸 `ret`，返回值是垃圾）；patch 后 `VerifyPatch` 读回。
  - AMSI patch 扩到 `AmsiScanBuffer` + `AmsiScanString`，两者都返回 `E_INVALIDARG`。
  - unhook 改为解析 PE 节表（**仅 windows/amd64**，386/arm64 返回明确"不支持"）：按 `PointerToRawData` 找函数体，恢复 ntdll / kernel32 /
    kernelbase 的**全部可执行节**，且只写差异字节。修掉了两个真 bug（RVA 当文件偏移、
    只恢复 16 字节）。
  - PPID 伪造：`PROC_THREAD_ATTRIBUTE_PARENT_PROCESS` 指同会话 explorer.exe。
  - BlockDLLs：`PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY` 只允许微软签名 DLL。
  - 覆盖路径：`Run`（shell）、`Execute`、`ExecuteFile`、`hollow`。
- **遗留**
  - **ETW 仍有绕过不了的路径**：`NtTraceEvent` 直调、ETW TI（威胁情报）provider
    走内核回调，用户态 patch 无效；`.NET` 的 `EventSource` 可能缓存函数指针，
    patch 时机晚于缓存就失效。
  - **影子 hook 清不掉**：unhook 对照的是磁盘文件字节。若 EDR 在 ntdll 里改成
    "跳去自己的 trampoline 再跳回"，内存与磁盘字节确实会不同（能恢复）；但若它把
    磁盘副本也改了、或在映射后才挂，这条路径就无效。内核态 hook 更无解。
  - **PPID 只改创建时刻的父字段**：句柄继承、审计日志（4688 的 CreatorProcessName
    来自真实调用者）、ETW provider 里的真实父进程都可能暴露。
  - **BlockDLLs 有兼容性代价**：只约束 DLL 加载签名，不清理已加载模块；对输入法、
    Shell 扩展、部分游戏反作弊有破坏性，因此 `conpty` / `hvnc` / `privesc` 三条
    交互式路径**刻意不接**（`TestSpawnPathsUseChildAttrs` 里注明了原因）。
  - **没有 EDR 实机对抗**：全部只在开发机（Win11 24H2）验证。
- 状态：✅（实现 + 实机单测）/ ⏳（EDR 对抗验证）

## 20. 对抗能力开关（P4，生成期 / 运行期两层）

**现状**：9 项能力可以逐项开关，生成期 9 项（编译期占位符）、运行期 7 项（`evasion`
命令）；前端有生成期面板与会话内面板（2026-10-20）。

- **已完成**
  - 生成期占位符：`ETW_PATCH` / `AMSI` / `SYSCALL_MODE` / `AUTO_DELETE`（原有）+
    `SLEEP_MASK_HIDE` / `STACK_SPOOF` / `PPID_SPOOF` / `BLOCK_DLLS` /
    `UNHOOK_ON_START`（新增），缺省全关。
  - 运行期：`EVASION=135`，`evasion [status|all on|all off|on <key>|off <key>|reset]`，
    覆盖存在 beacon 进程内存里，键不存在 = 用编译期值（三态，能区分"运行期关掉"与
    "本来就没开"）。
  - 团队级默认：服务端 `config.opsec.*` 11 个字段，回落链「请求 → 配置 → 内置默认」。
  - 前端：生成期「对抗能力」面板（9 项 + 3 套预设 + 置灰原因）、会话内
    「Evasion Switches」面板；`opsecPresets.test.ts` 13 个单测 + techdebt 棘轮
    （禁止 `isZh ?` 与 `as any`）。
- **遗留**
  - **运行期覆盖不持久**：重连/重启回到生成期值。刻意如此（见 OPSEC §6），但意味着
    "这台机器现在处于什么状态"只能现场 `evasion status` 问，服务端侧没有镜像。
  - **编译期关就是硬关**：运行期 `on` 不生效，只回执原因。想开必须重新生成载荷 ——
    否则载荷里得常驻全部代码路径，隐蔽性和体积都白给。
  - **`syscallMode` 不提供运行期切换**：trampoline 按模式生成、SSN 建表时固化，
    切换要全量重建且失败即静默退化成 userland hook（OPSEC §3 有完整论证）。
  - **启动期能力关不掉**：`etw`/`amsi`/`unhook` 的运行期 `off` 只阻止后续动作，
    不撤销已写入的补丁；撤销需要保留原始字节并处理并发调用。
  - **旧模板会 fail-closed**：P4 之前构建的模板里没有 5 个新占位符，生成的载荷启动时
    `placeholderChecks()` 命中残留并拒绝运行（报错要求重建模板）。全量重建 178 个
    模板之前，只有当前重建过的那几个传输能用。
  - **`autoDelete` 两侧默认值不同**：服务端内置默认 `1`、客户端未 patch 时读 `false`；
    现在四个生成入口都显式发送以消除歧义，但"绕过前端直接调 GenServer 且不带该字段"
    仍会拿到服务端默认 `1`。
  - **没有实机回归**：三平台持久化卸载、真实 EDR 下的开关效果均未验证。
- 状态：✅（实现 + 单测）/ ⏳（全量模板重建、实机回归）

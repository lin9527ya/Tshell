# OPSEC 现状与威胁模型

> 本文是项目 OPSEC 能力的**唯一权威声明**。凡是本文没写的保护，都应当被当作"没有"。
>
> 目的很具体：这个项目历史上出现过两次"看起来做了防护，其实没做"的事故
> （见 §5）。那类问题比不做更危险 —— 它会让运维在错误的假设下行动。
> 所以本文的每条断言都要求能指到代码与测试，做不到的就明确写"未做"。

---

## 1. 威胁模型

**防护对象**

| 场景 | 后果 | 本文对应项 |
|---|---|---|
| 载荷样本被逆向 | 攻击者拿到二进制，试图提取密钥、通信协议、持久化位置 | §2 A 层 |
| 服务端磁盘/备份被非 root 读取 | 拿到 `database.db` 或备份即拿到全部战果 | §2 C 层 |
| 运维日志被采集/归档 | journald、容器日志驱动、终端回滚长期留存敏感信息 | §2 C 层 |
| 流量被被动分析 | 通过请求节奏、路径、TLS 指纹识别 C2 | §2 B 层 |
| 主机侧被 EDR/分析人员检查 | 内存里的密钥、可执行内存页、可疑调用栈 | §2 A 层 |

**明确**不**防护的场景**（不要基于本文高估能力）

- 内存取证（对运行中进程做完整内存转储）：Go 运行时的字符串与切片随时可能被拷贝，
  本项目的 sleep mask 只覆盖**密钥材料**，不覆盖内存镜像。
- root/SYSTEM 级攻击者：能读进程内存、能改服务端文件的对手，本文所列措施基本无效。
- 目标环境的网络流量深度检测（做协议语义识别而非流量特征识别的设备）。
- 供应链攻击（构建机被入侵）。

**为什么要把"不防护"写这么细**：C2 框架最容易出的事故是运维高估工具能力，
在没做防护的环节上按"已经安全"去操作。

---

## 2. 能力矩阵

图例：**已实现·有测试** / **已实现·未实机验证** / **未做**

### A. 植入体运行时（目标主机侧）

| 能力 | 状态 | 位置 |
|---|---|---|
| sleep mask：睡眠期**就地**遮蔽会话密钥/上一把密钥/主密钥与盐，清空 AEAD 缓存，擦除 pad | 已实现·有测试（含并发与竞态用例） | `implant/core/app/run.go`、`core/encrypt`、`docs/MEMORY_OPSEC.md` |
| 按片遮蔽 + 在途判定（活跃会话不被打断，且不会"永远判忙"） | 已实现·有测试 | 同上 |
| W^X：trampoline 由 RWX 改为 RW→RX | 已实现·有测试（机器码逐字节校验） | `core/evasion` |
| 注入链：分配/写入/改保护走 `NtAllocateVirtualMemory`/`NtWriteVirtualMemory`/`NtProtectVirtualMemory`/`NtOpenProcess`，逐项失败回退宿主 API（syscall 模式为 direct/indirect 时生效） | 已实现·有测试（假后端断言路径与顺序；真实 kernel32 往返） | `core/commands/remotemem_windows.go`、`inject_windows.go` |
| 进程镂空 W^X：镜像先 RW、写入后把**可执行节**收紧为 RX（非可执行节保持 RW，loader 要写 IAT/全局） | 已实现·有测试（**未实机验证**） | `hollow_windows.go` `hardenImage` |
| 远程线程创建首选 `NtCreateThreadEx`（`CreateRemoteThread` 仅显式 `crt` 或整链回退） | 已实现 | `inject_windows.go` |
| APC 分发走 `NtQueueApcThread` | **未做**（仍是 kernel32 `QueueUserAPC`） | — |
| `jmp rel32` → `mov rax,imm64; jmp rax`（避免 ±2GB 射程溢出） | 已实现·有测试 | 同上 |
| 产品名不硬编码（编译期 MASQUERADE_NAME 注入） | 已实现 | `core/masquerade.go` |
| 构建不泄露路径/符号：`-s -w -trimpath -buildvcs=false` | 已实现（见 `build_templates.ps1`） | 构建脚本 |
| Ekko/Foliage 式**内存镜像**加密（含 .text） | **未做·已归档不可做**（Go runtime 常驻线程会执行被加密的代码段，见 §3） | — |
| 调用栈伪造（伪栈：`BaseThreadInitThunk+0x14` → `RtlUserThreadStart+0x21`，栈传参复制到伪栈） | 已实现·有测试（**仅 direct syscall 路径**，见 §3） | `core/evasion/spoof.go`、`spoof_windows.go` |
| 睡眠期 PEB 模块摘链（三条 `In*Order` 链表，醒来原/尾挂回） | 已实现·有测试（实机 Toolhelp 快照"摘链后看不到、挂回后看得到"） | `core/evasion/modhide_windows.go` |
| ntdll 函数体解析兼容 Win11 24H2（`test [0x7ffe0308],1` + `jne`，syscall 在偏移 0x12 而非 8） | 已实现·有测试（实机逐个 Nt\* 断言 + 真进内核） | `core/evasion/trampoline.go` `ParseSyscallStub` |
| 文件自删（删当前映像/改名解锁/延迟重试） | 已实现 | `commands_windows.go` `DeleteSelf` |
| 启动期能力**可撤销**（patch 台账 + `evasion off` 真正还原） | **已实现·有测试**（2026-10-20）：写入前存原始字节、逆序还原、被外部改过则跳过；台账仅在进程内 | `core/evasion/patch_registry_windows.go` |
| ETW patch 覆盖面扩到 5 个导出（`EtwEventWrite`/`Ex`/`Full`/`String`/`Transfer`），返回 `STATUS_SUCCESS` 而非裸 `ret` | 已实现·有测试（实机 5/5，patch 后返回值断言） | `core/evasion/evasion.go` `etwPatchTargets` |
| AMSI patch 覆盖 `AmsiScanBuffer` + `AmsiScanString` | 已实现·有测试（实机 2/2） | 同上 `amsiPatchTargets` |
| ntdll/kernel32/kernelbase（仅 amd64）**全可执行节**对照磁盘映像恢复（走 PE 节表换算，不再用 RVA 直索引文件；只写差异字节） | 已实现·有测试（种假 hook 后抹掉、幂等、本机无害） | `core/evasion/pe_image.go`、`unhook_windows.go` |
| 父进程伪造（PPID spoof）：`PROC_THREAD_ATTRIBUTE_PARENT_PROCESS` 指向同会话 explorer.exe | 已实现·有测试（实机读回子进程 EPROCESS 的父 PID = 伪造值） | `core/commands/procattr_windows.go` |
| BlockDLLs：`PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY` 只允许微软签名 DLL 进入子进程 | 已实现·有测试（实机 `GetProcessMitigationPolicy` 读回 `MicrosoftSignedOnly`） | 同上 |
| 清理现场 `cleanup [范围]`：卸载持久化（registry/schtasks/startup、crontab/shell rc、launchd）+ 清自更新 `.old`/`%TEMP%` 残留 + UAC 劫持键 + WMI 订阅 | 已实现·有测试（**未实机验证**） | `core/commands/cleanup.go`、`cleanup_{windows,linux,darwin}.go` |
| 自更新残留自清理（新映像启动后在后台重试删 `<exe>.old`） | 已实现·有测试（接线棘轮） | `core/app/init.go`、`commands.PurgeUpdateResidue` |
| 对抗能力**逐项生成期开关**：5 个新占位符（`SLEEP_MASK_HIDE`/`STACK_SPOOF`/`PPID_SPOOF`/`BLOCK_DLLS`/`UNHOOK_ON_START`）+ 原有 4 个（`ETW_PATCH`/`AMSI`/`SYSCALL_MODE`/`AUTO_DELETE`），缺省全关 | 已实现·有测试（服务端归一化 + 平台不可能组合直接 400） | `pkg/api/opsec_params.go`、`pkg/delivery/payload.go` |
| 对抗能力**运行期开关**：`evasion [status\|all on\|all off\|on <key>\|off <key>\|reset]`，覆盖只存内存 | 已实现·有测试（7 项能力状态机 + 别名 + 拒绝不可能组合） | `core/commands/evasion_cmd_windows.go`、`core/config/opsec.go` |
| 生成期关 = **硬关闭**：编译期占位符为 0 时运行期 `on` 不生效并回执原因（不静默假装打开） | 已实现·有测试 | `core/config/opsec.go` `EvasionImpossibleReason` |
| 团队级默认值：服务端配置 `config.opsec.*`（11 个字段），回落链「请求 → 配置 → 内置默认」 | 已实现·有测试（白名单校验、非法值报字段名） | `pkg/config/manager.go`、`handlers.go` `applyOpsecUpdates` |
| 前端：生成期「对抗能力」面板（9 项 + 3 套预设 + 平台/架构置灰原因）+ 会话内「Evasion Switches」面板 | 已实现·有测试（13 个前端单测 + techdebt 棘轮） | `Tshell-web/src/components/Opsec/` |

### B. 通信（C2 信道）

| 能力 | 状态 | 位置 |
|---|---|---|
| UA / URI 伪装（默认保留腾讯系接口路径） | 已实现 | `pkg/c2profile/default.go` |
| 请求头 padding（破除请求头长度指纹） | 已实现 | `implant/core/transport_http.go` `applyRequestPadding` |
| Host 头覆盖（域前置） | 已实现 | `transport_http.go` `req.Host = httpFrontHost` |
| TLS JA3 伪装（对齐主流浏览器 ClientHello） | 已实现 | `pkg/c2profile` `tls.ja3` |
| DNS beacon 子域前缀可定制 | 已实现 | `pkg/c2profile` `dns.beacon_prefix` |
| 心跳默认值与全局配置一致（5s，防两处漂移） | 已实现·有测试 | `pkg/c2profile/default_sleep_test.go` |
| 主密钥**运行时**派生（每会话 ECDH） | **12/12 已接线并重建载荷**（2026-10-20）：X25519+HKDF、`ReplaceMasterKey` 接缝、握手缓存、会话前交换均已实现并有测试；206 个产物逐个验证含 ECDH | `core/encrypt/ecdh.go`、`pkg/encrypt/ecdh.go` |
| 流量体积整形（定长分桶） | **已实现·有测试**（2026-10-20）：HTTP 家族请求/响应已定长分桶；**非 HTTP 传输的载荷长度仍未整形** | `core/communication/padding.go`、`api/communication/get.go` |

### C. 服务端数据面（磁盘/日志）

| 能力 | 状态 | 位置 |
|---|---|---|
| 战果（口令/NTLM/票据密钥）**静态加密** AES-256-GCM | 已实现·有测试（含"库文件里搜不到明文"的验收） | `pkg/security/atrest.go`、`pkg/database/loot_crypto.go` |
| 去重不因加密失效（GCM 随机化 → 改用 HMAC 指纹列） | 已实现·有测试 | `Loot.SecretFp` |
| 战果增/查/删**全量审计**，且审计不含密钥本体 | 已实现·有测试 | `pkg/api/loot.go` |
| 战果保留期清理（默认关闭，显式开启后每日执行） | 已实现·有测试 | `pkg/api/loot_retention.go` |
| MFA/TOTP 密钥静态加密 | 已实现（先于本轮） | `pkg/security/mfa.go` |
| 数据目录 0700 / 会话录像 0600 / DB 文件 0600 | 已实现·有测试（启动时收敛历史目录） | `pkg/security/fsperm.go` |
| 初始管理员密码不落日志（写 0600 文件） | 已实现·有测试（源码级守卫） | `pkg/database/initial_password.go` |
| 备份/审计文件 0600 | 已实现（先于本轮） | `pkg/backup`、`pkg/audit` |
| 审计日志防篡改（哈希链 + `-verify-audit` 校验器） | **已实现·有测试**（2026-10-20）：改动/删除/重排都能定位到行号；边界：截断末尾需外部锚点，清理旧日志后只覆盖保留窗口 | `pkg/audit/chain.go`、`tshell -verify-audit` |
| PostgreSQL 后端同样受静态加密保护 | 已实现（加密在应用层，与后端无关） | — |

---

## 3. 关键设计取舍（为什么不是别种做法）

**战果加密密钥从哪来**
优先 `security.at_rest_key_file`（32 字节原始密钥或等长 hex/base64），
未配置时由 JWT 私钥 HKDF 派生（info 与 MFA 用的不同，两把密钥互不相关）。

代价必须写明：**JWT 私钥泄露 ⇒ 战果也可解**。这不是新增弱点（私钥本来就等于
"能伪造任意管理员令牌"），但不要以为战果有独立于私钥的保护。要独立就把
`at_rest_key_file` 配到与数据库不同的介质上。

**未配置加密时是硬失败，不是明文降级**
`Seal` 返回 `ErrAtRestNotInitialized`，写入直接失败。本项目反复踩过"静默降级"
的坑，这里宁可让录入失败并在日志里喊出来，也不要让口令悄悄以明文落库。

**去重指纹用 HMAC 而非裸哈希**
裸 SHA256 会让持有数据库的人对常见口令做字典反查，等于把加密白做了。
用派生密钥做 HMAC 后没有密钥就无法离线比对。
代价：指纹仍是确定性的，能看出"两条记录的口令相同" —— 这是去重功能本身的必然代价。

**删除战果默认不自动发生**
`loot_retention_days` 默认 0（永久）。删除不可逆，不该由一次升级替运维做决定。

**镂空的 W^X 只收紧"可执行节"，不是整镜像**
注入走 RW→RX 是干净的：写完全部内容，整块改 RX。镂空不能照抄，因为写它的是**别人**：
loader 在 `ResumeThread` 之后才解析导入表，它要往 IAT（通常 `.rdata`）和 Go runtime 的
`.data` 全局里写。镂空用的是私有内存（`VirtualAllocEx`），不是 `SEC_IMAGE` 映射，
没有写时复制兜底 —— 把非可执行页设成只读，loader 一写就访问违例，症状是
"注入报成功、beacon 启动即 0xC0000005 退出"。所以这里的取舍是：
**消除 RWX（没有任何可执行页同时可写），但保留非可执行页的可写性**。

唯一的例外也要说清楚：拿不到 preferred base 且镜像带 `.reloc` 时，重定位写发生在
`ResumeThread` 之后、beacon 入口之前，不经过我们的代码，可执行节只能暂时保持可写。
这条路径在回包诊断里显式打出 `prot=[...RWX(重定位期)]`，不是静默降级。

**清理按"对象指向本程序"识别，不按"名字等于某值"识别**
持久化项的名字来自编译期 patch 的 `masquerade`（每份载荷不同），运行期无法从名字反推
"哪个是我的"。反过来按名字删也不行：会撞上同名无关项。所以 `cleanup` 的判据是
**该项的载荷/命令/plist 内容指向当前可执行文件路径**：Run 键按 `REG_SZ` 数据比对、
计划任务按 `Task To Run` 路径比对、启动文件夹按同名或同大小同扩展名、cron/bashrc 按行内
路径、launchd 按 plist 里的程序路径。

匹配偏保守：宁可漏删也不误删别人的持久化项。具体是
① `Author` 字段里出现本程序路径不认（否则会去删别人的任务）；
② 计划任务名的解析先认 `TaskName:`/`任务名:` 标签，控制台代码页转成 GBK 时退化为
"行值以 `\` 开头且不是运行命令行"——任务名本身按原始字节回传给 `schtasks /delete`；
③ 启动文件夹只在"同名"或"同大小 + 同扩展名"时删，不做模糊匹配。

清理全程 best-effort：单条失败记 ⚠️ 并继续，只有范围串非法才整体报错。回执要能让操作员
一眼看出"哪一条没清掉"，所以逐条 ✅/⚠️/⏭️，末行给汇总计数。

**`.old` 残留由新映像自己清**
`self_update` 是 `rename(self → self+".old")` 后拷新文件、起新进程、旧进程退出。旧文件名
被占用时删除会失败，所以不做成"旧进程退出前删"（它已经调过 `os.Exit`），而是在新进程
启动后 `go commands.PurgeUpdateResidue()` 带重试（40×500ms）删除，并且把 `%TEMP%` 下
`~*_update_*.exe|.dll` 一起扫掉。这样"更新过一次但没发过 cleanup"的机器也不会留 `.old`。

**Ekko 式镜像加密为什么做不了（不是"没时间做"）**
Ekko/Foliage 在睡眠期用 RC4 加密进程自己的 `.text/.rdata`，醒来还原 —— 前提是
"我能把所有线程停住"。Go 做不到：

① Go runtime 常驻后台线程（`sysmon`、GC worker、信号处理），加密 `.text` 期间它们
随时可能执行被加密的字节 → 必崩；
② 改用 `SuspendThread` 逐个挂起再加密会死锁：被挂起的线程可能正持有 `mheap/mspan`
锁或停在 GC safepoint，主线程之后任何一次分配都会卡住；
③ `runtime.stopTheWorld` 不对外暴露，也不允许在非 runtime 代码里安全使用。

所以 P2 只做**能安全做的那半**：镜像不加密，改为睡眠窗口内把本模块从 PEB 加载器
链表里摘掉（下面一条）。这条边界要如实告知：**这不是 Ekko，挡不住内存区域扫描。**

**调用栈伪造只覆盖 `evasion.Syscall()` 这一条路径**
伪造的做法是：把 `mov rsp, <伪栈>` 插在 syscall 之前，伪栈上预置
`BaseThreadInitThunk+0x14` 作为"调用方返回地址"、`RtlUserThreadStart+0x21` 作为
链尾，第 5~8 个参数从真栈复制到伪栈的 `+0x28` 处（x64 前 4 个参数在寄存器里，
栈回溯器只读栈）。醒来 `mov rsp, rbx` 复位，不需要栈平衡技巧。

只对 `Syscall()` 生效，因为只有它由我们自己生成机器码。**kernel32/ntdll 直接调用
（`CreateProcessW`、`VirtualAllocEx` 一类）的栈仍然长在 Go 线程栈上**，回溯会看到
`runtime.asmcgocall` 之类的 Go 帧。要彻底解决得换 `stdcall` 汇编桩 + 自定义线程，
那是另一个量级的改动。

indirect 模式也刻意不套伪栈：它的返回地址落在 ntdll 内（这是它存在的意义），
`mov rsp` 枢轴会让那个返回地址跑到伪栈上，"返回地址不在模块内"反而更可疑。

**模块摘链的边界：只动 PEB 链表，不动 VAD**
摘链影响的是 `InLoadOrder`/`InMemoryOrder`/`InInitializationOrder` 三条链表，也就是
`EnumProcessModules`、`CreateToolhelp32Snapshot(TH32CS_SNAPMODULE)`、
`GetModuleFileName(NULL)` 这一类走加载器的取证路径。**按内存区域扫（pe-sieve 一类）
仍然看得到文件映射的镜像页** —— 页表是内核对象，链表的可见性改不了它。

所以摘链只允许发生在睡眠窗口内（醒来第一件事挂回），不能在命令执行路径上长时间挂着：
`os.Executable()` 一类调用会受加载器状态影响，挂着跑业务是自找麻烦。

挂回有校验：睡眠可能几分钟，期间任何一次 DLL 懒加载都会改动我们记住的邻居指针。
硬写回去会把链表接成环或断链（症状是"进程下次 LoadLibrary 就崩，现场极难定位"），
所以挂回前先比对 `prev->Flink == next && next->Blink == prev`，对不上就退化为挂到
表尾，绝不硬写失效指针。

**门控：摘链跟随"主机层对抗"开关组，不单设开关**
`config.HostEvasionEnabled()` = ETW patch ∥ AMSI bypass ∥ syscall 模式非 none。
理由是这些开关共同表达"操作员愿意承担对抗措施的代价"，而摘链的代价（牺牲加载器
一致性）正属于这一类；三项全关时它是空操作，与"未 patch 的载荷不做任何对抗"一致。

**Win11 24H2 的 ntdll 函数体变长了一截（P2 实机才发现）**
24H2 起导出函数体是 `mov r10,rcx | mov eax,ssn | test byte [0x7ffe0308],1 | jne +3 |
syscall | ret | int 2eh | ret`，syscall 指令在偏移 **0x12**，不是老布局的 8。
原来写死 `body[8]==0x0F && body[9]==0x05`，结果在 24H2 上**所有** Nt\* 解析失败，
`PrepareTrampolines` 返回错误、调用方静默回退 kernel32 —— 也就是 P0 那条"注入链
syscall 化"在现代 Windows 上根本没生效，而且没有任何运行期症状。
现在改为校验前缀后**扫描**第一个 `0F 05` 并返回其真实地址，实机对名单里每个 Nt\*
都有断言。

**unhook 必须走 PE 节表换算，不能拿 RVA 当文件偏移**
磁盘上的 ntdll 里 `.text` 的 `VirtualAddress=0x1000`、`PointerToRawData=0x400`，
差 0xC00。旧实现用 `diskData[rva:rva+16]` 直接索引，等于把"别处"的字节写进代码段 ——
结果是**主动破坏 ntdll**，症状是随机时刻 Access Violation，比不 unhook 更糟。
现在改成：解析 PE → 按节 `PointerToRawData` 找到函数体 → 每个可执行节与内存逐字节比对
→ 只回写真正不同的字节。另外恢复必须用 `PAGE_EXECUTE_READWRITE` 而不是 `PAGE_READWRITE`：
`VirtualProtect` 自己就在 ntdll 里执行，返回时那条 `ret` 若落在刚被去掉执行位的页上，
进程当场静默死亡（实测 `0xC0000005`，连 Go 的 panic 都来不及打）。瞬时 RWX 只有
几微秒，代价可以接受；这一点在 `writeBack` 的注释里也写了，避免后人"顺手改成 RW"。

**PPID 伪造在 x64 上吃的是 HANDLE，不是 PID（24H2 实测）**
同一个属性 `PROC_THREAD_ATTRIBUTE_PARENT_PROCESS`，传 4 字节 PID 报
`ERROR_BAD_LENGTH`，传 8 字节 PID 报 `ERROR_INVALID_HANDLE`，传 `OpenProcess(
PROCESS_CREATE_PROCESS)` 拿到的句柄才对 —— 也就是内核把它当句柄用。所以实现里先开
句柄、把句柄活到 `CreateProcess` 之后（`childAttrs.parentHandle`，`Close()` 里关），
失败才回退到 4 字节 PID 以兼容老系统。验证不能看返回值：CreateProcess 对不支持的属性
会静默忽略，必须读子进程 EPROCESS 的 `InheritedFromUniqueProcessId`。

**BlockDLLs 的常量位写错过一次，而且不报错**
`PROCESS_CREATION_MITIGATION_POLICY_BLOCK_NON_MICROSOFT_BINARIES_ALWAYS_ON` 是
高 32 位的第 12 位（`0x0000100000000000`）。最初按"高 32 位第 0 位"写成
`0x0000000100000000`（那是禁止动态代码的位置），`Update` 照常成功、子进程照常创建，
但 `GetProcessMitigationPolicy(ProcessSignaturePolicy)` 读回是 0 —— 完全是"假生效"。
所以这条能力的判据固定在读回的 `MicrosoftSignedOnly` 位上。

**ETW 只 patch 一个入口等于没 patch**
`EtwEventWrite` 只是最常用的那个；`EtwEventWriteEx`（.NET/部分 EDR 走这条）、
`EtwEventWriteFull`、`EtwEventWriteString`、`EtwEventWriteTransfer` 任一条留着，
事件照样落地。旧实现还写过裸 `ret`（`C3`）—— 返回时 EAX 是残留值，调用方可能把
"成功"判成失败。现在统一写 `mov eax, STATUS_SUCCESS; ret`，并且 patch 后读回校验，
防止"VirtualProtect 成功但写错地址"。

**对抗能力为什么要有开关，以及哪一半能运行期切**
P0–P3 把能力做完后，它们是"编译期写死 + 门控跟着 `HostEvasionEnabled()`
（ETW∥AMSI∥syscall≠none）走"，结果是**勾一个带出一串**：想开 ETW patch，顺带就
被摘链；想关 BlockDLLs 保住第三方输入法，只能重新生成载荷。P4 拆成两层：

- **生成期**（编译期占位符，`1`/`0`）：`etw`、`amsi`、`syscallMode`、`autoDelete`、
  `sleepMaskHide`、`stackSpoof`、`ppidSpoof`、`blockDLLs`、`unhookOnStart`。
  这是**能力是否存在于这个载荷里**的开关 —— 关掉就真的没有那段代码路径。
- **运行期**（`evasion` 命令，覆盖只在内存）：同样 9 项里除 `syscallMode`/`autoDelete`
  外的 7 项（`etw`/`amsi`/`modhide`/`spoof`/`ppid`/`blockdlls`/`unhook`）。

**编译期关 = 硬关闭**，运行期 `on` 不生效并明确回执原因。这条是刻意的：如果运行期能
把编译期关掉的能力"打开"，载荷里就必须保留全部代码路径（体积暴露 + 静态特征全在），
"关掉"就只是自欺欺人。反过来，编译期开、运行期关是允许的 —— 那才是操作员真正需要的
"这一票别用"。

**为什么 `syscallMode` 不能运行期切**
trampoline 是**按模式生成**的（direct 直接放 `syscall; ret`，indirect 放
`jmp [ntdll stub]`），SSN 在查询表建立时就固化了。切换模式要重解析 SSN、重建全部桩、
并让已缓存的调用点失效；失败时整条注入链会静默退回 userland hook —— 一个"点一下按钮，
隐蔽性掉一档"的按钮不该存在。生成期固定，前端也把它标成不提供运行期切换。

**启动期能力与运行期能力的区别**
`etw`/`amsi`/`unhook` 是对**已加载模块**打补丁，只在启动期做一次：运行期 `on` =
立即执行一次（有用），运行期 `off` = 不会撤销已写入的补丁（只在回执里说明）。
`unhook` 必须在 ETW/AMSI patch **之前**跑，否则磁盘原始字节会把刚打的补丁盖回去，
得到"报告成功、实际失效"的载荷 —— `evasion.Init()` 的顺序就是为此固定下来的。

**`autoDelete` 的默认值差异**
服务端内置默认 `1`（保持历史行为：载荷跑完自删），客户端占位符未 patch 时读作 `false`。
两边不一致时以"服务端显式发送"为准：四个生成入口（EXE 单包、EXE 分阶段、DLL、
监听器 DLL）现在都显式发送，做到所见即所得；前端把该项标成"默认开"。

---

## 4. 部署 checklist

首次部署与每次升级后建议逐条确认：

- [ ] `security.at_rest_key_file` 指到**与数据库不同介质**的 0600 文件（不配也能跑，但保护等级低一档）
- [ ] `database.path`、`config.yaml`、`jwt_private.pem`、`initial-admin-password.txt` 均为 0600
      （启动日志会告警权限过宽的文件；数据目录会被自动收敛到 0700）
- [ ] 首次登录后**删除** `initial-admin-password.txt`，并修改管理员口令
- [ ] `auth.default_password` 改成强口令（启动日志对弱口令告警）
- [ ] 按合规要求决定 `security.loot_retention_days`（0 = 永久，启动日志会显式说明）
- [ ] MFA 已为管理员开启
- [ ] 反向代理终止 TLS 时，确认整链路 HTTPS 后再开 `security.force_hsts`
- [ ] 生产载荷用真实证书签名（见 `docs/CODE_SIGNING.md`；**本仓库未用真实证书验证过**）
- [ ] 备份目录为 0700，且备份文件本身**包含加密后的战果**（加密在写入前完成，无需额外处理）

---

## 5. 反面教训（评审时当检查项用）

这几起事故都造成过"以为做了，其实没做"，值得每次改动时对照：

**⓪ 只测了单元测试就上线（2026-10-20，最严重的一次）**

ECDH 从第一行代码到重建全部载荷，**没有跑过一次端到端**，只有单元测试与
交叉编译全绿。上线后立刻暴露：客户端能上线，但命令控制 / 文件管理 / 进程 /
截图全部失效。两个 bug 都属于"单测必然绿、真实链路必挂"：

- **X-Kx 头只发了一次**：服务端每个 HTTP 请求独立解析密钥，但客户端只在
  首包带了头，`Recv()` / `SendResult()` 都没带 → 服务端回落静态密钥、
  却要解用派生密钥加密的 MetaInfo → 拿不到 uid。首包带对了所以**上线成功**，
  故障因此极具迷惑性
- **握手缓存固定 TTL**：活跃会话的条目过期后，服务端会重建临时密钥对、
  派生出与客户端不同的主密钥 → "跑一段时间后全部失效，重启又好"，
  极易误判为网络问题。已改为命中即续期（滑动过期）

教训：**加密链路的验证必须在真实链路上做一次**，"包能编过 + 单测绿"跟
"握手能对上"没有因果关系。此后凡改密钥/派生/握手，先跑一次端到端再谈完成。

**① sleep mask 保护的是副本**

```go
key, _ := encrypt.GlobalClientKeyManager.GetSessionKey() // 返回的是**拷贝**
sleepmask.SleepWithMask(d, key)                          // 对拷贝就地 XOR
```

被 XOR 的是一个谁也看不到的局部变量，真正的密钥整个睡眠期间原样躺着，
AEAD 缓存里还另有一份。现在改为 `MaskForSleep` **就地遮蔽真身**并清空缓存。

教训：**"调用了某个防护函数"不等于"受保护对象被改了"**。要断言被保护对象的字节确实变了。

**② kirbi 的 enc-part 被误认为需要密钥**

客户端与服务端注释都写着"enc-part 需要密钥才能解，所以服务端无法解析"。
实际上标准 kirbi 的 enc-part `etype` 为 0、cipher 就是明文 DER
（`decoder.decode(cipher, EncKrbCredPart())`，不解密）。
于是这个被判定为"必须实机才能做"的功能，其实**完全可以离线实现并用真实夹具验证**。

教训：**基于传闻的技术判断要先用夹具/文档证伪**，否则会把能做的事长期挂着。

> 这两条共同点：错误都写在注释里，而且看起来很有道理。
> 所以本文要求每条能力都指到**代码 + 测试**，而不是"我记得做了"。

---

## 6. 未做项与阻塞

| 项 | 为什么没做 | 接续条件 |
|---|---|---|
| 内存镜像加密（Ekko 级） | **已归档为不可做**（2026-10-20）：Go runtime 常驻 sysmon/GC/信号线程会执行被加密的代码段，`SuspendThread` 方案会因 mheap/mspan 锁与 GC safepoint 死锁，`stopTheWorld` 不对外暴露 | 除非改 runtime 或改用 C 植入体，否则不排期 |
| 调用栈伪造覆盖面（kernel32/ntdll 直调路径） | 已实现 direct syscall 路径的伪栈（2026-10-20）；`CreateProcessW`/`VirtualAllocEx` 这类直调仍长在 Go 线程栈上 | 需 `stdcall` 汇编桩 + 独立线程，属另一个量级 |
| 模块摘链的 VAD 层可见性 | 已实现 PEB 链表摘除（2026-10-20）；页表/VAD 不动，按内存区域扫仍可见镜像页 | 需要改映射方式（`SEC_IMAGE`+回填）或 unbacked 执行内存，代价高于收益 |
| APC 分发 syscall 化（`NtQueueApcThread`） | 未接：APC 路径还要同时改线程恢复（`NtResumeThread`）与句柄管理，改一半没有意义 | 有实机可验证注入成功/失败的时机 |
| 主密钥运行时派生（ECDH） | **已完成（2026-10-20）**：12/12 传输接线 + 206 个载荷产物全量重建；**残留风险**：静态主密钥仍作为占位符编译进载荷（生成期被 patch 覆盖），所以 `strings` 仍可提取到它 —— 见 §4 第 1 条 | 需要改为"载荷内不含密钥"的注入模型 |

### ECDH 之后的两个**未解决**安全性质（别把它当成"已经安全了"）

**A. 客户端是"静默降级"的 —— 主动攻击者可强制退回静态密钥**

12 条传输的 `kxHandshake()` 在任何失败路径（拿不到服务端公钥、公钥非法、网络中断）
都**直接 return**，随后照旧用静态主密钥生成 MetaInfo。设计意图是"老服务端也能连"，
代价是：能阻断 KX 应答的人（MITM、按域名拦截的设备）可以让客户端退回静态链路，
而静态密钥**就在载荷里**（`strings` 可提取）→ 那条链路可被解密。

服务端侧确实是"不降级"的（带了 KX 就必须合法，否则 400/断开），
但**决定权在客户端**，所以服务端的严格并不构成保护。

解除条件（未做）：加"必须 ECDH"的强制开关（生成期占位符），
强制模式下 KX 失败即**拒绝上线**而不是回落。这需要改 12 个传输的 Connect 流程。

**B. ECDH 未认证 —— 主动 MITM 本来就能拦**

交换只做 X25519 + HKDF，没有签名、没有证书固定（http_forward/https_forward 的
TLS 校验只覆盖"服务端证书是否可信"，不绑定载荷身份）。所以能主动介入链路的人
可以直接做中间人，而不必走 A 的降级路径。

解除条件（未做）：给服务端公钥加签名（预置公钥/证书），或把 KX 绑到已有的
TLS 通道上并固定指纹。两者都需要生成期注入密钥材料 —— 与"载荷里不含密钥"同一批工作。| 流量体积整形 | **已完成（2026-10-20）**：两侧统一 2 的幂定长桶、值改标准 base64。**未做**：非 HTTP 传输的载荷长度整形 | 需要按帧格式逐传输设计 |
| 审计日志防篡改（哈希链） | **已完成（2026-10-20）**：链在 flushMu 串行区内计算、重启/轮转自动接续、`-verify-audit` 定位断点。**未做**：链尾外部锚点（截断末尾仍不可发现） | 需要独立介质存锚点 |
| `cleanup` 统一清理现场命令 | 已实现（2026-10-20），**未在实机验证过持久化卸载**（需真实注册表/任务计划/cron/launchd 环境） | 授权 Windows/Linux/macOS VM 各一台做回归 |
| 自更新 `.old` 残留清理 | 已实现（新映像启动后后台重试删除） | 同上 |
| ETW/AMSI patch 覆盖面 | 已扩到 5 个 ETW 导出 + 2 个 AMSI 入口（2026-10-20）；仍未覆盖 `NtTraceEvent` 直调路径与 ETW TI（威胁情报）提供者 | 需逐个 provider 做实测，且 TI 走内核回调，用户态 patch 无效 |
| unhook 的边界 | 已实现三模块全可执行节对照磁盘恢复（2026-10-20）；**影子 hook**（在 ntdll 里改指令但保留磁盘副本）与内核态 hook 均无法用此法清除 | 需对比内存页与磁盘页的"执行流"而非字节，或改用 direct syscall 绕开 |
| PPID 伪造的作用范围 | 已实现（2026-10-20）；只影响创建时刻写进 `EPROCESS` 的父字段，不影响句柄继承、不影响同会话审计日志 | 无 |
| BlockDLLs 的副作用 | 已实现（2026-10-20）；只约束 DLL 加载签名，不阻止已加载的非微软模块；对 IME/Shell 扩展有兼容性风险，因此 `conpty`/`hvnc`/`privesc` 三条交互路径**刻意不接** | 无 |
| 运行期开关**不跨会话** | 覆盖只存在 beacon 进程内存里，重连/重启即回到生成期值。刻意不做持久化：否则"临时开一次"会变成永久的隐蔽性下降，且服务端无法知道某台机器实际处于什么状态 | 需要持久化时用生成期开关 |
| 运行期**不能切** `syscallMode` | trampoline 按模式生成、SSN 在表建立时固化，切换要全量重建且失败即静默退化成 userland hook（见 §3） | 无（设计决定） |
| 启动期能力"关不掉" | `etw`/`amsi`/`unhook` 的运行期 `off` 只阻止后续动作，**不撤销**已写入的补丁 —— 撤销需要保留原始字节并处理并发调用 | 排期（需逐导出备份原始字节） |
| 旧模板 + 新客户端 | 新客户端源码含 5 个新占位符字面量，P4 之前构建的模板里没有它们 → 生成出的载荷启动时 `placeholderChecks()` 命中残留并**拒绝运行**（fail-closed，报错文案要求重建模板） | 全量重建 178 个模板 |
| 代码签名真实证书验证 | 无 EV/OV 证书 | 提供证书 |
| `kerberos ptt` 运行时接受度 | 文件格式已验证（`pkg/cred/kirbi.go`），LSA 是否认可需域环境 | 提供 AD 环境 |

---

## 7. 验证方式

```bash
# 服务端
cd Tshell-server
go vet ./...
go test ./pkg/security/ ./pkg/database/ ./pkg/api/ ./pkg/c2profile/ -count=1

# 植入体（内存 OPSEC）
cd Tshell-client/implant
go test ./core/encrypt/ ./core/app/ ./core/evasion/ -race -count=1
# 注入/镂空的 syscall 接线与 W^X（Windows；本机 -race 链接会失败，见 ROADMAP §5）
go test ./core/commands/ -run 'TestStagePayload|TestReserveImage|TestWriteImage|TestHardenImage|TestRemoteMemory|TestHostBackend|TestInjectionPaths' -count=1
# 清理现场的判定逻辑（平台副作用不跑，只验"删哪个对象"）
go test ./core/commands/ -run 'TestParseCleanupSpec|TestCleanup|TestRegRunValueNames|TestTaskNamesForExe|TestStartupFileMatches|TestFilterLinesReferencing|TestPlistReferencesPath|TestRemoveWithRetry|TestFindUpdateResidues|TestRenderCleanupReport' -count=1
# 接线棘轮：dispatch case / 启动期残留清理 / 睡眠期摘链
go test ./core/app/ -run 'TestCleanup|TestSleepMaskHidesModule|TestHostEvasionGate' -count=1
# 调用栈伪造（伪栈字节布局 + 实机往返）与模块摘链（合成链表 + 真实 PEB 往返）
go test ./core/evasion/ -run 'TestBuildSpoof|TestSqlSpoofRatchet|TestSpoof|TestDetach|TestAttach|TestFindSelfEntry|TestHide' -count=1 -v
# ntdll 解析兼容性（24H2 布局 + 实机每个 Nt* 都要解析成功 + 一次真 syscall 进内核）
go test ./core/evasion/ -run 'TestParseSyscallStub|TestPrepareTrampolinesResolves|TestIndirectModeAlso|TestSyscallReachesKernel' -count=1 -v
# PE 节表换算 / ETW-AMSI 覆盖面 / unhook（会临时改本进程的 ntdll，仅测试进程内）
go test ./core/evasion/ -run 'TestParsePEImage|TestFileOffsetForRVA|TestSectionImage|TestPatchETW|TestEtwEventWriteReturnsSuccess|TestPatchAMSI|TestUnhook' -count=1 -v
# PPID 伪造 / BlockDLLs：读回内核记录与签名策略位，而不是看 CreateProcess 的返回值
go test ./core/commands/ -run 'TestPickSpoofParent|TestPPIDSpoof|TestBlockDLLs|TestCreateProcessWithoutAttrs|TestSpawnPaths' -count=1 -v
# 对抗能力开关：能力表/覆盖三态/不可能组合的原因文案 + EVASION 命令与接线棘轮
go test ./core/config/ ./core/commands/ -run 'TestEvasion' -count=1 -v
# 服务端：占位符归一化/校验、配置回落、命令表
cd ../../Tshell-server && go test ./pkg/api/ ./pkg/delivery/ ./pkg/config/ ./pkg/sendcommand/ -run 'TestResolveOpsec|TestNormalizeOnOff|TestNormalizeEnums|TestPatchOPSEC|TestApplyOpsec|TestApplyConfigUpdatesRoutesOpsec|TestResolveCommandEquivalence' -count=1
```

**关键断言不是"测试通过"，而是外部证据**：

| 断言 | 怎么验 |
|---|---|
| 库文件里没有明文口令 | `strings database.db \| grep <测试口令>`（对应 `TestDatabaseFileContainsNoPlaintextSecret`） |
| 载荷里没有固定主密钥 | `strings <payload> \| grep LISTENER_MASTER_KEY` —— **仍会命中**。ECDH 只保证"会话密钥由每次握手派生"，载荷里那把静态密钥是生成期 patch 进去的占位符，仍然存在。**该验收项未满足**，原因与解除条件见 §6 |
| 日志里没有口令 | 捕获 stdout 断言（对应 `TestNoPasswordInLogStatements`） |
| 目录权限已收敛 | `stat -c %a Downloads sessions plugins`（对应 `pkg/security/fsperm_test.go`，POSIX 用例在 CI 的 ubuntu job 执行） |
| 注入不申请 RWX、镂空的可执行节最终是 RX | `TestStagePayloadAllocWriteThenExecute` / `TestHardenImageOnlyTightensExecutableSections`；实机侧看注入回包的 `后端=` 与镂空回包的 `prot=[...]` |
| syscall 模式没有静默退回宿主 API | `TestInjectionPathsDoNotBypassRemoteMemory`（源码级棘轮）+ 回包里的 `调用序列=` |
| 清理不会误删别人的持久化项 | `TestTaskNamesForExeIgnoresPathInAuthor` / `TestRegRunValueNames`（不误选）/ `TestStartupFileMatches` |
| 清理命令真的接上了 | 服务端 `TestResolveCommandEquivalence` 的 cleanup 用例 + 客户端 `TestCleanupDispatched` / `TestCleanupPurgedOnStartup`（源码级棘轮） |
| ntdll 解析在 24H2 上不再静默失败 | `TestPrepareTrampolinesResolvesEveryName`（实机遍历名单，缺一个即失败）+ `TestSyscallReachesKernel`（真进内核，NTSTATUS=0） |
| 伪栈真的被 syscall 用上了 | `TestSpoofAllocatesWithStackArgs`（6 参数 NtAllocateVirtualMemory 走伪栈，`VirtualQuery` 断 `PAGE_READWRITE`）+ `TestSpoofMatchesUnspoofed`（与直连 trampoline 结果一致） |
| 摘链真的让模块枚举看不到 | `TestHideModuleDisappearsFromModuleList`：摘链后 `CreateToolhelp32Snapshot(TH32CS_SNAPMODULE)` 里**没有**本模块，挂回后又有 |
| 摘链不会把加载器链表接坏 | `TestAttachFallsBackToTailWhenNeighborsChanged`（邻居被动过时挂表尾）+ `TestFindSelfEntryStopsOnCycle`（自环不死循环） |
| ETW patch 覆盖了全部常用入口 | `TestPatchETWCoversAllExports`（实机 5/5）+ `TestEtwEventWriteReturnsSuccess`（patch 后调用返回 `STATUS_SUCCESS`，不是裸 `ret` 的垃圾值） |
| unhook 恢复的是**对的位置** | `TestUnhookNTDLLRestoresPlantedHook`：在 `NtLoadDriver` 上种 `48 B8 imm64 FF E0` 假 hook，unhook 后逐字节比对；旧实现用 RVA 直索引文件会写到别处，这条测试会红 |
| unhook 不破坏系统 | `TestUnhookAllDefaults` 在本机真跑三模块（ntdll 1153537B/3 节、kernel32 524315B/1 节、kernelbase 1249117B/1 节，`Restored=0` 说明无内联 hook）+ `TestUnhookIdempotent`（第二次恢复 0 字节） |
| PPID 伪造经得起内核复核 | `TestPPIDSpoofIsWhatTheKernelRecords`：读子进程 EPROCESS 的 `InheritedFromUniqueProcessId`，等于 explorer 的 PID 且不等于自身 —— 返回值成功不算证据 |
| BlockDLLs 真的落地了 | `TestBlockDLLsPolicyApplied`：`GetProcessMitigationPolicy(ProcessSignaturePolicy)` 读回 `MicrosoftSignedOnly` 位（写错常量时这条会红，而 `Update` 仍然成功） |
| spawn 路径没绕过属性封装 | `TestSpawnPathsUseChildAttrs`（源码级棘轮：`commands_windows.go` 里直连 `windows.CreateProcess` 只允许剩自删辅助那一处） |
| 开关"勾了不生效"不会发生 | 服务端 `TestResolveOpsecRejectsInvalidValues` / `TestResolveOpsecPlatformWindows386` / `TestResolveOpsecStackSpoofRequiresDirect`（非法或平台不可能的组合直接 400，不静默关掉）；客户端 `TestEvasionControlRefusesImpossible`（编译期关的能力运行期 `on` 被拒并说明原因） |
| 生成期开关真的进了载荷 | `TestPatchOPSECSwitchesAllReplaced`（5 个占位符都被替换）。外部证据分两步：**模板**解压后 `strings \| grep SLEEP_MASK_HIDE_PLACEHOLDER` **应当命中**（模板就是未 patch 的构建，占位符必须还在）；**生成出来的载荷**同一个 grep **必须无命中**（有命中说明模板与服务端版本不匹配） |
| 门控读的是运行期值而不是编译期常量 | `TestSleepMaskHidesModuleDuringSleep` / `TestHostEvasionGateDefaultsOff`（每个能力都经 `EvasionEnabled()`）+ `TestEvasionGatesAreRuntimeRead`（run.go / procattr / spoof 的调用点） |
| 运行期关掉真的能关 | `TestEvasionOverrideTogglesActive`（覆盖优先、显式关与"本来没开"可区分）+ `TestEvasionStatusStartupNote`（启动期能力在状态里带说明） |
| 配置白名单不会放坏值进载荷 | `TestApplyOpsecUpdatesRejectsInvalid`（6 组非法值，含非字符串类型）+ `TestApplyOpsecUpdatesIgnoresUnknown` |
| 前端不会发"勾了但载荷里没有"的组合 | `opsecPresets.test.ts`（`normalizeForPlatform` 三平台 + `capabilityDisabledReason` 四种原因）+ `payloadUtil.test.ts`（9 个字段逐个转发） |

> 注意：`fsperm_test.go` 里依赖真实权限位的用例在 Windows 会 skip。
> 为了不让"本机跳过"变成"没人验证"，判定逻辑（`needsTighten`）抽成了纯函数并在
> 所有平台真跑；实际 `chmod` 生效由 CI 的 ubuntu job 覆盖。

---

## 8. 相关文档

- `docs/MEMORY_OPSEC.md` —— 内存侧细节与 sleep mask 的并发设计
- `docs/CREDENTIAL_ACCESS.md` —— 凭据获取链路
- `docs/STORAGE.md` —— 存储后端（含静态加密说明）
- `docs/CODE_SIGNING.md` —— 载荷签名步骤
- `docs/REMAINING_WORK.md` —— 全局未做清单

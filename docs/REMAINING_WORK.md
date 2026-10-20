# 剩余优化项说明（Windows 接力用）

> **基线**：分支 `vshell-ui` 的最新提交（以 `git log -1` 为准；CI 状态见 GitHub Actions；
> `e716763` 及之前的提交均六 job 全绿）。已完成项见
> [`ROADMAP.md`](./ROADMAP.md) §2，逐条限制见 [`KNOWN_LIMITATIONS.md`](./KNOWN_LIMITATIONS.md)。
> 本文只列**还没做**的项，每项给：为什么没做 / 在 Windows 上怎么开始 / 验收标准。
>
> 所有"未做"都只有两类原因：**缺实机**（Windows/macOS/域环境）或**属于大改**
> （协议迁移/依赖取舍）。没有因为"忘了"而漏掉的项 —— 每轮都做过可达性核对。

---

## 0. 在 Windows 上继续之前

```powershell
git pull                      # 或整目录复制；模板已入库，clone 即可编译
cd Tshell-server; go test -race ./...        # 基线：应全绿
go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 -checks=all,-ST*,-U1000 ./...
cd ../Tshell-web; npm ci --legacy-peer-deps; npx vitest run; npm run type-check
```

- 构建顺序：`build_templates.ps1` → `npm run build:embed` → 服务端**文件模式**构建
- **改了植入体源码就必须重建模板**（`pkg/api/server/` 是 go:embed 依赖；漏了会出现
  "功能写了但载荷里没有"——本项目踩过两次：Firefox、Kerberos）
- 模板是二进制，`.gitattributes` 已钉成 `binary`（防 autocrlf 写坏）
- 旧 `config.yaml` 里的 `jwt.algorithm: HS256` 必须改 `RS256`，否则新版拒绝启动

---

## 1. kirbi ↔ ccache 互转 —— **文件格式部分已完成（2026-10-20）**

**现状**
- ccache 解析已完成：`Tshell-server/pkg/cred/ccache.go`（v3/v4，真实夹具）
- **kirbi 解析与互转已完成**：`Tshell-server/pkg/cred/kirbi.go` + 10 项测试
  - `ParseKirbi`（kirbi → CCache）、`(*CCache).MarshalKirbi`（CCache → kirbi）、
    `(*CCache).Marshal`（CCache → ccache v4 字节）；另 4 项制品层测试
  - 真实夹具交叉验证：`ccache-v4-kirbi` 解出的 client/server/keytype/旗标/票据 DER
    与 `ccache-v4` **逐字段一致**；两个方向都做了往返测试
  - `ParseTicketArtifact` 现在**直接解析** `kirbi/*.kirbi` 条目，不再依赖客户端清单；
    清单里同名条目不重复登记（以二进制解析结果为准）
  - 顺带更正一处**在客户端与服务端注释里各写了一遍的错误说法**：
    "kirbi 的 enc-part 需要密钥才能解" —— 实际 etype 为 0，cipher 就是明文 DER，
    所以这一段根本不需要密钥，可完整离线验证
- 仍缺（需 Windows/AD）：**ptt 的运行时接受度** —— 产出的 kirbi 被
  `kerberos::ptt` / LSA 认可才算端到端成功。这是运行时行为，不是文件格式问题

**实现时踩到的坑（留给后续维护者）**
- `KrbCredInfo` 的字段是 **EXPLICIT** context tag（`a1 { 1b "REALM" }`），不是隐式
  （`81 "REALM"`）。按隐式写或读会**静默丢字段**（realm 在、用户名没了），不报错
- `derTLV(derSequence, <已经是 SEQUENCE 的 TLV>)` 会多套一层，变成
  `30 { 30 { a0, a1 } }`；解析侧按一层拆就找不到组件。这两处都只有**往返测试**能抓
- 写 kirbi 时时间字段按 map 遍历会产生不确定的字节序，已按 tag 排序保证输出稳定

**剩余验证（Windows 上做）**
- 自签 AD 环境拿一张 TGT → 导出 ccache → `MarshalKirbi` 转 kirbi → Windows
  `kerberos ptt <base64>` 或 mimikatz `kerberos::ptt` → `klist` / `kerberos list`
  能看到该票据
- 反方向：`kerberos` 导出 kirbi → 服务端 `ParseKirbi` → `Marshal` 出 ccache →
  Linux `kinit -c` 接受

**验收**：上述两条在真实 AD 环境走通。文件格式层面已用真实夹具验证完毕，
未通过的部分只可能是 ptt 的运行时细节（如 LSA 对 enc-part 的具体期望）。

---

## 1.5 OPSEC 数据面 —— **本轮已完成（2026-10-20）**

按「全程保持 OPSEC」的要求做了一轮系统梳理与实现，现状与威胁模型见 **`docs/OPSEC.md`**
（该文档是 OPSEC 能力的唯一权威声明；没写的就是没有）。本轮完成的项：

- **战果静态加密**：`loot.secret` 改为 AES-256-GCM 密文（`pkg/security/atrest.go`）。
  关键细节：GCM 随机化会打破原来的 `secret = ?` 去重，故新增 `secret_fp`
  （带密钥 HMAC）作为去重键 —— 这是最容易漏的一步，已用测试钉住
- **战果审计**：增/查/删全量留痕（此前只有 `/cred/*` 有 CRED_ACCESS，
  loot 一行审计都没有），且审计**不含密钥本体**
- **规则化落盘权限**：数据目录 0700、会话录像 0600、DB 文件 0600，
  启动时收敛历史宽权限目录并打印改过哪些
- **初始管理员密码不进日志**：改写入 0600 文件；顺带发现第二处同类泄露
  （旧版明文密码迁移路径也在打印随机密码），一并修掉，并加了源码级守卫测试
- **战果保留期**：`security.loot_retention_days`（默认 0=永久，删除不可逆故不默认开）
- **心跳默认值对齐**：画像 1000ms 与全局 5000ms 不一致，已统一并加防漂移测试

仍属 OPSEC 未做项（均需实机/排期，详见 `docs/OPSEC.md` §6）：
主密钥运行时派生（ECDH，XL）、内存镜像加密、调用栈伪造、APC 分发 syscall 化、
流量体积整形、审计日志防篡改。

**P0/P1 已完成（2026-10-20）**：注入链分配/写入/改保护 sycall 化（逐项回退）、
进程镂空 W^X（可执行节 RW→RX）、`cleanup` 清理现场命令（cmdType 134）、
自更新 `.old` 残留由新映像启动后自动清理。三项都只有单测 + 源码级棘轮，
**未在真实目标/实机验证**（见 `docs/KNOWN_LIMITATIONS.md` §1、§17）。
## 2. BloodHound 采集器（**L**，需域环境）

**现状**：只有 `ad_enum`（命令级 `net` 调用），没有任何 LDAP 采集。

**为什么没做**
- 需要在**植入体**里引入 LDAP 客户端（依赖会进全部模板（200+ 个），体积/供应链都要评估 ——
  此前为省体积连纯 Go SQLite 驱动都没引，自己写了只读读取器）
- 没有域环境无法验证查询/分页/ACL 解析

**怎么开始**
- 依赖取舍二选一：
  (a) `go-ldap/ldap`（纯 Go，模板会胀大，需实测体积）；
  (b) 自己写最小 LDAP 客户端（Bind + Search，BER 编码，工作量中等）
- 入口：`implant/core/commands/bloodhound_windows.go` + `dispatch.go` 命令类型；
  输出 JSON（users/computers/groups/gpos/ous/sessions/acls）→ 制品回传 →
  服务端存到 Downloads，操作员导入 BloodHound CE
- 只读查询（`LDAP_SERVER_SD_FLAGS` 拿 nTSecurityDescriptor 做 ACL）

**验收**：域内一台主机采集产出的 ZIP 能被 BloodHound CE 直接导入并出图。

---

## 3. 内存 OPSEC 三项（**XL**）

现状：sleep mask 遮蔽密钥材料（会话/上一把/主密钥/盐 + AEAD 缓存，
`implant/core/app/run.go`）+ 睡眠期模块摘链；direct syscall 路径已布伪栈；
镜像不加密（Ekko 在 Go 上不可行，见 3.1）；主密钥仍是编译期 patch。

**已完成（2026-10-20，P0）**：注入与镂空的内存原语已接 syscall
- 新增 `implant/core/commands/remotemem_windows.go`：`NtOpenProcess` /
  `NtAllocateVirtualMemory` / `NtWriteVirtualMemory` / `NtProtectVirtualMemory` /
  `NtReadVirtualMemory` / `NtUnmapViewOfSection` / `NtQueryInformationProcess` 优先走
  trampoline，**逐项**失败回退宿主 API（注入不能因为规避手段缺失而整条失效）
- 镂空不再申请 `PAGE_EXECUTE_READWRITE`：镜像先 RW、写完把**可执行节**收紧为 RX
  （`hardenImage`）。非可执行节保持 RW 是 loader 写 IAT/全局的硬需求，理由与唯一例外
  （需重定位时保留可写）写在函数注释与 `docs/OPSEC.md` §3
- 14 项单测：路径/顺序/保护参数用假后端钉死，另有一项真实 kernel32 往返 + `VirtualQuery`
  确认 RX 生效；源码级棘轮防止有人再直接调 `VirtualAllocEx` 等
- **未实机验证**；APC 分发仍是 `QueueUserAPC`（见 `docs/OPSEC.md` §6）

### 3.1 Ekko / Foliage 级 sleep mask（需 Windows 实机）
- **结论（2026-10-20，P2）：在 Go 上不可行，已归档**
- 原因（不是排期问题，是机制冲突）：Ekko 要加密自身 `.text/.rdata`，前提是"能停住
  所有线程"。Go 的 `sysmon`/GC worker/信号线程常驻，加密窗口内随时执行被加密字节
  → 必崩；改 `SuspendThread` 会因被挂线程持有 `mheap/mspan` 锁或停在 GC safepoint
  而死锁；`runtime.stopTheWorld` 不对外暴露。
- 替代（已实现）：睡眠期 **PEB 模块摘链**（`core/evasion/modhide_windows.go`），
  镜像不加密、只让加载器枚举看不到；门控 `config.HostEvasionEnabled()`，默认关。
  边界：不动 VAD，按内存区域扫仍可见（见 `docs/KNOWN_LIMITATIONS.md` §18）。
- 若仍要做真 Ekko：只能改为 C 植入体或 fork 出独立的最小 loader 进程承载 beacon，
  两者都是换架构级别的决定。

### 3.2 调用栈伪造（**已实现 direct syscall 路径**）
- 现状（2026-10-20，P2）：`evasion.Syscall()` direct 模式下布伪栈
  （`BaseThreadInitThunk+0x14` → `RtlUserThreadStart+0x21`，栈传参复制到伪栈 `+0x28`），
  桩按 SSN 惰性生成 RW→RX。实机测试：6 参数 `NtAllocateVirtualMemory` 走伪栈成功、
  与直连 trampoline 结果一致、16 goroutine 并发无竞态。
- 顺带修掉一个真 bug：`ParseSyscallStub` 写死 syscall 偏移 8，在 Win11 24H2 上
  （偏移 0x12）导致**所有** Nt\* 解析失败 → syscall 模式静默退化成 kernel32。
  现在改为扫描 `0F 05`，实机逐个 Nt\* 断言 + 一次真进内核。
- 遗留：kernel32/ntdll 直调（`CreateProcessW`/`VirtualAllocEx`）仍长在 Go 线程栈上；
  indirect 模式刻意不套伪栈。要做全得引入 `stdcall` 汇编桩 + 独立线程。
- 验收缺口：**没有在真实 EDR 主机上跑过**，伪栈能否骗过具体产品的回溯器需实测。

### 3.2b ETW/AMSI 覆盖面、unhook、PPID、BlockDLLs（**已实现，2026-10-20 的 P3**）
- ETW：patch 从 1 个导出扩到 5 个（`EtwEventWrite`/`Ex`/`Full`/`String`/`Transfer`），
  统一返回 `STATUS_SUCCESS`（旧实现裸 `ret`，返回 EAX 是垃圾）；patch 后读回校验。
- AMSI：`AmsiScanBuffer` + `AmsiScanString` 两个入口。
- unhook：新增 `core/evasion/pe_image.go` 解析 PE，按节表换算文件偏移，恢复
  ntdll/kernel32/kernelbase 的**全部可执行节**且只写差异字节。修掉两个真 bug：
  ① 旧实现把 RVA 当文件偏移（`diskData[rva:rva+16]`），会主动写坏代码段；
  ② 只恢复固定名单里导出的前 16 字节，深 hook 与非导出函数全漏。
- PPID 伪造：`PROC_THREAD_ATTRIBUTE_PARENT_PROCESS` 指同会话 explorer.exe；
  实测 x64 上该属性吃 **HANDLE** 而非 PID（4 字节 PID 报 `ERROR_BAD_LENGTH`，
  8 字节 PID 在 CreateProcess 时报 `ERROR_INVALID_HANDLE`）。
- BlockDLLs：`PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY` 只允许微软签名 DLL；
  常量是高 32 位第 12 位（`0x0000100000000000`），写错位不会报错、只是不生效。
- 已接路径：`Run`/`Execute`/`ExecuteFile`/`hollow`；`conpty`/`hvnc`/`privesc`
  刻意不接（交互式 UI 与 Token 路径，BlockDLLs 会破坏输入法/Shell 扩展）。
- 遗留：ETW 的 `NtTraceEvent` 直调与 ETW TI（内核回调）无法用用户态 patch 覆盖；
  内核态 hook 与"改了磁盘副本的 hook"unhook 无效；PPID 只改创建时刻的父字段
  （句柄继承、4688 审计日志仍可能暴露真实调用者）；**未在真实 EDR 主机验证**。
  详见 `docs/KNOWN_LIMITATIONS.md` §19。

### 3.2c 对抗能力逐项开关（**已实现，2026-10-20 的 P4**）
- 问题：P0–P3 的能力都是"编译期写死 + 门控跟着 `HostEvasionEnabled()`
  （ETW∥AMSI∥syscall≠none）走"，勾一个带出一串；想关某一项只能重新生成载荷。
- 生成期：占位符从 4 个扩到 9 个（新增 `SLEEP_MASK_HIDE` / `STACK_SPOOF` /
  `PPID_SPOOF` / `BLOCK_DLLS` / `UNHOOK_ON_START`），缺省全关；服务端
  `pkg/api/opsec_params.go` 统一归一化与校验（非法值、平台不可能组合直接 400，
  不静默关掉 —— 静默关掉正是"勾了不生效"的病根）。
- 运行期：`EVASION=135`，`evasion [status|all on|all off|on <key>|off <key>|reset]`，
  覆盖只在内存（重连即回到生成期值），三态语义可区分"运行期关掉"与"本来没开"。
  编译期关 = 硬关闭，运行期 `on` 被拒并说明原因；`syscallMode` 不提供运行期切换。
- 团队级默认：服务端 `config.opsec.*`（11 字段），回落链「请求 → 配置 → 内置默认」。
- 前端：生成期「对抗能力」面板（9 项 + stealth/balanced/compatible 三预设 +
  平台/架构置灰原因）+ 会话内「Evasion Switches」面板。
- 遗留：运行期覆盖不持久、启动期能力"关不掉"（不撤销已打补丁）、旧模板
  fail-closed（**必须全量重建 178 个模板**）、未做三平台实机回归。
  详见 `docs/KNOWN_LIMITATIONS.md` §20。

### 3.3 主密钥运行时派生（**协议迁移**，服务端 + 客户端同时改）- 目标：主密钥不再明文 patch 进二进制（`pkg/delivery/payload.go` 的
  `LISTENER_MASTER_KEY_*` 占位符），改为首连时 X25519 临时交换派生
- 为什么没做：要在 **13 种传输的首包**插入交换；轮询型传输（HTTP/DNS/OSS）服务端
  要为每个 beacon 缓存临时密钥直到会话建立 —— 一次真正的协议迁移
- 验收：二进制里提取不到可用主密钥；首次上线 + 命令往返在 13 传输上全绿

---

## 4. Windows / macOS 实机验证（**M**，纯验证）

所有代码路径齐全但**从未在真机跑过**。建议按下面顺序逐项验（每项记录日志/输出）：

| 场景 | 命令/入口 | 预期 |
|---|---|---|
| 上线 + 命令往返 | 任意载荷 | 会话在线，`shell whoami` 回显 |
| 提权 | `getsystem` / `getsystem uac` | 新会话或明确失败（不炸会话）；`uac_cleanup` 还原注册表 |
| 票据枚举 | `kerberos list` | 列出缓存票据（服务名/客户端/时间/旗标） |
| 票据导出 | 凭据获取 → ticket → lsa | 制品回传，凭据库出现 `type=ticket, source=kerberos:lsa` |
| 票据清除/注入 | `kerberos purge [服务名]` / `kerberos ptt <base64>` | purge 后 list 为空；ptt 后 list 出现该票据 |
| 凭据族 | DPAPI / Chrome|Edge / Firefox / cards | 口令/卡号入凭据库；自动填充只在制品里 |
| 交互式 PTY | 终端页 | 双向交互、窗口大小、Ctrl+C |
| 持久化 | `persist` 各技术 | 重启后回连；卸载干净 |
| 规避 | `etw_patch` / `unhook_ntdll` | 不崩、行为符合预期 |
| 截屏 / HVNC | 对应页面 | 帧率与输入正常 |
| macOS | 持久化/PTY/截图/监控拉取 | 同上（LaunchAgent/Daemon） |

发现的问题按"先复现、再补单测、再修"处理；每修一项把 `KNOWN_LIMITATIONS` 的
状态行改掉。

---

## 5. 前端剩余（**S~M**）

- **`ClientFiles.vue` 拆分**：**进行中（2026-10-20）**，2101 → 2023 行
  - 已抽出三个模块（模板未动，只搬逻辑，与原建议一致）：
    - `files/format.ts` —— 格式化/解析（32 项测试）
    - `files/decodeContent.ts` —— 编辑器内容解码（27 项测试）
    - `files/useFileDownload.ts` —— **下载进度轮询与终态判定**（19 项测试）
  - `useFileDownload` 是这批里最值得抽的：它同时追"字节进度"与"任务终态"两条线，
    另有超时兜底与"无 task_id 时退回按百分比判定"的兼容分支。历史上两次修复都在这里
    （轮询句柄只在 pct≥100 清理导致关掉对话框后永久轮询；只盯百分比导致失败要等 6 分钟
    才提示）。设计上把**判定**（`evaluateDownloadPoll`，纯函数）与**调度**（setInterval）
    分开，边界不必依赖假计时器
  - 仍可继续拆：上传流程、编辑/保存流程（各自一组相关状态），模板可按"上传/下载/编辑"
    三个子组件切 —— 但切模板要连同 scoped 样式搬，且无视觉回归测试，收益小于风险，
    建议有视觉回归手段后再做
- **redirector 模板的 UI 入口**：**已完成（2026-10-20）**。监听器页「更多 → 重定向器配置」
  打开 `layout/Dialogs/RedirectorDialog.vue`：按当前画像生成 nginx/apache 配置，
  可填后端/域名/诱饵/证书路径，生成后展示文件名、行数、转发路径清单与全文，
  支持复制与下载。7 项组件测试。此前只有 CLI（`tshell -gen-redirector`）与
  `GET /api/redirector/template`
- 其他（低优先）：生命周期钩子里的 `any` 已清零，棘轮上限为 0 —— 新代码别再加

---

## 6. 水平扩展（**XL，已决策不做**）

单副本是**明确决策**而不是遗漏：连接池/在线状态/会话锁/事件流/命令队列都在进程内存。
`deploy/k8s/tshell.yaml` 已按单副本写实（`replicas: 1`、`strategy: Recreate`、
`ReadWriteOnce` PVC、无 HPA）。要让 `replicas>1` 成立，需要：共享状态外置
（Redis/Postgres）+ leader 选举 + 任务/审计的一致性模型 —— 属于独立的架构项目。

---

## 7. 供应链剩余

- **代码签名**：已实现（`pkg/signing`，默认 osslsigncode），但**未用真实证书验证**。
  拿到 EV/OV 证书后在 Windows 上按 `docs/CODE_SIGNING.md` 的步骤跑一次并更新状态
- 硬件令牌/PKCS#11 签名器：需厂商工具，通过 `signing.args` 接入（未验证）

---

## 8. 交接检查清单

- [ ] `git log --oneline -5` 与远端一致；CI 全绿
- [ ] 服务端 `go test -race ./...`、staticcheck 全绿
- [ ] 植入体 `go test ./...` + 13 传输交叉编译全绿
- [ ] 前端 `vitest`（含 `techdebt` 棘轮）全绿
- [ ] 模板与源码同步（改了植入体就重建）
- [ ] 旧配置的 `jwt.algorithm` 已改 `RS256`

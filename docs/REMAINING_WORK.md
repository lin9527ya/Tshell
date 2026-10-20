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

### 3.3 主密钥运行时派生（**协议迁移**，服务端 + 客户端同时改）

**进度（2026-10-20）**

- ✅ **1a 密码学核心 + 接缝**：`pkg/encrypt/ecdh.go`、`implant/core/encrypt/ecdh.go`
  - X25519（标准库 `crypto/ecdh`）+ HKDF-SHA256（info = `tshell-ecdh-v1`）
  - 拒绝长度错误与**低阶点**：X25519 对低阶点不报错，共享密钥会是全零，
    攻击者可借此把它固定成已知值（小子群攻击）
  - `ReplaceMasterKey` 是唯一接缝：会话密钥重派生、AEAD 缓存、sleep mask
    全都只依赖 `globalMasterKey`，换源后下游零改动
  - 互通性：RFC 7748 §6.1 向量 + **同一个黄金向量在两侧各钉一次**（`4fd4f22a…4b8`）。
    跨 module 无法互相 import，这是唯一可靠办法；只测"两端算一样"没用 —— 一起错照样通过
- ✅ **1b-0 握手缓存**：`pkg/encrypt/kx_cache.go`
  - 按客户端公钥哈希存（握手时服务端还不知道 uid）、TTL 30s、容量 4096 LRU 淘汰、
    过期清零、Get 返回副本、解出 uid 后 `Bind`
- ✅ **1b 传输接线**（2026-10-20 完成）：`http` + `https`
  - 协议：请求头 `X-Kx: base64(clientPub)` → 响应头 `X-Kx: base64(serverPub)`
  - **关键顺序**：必须在生成 MetaInfo **之前**交换。MetaInfo 含主机名/用户名/进程，
    先加密再交换的话首包仍会被静态密钥解开，改造就只剩表面效果。所以首包是
    "只带公钥的 GET"（服务端识别为纯握手直接回 200），交换完成后才生成 MetaInfo
  - 服务端只改 `metaMK` 的来源（它同时用于解 MetaInfo 与派生会话密钥），
    下游 RegisterClient / nonce 唯一化 / 命令加解密全部自动跟随
  - 自愈：服务端把**公钥也存进缓存**并在每次响应回传，客户端每次据此重派生 ——
    否则条目过期重建密钥对后，客户端手里的主密钥就与新密钥不匹配
  - 降级安全：无 `X-Kx` → 走原静态密钥路径（逐字节同老行为）；
    声明了 `X-Kx` 但交换失败 → **明确 400 拒绝**，不降级
  - 顺带修掉 `getRaw` 里 err 检查在 `Header.Set` 之后导致的 nil 解引用
- ✅ **1c 首条传输：`tcp`**（2026-10-20 完成）
  - 分帧已存在（`len(4)|msgType(4)|payload`），故新增 `msgType=200`（KX），
    两端改动都局限在各自的消息分发 switch 里
  - 派生主密钥挂在**连接级状态** `connKXState` 上跨帧保留 —— 不用全局 map，
    省掉生命周期与清理；`processMessage` 是私有函数、只有一个调用方，改签名成本低
  - 修掉一个真 bug：覆盖主密钥时**盐没跟着覆盖**，两端用不同盐派生会算出不同密钥，
    现场只表现为"解密失败"。现在由 `resolveMetaKey` 固定优先级并有专门用例钉住
- ✅ **1c 已接 11 条**：`http` / `https` / `tcp` / `kcp` / `websocket` /
  `tcp_forward` / `kcp_forward` / `ws_forward` / `http_forward` / `https_forward` /
  `dns`
  - `websocket`：HTTP 升级，公钥搭在升级请求/响应头（`upgrader.Upgrade(w,r,respHeader)`）。
    关键时序：WS 客户端的 MetaInfo 是**升级后第一帧**，所以一个来回即可完成交换
  - `kcp`：分帧与 tcp 一致，复用 `msgType=200`。差别只在派生密钥怎么交回 ——
    KCP 的 `metaMasterKey` 是循环外的局部变量，helper 直接返回值由调用方赋值即可
  - `tcp_forward` / `kcp_forward`（2026-10-20 完成）只需补**客户端侧**：
    它们的服务端复用 `HandleTcpConnectionForWithKeys` / `HandleKCPConnectionForWithKeys`，
    KX case 在共享循环里。**注意一个反直觉的点**：forward 虽然方向相反
    （beacon 监听、C2 拨入），但**协议角色不变** —— 仍由 beacon 发 firstBlood，
    所以交换也由 beacon 发起，与反向模式完全同一套代码
- ✅ `ws_forward`（2026-10-20）：方向与反向相反（C2 拨号、beacon upgrader），
  由 C2 发起交换；beacon 侧 `kxRespond` 读请求头、用 `upgrader.Upgrade(w,r,respHeader)` 回公钥
- ✅ `http_forward` / `https_forward`（2026-10-20）：**服务端零改动** ——
  协议循环复用 `tcp.HandleTcpConnectionForWithKeys`，KX case 在共享循环里。
  beacon hijack 后自己手写 200 响应，之后已是裸流，所以只需在写元数据前发 KX 帧
- ✅ `dns`（2026-10-20）：最受限的通道（子域标签 ≤63 / TXT ≤255）。
  32 字节公钥 base32 后正好 52 字符，两处限制都满足（已用测试钉住）。
  查询 `<b32(pub)>.1_1.kx.<uid>.<domain>`、响应 TXT 回服务端公钥；
  因格式与 `mi` 同形，**uid 在这一步就已知**，服务端直接按 uid 绑定派生结果，
  后续 `mi` 分块按 uid 命中即可
- ⬜ **1c 剩余 1 条**：`oss`
  - 形态与其它 11 条都不同：没有请求/响应语义，只有「上传对象 + 列表轮询」。
    交换需要"上传 kx 请求对象 → 服务端处理后写响应对象 → 客户端轮询取回"，
    多一次 RTT 且要处理轮询超时；服务端侧还要在对象处理循环里加 kx 分支。
    这是设计工作而非接线工作，单独一轮处理
  - `ws_forward`（已勘察，落点明确）：与前面几条**方向真的相反** ——
    C2 侧是 WS 拨号方、beacon 侧是 upgrader。所以交换由 **C2 发起**：
      1. `pkg/api/forward_connection.go` 的 `dialForwardSession` 里
         `gorillaws.DefaultDialer.Dial("ws://"+address+"/ws", nil)` ——
         第三参当前是 `nil`，改成带 `X-Kx` 的 header 即可
      2. 从 dial 返回的 `resp.Header` 取服务端（beacon）公钥并派生
      3. beacon 侧（`implant/.../transport_ws_forward.go`）在
         `upgrader.Upgrade(w, r, nil)` 改为读 `X-Kx`、派生、并把公钥放进
         `upgrader.Upgrade(w, r, respHeader)`
      4. 派生密钥要显式传给 `websocket.HandleWebSocketForWithKeys`（当前
         `handleWSConn` 的 `derivedMK` 参数已存在，但 forward 入口传的是 nil，
         需要多一个参数而不是靠 `NormalizeListenerKeyConfig(vkey)` 隐式复用 ——
         后者依赖"32 字节恰好原样返回"的实现细节，太脆）
  - `http_forward`/`https_forward`：结构类似（C2 侧 dial、beacon 侧 ServeHTTP），
    但入口是 HTTP 请求而非 WS 升级，可复用 1b 的头方案
  - `dns`/`oss` 编码通道不同（base32 子域 / 对象名），需单独设计

**多传输构建的一个坑（已修，勿回退）**：`kxMsgType` 必须放在**无 build tag** 的
`cmd/single/kx_const.go` 里共享。tcp 与 kcp 若各自定义一份，`transport_multi`
（同时编入所有传输）会因重复声明而编译失败 —— 这类失败只在多传输构建里出现，
单传输构建完全正常，很容易漏。
- ⬜ **1d 模板重建**（未做，必须做）：客户端代码变了 ⇒ 178 个预置模板要重建，
  否则旧模板会 fail-closed 拒绝运行（现有设计如此，属正确行为）

**1c 为什么比看起来大（勘察结论，2026-10-20）**

两类传输的接入方式根本不同，不能照抄 1b：

1. **裸流型**（tcp/kcp/ws 及 forward 变体）：没有 HTTP 头可用，必须加**固定首帧**
   `KX1 | pub(32)` / `KX2 | pub(32)`，两端都要改帧格式
2. **服务端分发点各不相同**（`pkg/api/listener.go` 的 start 分支）：
   `tcp` 走 `connection/tcp.HandleTcpConnectionForWithKeys`、`kcp` 走 `AcceptKCP`、
   `websocket` 走 Gin、`dns`/`oss` 各有轮询入口 —— 每条的接入点与帧格式都不同。

> **更正（2026-10-20 自查）**：我先前在本文件写过"其余传输的服务端是
> `Tshell-client/Reacon_*` 独立模块、产物入库在 `pkg/api/server/<transport>/`"，
> **那是错的**。查证结果：
> - `pkg/api/server/<transport>/*` 由 `pkg/api/server.go` 的 `//go:embed server/*`
>   嵌入，按 `protocol/os/arch/format` 匹配后**下发给客户端** —— 是**客户端载荷**，
>   不是服务端（见 server.go 错误文案"未找到匹配的客户端二进制"）
> - 各传输的**服务端就在主服务端代码里**（`pkg/api/listener.go` + `pkg/connection/*`）
> - `Tshell-client/Reacon_*` 是另一些独立客户端实现（多数已归档到 `_archived_reacon/`）
>
> 所以 1c 的真实工作是：**改服务端分发点 + 改客户端 + 重建 178 个客户端载荷（1d）**。
> 比"照抄 10 个文件"大，但**不涉及另一个服务端模块** —— 比原先判断更乐观。

所以 1c 的实际工作是：改服务端分发点 + 改客户端 + **重建 178 个客户端载荷/模板**。
forward 类仍是特例：客户端**监听**、服务端拨入后 hijack，连方向都是反的
（参照 `HTTPForwardClient.Connect`）。

**TCP 已勘察到的具体落点（可直接照做）**：帧格式是
`len(4) | msgType(4) | payload`，firstBlood 为 `msgType=1`。
接入方式不必新增裸流头，而是**加一个新的 msgType（KX）**：
客户端先发 KX 帧（载荷 b64(clientPub)），服务端派生后回 KX 帧（b64(serverPub)），
随后照旧发 firstBlood —— 两端的改动都局限在各自的消息分发 switch 里。
需要注意：`processMessage` 每次调用都会重新从监听器配置取主密钥，
所以派生结果必须挂在**连接级状态**上跨帧保留（该函数是私有的，只有一个调用方，
改签名成本低）。

建议顺序：先打通 `tcp` 一条（帧最简单、两端都在活跃代码里）验证 `KX1/KX2` 设计；
再按同模式铺 kcp/ws；dns/oss 单独设计（编码通道不同）；forward 类最后做。

**已定协议约定（供 1b/1c 遵守）**

- 公钥走**明文头**可接受：公钥本就公开，攻击者拿到也算不出共享密钥。
  但**首包不得携带任何敏感载荷**（MetaInfo 顺延到第二条消息），
  否则静态密钥仍能解出敏感数据，改造只剩表面效果
- 服务端按 `sha256(clientPub)` 命中缓存；客户端握手后继续重复带同一公钥头直到会话建立；
  服务端在首条有效消息解出 uid 时 `Bind`
- `security.ecdh_mode: required|preferred|off`（默认 `preferred`）；
  `required` 下旧载荷必须被**明确拒绝**并提示重新生成，不能静默降级

- 目标：主密钥不再明文 patch 进二进制（`pkg/delivery/payload.go` 的
  `LISTENER_MASTER_KEY_*` 占位符），改为首连时 X25519 临时交换派生
- 为什么这是 XL：要在 **12 条传输的首包**插入交换；轮询型传输（HTTP/DNS/OSS）服务端
  要为每个 beacon 缓存临时密钥直到会话建立
- 验收：二进制里提取不到可用主密钥；首次上线 + 命令往返在全部传输上全绿

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

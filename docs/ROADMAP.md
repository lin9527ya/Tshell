# Tshell 优化路线图与现状

> **用途**：接着往下做的工作台。已完成的部分给出证据与代码位置；未完成的部分给出
> **目标 / 现状 / 入口 / 验收 / 工作量**，换台机器（含 Windows）也能直接开工。
>
> - 版本基线：**v1.0.0**（提交 `5e96559`），最后更新：2026-10-18（凭据模块第二批：DPAPI + 浏览器）
> - 逐条"已知限制"的完整描述见 [`KNOWN_LIMITATIONS.md`](./KNOWN_LIMITATIONS.md)，本文做索引与待办
> - 架构与部署分别见 [`../ARCHITECTURE.md`](../ARCHITECTURE.md)、[`../DEPLOYMENT.md`](../DEPLOYMENT.md)
> - 本轮改动清单与验证结果见 [`../OPTIMIZATION_CHECKLIST.md`](../OPTIMIZATION_CHECKLIST.md) N 组

---

## 0. 开工前先做三件事

```bash
git describe --tags --always          # 确认在 v1.0.0 或其之后
bash build.sh --server-only           # 只重建服务端（约 1~2 分钟，CGO）
go test -race ./Tshell-server/...     # 见 §4 回归基线（注意用外层仓库根目录）
```

三个必须知道的坑：

1. **git 只在最外层仓库跑**。`Tshell-server/`、`Tshell-client/`、`Tshell-web/` 里各有一个
   **上游遗留的嵌套 `.git`**（tag `0.0.5`）。在子目录里跑 `git describe` 会取到那套陈旧信息
   —— 构建脚本已用 `git -C <外层根>` 修好，手动敲命令时也要注意。
2. **模板是 `go:embed` 依赖**，已在版本库内（`Tshell-server/pkg/api/server/`，约 587MB）。
   只有改动植入体源码时才需要 `bash build_templates.sh`（全矩阵约 20 分钟）。
3. **重建模板期间不要同时构建服务端**：`go:embed` 的 stat→open 之间可能跨越一次文件替换，
   报 `unexpected length`。顺序永远是：先模板，再服务端。

---

## 1. 当前状态（v1.0.0 + 凭据模块第二批）

| 维度 | 状态 |
|---|---|
| 传输 | 12 种传输 + 单二进制多传输（运行时回退链 + 主动健康探测）、多级中继 `Tshell-relay` |
| 加密 | AES-256-GCM 全链路、监听器主密钥 + HKDF 会话密钥、per-session nonce 唯一化、24h 轮换、sleep mask（全传输） |
| 服务端 | 任务一等对象、审计落库、多操作员协作、Prometheus 指标、备份恢复、存储抽象（SQLite/PostgreSQL + 版本化迁移）、RBAC/限流/隐藏访问 |
| 前端 | 会话表 / Beacon 控制台 / 任务中心 / 事件日志 / 凭据库 / Payload 与 Attacks 对话框族 / 密钥管理 / 审计 |
| 后渗透 | LSASS 转储、**DPAPI 主密钥集**、**浏览器口令与 Cookie**（内存解密）、**提权（getsystem + UAC 绕过）**、端口扫描、令牌操作链、横向移动、持久化族 |
| 工程 | CI 六 job（vet / staticcheck / race / 覆盖率 20% / PG 集成 / 可复现构建）、可复现构建 + SBOM、redirector 模板、模板随仓库提供 |

**一句话**：单机单操作员的完成度高；**团队化/规模化的完成度仍偏低**（多副本、MFA），
**后渗透深度仍是最大短板**（凭据侧刚补上 DPAPI/浏览器两块，无提权模块、无 Kerberos/BloodHound）。

---

## 2. 已完成（含证据）

### 2.1 传输与隐蔽
- 12 种传输（TCP/KCP/HTTP(S)/WS/OSS/DNS + 5 种正向）+ **单二进制多传输**：
  运行时端点回退链、60s/3s 主动健康探测、连续 2 次失败切端点
  （`Tshell-client/implant/cmd/single/transport_multi.go`）
- **多级中继**：`Tshell-relay/`（纯标准库），TCP/UDP 双向转发、级联、TLS 终止、
  连接/单 IP 上限、空闲超时、UDP 流表、`/healthz`；中继不持有密钥（端到端加密）
- **sleep mask 覆盖全部传输**：持久型传输实现 `PollInterval`，空闲路径走
  `SleepMaskedWithJitter`；遮蔽的是**真实密钥材料**（会话/上一把/主密钥/盐 + 清空 AEAD 缓存），
  分片（2s）+ 在途门控（`implant/core/app/run.go:106,123`）
- syscall trampoline 已改为 **RW → 写码 → RX** 两段（`implant/core/evasion/syscall.go:95`）

### 2.2 加密与密钥
- 会话密钥唯一化（每连接随机 nonce 重派生 + 初始密钥下发）、24h 轮换（ACK + 重叠期）、
  默认/空密钥三处 fail-closed（监听器启动 / 生成载荷 / firstBlood）
- 占位符残留校验：生成载荷时严格检查（多传输端点列表除外）

### 2.3 服务端运营能力- **任务一等对象**：`tasks` 表 + 状态机 `queued→dispatched→completed|failed|timeout` +
  `POST /api/task/:id/cancel` + 前端任务中心（`pkg/api/tasks.go`）
- **审计落库**：`AuditLogDB` 双写（文件留人读）+ 库表分页查询（`pkg/audit/db.go`）
- **多操作员协作**：presence / 会话锁 / 操作广播（`pkg/collab/collab.go`）
- **可观测性**：Prometheus `/metrics`（beacon 在线数、队列深度、任务吞吐/失败率、
  DB 连接池、审计丢弃计数）；`logx` 支持 JSON 与文件输出
- **备份恢复**：`VACUUM INTO` + 保留策略 + 手动触发（`pkg/backup/`，PostgreSQL 下禁用并提示 pg_dump）
- **存储抽象**：`database.DB()`（`Store` 接口，`pkg/database/store.go`）+ 回归棘轮
  `Tshell-server/store_access_test.go`；PostgreSQL 方言层 `pkg/database/dialect.go`；
  版本化迁移 `pkg/database/migrations.go`
- **redirector 模板**：nginx / apache 生成（`pkg/redirector/`、`docs/REDIRECTOR.md`），
  含 HTTP/2 写法兼容性说明
- 死配置已接线：`database.max_*`、`logging.format/output`、审计三项配置

### 2.4 前端
- vshell 风格壳 + 任务中心（进度真实、可取消）、凭据库、事件日志、批量操作、
  报告三件套（IOC / 社工 / TTP）
- 凭据获取对话框支持**三类制品**：类型下拉（LSASS / DPAPI / 浏览器）+ 技术随类型联动，
  制品列表按来源显示徽标，取回时按类型定扩展名

### 2.5 后渗透（凭据侧第二批，2026-10-18）
- **DPAPI 主密钥集**（`kind=dpapi`）：解密用户主密钥 → ZIP 制品 → 服务端逐条目校验后登记
- **浏览器凭据与 Cookie**（`kind=browser`）：Chrome/Edge 主密钥（DPAPI）→ AES-GCM 逐条解密
  （v10/v20/老式明文），**植入体内存内完成、明文不落盘**；服务端解析入库到凭据库
- 支撑件：自写纯 Go SQLite 只读读取器（含 WAL 覆盖、溢出页链），
  见 `Tshell-client/implant/core/sqlite/`；已知限制见 KNOWN_LIMITATIONS §12h
- 详见 `docs/CREDENTIAL_ACCESS.md` §7/§9

### 2.6 提权（阶段三，2026-10-18）
- `getsystem [auto|token|uac|uac_cleanup]`：令牌窃取（`CreateProcessAsUserW` 派生新会话）
  与 UAC 劫持（`fodhelper`/`computerdefaults`）两条路径，按痕迹从小到大自动尝试
- 只派生新进程，不动当前会话的令牌模拟状态：失败不炸会话，注册表劫持**必须还原**
  （删不掉时退化为清空值）
- 详见 `docs/PRIVILEGE_ESCALATION.md`（含 VM 验证清单与失败排查表）

### 2.7 工程与供应链
- CI 六 job：server（build / file-mode build / vet / **staticcheck** / race + 覆盖率 20%）、
  implant（13 传输交叉编译 + multi 标签用例 + staticcheck）、relay、postgres（service container）、
  reproducible（双次构建比对 + manifest + SBOM）、web（lint / type-check / vitest / build）
- **staticcheck 门禁**：`-checks=all,-ST*,-U1000`（排除风格与未使用符号，只留真实缺陷）
- 模板随仓库提供 → 克隆即可编译；`.gitattributes` 把模板钉成 binary（防 Windows autocrlf 写坏）

---

## 3. 未完成（按优先级）

工作量标注：**S** ≈ 半天~1 天，**M** ≈ 2~5 天，**L** ≈ 1~2 周，**XL** ≈ 1 个月+

### P0 —— 与 CS/Sliver 差距最大，直接影响实战

#### 3.1 凭据获取模块族（DPAPI / 浏览器 / Kerberos / BloodHound）— **L**
- **目标**：补齐 AD 攻击链的凭据侧：DPAPI 解密（`CryptUnprotectData`）、浏览器
  凭据与 cookie（Chrome/Edge 的 Local State + Login Data）、Kerberos 票据（list/purge/
  ptt）、BloodHound 采集器
- **现状（2026-10-18 更新）**：
  - ✅ LSASS 转储链路（`Tshell-server/pkg/cred/dump.go`、`minidump.go`、
    `Tshell-client/implant/core/services/cred_windows.go`）
  - ✅ **DPAPI 主密钥集**（`kind=dpapi`）：客户端解密 `Protect\<SID>` 下的用户主密钥、
    打包 ZIP 回传；服务端逐条目 CRC + manifest 校验后登记（`pkg/cred/zipcheck.go`）
  - ✅ **浏览器凭据与 Cookie**（`kind=browser`）：Chrome/Edge 的 `Local State` →
    DPAPI 主密钥 → `Login Data`/`Cookies` 逐条 AES-GCM 解密（**植入体内存内完成，
    明文不落盘**）；服务端解析后把口令/Cookie 写入凭据库
    （`pkg/cred/browser.go`、`pkg/api/cred_ingest.go`）
  - ✅ 支撑件：自写纯 Go SQLite 只读读取器（含 WAL），
    见 `Tshell-client/implant/core/sqlite/` 与 KNOWN_LIMITATIONS §12h
  - ⏳ Kerberos 票据、BloodHound 采集器、Firefox(NSS)、Chromium `Web Data`
- **入口**：
  - 客户端：`implant/core/services/cred.go`（按 kind 分派）、`browser*.go`、`dpapi_windows.go`、
    `core/sqlite/`（读取器）
  - 服务端：`pkg/cred/`（kinds/techs/形态校验/解析）、`pkg/api/cred.go`、`pkg/api/cred_ingest.go`
  - 前端：`Tshell-web/src/layout/Dialogs/CredentialDumpDialog.vue`、`src/api/cred/index.ts`
- **验收**：每个模块独立开关 + 独立单测；产物落 loot 表并按 uid/类型过滤可查；
  不落盘明文口令（内存解析 → 直接回传）—— 已满足（单测覆盖；实机验证待做）
- **Firefox（NSS）**：✅ **已完成并用真实 Firefox 产物端到端验证**（2026-10-19）
  - **先更正上一轮的一处误判**：上一轮实现了 PKCS#12 PBE 并以为"正是 key4.db 用的那套"。
    对照参考实现（`lclevy/firepwd` 的 `firepwd.py`，支持到 NSS 3.49 / Firefox 144）
    后确认**并非如此**。Firefox 实际用的算法是：
    | 路径 | 实际算法 |
    |---|---|
    | `logins.json`（旧） | 裸 `des-ede3-cbc`：主密钥直接当密钥，IV 取自条目 ASN.1 |
    | `logins.json`（Firefox 144+） | 裸 `aes-256-cbc`：主密钥截断到 32 字节，IV 取自 ASN.1 |
    | `key4.db`（NSS 3.49+） | **PBES2**：PBKDF2-HMAC-SHA256 + `aes-256-cbc` |
    | `key4.db`（更早） | Mozilla 自有 KDF（HMAC-SHA1 组合）+ 3DES |
    `pkcs12.go` 保留（它实现的是 RFC 7292 附录 B，本身正确且有 KAT），
    但**不要**拿它解 Firefox 数据 —— 这条已写进 `nss_crypto.go` 的文件头注释
  - `implant/core/services/nss_crypto.go`：上述四条路径的原语 ——
    `pbes2AES256CBCDecrypt`、`nssTripleDESCBCDecrypt`、`nssAES256CBCDecrypt`、
    `nssMozKDF`，以及 **NSS 的 IV 怪癖** `nssIVFromOctetString`
    （PBES2 参数里的 IV 只存 14 字节，NSS 实际是**从 DER tag 字节起读 16 字节**，
    即把 `04 0e` 当成 IV 前两字节；写成"补两个 0"会静默解不开）
  - `implant/core/services/firefox_nss.go`：容器解析与编排 ——
    DER 解析两种 PBE 容器、`metadata.item2` 口令校验链、`nssPrivate.a11` 主密钥、
    `logins.json` 条目（按条目内声明的 OID 选 3DES/AES）、
    跨平台 profile 目录发现（Windows/macOS/Linux/snap）
  - **验证方式（这一项的关键）**：向量取自 `unode/firefox_decrypt` 测试套件里的
    **真实 Firefox 配置**（`test_profile_firefox_nopassword_114` / `_59`），
    真值由该套件用 libnss 产出，期望明文取自其 `exported_passwords.csv`。
    实测两条 profile 的凭据**全部逐字节对上**，包括非 ASCII（`сЮЛОажс$4vz*VçàhxpfCbmwo`）、
    制表符与各类引号（`Apassword\twithtabs,;colonandsemi'"andquotes`）。
    那套夹具是 GPL-3，**未**随本仓库分发 —— 只嵌入从中抽出的数据字节
  - **真实数据抓到 4 个自造夹具抓不到的问题**：
    1. 真实表名是 `metaData`（驼峰），而 SQLite 标识符大小写不敏感、
       调用方习惯写 `metadata` —— 共享的 `core/sqlite` reader 原先是精确匹配，
       已改为按 SQLite 语义做 ASCII 大小写不敏感匹配
    2. `a11` 的明文**就是裸主密钥**（24/48 字节），不是 `SEQUENCE{OCTETSTRING}` 包装
    3. `password-check` 的比对形态取决于**填充是否已剥离**：firepwd 比的是带填充的
       `password-check\x02\x02`（它不去填充），本实现统一去填充，故比对 14 字节裸串。
       照抄参考实现的 16 字节常量会一直校验失败
    4. 夹具里有一条**故意损坏**的条目（用户名密文填充非法，对应参考套件的
       `handle_corrupted_passwords.t`）。原先实现会静默丢弃整条 —— 对凭据工具而言
       丢条目等于丢证据。已改为保留条目并标注**哪个字段**没解出来
  - 26 项密码学单测 + 9 项 Firefox 集成测试；13 种传输标签交叉编译通过
  - 另有 `TSHELL_FIREFOX_PROFILE=<dir>` 环境变量入口，指向真实 profile 可跑全链路
- **剩余**：Kerberos / BloodHound（需 Windows / 域环境）；
  Windows 实机验证（见 `docs/CREDENTIAL_ACCESS.md` §9）

#### 3.2 提权模块（getsystem / UAC bypass）— **M**
- **目标**：至少 getsystem（命名空间 + 令牌复制）+ 一种 UAC bypass（fodhelper 或
  computerdefaults）
- **现状（2026-10-18 更新）**：✅ 已实现 `getsystem [auto|token|uac|uac_cleanup]`
  - `token`：从已提权进程（winlogon/lsass/services/wininit/smss/explorer）复制
    SYSTEM/高完整性令牌 → `CreateProcessAsUserW` 派生**新会话**（痕迹最小，
    不动注册表）；`explorer.exe` 在候选里是因为"已提权过一次"的机器上它常持有
    高完整性令牌
  - `uac`：`fodhelper.exe`/`computerdefaults.exe` 注册表劫持 → 高完整性新会话 →
    **立即还原注册表**；删不掉时退化为清空劫持值（绝不留下可用劫持）
  - 只派生新进程、**不动当前会话的令牌模拟状态**：成功失败都不影响已有 beacon；
    新会话透传 `os.Args[1]` 启动口令（否则会因校验失败立刻退出）
  - 入口：`Tshell-client/implant/core/commands/privesc_windows.go`（编排 + 令牌/进程）、
    `privesc_uac_windows.go`（UAC 劫持 + 注册表）、`privesc_stub.go`（非 Windows）、
    `core/app/dispatch.go`（cmdType 132，跳过节流）
  - 协议：`Tshell-server/pkg/command/cmdType.go`、`pkg/sendcommand/sendCommand.go`；
    TTP 报告映射 `report.go`；控制台帮助 `BeaconConsole.vue`
- **验收**：失败不炸会话（可回滚）✅、成功后新会话/令牌可用 ✅（设计上）、
  有单测覆盖参数校验 ✅ —— 见 docs/PRIVILEGE_ESCALATION.md §6
- **剩余**：**Windows 实机验证**（VM 清单见该文档 §7）；`uac` 路径在较新 Windows 上
  是否仍可用需现场确认；UAC 之外的提权链（服务配置错误/AlwaysInstallElevated 等）未做

#### 3.3 内存 OPSEC 升级（Ekko 级）— **L**
- **目标**：(a) Ekko/Foliage 式 sleep mask（睡眠期加密**自身内存镜像**，用 ROP/定时器
  还原），(b) 调用栈伪造（返回地址指向合法模块），(c) 主密钥改为**首连时派生**而非编译期 patch
- **现状**：
  - sleep mask 只遮蔽**密钥材料**，不加密镜像；无 ROP 定时器（`implant/core/app/run.go:123`）
  - 调用栈伪造未实现（`spoofArgv` 只是 Linux 的 argv[0] 伪装，`core/commands/hide_linux.go:155`）
  - 主密钥仍是编译期 patch 进二进制（`Tshell-server/pkg/delivery/payload.go:122` 的
    `LISTENER_MASTER_KEY_...` 占位符），逆向单点
- **入口**：`implant/core/app/run.go`、新增 `core/sleepmask/`、`core/evasion/`；
  主密钥改造需服务端配合（`pkg/encrypt/`、`pkg/delivery/payload.go`）
- **验收**：睡眠期用自写内存扫描器看不到 beacon 镜像特征；主密钥不在二进制里可提取；
  现有 e2e（上线 + 命令往返）不回归
- **现状核查（2026-10-19）**：三个子项都卡在"需要实机 / 超大改动"，不宜在无验证条件下动：
  - **(a)** 现有 mask 已遮蔽会话密钥 / 上一把密钥 / 主密钥 / 盐 / AEAD 缓存
    （`core/encrypt/encrypt.go` 的 `MaskForSleep`），缺的是"加密自身镜像 + ROP 定时器还原"。
    **为什么不能盲写**：加密进程自身镜像一旦范围或时机算错，症状就是进程直接崩
    —— 没有 Windows 实机验证就提交，等于交付一个可能自杀的 sleep mask。
  - **(b)** 调用栈伪造**完全未实现**（现有 `hide_linux.go` 只是 argv[0] 伪装，属另一件事）。
    需要在 Windows 下构造合法返回地址链，必须实机验证。
  - **(c)** 主密钥目前是**明文 ASCII 按固定偏移 patch 进二进制**
    （`pkg/delivery/payload.go` 的 `LISTENER_MASTER_KEY_*` 占位符等长替换）。
    改成"首连时派生"要在**两端 13 种传输的首包**里插入 X25519 临时交换，服务端还要为
    轮询型传输缓存每个 beacon 的临时密钥直到会话建立 —— 这是一次协议迁移（XL），
    且必须用真实载荷端到端跑通才算数。
- **建议顺序**：先在授权 Windows VM 上做 (a)/(b)；(c) 单独作为一次协议迁移排期

### P1 —— 明显落后现代 C2

#### 3.4 水平扩展（多副本）— **XL**
- **目标**：k8s 的 `replicas`/HPA 真正可用
- **现状**：单实例（连接池/在线状态/会话锁/事件流/命令队列都在进程内存）；
  PostgreSQL 后端已可切（前置条件已具备）
- **本次决定（2026-10-19）**：走验收条款里的**第二条分支** ——
  明确声明单副本并删掉 HPA，而不是留一份"看起来能扩"的清单。
  - 新增 `deploy/k8s/tshell.yaml`（已入库）：`replicas: 1`、
    `strategy: Recreate`（滚动更新期间会同时存在两个 Pod，即使几秒也会双写 SQLite）、
    `ReadWriteOnce` PVC（误加副本时的物理防线）、PDB `minAvailable: 0`
    （如实表达无冗余，而不是让节点排空永远卡住）、**无 HPA**、
    探针用 `tcpSocket`（服务端没有未鉴权健康端点，而给 C2 面板新开一个
    未鉴权端点本身就是信息暴露面）
  - 顺带修掉旧草稿里两个会直接出事的配置：`algorithm: HS256`
    现在会被**拒绝启动**（只支持 RS256）；两副本挂同一 SQLite PVC 必然写坏库
  - `DEPLOYMENT.md` 的「单实例限制」指向新清单并说明为什么旧草稿不能套用
  - 13 项结构断言用 js-yaml 校验通过（replicas/策略/无 HPA/PDB/RS256/非 root/探针…）
- **真正的水平扩展仍需**（未做，XL）：把 `pkg/connection/`（在线状态、连接池）、
  `pkg/command/`（队列、事件总线）、`pkg/collab/`（会话锁）外置到 Redis/Postgres，
  并解决 SQLite 单写者限制 + 加 leader 选举。
  验收标准仍是"两副本经 LB 后任务不重不漏"

#### 3.5 平台自身安全补强 — **M**
- **MFA/TOTP**：✅ 已实现（2026-10-18）两步登录 + 自助绑定/关闭 + 管理员重置
  - 零新依赖自实现 RFC 6238（HMAC-SHA1/30s/6 位/±1 窗口）+ 自实现 otpauth 二维码
    （`pkg/security/otp`，含 RFC 官方向量与编解码往返测试）
  - TOTP 密钥 **AES-256-GCM 密文落库**，密钥由 JWT 私钥 HKDF 派生（`pkg/security/mfa.go`）
  - 两步登录用 `aud` 严格隔离：中间态令牌**不能**当访问令牌
    （`TestMFAPendingTokenAudienceIsolation` + `TestPendingTokenCannotAccessProtectedRoutes`）
  - 恢复码 10 个、bcrypt 存哈希、一次性、输入宽容（手抄场景）
  - 动态码**防重放**（同一时间步只能用一次）；动码输错**不计入**锁号
  - 入口：`pkg/api/mfa.go`、`pkg/api/users.go`、`pkg/security/{otp,mfa,jwt_manager}.go`、
    前端 `views/login/LoginView.vue` + `views/settings/MFAView.vue`（`account` 页签）
  - 详见 `docs/AUTH_MFA.md`（含 13 项集成测试清单与已知限制）
- **HSTS**：✅ 已设置（仅 HTTPS 请求下发；`security.force_hsts` 供反代场景显式开启）
- **`jwt.algorithm`**：✅ 从死配置变成真约束 —— 非 `RS256`/空值**拒绝启动**，
  默认值由 `HS256` 修正为 `RS256`（`pkg/config/manager.go` 的 `ValidateJWTAlgorithm`）
- **剩余**：IP 白名单默认关闭属部署选择（不改默认值）；
  防重放状态在内存里（多副本需外置，属 §3.4）

#### 3.6 per-listener C2 画像 — **M**
- **目标**：每个监听器可绑定独立画像（URI / WS 路径 / DNS 前缀 / padding）
- **现状（2026-10-19 更新）**：✅ 已实现
  - `listener` 表新增 `profile` 列（迁移 v4，默认空串 = default，**老库行为不变**）
  - `registerHTTPRoutes` / WS 路径按**该监听器绑定的画像**注册（原来用 `GetDefault()`，
    WS 路径硬编码 `/ws`）
  - 生成载荷的一致性基准改为**该监听器的画像**：用画像 A 的监听器生成画像 B 的载荷
    仍会被拒绝（避免生成必然连不上的载荷），但用画像 A 生成画像 A 的载荷可以通过
  - 同机可并存多套特征：两个监听器各绑不同画像，各自的 mux 互不干扰
    （`TestTwoListenersDifferentProfilesCoexist`）
  - 画像缺失时**回落 default 并告警**，而不是拒绝启动（升级路径不能因为少个文件就挂）
  - 入口：`pkg/api/listener.go`（`registerHTTPRoutes`/`resolveWSRoute`/`resolveListenerProfile`）、
    `pkg/api/server.go`（`listenerProfileForPayload`/`profileListenerMismatch`）、
    `pkg/c2profile/manager.go`（`DefaultProfileName`/`Default*` 常量）、
    前端 `EditListenerDialog.vue` 的画像下拉
- **验收**：用非 default 画像生成载荷能连上 ✅（路由注册与校验基准的测试）；
  `docs/KNOWN_LIMITATIONS.md` 相应条目已勾掉

#### 3.7 领域 repository（把 SQL 收口）— **M**
- **目标**：`pkg/api` 里不再有裸 SQL / xorm 链式查询，改成 `pkg/database` 的领域方法
- **现状（2026-10-19 更新）**：✅ 已完成
  - 业务代码里 `database.DB()` 的**直接查询从 233 处降到 0**（跨 24 个文件）
  - 领域层三个文件：`domain_core.go`（监听器/客户端/备注/下载/命令历史/战利品）、
    `domain_ops.go`（投递/访问日志/隧道/仪表盘/SOCKS5/任务/插件/stager 令牌）、
    `domain_user.go`（账号 + MFA 三列集中列名）
  - 统一哨兵错误 `ErrStoreUnavailable`：业务层用 `errors.Is` 区分环境问题与数据问题
  - 棘轮测试 `store_access_test.go`：**上限 0**，业务代码再出现直接查库即 CI 红
    （只统计非 `_test.go` —— 测试造数据直接读库是正当手段）。
    只留两个例外：`DB() == nil` 就绪判定、`DB().NewSession()` 跨语句事务
- **收口过程中修掉的真问题**（均有测试钉住）：
  1. `report.go` 的 `Desc("i_d")`：该表主键列在标签里被钉成 `'id'`，`i_d` 不存在，
     查询一直报错而调用方按 `err == nil` 跳过 —— 社会工程报告的"点击列表"**永远是空的**
  2. 客户端删除的级联删除**漏了 loot 表**（两处实现一处删 4 张、一处删 3 张，
     都会残留指向已删除 uid 的孤儿战利品行）→ 统一到 `PurgeByUID`
  3. `UpdateAllCols` 一度从 `l.Id` 取主键，而调用方构造的更新结构体 Id 是零值
     → 更新零行却返回成功（"保存了但没生效"）→ 改成 id 显式传参
  4. 投递端口冲突的"四种地址变体拼接"从调用处的 SQL 字符串收进方法并加单测
- **入口**：`pkg/database/domain_*.go`、`pkg/api/*.go`

### P2 —— 体验与整洁度 / 需要实机

#### 3.8 实机验证（Windows / macOS）— **M**
- 代码路径齐全但**未在真机验证**：Windows（ConPTY 交互终端、持久化、注入、ETW/AMSI、
  截屏/HVNC）、macOS（LaunchAgent/LaunchDaemon 持久化、PTY、截图、监控拉取）
- 入口：见 `KNOWN_LIMITATIONS.md` §12e；建议先跑一遍"上线 → 命令往返 → 文件 → 终端 → 截屏"

#### 3.9 中继能力补齐 — **S~M**
- **DNS 中继**：✅ 已实现（2026-10-19）。中继看懂 DNS 报文头（只读 12 字节，不解析记录段）：
  逐查询**改写事务 ID**（多 beacon 共用中继时 ID 撞车会让响应串台）、
  UDP 响应带 **TC 位时自动改走 TCP** 并清 TC、同端口 **UDP+TCP 双监听**
  （只做 UDP 会在长 TXT 回传上失败）。入口 `Tshell-relay/dns.go`。
- **多上游**：✅ 已实现。`upstreams` 数组启用轮询 + 故障转移；
  健康判定**被动（连续 3 次失败移出）+ 主动（后台探测不健康的上游，成功即恢复）**；
  全部不健康时仍轮转尝试（否则永远无法被动恢复）；`/healthz` 暴露上游健康度。
- 验收：10 项单测覆盖 ID 改写/撞车/TC 回退/故障转移/阈值抗抖动/配置回落，
  见 `docs/RELAY.md` §6–§8

#### 3.10 供应链 — **M**
- **LOLBin 启动方式目录**：✅ 已实现（2026-10-19）。`pkg/delivery/launcher.go`
  - 此前只有"下载并直接执行"（IWR/certutil/bitsadmin/curl），**没有"落地之后怎么起"** ——
    而直接起 payload.exe 意味着父进程就是调用它的 shell、控制台可能闪窗、
    某些环境只放行特定签名 LOLBin
  - 11 种方式：`direct` / `conhost --headless`（无窗口，Win11+）/ `forfiles`（父进程伪装）/
    `pcalua` / `wmic process call create` / `explorer` / `schtasks` /
    `rundll32`（DLL）/ `regsvr32`（DLL）/ `regsvr32` Squiblydoo（远程 SCT）/
    `mshta`（远程 HTA）
  - 每种都声明：支持哪种载荷形态、换来什么（Buys）、前提与坑（Caveats）
  - **形态不匹配明确拒绝**，不给一条点了必然失败的命令；
    `shellcode` 没有启动方式（它不是可执行文件），返回解释说明需要注入器
  - 显式记录 installutil/msbuild/regasm/regsvcs **对本项目原生 PE 无效**及原因 ——
    不写清楚运维会照着公开资料白试一遍
  - 两个容易写错的点已编码并单测：`forfiles /c` 的内层引号必须写 `\"`；
    含空格路径/以反斜杠结尾的路径的引号与反斜杠翻倍规则（`CommandLineToArgvW` 语义）
  - API：`/webdelivery/commands` 一并回 `launch_methods` / `launch_note` /
    `launch_unsupported`（形态按保存的文件名后缀推断，因为 `format` 没落库）
  - 测试 27 项
- **代码签名**：仍未做 —— 需要一张 Windows 代码签名证书（EV/OV）。
  签名本身是 `signtool`/`osslsigncode` 的外部步骤，接入点在
  `build_templates.ps1` 与 `pkg/delivery/payload.go` 生成之后；
  没有证书无法验证签名结果，因此没有盲写。

#### 3.11 前端债 — **M**
- **Pivot 拓扑图**：✅ 已实现（2026-10-19）。新增 `views/graph/`：
  - `layout.ts` 分层布局（server→listener→client→tunnel→target）。**不用力导向**：
    这份数据本身是分层的，而且力导向每次刷新坐标都不同 —— 同一个拓扑看起来像变了张网；
    另外不引第三方图库（为一个视图拉进几百 KB 不划算）
  - `PivotGraphView.vue`：SVG 渲染 + 图例 + 点击节点看 meta + 相关边高亮；
    离线客户端画成灰色虚线空心（否则一片点看不出谁掉线）；
    403 给出"需要 operator 及以上"的页面内提示（`/graph/data` 是 operatorOnly，不该弹全局报错）
  - 入口：隐藏路由 `/graph`（顶栏固定 8 项，加第 9 项会挤掉布局）+ 顶栏工具栏按钮
    （i18n 里 `tb.pivot_graph` 就是为它准备的）；按钮可见性直接读路由表的 permiss，避免两处漂移
  - 测试 21 项（布局 11 + 组件 10）
- **报告导出**：✅ 已加 CSV（2026-10-19）。`views/reports/export.ts`：
  - 三个报告都能导出 CSV（Excel 直接打开）+ 保留 JSON
  - **CSV 公式注入防护**：报告字段（主机名/User-Agent/文件名/命令）来自被控端可控输入，
    以 `=` `+` `-` `@` 开头的单元格在 Excel/WPS 里会被当公式执行 —— 导出前加前导单引号
  - RFC 4180 转义（含逗号/引号/换行的字段）＋ UTF-8 BOM（否则 Excel 中文乱码）
  - 测试 21 项
- **i18n 内联三元**：✅ 已清零（2026-10-19，1655 → 0）
  - 新增 `i18nStore.tl(zh, en)`：**机械可迁移**是它存在的首要理由 ——
    1600+ 处内联三元形态统一但 grep 不出"哪些是待翻译文案"，`tl(` 把每一处都标出来
  - 更关键的是它给**字典化留了升级点**：以后把 tl 改成"先查 dict、查不到回退字面量"，
    调用点一处都不用动。把迁移从"一次大爆炸"变成可增量
  - 两个迁移脚本（`scripts/migrate-i18n.cjs` / `-pass2.cjs`，可重复运行的 dry-run 模式）：
    第一遍吃"单行单引号纯字面量"1616 处，第二遍吃成员访问（`x.zh/x.en`）与模板字符串 25 处，
    剩下 14 处人工（嵌套三元、字符串拼接、含嵌套插值的模板）
  - **刻意不动的 6 处**：`api/cred/index.ts` 与 `stores/screenshots.ts` 里 `isZh` 是
    **函数参数**（它们不是组件，把语言作为显式参数传入是正确设计），
    改成 store 调用反而破坏可测试性
- **`as any`**：63 → **32**（2026-10-19）
  - 消掉的三批都是"用类型声明代替断言"的真实改进：
    axios 配置扩展（`declare module 'axios'`，字段名拼错现在会编译失败，
    此前 `as any` 会让 `metadata` 写成 `metaData` 静默拿到 undefined）、
    `performance.memory` 显式接口、厂商前缀全屏 API 的 `declare global`（10 处 → 1 处声明）
  - 顺带修掉一处纯噪音：`(injectedUid as any).value` —— 它的类型本来就是
    `Ref<string> | ComputedRef<string>`，两者都有 `.value`，断言毫无必要
- **棘轮守卫**：`src/techdebt.test.ts` 钉住两条 ——
  组件内联三元上限 **0**（清零后不允许回涨）、`as any` 上限 **32**（只许降）。
  另有第三条钉住 `tl` 按 locale **真的分支**：它若退化成恒返回中文，
  1600 处调用点的语言切换会静默失效而没人立刻发现
- **大文件拆分**：已开始（2026-10-19）
  - `views/client/View/ClientFiles.vue` 2213 → 2128 行：把**纯格式化/解析逻辑**
    抽到 `views/client/files/format.ts`（32 项测试）。选这批先抽的理由：
    ① 该视图此前**没有任何测试**，在没安全网时动最大的文件是危险的；
    ② 这些纯函数边界最多、最容易出错（大小换算、秒/毫秒分界、DD/MM 与 YYYY/MM 两种
    日期形态、`<DIR>`/`-`/`undefined` 三种占位符）；
    ③ 同类逻辑原本有重复实现（`formatFileSize` 与 `formatUploadFileSize` 几乎一样），
    收口后才能保证"显示的大小"与"解析出的字节数"互相对得上（已有来回换算测试钉住）
  - **抽取只做搬运，不顺手"修"不一致**：`getFileIconClass` 与 `getFileNameClass`
    的扩展名表本来就不一样（前者含 dylib/vbs、有 zip 分支，后者没有），
    统一会让 .dylib/.vbs 在文件名列突然变色 —— 属可见的视觉变更，应单独提一次，
    而不是夹在"重构"里悄悄发生。这条已写进代码注释与测试
  - `views/plugins/PluginsView.vue` 1948 → 1793 行：把两个**输出过滤函数**
    （合计约 170 行纯字符串处理）抽到 `views/plugins/output-filter.ts`（26 项测试）。
    选它的理由：这是该视图最长最绕的逻辑，且直接决定操作员看到什么 ——
    过滤错了就是"执行成功但看不到结果"，而此前同样没有一行测试
  - **组件级测试已补**（2026-10-19）：`PluginsView.test.ts` 4 项 ——
    挂载加载列表、选中后加载详情并渲染字段、渲染无 "is not a function"、
    未选模式时按钮文案。这几项专盯"抽出去的东西单测很足、**调用点却没人管**"
    这个空档：`output-filter.ts` 有 26 项单测，但组件把 mimikatz 路径接到通用过滤器、
    或 `filterOpts` 传错提示文案，单测全绿而页面行为是错的
  - 写这个测试时踩到 4 个"看起来像渲染问题"的坑，都记在测试注释里：
    ① API 方法名是 snake_case（`get_plugins` 而不是 `getPlugins`）——
       名字写错会被组件 catch 掉，只表现为"列表是空的"
    ② 插件名渲染在 `el-select` 的**下拉里**（teleport 到 body），`wrapper.text()` 取不到
    ③ 页面上有多个 `el-select`，`findComponent` 只返回第一个，必须按 placeholder 挑
    ④ `handlePluginChange` 读的是 v-model 值，只发 `change` 不发 `update:modelValue`
       会让函数直接 return，表现为"详情接口没被调用"
  - `ClientFiles.test.ts` 4 项（2026-10-19）：挂载拉磁盘列表、渲染无报错、
    **图标/名称类名确实来自 format.ts**、文件大小走 `formatFileSize`（1KB→"1.0 KB"）。
    最后两项是重点：`format.ts` 自己有 32 项单测，但组件若又留一份本地实现，
    单测全绿而页面走旧逻辑 —— 只有挂载组件才看得出来
  - **仍未做**：两个大文件的**模板**（ClientFiles 517 行 / PluginsView 295 行）
    与其余编排逻辑（API 调用、轮询、拖拽缩放）尚未拆。两个视图现在都有组件级安全网了

---

## 4. 回归基线（每次改完跑这些）

```bash
# 服务端（在 Tshell-server/ 下）
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 -checks=all,-ST*,-U1000 ./...
go test -race ./... -count=1 -coverprofile=/tmp/cover.out   # 覆盖率门禁 20%
go tool cover -func=/tmp/cover.out | tail -1

# 植入体（在 Tshell-client/implant/ 下）
for t in tcp kcp http https oss websocket dns tcp_forward kcp_forward ws_forward \
         http_forward https_forward multi; do
  go build -tags transport_$t -o /dev/null ./cmd/single || echo "FAIL $t"; done
go test -tags transport_multi ./cmd/single/ -count=1
go test -race ./... -count=1

# 中继 / 前端
( cd Tshell-relay && go test -race ./... -count=1 )
( cd Tshell-web && npm run lint && npm run type-check && npx vitest run && npm run build )

# 端到端与可复现
bash scripts/e2e.sh                 # 若脚本可用（依赖 debug 构建日志行）
./scripts/repro-build.sh --check --out /tmp/repro
```

CI 会在 push 时跑同一套门禁（`.github/workflows/ci.yml`，六个 job）。

另外，根目录的 [`../OPTIMIZATION_CHECKLIST.md`](../OPTIMIZATION_CHECKLIST.md) 是**回归检查清单**
（逐条 grep 验证既有优化是否还在位，经历过一次大面积回滚后建立的），
改动涉及关键路径时顺手跑一遍，与本文件的"待办"是互补关系：
**本文件说"接下来做什么"，那份清单说"别把已有的做没了"。**

---

## 5. Windows 环境注意（复制过去后照做）

1. **必须重建的**：`Tshell-server/tshell-server` 是 Linux ELF（653MB）、
   `Tshell-web/node_modules` 是 Linux 装的（esbuild/rollup 有平台相关二进制）→
   Windows 上 `npm ci --legacy-peer-deps` 重装、用 `build.ps1` 重建服务端。
2. **必须带过去的**（`.gitignore` 忽略、不在版本库里）：`config.yaml`（含密钥/口令）、
   `database.db`、`jwt_private.pem`/`jwt_public.pem`/`jwt_state.json`、`token.txt`。
   想保留现有库与账号密码就复制文件，不要只 clone。
3. **模板已入库**（587MB），clone 即可编译；只有改了植入体才需要
   `powershell -File build_templates.ps1`（全矩阵，耗时较长）。
4. **依赖**：Go ≥ 1.25（implant `go.mod` 要求 1.25，服务端 1.24）、Node ≥ 18、
   TDM-GCC（仓库根目录有 `tdm64-gcc-10.3.0-2.exe`）。
   ⚠️ **不要在没确认的情况下跑 `go mod tidy`**（在 `Tshell-client/implant/` 下）：
   它会把 `go` 指令顶到 1.26、并把 `golang.org/x/*` 升到要求 Go 1.26 的版本，
   CI（Go 1.25）会立刻构建失败。需要整理依赖时：设 `GOTOOLCHAIN=local`，
   并把 `x/sys v0.39.0 / x/text v0.32.0 / x/crypto v0.46.0 / x/net v0.48.0 /
   x/sync v0.19.0 / x/time v0.14.0 / x/mod v0.31.0 / x/tools v0.40.0` 钉住。
5. **构建顺序**：`build_templates.ps1` → `npm run build:embed` → 服务端**文件模式**
   `go build -o tshell-server.exe .\main.go`（包模式在 TDM-GCC 下可能产出跑不起来的 cgo exe）。
   内存紧张加 `GOMEMLIMIT=3GiB`。
6. **不要**在 `Tshell-server/` 等子目录里跑 git（有嵌套 `.git`，会拿到上游的旧 tag/commit）。
7. 模板是二进制：`.gitattributes` 已声明 `binary`，避免 `core.autocrlf=true` 把 gzip 流写坏。
   同一文件里也锚定了 `Tshell-client/implant/core/sqlite/testdata/**`（SQLite 样本 + WAL 也是二进制）。
8. **Windows 本机跑测试的既有失败**（平台语义差异，非回归；CI 的 Linux job 通过）：
   - `-race` 链接失败（TDM-GCC 缺 `WaitOnAddress`/`WakeByAddressSingle` 符号）→ 本机用
     `go test ./...` 验证，race 门禁交给 CI；
   - 权限类断言：`TestInsecurePermPaths`、`backup.TestCreateProducesVerifiedSnapshot`
     （Windows 下 `0600` 报出来是 `666`/`-rw-rw-rw-`）；
   - symlink 类：`cna.TestBuiltinOpen*`（需要开发者模式或管理员）；
   - 端口占用判定：`proxy.TestStartTunnelFailsWhenAgentPortBusy`；
   - 文件占用清理：`logx.TestSetOutputFile`（Windows 不允许删除仍被打开的文件）。

---

## 6. 相关文档索引

| 文档 | 内容 |
|---|---|
| [`KNOWN_LIMITATIONS.md`](./KNOWN_LIMITATIONS.md) | 逐条已知限制与状态（✅/⏳） |
| [`STORAGE.md`](./STORAGE.md) | 存储后端、方言层、Store 访问层 |
| [`OBSERVABILITY.md`](./OBSERVABILITY.md) | 指标、日志、告警 |
| [`MEMORY_OPSEC.md`](./MEMORY_OPSEC.md) | sleep mask 与内存 OPSEC 现状 |
| [`CREDENTIAL_ACCESS.md`](./CREDENTIAL_ACCESS.md) | 凭据获取链路 |
| [`BACKUP_RESTORE.md`](./BACKUP_RESTORE.md) | 备份/恢复与演练 |
| [`REPRODUCIBLE_BUILDS.md`](./REPRODUCIBLE_BUILDS.md) | 可复现构建与 SBOM |
| [`REDIRECTOR.md`](./REDIRECTOR.md) | nginx/apache 重定向器模板 |
| [`RELAY.md`](./RELAY.md) | 多级中继部署 |
| [`../DEPLOYMENT.md`](../DEPLOYMENT.md) | 构建、配置、部署、单实例约束 |
| [`../OPTIMIZATION_CHECKLIST.md`](../OPTIMIZATION_CHECKLIST.md) | 既有优化的 grep 回归检查清单 |

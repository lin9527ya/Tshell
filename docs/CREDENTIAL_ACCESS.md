# 凭据获取（阶段三）

> 状态：**三类制品（LSASS 转储 / DPAPI 主密钥集 / 浏览器凭据与 Cookie）的服务端链路、
> 协议与客户端实现均已完成并有单测覆盖；Windows 端尚未在真实目标上验证**
> （见 §6 与 §9 的"验证状态"）。

## 1. 链路（以 LSASS 模块为例）

```
操作员（会话行 → 凭据获取）
   │  POST /api/cred/dump {uid, kind=lsass, tech}
   ▼
服务端 pkg/api/cred.go
   ├─ 校验：会话存在 / Windows / **类型与技术的组合合法** / 未被其他操作员占用
   ├─ 建任务 kind=cred（谁/何时/对哪台机器/用了什么技术）
   ├─ 登记"等待回传的制品"（uid + 期望文件名 + 形态 + 任务 + 操作员 + 30 分钟有效期）
   └─ 下发 CRED_DUMP 帧：cmdBuf = "kind|tech|fileName"
   ▼
客户端 core/services/cred.go（Windows）
   ├─ 按 kind 分派：lsass 转储 / dpapi 打包 / browser 内存解密
   ├─ 制品落到 %TEMP%\<fileName>
   ├─ 经**下载通道**回传（复用 v2 下载协议：type22 头 + type26 分块）
   └─ 回传结束后删除目标机上的制品文件
   ▼
服务端下载完成回调 → matchCredArtifact
   ├─ 按文件名精确匹配到那次请求（匹配不上就按普通下载处理，不错认）
   ├─ 按制品形态校验：minidump（MDMP 签名/数据流）| ZIP（逐条 CRC + manifest）| JSON（schema + 条数）
   ├─ browser 制品额外解析：明文口令/Cookie 写入凭据库（loot）
   ├─ 成功 → 任务 completed + 登记制品（loot）+ 审计 + 事件 + 指标
   └─ 失败 → 任务 failed，**不登记制品**（半截文件/空文件绝不当作"已获取"）
```

## 2. 为什么服务端不解析转储

解析 LSASS 转储 = 重新实现 mimikatz 的 sekurlsa（PE 解析、内存描述符、LSA 密钥、
WDigest/Kerberos/DPAPI 结构）。服务端做这件事收益低、维护成本高，而且**没有任何
安全收益** —— 解析结果最终还是要给操作员看。

服务端负责的是三件更有价值的事：
1. **归属**：制品属于哪次请求、哪个任务、哪个操作员；
2. **真实性**：制品是不是一个完整可用的转储（空文件/半截文件必须显式失败）；
3. **可审计**：谁在什么时候对哪台机器发起过凭据获取（`CRED_ACCESS` 审计 + 指标）。

解析在操作员本地做：

```bash
pypykatz lsa minidump lsass-DC01-20261018-120000.dmp
# 或 mimikatz: sekurlsa::minidump <文件>  →  sekurlsa::logonpasswords
```

## 3. 技术选型（PPL 是分水岭）

现代 Windows 上 LSASS 通常以 **PPL（Protected Process Light）** 运行，未签名进程
即使有 SeDebugPrivilege 也读不到它的内存。三条路的差别就在这里：

| tech | 做法 | 对 PPL | 痕迹 |
|---|---|---|---|
| `comsvcs` | 签名进程 `rundll32.exe` 调用签名库 `comsvcs.dll!MiniDump` | **有效**（转储由签名进程完成） | 长：子进程 + 磁盘落盘，EDR 覆盖好 |
| `dbgcore` | 自身进程内调用签名库 `dbgcore.dll!MiniDumpWriteDump` | 被拒（调用方未签名 → ACCESS_DENIED） | 短：无子进程，行为链短 |
| `auto`（默认） | 先 comsvcs，失败回退 dbgcore | 视目标而定 | 由实际走通的那条决定 |

选择建议：目标是普通工作站（LSASS 未受保护）时 `dbgcore` 更安静；域控等开启 PPL 的
目标只能用 `comsvcs`，并接受它的痕迹。

## 4. 失败是显式的

凭据获取最怕"看起来成功了"。这条链路上每一处失败都会显式落到任务终态：

| 失败点 | 表现 |
|---|---|
| 非 Windows 会话 | 接口 400，不下发（不让操作员对着 Linux 会话等一个必然失败的任务） |
| 会话被其他操作员占用 | 409（凭据获取是最不该撞车的操作） |
| 下发通道不可用 | 503 + 任务 failed + 撤销登记（不留"等待中"的幽灵） |
| 客户端权限不足 / PPL 拒绝 | 客户端返回明确错误（非 Windows 桩同样返回错误而非空路径） |
| 转储写出 0 字节 | 客户端本地即拒绝，不发起回传 |
| 回传中断（断线） | 任务 failed |
| 回传的文件不是完整 minidump | 任务 failed + **不登记制品** |
| 30 分钟未回传 | 登记过期，不再把后来的同名文件认作该请求的产物 |

## 5. 操作入口

- **会话行 → 更多 → 凭据获取（LSASS）**：选技术、看"等待回传/已获取制品"、取回制品。
  弹窗明确写着"服务端不做解析"与本地解析命令。
- `POST /api/cred/dump`（admin/operator）
- `GET /api/cred/artifacts?uid=`（admin/operator）
- 制品本体在服务端 `Downloads/<客户端ID>/` 下，用文件下载接口取回。
- 指标：`tshell_cred_artifacts_total{kind}`；审计动作：`CRED_ACCESS`。

## 6. 验证状态（重要）

**已在本仓库验证**（`go test -race ./...` 全绿）：

- 请求登记与匹配：精确匹配、不同 uid 不匹配、同名请求拒绝、过期清理、取消；
- 制品校验：有效转储通过、空文件/短文件/签名不符/零流/缺系统信息流/目录越界/流越界/
  异常目录条数全部拒绝；
- 归属链路：回传完成后任务置 completed、登记 `loot(type=dump, source=lsass)`、
  待回传记录清空；无效制品 → 任务 failed 且不登记；回传中断 → 任务 failed；
  无关文件回传不误认；
- 接口：参数校验（uid 缺失/会话不存在/非 Windows/类型与技术非法）、
  下发失败时回滚登记与任务；
- 客户端：命令参数校验（含路径穿越拒绝）、非 Windows 桩显式报错；
- 交叉编译：windows/amd64、linux/amd64、darwin/amd64 全部通过。

**尚未验证**（需要真实 Windows 目标，本仓库的 CI 只能交叉编译）：

1. `comsvcs` 路径在开启 PPL 的目标上是否真的成功（rundll32 参数形式、
   `full` 参数在新版 Windows 上的行为）；
2. `dbgcore` 路径的返回码判定（`MiniDumpWriteDump` 在部分失败场景下返回 0 但
   仍写出部分内容 —— 目前靠"文件大小为 0 则拒绝 + 服务端 minidump 校验"兜底，
   服务端校验会拦住半截转储，但客户端侧的失败原因不如直接看返回码清晰）；
3. 大转储（数百 MB ~ 1GB）经下载通道回传的耗时与稳定性（分块 2MB，
   轮询型传输下会持续多个回连周期）；
4. 目标机上 `%TEMP%` 的可用空间（转储体积与 LSASS 内存相当）。

建议的首次验证方式：在一台**授权的**测试虚机上按 §1 链路走一遍，
对照 `CRED_ACCESS` 审计与任务终态确认每一步。

## 7. 第二批模块：DPAPI 主密钥与浏览器凭据（本轮实现）

在第一批（LSASS）的制品管线之上加了两个模块。二者与 LSASS 的**关键区别**是
制品内容已经是明文/密钥，因此服务端不只是"校验形态"，还多做一步解析入库。

| kind | 制品 | 形态 | 客户端做什么 | 服务端做什么 |
|---|---|---|---|---|
| `lsass` | 内存转储 | `.dmp` | 转储落盘 → 回传 → 删除 | 只校验 minidump 形态（不变） |
| `dpapi` | DPAPI 主密钥集 | `.zip` | 逐个解密 `Protect\<SID>` 下的主密钥，打包 manifest + `keys/<GUID>.json` | 校验 ZIP（逐条读、CRC、必须有 manifest），登记 `loot(type=dump, source=dpapi)` |
| `browser` | 口令与 Cookie | `.json` | 内存内 DPAPI→AES-GCM 解密，只写明文 JSON | 解析 JSON：口令入 `loot(type=pwd, source=browser:<浏览器>)`、Cookie 入 `loot(type=cookie)`；制品本体留档 |

### 7.1 浏览器凭据的解密链路

```
Local State（os_crypt.encrypted_key, base64("DPAPI"+blob)）
  │  CryptUnprotectData（当前用户作用域）
  ▼
32 字节浏览器主密钥
  │
  ├─ Login Data / logins.password_value   ─┐
  └─ Network/Cookies / cookies.encrypted_value ─┤ 按前缀选算法：
                                                │   v10 → AES-256-GCM（12B nonce + 密文 + 16B tag）
                                                │   v20 → AES-256-GCM，密钥 = SHA256(32B 前缀 || 主密钥)
                                                │   其他 → 按明文（老格式）
                                                ▼
                                          明文口令 / Cookie 值
```

要点：

- **明文不落盘**：解密全在植入体内存里完成，只有汇总后的 JSON 短暂落在 `%TEMP%`，
  回传完成后删除（`exfilFile(path, true)`）。
- **不引依赖**：读 `Login Data` / `Cookies` 用自写的纯 Go SQLite 只读读取器
  （`implant/core/sqlite/`）。植入体是 `CGO_ENABLED=0` 全矩阵交叉编译，
  引 SQLite 驱动会让 141 个模板每个胀大数 MB。
- **读取器带 WAL 支持**：Chromium 系默认 `journal_mode=WAL`，最新写入常常只在
  `-wal` 里；不读 WAL 会稳定漏掉最新数据。
- **库被独占也能读**：先 `os.Open` 直读，失败则复制副本再读（副本只含密文）。

### 7.2 DPAPI 主密钥制品的用途

主密钥是**通用开锁器**：拿到 `masterkey` 后，同一用户的其他 DPAPI 数据都能离线解开
（Credential Manager、Vault、保存的 RDP 凭据、EFS 私钥、以及浏览器主密钥本身）。
因此制品里同时给出明文 masterkey（base64）与 `manifest.json`（SID/用户/主机/时间）。

解析示例：

```bash
# 从制品里取 masterkey 后
pypykatz dpapi masterkey <GUID> <masterkey-base64>   # 或用 impacket 的 dpapi 模块
```

### 7.3 操作入口与接口

- 会话行 → 更多 → **凭据获取** → 选类型（LSASS 内存转储 / DPAPI 主密钥集 /
  浏览器凭据与 Cookie）→ 技术随类型联动（`auto|comsvcs|dbgcore` / `keys` /
  `all|chrome|edge`）。
- `POST /api/cred/dump {uid, kind, tech}`；`GET /api/cred/artifacts?uid=`（三类制品都返回）。
- 凭据库新增 `cookie` 类型（前端过滤 pill 对齐 `Tshell-server/pkg/api/loot.go` 的枚举）。

### 7.4 失败是显式的（第二批新增行）

| 失败点 | 表现 |
|---|---|
| 类型与技术误配（如 `browser` + `comsvcs`） | 服务端 400 列出可选技术；客户端再拒一次 |
| 浏览器未安装 / 无 `Local State` | 该浏览器记入制品 `errors[]`；全部不可用时任务 failed |
| `Local State` 的 DPAPI 解密失败（换了用户上下文） | 该浏览器整体失败并在 `errors[]` 说明，不产出半截数据 |
| 单个口令/Cookie 解不开 | 跳过该条并计入 `errors[]`（GCM 认证失败说明密钥不符或数据被改） |
| 制品 JSON 非法 / 无任何可用条目 | 服务端任务 failed + 不登记制品（与 LSASS 同语义） |
| 制品超限（>8MB / 口令 >2000 / Cookie >20000） | 服务端拒收；客户端采集时先截断并在 `errors[]` 说明 |
| ZIP 截断 / CRC 不符 / 缺 manifest | 任务 failed + 不登记（逐个条目读一遍才算完整） |

## 8. 后续模块（阶段三 · 第 13 项剩余）

| 模块 | 说明 | 复用点 |
|---|---|---|
| DPAPI 凭据/主密钥 | ✅ 本轮实现（见 §7） | 制品类型 `dpapi` |
| 浏览器凭据 / Cookie | ✅ 本轮实现（见 §7） | 制品类型 `browser` |
| Kerberos 票据（文件式） | ✅ 2026-10-19 实现（见 §10）：采集 MIT ccache 文件 | 制品类型 `ticket` |
| BloodHound 采集 | 域对象与 ACL 采集，产出 JSON | 制品类型加 `bloodhound` |
| 提权模块 | getsystem（命名管道模拟）/ UAC 绕过 | 属"动作"类，不需要制品管线 |
| 端口扫描 | ✅ 已实现（TCP connect 扫描，跨平台）：`scan <目标> <端口> [超时毫秒]` | 结果直接作为命令回显 |
| Firefox / NSS 凭据 | ✅ 已实现并接线（`tech=firefox`） | 复用 `browser` 制品的 JSON 结构 |
| Chromium `Web Data` | ✅ 2026-10-19 实现（`tech=cards`，见 §11）：信用卡入库、自动填充仅留制品 | 复用 `browser` 管线 |

服务端侧每加一个模块只需：`pkg/cred` 的 `techsByKind` 加一项、客户端加一个取法、
前端 `CRED_TECHS` 加一项 —— 登记/校验/归属/审计链路完全复用。

## 9. 第二批的验证状态

**已在仓库内验证**：

- 服务端：类型×技术矩阵（合法/误配/未知）、ZIP 校验（完整/空/截断/CRC 错/缺 manifest/条目超限）、
  浏览器 JSON 校验（合法/空/截断/无可用条目/体积与条数超限）、入库字段与去重
  （口令按 `(uid,type,host,secret,source)` 去重）、制品列表覆盖三类来源；
  `go test -race ./pkg/cred/... ./pkg/api/...` 全绿。
- 客户端：v10/v20/明文三分支解密、篡改与截断必须报错、`Local State` 各类畸形输入、
  制品 JSON 字段与服务端 schema 对齐、类型×技术矩阵、非 Windows 桩显式报错；
  `go test ./core/services/... ./core/sqlite/...` 全绿。
- 交叉编译：windows/amd64（真实现）与 linux/darwin（桩）全部通过，13 种传输标签全绿。
- 真实库只读验证：`TestRealChromeDatabase` 直接读本机 Chrome/Edge 的
  `Login Data` / `Cookies`（存在时执行，否则跳过）。

**尚未验证**（需要授权 Windows 目标）：

1. `CryptUnprotectData` 在真实用户上下文里解出的主密钥长度与可解性
   （本机开发环境下浏览器库里没有已保存的口令，无法覆盖"有数据"的分支）；
2. v20（App-Bound Encryption）在较新 Chrome 上是否仍需附加参数
   —— 若目标浏览器启用了 App-Bound，纯 `CryptUnprotectData` 可能被拒；
3. `Login Data` 被浏览器独占时的副本回退路径；
4. 大 Cookie 库（数万条）经下载通道回传的耗时。

**已知限制（见 KNOWN_LIMITATIONS §12h/§12i）**：

- 自写 SQLite 读取器的**溢出页切分**在极端样本上尚未逐字节对齐
  （结构判据保证"链正好用完"，但页内片段与溢出页衔接处可能有偏移）；
  浏览器库里 >1KB 的 Cookie 密文才会走到这条路径，实测样本见
  `scripts/fit_sqlite_overflow.py`。


## 10. Kerberos 票据（文件式 ccache，2026-10-19）

**两种技术**：
- `tech=files`：本机 MIT ccache 文件（任意平台）
- `tech=lsa`：**Windows LSA 凭据缓存**（`LsaCallAuthenticationPackage`，对应
  `klist` / `Rubeus dump` 的能力）—— `KerbQueryTicketCacheEx2Message`（20）枚举
  当前登录会话的票据缓存，`KerbRetrieveEncodedTicketMessage`（8）逐张导出 **kirbi
  （KRB_CRED）**。纯 syscall（secur32.dll），不引 CGO；结构体布局对照 ntsecapi.h
  并有单测断言（`lsa_types_test.go`）。
  导出的是未经解密的 KRB_CRED：enc-part 需要密钥才能解，**服务端拿不到客户端
  principal**，因此 `tech=lsa` 的元数据（client/realm/server/时间/旗标）由客户端
  随清单上报，服务端只做形态校验与入库。

**制品布局**（客户端 `implant/core/services/ticket.go` 生成，服务端
`pkg/cred/ticket.go` 解析）：

```
manifest.json        {host,user,collected_at,errors,files:[{name,size}],
                      lsa_tickets:[{file,client,realm,server,start,end,flags,...}]}
tickets/<文件名>      原始 ccache（v3/v4）        —— tech=files
kirbi/<序号>-<服务>.kirbi  原始 KRB_CRED         —— tech=lsa
```

**采集范围**：`KRB5CCNAME`（`FILE:` 前缀或裸路径；`KEYRING:`/`DIR:` 等非文件后端会记一条
说明）→ 临时目录（Windows `%TEMP%`）→ 用户主目录 → 非 Windows 的 `/tmp`，
文件名前缀 `krb5cc`，单文件上限 4MB、单次上限 64 个。

**服务端**：ccache 条目逐条 `ParseCCache`（**客户端不解析**，避免两套实现漂移），
kirbi 条目不解析二进制、元数据取自清单；每张票据登记一条
`loot(type=ticket, source=kerberos:ccache|kerberos:lsa)`：
`user` = principal、`realm` = 域、`secret` = 制品内条目名、note 带票据数与最晚到期时间；
原始 ccache 留在制品文件里，导入方式 `kinit -c <解压出的文件>`（Windows 用 Rubeus ptt /
impacket）。坏条目只进 `errors`，不影响其它缓存；一个能解出 principal 的都没有时整单显式失败。

**验证**：`pkg/cred/ticket_test.go` 用 impacket 的真实 ccache 夹具（v3/v4）打 ZIP 断言
解析结果、坏条目隔离、`ValidateArtifact` 按 kind 分派；客户端 `ticket_test.go` 覆盖
`KRB5CCNAME` 优先、非文件后端提示、制品布局与类型×技术校验。

## 11. Chromium `Web Data`：信用卡与自动填充（2026-10-19）

**开关**：`kind=browser, tech=cards` —— **单独技术**，不并入 `all`。
理由是噪声：自动填充一个用户几百条地址碎片很常见，混进常规采集会把有用条目淹掉。

**采集**（`<profile>/Web Data`，只读这一张库，不碰 Login Data/Cookies）：
- `credit_cards`：`card_number_encrypted` 与口令同为 v10/v20 AES-GCM（**同一把浏览器主密钥**），
  在植入体内存里解出；`name_on_card` / 有效期 / `nickname` 库里本就是明文
- `autofill`：字段名与值都是明文（Chromium 不加密）

**入库策略**：
- 信用卡 → `loot(type=card, source=browser:card)`，`secret` = 卡号，note 带持卡人/有效期/昵称
- 自动填充 → **不入库**，只在制品 JSON 里（`browsers[].autofill`）；制品备注与任务结果里会
  标注条数（`%d 条自动填充（未入库，见制品）`），避免操作员以为漏采

**上限**：客户端单配置 200 张卡 / 300 条自动填充，服务端 `MaxBrowserCards` /
`MaxBrowserAutofill` 同值收口；超限显式失败而不是静默截断。

**失败是显式的**：单张卡解不开只跳过该条并记日志；整份制品里一条可用条目都没有时校验失败。

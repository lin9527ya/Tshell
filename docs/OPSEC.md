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
| `jmp rel32` → `mov rax,imm64; jmp rax`（避免 ±2GB 射程溢出） | 已实现·有测试 | 同上 |
| 产品名不硬编码（编译期 MASQUERADE_NAME 注入） | 已实现 | `core/masquerade.go` |
| 构建不泄露路径/符号：`-s -w -trimpath -buildvcs=false` | 已实现（见 `build_templates.ps1`） | 构建脚本 |
| Ekko/Foliage 式**内存镜像**加密（含 .text） | **未做**（需 Windows 实机；Go runtime 无法安全加密自身代码段） | — |
| 调用栈伪造 | **未做**（需汇编级栈构造 + 实机调试） | — |
| 注入路径全 syscall 化（`VirtualAllocEx`/`WriteProcessMemory`/`CreateRemoteThread` 仍走 kernel32） | **未做**（需实机） | — |
| 清理现场（卸载持久化 + 删除落盘副本） | **未做** | 见 §6 |

### B. 通信（C2 信道）

| 能力 | 状态 | 位置 |
|---|---|---|
| UA / URI 伪装（默认保留腾讯系接口路径） | 已实现 | `pkg/c2profile/default.go` |
| 请求头 padding（破除请求头长度指纹） | 已实现 | `implant/core/transport_http.go` `applyRequestPadding` |
| Host 头覆盖（域前置） | 已实现 | `transport_http.go` `req.Host = httpFrontHost` |
| TLS JA3 伪装（对齐主流浏览器 ClientHello） | 已实现 | `pkg/c2profile` `tls.ja3` |
| DNS beacon 子域前缀可定制 | 已实现 | `pkg/c2profile` `dns.beacon_prefix` |
| 心跳默认值与全局配置一致（5s，防两处漂移） | 已实现·有测试 | `pkg/c2profile/default_sleep_test.go` |
| 主密钥**运行时**派生（每会话 ECDH，载荷内不含固定密钥） | **未做**（XL：跨 13 条传输的协议迁移） | 见 §6 |
| 流量体积整形（固定大小填充） | **未做**（目前只 pad 请求头） | — |

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
| 审计日志本身不被篡改（仅追加、哈希链） | **未做** | — |
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

这两起事故都造成过"以为做了，其实没做"，值得每次改动时对照：

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
| 内存镜像加密（Ekko 级） | 算错=进程自杀，必须实机反复验证 | 授权 Windows VM |
| 调用栈伪造 | 需汇编级栈构造 + 调试 | 同上 |
| 注入路径全 syscall 化 | 同上 | 同上 |
| 主密钥运行时派生（ECDH） | XL：13 条传输的协议迁移，需分步骤 + 过渡期兼容 | 无外部阻塞，属排期问题 |
| 流量体积整形 | 需要与 padding 策略统一设计，否则两种填充互相打架 | 排期 |
| 审计日志防篡改（哈希链） | 需要与日志轮转/清理策略一起设计 | 排期 |
| `cleanup` 统一清理现场命令 | 需 Windows 实机验证持久化卸载 | 同上 |
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
```

**关键断言不是"测试通过"，而是外部证据**：

| 断言 | 怎么验 |
|---|---|
| 库文件里没有明文口令 | `strings database.db \| grep <测试口令>`（对应 `TestDatabaseFileContainsNoPlaintextSecret`） |
| 载荷里没有固定主密钥 | `strings <payload> \| grep LISTENER_MASTER_KEY`（**当前仍会命中，见 §6**） |
| 日志里没有口令 | 捕获 stdout 断言（对应 `TestNoPasswordInLogStatements`） |
| 目录权限已收敛 | `stat -c %a Downloads sessions plugins`（对应 `pkg/security/fsperm_test.go`，POSIX 用例在 CI 的 ubuntu job 执行） |

> 注意最后一条：`fsperm_test.go` 里依赖真实权限位的用例在 Windows 会 skip。
> 为了不让"本机跳过"变成"没人验证"，判定逻辑（`needsTighten`）抽成了纯函数并在
> 所有平台真跑；实际 `chmod` 生效由 CI 的 ubuntu job 覆盖。

---

## 8. 相关文档

- `docs/MEMORY_OPSEC.md` —— 内存侧细节与 sleep mask 的并发设计
- `docs/CREDENTIAL_ACCESS.md` —— 凭据获取链路
- `docs/STORAGE.md` —— 存储后端（含静态加密说明）
- `docs/CODE_SIGNING.md` —— 载荷签名步骤
- `docs/REMAINING_WORK.md` —— 全局未做清单

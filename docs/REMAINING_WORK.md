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

现状：sleep mask 只遮蔽**密钥材料**（会话/上一把/主密钥/盐 + AEAD 缓存，
`implant/core/app/run.go`），不加密自身镜像；无调用栈伪造；主密钥仍是编译期 patch。

### 3.1 Ekko / Foliage 级 sleep mask（需 Windows 实机）
- 目标：睡眠期加密**自身内存镜像**，用 ROP/定时器（`CreateTimerQueueTimer` +
  `VirtualProtect`/`NtContinue`）还原
- 为什么没做：加密范围/时机算错 = 进程自杀，必须在实机（建议授权 VM）反复验证
- 入口：`implant/core/app/run.go`、新增 `core/sleepmask/`
- 验收：睡眠期用自写内存扫描器（或 PE-sieve）扫不到 beacon 镜像特征；13 传输回归通过

### 3.2 调用栈伪造（需 Windows 实机）
- 目标：敏感调用（`VirtualAllocEx`/`CreateRemoteThread`）的返回地址指向合法模块
- 入口：`core/evasion/syscall.go` 的 trampoline（当前已是 RW→RX）
- 验收：ETW-TI / 栈回溯看不到非模块返回地址；注入链路不回归

### 3.3 主密钥运行时派生（**协议迁移**，服务端 + 客户端同时改）
- 目标：主密钥不再明文 patch 进二进制（`pkg/delivery/payload.go` 的
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

- **`ClientFiles.vue` 拆分**：当前 **2101 行**（下载/上传/编辑/浏览混在一个文件）。
  已有安全网：组件级测试 + 全量 294 用例。建议按"上传 / 下载 / 编辑"三个组合式函数
  拆，保持模板不变，逐步搬逻辑
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

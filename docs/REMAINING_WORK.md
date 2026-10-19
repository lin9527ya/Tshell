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

## 1. kirbi ↔ ccache 互转（**M**，需 Windows/AD 验证）

**现状**
- ccache 解析已完成：`Tshell-server/pkg/cred/ccache.go`（v3/v4，真实夹具）
- LSA 导出的 kirbi（KRB_CRED）已能作为制品回传、入库（`tech=lsa`），但**服务端不解析二进制**
- 缺：`ccache → kirbi`（给 Windows 侧 ptt 用）与 `kirbi → ccache`（给 Linux 工具链用）

**为什么没做**
- `ccache→kirbi` 要构造 KRB_CRED 的 `enc-part`（EncKrbCredPart：会话密钥 + 客户端
  principal）。加密用的密钥/被 LSA 接受的形态**只能在 Windows 上验证**；ASN.1 写错
  不会报错，只会产出"能解析但 ptt 不认"的票据 —— 盲写等于交付假功能
- `kirbi→ccache` 需要密钥解密 enc-part（impacket 的 `ticketConverter -key`）

**怎么开始**
- 新文件 `pkg/cred/kirbi.go`：KRB_CRED 的 ASN.1 编码/解码（DER，可参考
  `pkg/cred/ccache.go` 已有的 DER 工具；kirbi 夹具可取自 impacket 的 `ccache-v3-kirbi` / `ccache-v4-kirbi`）
- 先做 **ccache→kirbi**（不涉密钥）：把每个 credential 的 `Ticket []byte`（本身就是
  DER Ticket）放进 `tickets [2]`；`enc-part` 用首个 credential 的会话密钥构造
- 验证步骤：自签 AD 环境拿一张 TGT → 导出 ccache → 转 kirbi → Windows
  `kerberos ptt <base64>` 或 mimikatz `kerberos::ptt` → `klist` 看到票据

**验收**：ccache→kirbi→ptt 后 `kerberos list` 能看到该票据；反方向 `-key` 解出的
ccache 可被 `kinit -c` 接受。

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
- **redirector 模板无 UI 入口**：目前只有 CLI（`tshell -gen-redirector`）与
  `GET /api/redirector/template`。需要的话在监听器页加一个"生成重定向器配置"对话框
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

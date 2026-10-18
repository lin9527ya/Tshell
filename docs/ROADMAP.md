# Tshell 优化路线图与现状

> **用途**：接着往下做的工作台。已完成的部分给出证据与代码位置；未完成的部分给出
> **目标 / 现状 / 入口 / 验收 / 工作量**，换台机器（含 Windows）也能直接开工。
>
> - 版本基线：**v1.0.0**（提交 `5e96559`），最后更新：2026-10-18
> - 逐条"已知限制"的完整描述见 [`KNOWN_LIMITATIONS.md`](./KNOWN_LIMITATIONS.md)，本文做索引与待办
> - 架构与部署分别见 [`../ARCHITECTURE.md`](../ARCHITECTURE.md)、[`../DEPLOYMENT.md`](../DEPLOYMENT.md)

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

## 1. 当前状态（v1.0.0）

| 维度 | 状态 |
|---|---|
| 传输 | 12 种传输 + 单二进制多传输（运行时回退链 + 主动健康探测）、多级中继 `Tshell-relay` |
| 加密 | AES-256-GCM 全链路、监听器主密钥 + HKDF 会话密钥、per-session nonce 唯一化、24h 轮换、sleep mask（全传输） |
| 服务端 | 任务一等对象、审计落库、多操作员协作、Prometheus 指标、备份恢复、存储抽象（SQLite/PostgreSQL + 版本化迁移）、RBAC/限流/隐藏访问 |
| 前端 | 会话表 / Beacon 控制台 / 任务中心 / 事件日志 / 凭据库 / Payload 与 Attacks 对话框族 / 密钥管理 / 审计 |
| 工程 | CI 六 job（vet / staticcheck / race / 覆盖率 20% / PG 集成 / 可复现构建）、可复现构建 + SBOM、redirector 模板、模板随仓库提供 |

**一句话**：单机单操作员的完成度高；**团队化/规模化的完成度仍偏低**（多副本、MFA），
**后渗透深度是最大短板**（凭据获取只有 LSASS、无提权模块）。

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

### 2.3 服务端运营能力
- **任务一等对象**：`tasks` 表 + 状态机 `queued→dispatched→completed|failed|timeout` +
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

### 2.5 工程与供应链
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
- **现状**：只有 LSASS 转储链路（`Tshell-server/pkg/cred/dump.go`、`minidump.go`、
  `Tshell-client/implant/core/services/cred_windows.go`）；loot 表与 CRUD 就绪，
  但 `pkg/api/loot.go` 顶部注释自述"提取命令族（LSASS/DPAPI/...）V1 仅库表"
- **入口**：
  - 客户端：`implant/core/services/cred_windows.go`（新增模块）+ `core/app/dispatch.go`（命令分发）
  - 服务端：`pkg/cred/`（制品请求/匹配）、`pkg/api/loot.go`
  - 前端：凭据库页（`Tshell-web/src/views/`）
- **验收**：每个模块独立开关 + 独立单测；产物落 loot 表并按 uid/类型过滤可查；
  不落盘明文口令（内存解析 → 直接回传）
- **建议**：按"单模块 + 独立开关 + 单独测试"推进，不要一次全上

#### 3.2 提权模块（getsystem / UAC bypass）— **M**
- **目标**：至少 getsystem（命名管道 + 令牌复制）+ 一种 UAC bypass（fodhelper 或
  computerdefaults）
- **现状**：**完全没有**（全仓无 getsystem / uac_bypass 实现）
- **入口**：`implant/core/commands/`（新增 `privesc_windows.go`）+ `core/app/dispatch.go`
  命令表 + 前端 Beacon 控制台命令提示
- **验收**：失败不炸会话（可回滚）、成功后新会话/令牌可用、有单测覆盖参数校验

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

### P1 —— 明显落后现代 C2

#### 3.4 水平扩展（多副本）— **XL**
- **目标**：k8s 的 `replicas`/HPA 真正可用
- **现状**：单实例（连接池/在线状态/会话锁/事件流/命令队列都在进程内存）；
  PostgreSQL 后端已可切（前置条件已具备）；工作区的 `k8s-deployment.yaml` 是旧草稿
  （`replicas: 2` + 两个 HPA），与代码能力不符且未入库
- **入口**：`pkg/connection/`（在线状态、连接池）、`pkg/command/`（队列、事件总线）、
  `pkg/collab/`（会话锁）→ 外置到 Redis/Postgres；新增 leader 选举
- **验收**：两副本经 LB 后任务不重不漏；**或者**明确把清单改成单副本并删掉 HPA（成本更低）
- **替代方案（推荐先做）**：单副本 + 定时备份 + 冷备，已在 `DEPLOYMENT.md` 写明

#### 3.5 平台自身安全补强 — **M**
- **MFA/TOTP**：完全没有（登录只有口令 + 限流）
  - 入口：`pkg/api/users.go`（登录流程）、`pkg/security/`、前端登录页
- **HSTS**：未设置（CSP 已有，见 `pkg/security/middleware.go:152`）
- **`jwt.algorithm` 是死配置**：默认写 HS256，实际实现是 RS256
  （`pkg/config/manager.go:117`）→ 要么接线成真实约束，要么删除
- **IP 白名单默认关闭**：`config.example.yaml` 的 `security.enable_ip_whitelist: false`（开启后 `whitelist_ips` 才生效）

#### 3.6 per-listener C2 画像 — **M**
- **目标**：每个监听器可绑定独立画像（URI / WS 路径 / DNS 前缀 / padding）
- **现状**：监听器**只按 default 注册**（`pkg/api/listener.go:560` 用 `GetDefault()`，
  WS 路径硬编码 `/ws`）；生成载荷时 `profileListenerMismatch` 会**拒绝**与 default 不一致的
  画像（`pkg/api/server.go:683`）—— 这是 fail-fast，不是支持
- **入口**：监听器表加 `profile_name`；`registerHTTPRoutes` 按画像注册；WS 路径去硬编码；
  `pkg/c2profile/`
- **验收**：用非 default 画像生成载荷能连上（e2e）；`docs/KNOWN_LIMITATIONS.md` 相应条目可勾掉

#### 3.7 领域 repository（把 SQL 收口）— **M**
- **目标**：`pkg/api` 里不再有裸 SQL / xorm 链式查询，改成 `pkg/database` 的领域方法
- **现状**：访问点已统一到 `database.DB()`（`Store` 接口），但查询语句仍散在各业务包
- **入口**：`pkg/database/`（新增领域文件，如 `clients_store.go`）、`pkg/api/*.go`
- **验收**：`pkg/api` 中 `database.DB()` 的直接调用集中到少数几个领域入口；
  可加一个棘轮测试（类似 `store_access_test.go`）

### P2 —— 体验与整洁度 / 需要实机

#### 3.8 实机验证（Windows / macOS）— **M**
- 代码路径齐全但**未在真机验证**：Windows（ConPTY 交互终端、持久化、注入、ETW/AMSI、
  截屏/HVNC）、macOS（LaunchAgent/LaunchDaemon 持久化、PTY、截图、监控拉取）
- 入口：见 `KNOWN_LIMITATIONS.md` §12e；建议先跑一遍"上线 → 命令往返 → 文件 → 终端 → 截屏"

#### 3.9 中继能力补齐 — **S~M**
- DNS 中继、多上游（当前只有 TCP/UDP + 级联）：`Tshell-relay/`（`tcp.go`/`udp.go`/`config.go`）

#### 3.10 供应链 — **M**
- 代码签名（Windows 载荷）、LOLBin 侧载（`rundll32`/`mshta` 之外的组合）

#### 3.11 前端债 — **M**
- Pivot 拓扑图是空壳：`api/clients/index.ts` 的 `get_graph_data()`（`/graph/data`）与请求层特判都在，但**没有任何组件消费**
- 报告只导出 JSON dump（无 CSV/PDF/HTML）
- i18n 覆盖率低：`isZh ? ... : ...` 内联三元 **1496 处**，字典在 `Tshell-web/src/stores/i18n.ts`（仅 478 行）
- `as any` **51 处**；最大单文件 `views/client/View/ClientFiles.vue` **2213 行**、`views/plugins/PluginsView.vue` **1947 行**

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
5. **构建顺序**：`build_templates.ps1` → `npm run build:embed` → 服务端**文件模式**
   `go build -o tshell-server.exe .\main.go`（包模式在 TDM-GCC 下可能产出跑不起来的 cgo exe）。
   内存紧张加 `GOMEMLIMIT=3GiB`。
6. **不要**在 `Tshell-server/` 等子目录里跑 git（有嵌套 `.git`，会拿到上游的旧 tag/commit）。
7. 模板是二进制：`.gitattributes` 已声明 `binary`，避免 `core.autocrlf=true` 把 gzip 流写坏。

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

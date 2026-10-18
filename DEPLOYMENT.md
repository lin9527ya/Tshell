# Tshell 部署说明

## 一、构建

### 前置
- Go ≥ 1.21(服务端/客户端)
- Node ≥ 18(前端,建议 20)
- GCC(仅 Windows DLL 模板需要)

### 构建步骤

```bash
# 1. 客户端模板(输出到 Tshell-server/pkg/api/server/{proto}/)
#    **模板已随仓库提供**(206 个文件,约 587MB),克隆后可直接编译,无需这一步。
#    仅当 implant 源码改动、需要更新模板时才重跑:
cd Tshell-client/implant
bash build_templates.sh

# 2. 前端(dist 自动嵌入服务端)
cd Tshell-web
npm ci --legacy-peer-deps
npm run build:embed        # 产物写入 Tshell-server/dist 并生成 build-report.json

# 3. 服务端(嵌入前端 + 模板;大二进制建议限制编译内存)
cd Tshell-server
# 文件模式构建(已验证配方;包模式 `.` 在 Windows+TDM-GCC 下可能产出无法执行的 cgo exe)
GOMEMLIMIT=3GiB go build -ldflags "-s -w" -trimpath -o tshell-server.exe .\main.go
```

> 大二进制说明:服务端约 1.3~1.4GB(含全部平台模板 + 前端 embed)。内存不足时用 `GOMEMLIMIT=2-3GiB` 构建,或临时精简 `pkg/api/server/{proto}` 目录(仅保留常用协议模板)。

## 二、配置(config.yaml)

| 项 | 说明 |
|----|------|
| `port` | API 服务端口(默认 8089) |
| `database.path` | SQLite 路径(WAL 模式自动开启) |
| `secret_key_file` | JWT 签名私钥文件(首次启动自动生成,须妥善保管) |
| `auth.default_username/password` | 首次启动创建的管理员 |
| `auth.force_change_password` | 是否强制首次改密 |
| `auth.enable_hidden_access` | 隐藏访问入口(**默认关闭**;开启时需 ≥8 字符强密钥,自动限流) |
| `logging.level` | debug/info/warn/error(排障时建议 info) |

### 必读安全配置
1. **监听器密钥**:每个监听器在"监听管理"页配置独立 VKey(≥32B)/加密盐(≥16B)。**默认/空密钥会被拒绝**(监听器启动、客户端生成、firstBlood 三处强制)。客户端模板未替换占位符时拒绝运行。
2. **初始密码**:首次启动后立即改密(默认密码策略:≥8 位含大小写与数字)。
3. **JWT 私钥**:`jwt_key.hex`/`jwt_private.pem` 不要提交到仓库或泄露。

## 三、启动

```bash
./tshell-server -p 8089                # 使用默认 config.yaml
./tshell-server -config /path/config.yaml -p 8099
```

启动日志出现 `Listening on port 8089` 即就绪。Web 界面:http://服务器:8089

### 监听器说明
- **重启服务端后监听器不会自动恢复**:需在 Web 界面"监听管理"点击启用,或调用 `POST /api/listener/open`。
- 反向隧道(*_forward)在"正向监听"页生成客户端后由客户端侧建立。

## 四、部署拓扑

```
浏览器 ──HTTP──> Nginx(可选: TLS 终结 / CSP / 静态缓存)──> Tshell-Server:8089
                                                            │
                     ┌──────────────────────────────────────┤
                     ▼                                      ▼
             持久监听器(TCP/KCP/WS)                 轮询监听器(HTTP/HTTPS/OSS/DNS)
                     │                                      │
             受害机植入体(模板由 GenServer 生成,含自定义密钥)
```

- Nginx 参考配置见 `Tshell-web/nginx.conf`(已含收紧的 CSP)。
- 建议 Nginx 层启用 HTTPS 并代理 `/api/`(含 WebSocket Upgrade 头)。

### 存储后端

默认 SQLite（单文件、零依赖）。要切 PostgreSQL，在 `config.yaml` 设置
`database.type: postgres` 与 `database.dsn`，服务端启动时自动建表与迁移；
内置备份在 PostgreSQL 下不启用（请用 pg_dump）。详见 **docs/STORAGE.md**。

### 单实例限制（重要）

**必须单副本部署，不支持水平扩展。** 这不是"还没做"，而是当前架构的硬约束：

| 状态 | 位置 | 多副本会怎样 |
|---|---|---|
| 连接池 / 监听器 | 进程内存 | 客户端只连到某一个副本，另一个副本看不到该会话 |
| 在线操作员、会话锁 | 进程内存 | 两个副本各记各的，锁形同虚设 |
| 事件流订阅者 | 进程内存 | 前端连到哪个副本就只能收到哪个副本的事件 |
| 命令队列 / 任务队列 | 进程内存 | 命令下发到 A、回包到达 B（B 不认识这个会话） |
| 数据库 | 单文件 SQLite | 多副本抢同一个文件，SQLite 单写者，写竞争会直接报错 |

因此：

- **不要**给 Deployment 设置 `replicas > 1`，**不要**配 HPA。工作区里那份
  `k8s-deployment.yaml` 是旧草稿，**不要直接套用** —— 它除了 `replicas: 2` + 两个 HPA
  与代码能力不符之外，还有两个会直接出事的配置：`algorithm: HS256` 现在会被服务端
  **拒绝启动**（只支持 RS256），以及两个副本挂同一个 SQLite PVC 必然写坏库。
- **请用 `deploy/k8s/tshell.yaml`**（已入库的单副本清单）：`replicas: 1`、
  `strategy: Recreate`（滚动更新期间会同时存在两个 Pod，即使几秒也会双写 SQLite）、
  `ReadWriteOnce` PVC（作为误加副本时的物理防线）、PDB `minAvailable: 0`
  （如实表达单副本没有冗余，而不是让节点排空永远卡住）、无 HPA。
  探针用 `tcpSocket` 而不是 HTTP —— 服务端没有未鉴权的健康端点，
  而给 C2 面板加一个未鉴权端点本身就是信息暴露面，不值得为了探针好看去开。
- 需要更高吞吐时，横向扩展的是**监听器 + 重定向器**（见 docs/REDIRECTOR.md），
  服务端本身保持单实例；
- 高可用请做**冷备**：定时备份（docs/BACKUP_RESTORE.md）+ 备机就绪，故障时用
  `-restore` 恢复后切流量。这是当前架构下唯一诚实的 HA 方案。

真正的水平扩展需要先做存储抽象（Postgres）+ 共享状态（会话/队列/锁外置）
+ leader 选举，属于独立的架构工程，未在本阶段实现。

## 五、日常运维

| 操作 | 方式 |
|------|------|
| 生成客户端 | Web"客户端生成"页(必须填 VKey/盐,与服务端监听器一致) |
| 会话密钥轮换 | 密钥管理页可轮换监听器密钥;会话密钥 24h 自动过期 |
| 审计 | 审计日志页(操作/登录/安全拦截);交付访问日志自动保留 30 天 |
| 离线清理 | 断线客户端由离线检测器标记 + 队列自动清理 |
| 回归验证 | `bash scripts/e2e.sh`(一键:启动→监听器→客户端→唯一化+命令;注意其判定依赖 debug 构建的客户端日志,release 模板下必然失败——见 KNOWN_LIMITATIONS) |
| 数据库备份/恢复 | 见 docs/BACKUP_RESTORE.md(`./tshell -backup` / `-restore`) |
| 指标 | `/metrics`(Prometheus,默认仅本机;见 docs/OBSERVABILITY.md) |

## 六、升级注意事项

- **多传输载荷**：生成时填"多传输端点"（`scheme://addr,...`）即用 multi 模板；所有端点必须与所选监听器使用相同的 VKey/盐。重建模板后需重建服务端才会生效（模板经 `go:embed` 嵌入）。
- **三端同步**:模板(build_templates.sh)→ 前端(build:embed)→ 服务端重建。加密/协议变更(如会话唯一化、nonce 协商)要求服务端与客户端**同时升级**,旧客户端无法与新版服务端通信。
- 修改监听器密钥后,已生成的客户端需重新生成。

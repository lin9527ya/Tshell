# 可观测性（指标 / 日志 / 排障）

## 1. 指标端点 `/metrics`

Prometheus 文本格式，由 `pkg/metrics` 提供（独立注册表，只暴露明确登记的指标）。

### 访问控制

指标含 **beacon 在线数、任务吞吐** 这类运行情报，默认不裸奔：

| 配置 | 行为 |
|---|---|
| `metrics.enabled: false` | 端点返回 404 |
| `metrics.token: "<串>"` | 必须带 `Authorization: Bearer <token>`（常量时间比较）；**本机来源也要带** |
| 未配 token | 仅允许回环来源（Prometheus 与服务端同机） |
| 未配 token + `metrics.allow_remote: true` | 允许跨机抓取（**等于公开运行情报，仅限隔离网络**；启动日志会告警） |

任何情况下，持 admin 角色的会话（`Authorization2` 或 HttpOnly Cookie）都可查看。

### 抓取配置

```yaml
# prometheus.yml
scrape_configs:
  - job_name: tshell
    scrape_interval: 15s
    authorization:
      credentials: "<metrics.token>"
    static_configs:
      - targets: ["10.0.0.10:8081"]   # 服务端监听地址
```

同机抓取（无需 token）：

```yaml
  - job_name: tshell
    static_configs:
      - targets: ["127.0.0.1:8081"]
```

### 指标清单

**容量与在线状态**（抓取时现算，无后台维护成本）

| 指标 | 类型 | 含义 |
|---|---|---|
| `tshell_beacons_online` | gauge | 当前在线 beacon 数 |
| `tshell_beacons_total` | gauge | 累计上线过的 beacon 数（含离线） |
| `tshell_connections_active{protocol}` | gauge | 连接池活跃连接数（按传输类型） |
| `tshell_command_queue_depth` | gauge | 全部客户端下行命令队列的待发命令总数 |
| `tshell_command_queue_max_per_client` | gauge | 单客户端队列最长积压（背压预警） |
| `tshell_event_subscribers` | gauge | 事件流 WebSocket 订阅者数 |
| `tshell_db_connections_open/in_use/idle` | gauge | SQLite 连接池状态 |
| `tshell_db_wait_total` | gauge | 等待数据库连接的累计次数（连接池不足信号） |

**吞吐与延迟**

| 指标 | 类型 | 含义 |
|---|---|---|
| `tshell_tasks_total{kind,status}` | counter | 任务数（kind=console/batch/exec/download，status=五态；批量超时收敛记入 `kind="unknown"`） |
| `tshell_task_duration_seconds{kind}` | histogram | 任务从下发到终态的耗时 |
| `tshell_http_requests_total{method,route,status}` | counter | HTTP 请求数（route 是**路由模板**，status 归并为 2xx/3xx/4xx/5xx） |
| `tshell_http_request_duration_seconds{method,route}` | histogram | HTTP 耗时 |

**安全与审计**

| 指标 | 类型 | 含义 |
|---|---|---|
| `tshell_audit_dropped_total` | counter | 审计日志因落盘队列满被丢弃的条数（**审计缺口的唯一信号，应告警**） |
| `tshell_security_block_total{reason}` | counter | 安全拦截次数 |
| `tshell_operator_actions_total{action}` | counter | 操作员动作数（lock/command/batch/exec，协作审计） |

外加 Go runtime（`go_goroutines`、`go_memstats_*`、GC）与进程采集器（CPU、FD、RSS）。

### 建议的告警规则

```yaml
groups:
  - name: tshell
    rules:
      - alert: AuditLogsDropped          # 审计丢了 = 合规与溯源缺口
        expr: increase(tshell_audit_dropped_total[10m]) > 0
      - alert: CommandQueueBacklog       # 命令下发积压：目标不回连或吞吐不足
        expr: tshell_command_queue_max_per_client > 100
      - alert: BeaconDrop                # 在线数骤降：链路或监听器故障
        expr: delta(tshell_beacons_online[10m]) < -5
      - alert: TaskFailureRate           # 任务失败率
        expr: |
          sum(rate(tshell_tasks_total{status=~"failed|timeout"}[15m]))
            / sum(rate(tshell_tasks_total[15m])) > 0.3
      - alert: SlowAPI
        expr: histogram_quantile(0.95, sum by (route) (rate(tshell_http_request_duration_seconds_bucket[5m]))) > 2
      - alert: DBPoolSaturated
        expr: rate(tshell_db_wait_total[5m]) > 1
```

## 2. 日志

- 格式：`logging.format: json|text`；输出：`logging.output: stdout|file`（文件落在 `logging.audit_log_dir`）。
- 级别：`logging.level: debug|info|warn|error`。
- 慢请求：≥2s 的请求单独打 warn，带 `request_id`、路由模板、状态码、耗时、用户名，
  与错误响应里的 `request_id` 可关联（`pkg/metrics/middleware.go`）。
- 审计：文件 + SQLite 双写；丢弃条数进 `tshell_audit_dropped_total`。

## 3. 排障入口

| 需求 | 入口 |
|---|---|
| 看进程内部状态（goroutine 栈、heap） | `TSHELL_PPROF=1` 启动，`127.0.0.1:6060/debug/pprof/`（仅回环） |
| 看密钥体系状态 | `GET /api/key-management/key-status/:client_id` |
| 看某会话的队列与最近下发 | `GET /api/client/tasks/:uid` |
| 看任务终态与归属 | `GET /api/tasks`（任务中心） |
| 看谁在线、谁占用了哪台主机 | `GET /api/collab/operators`、`GET /api/collab/locks` |

## 4. 与安全中间件的交互

| 中间件 | 对 /metrics 的影响 |
|---|---|
| 隐藏访问（`security.enable_hidden_access`） | 指标启用时 `/metrics` 自动绕过隐藏入口（抓取方无法完成隐藏入口握手）；**指标关闭时不绕过**，避免扫描者靠"伪装页 vs 404"的差异发现该路径 |
| API 限流（`security.api_rate_limit`，默认 100 次/分/IP） | 抓取间隔 ≥1s 不会触发；请勿把 scrape_interval 设到亚秒级 |
| IP 白名单（`security.enable_ip_whitelist`） | 启用后 Prometheus 所在 IP 必须在白名单内 |
| JWT | 不影响：抓取走 token 或回环，不经过 `protected` 组 |

## 5. 设计取舍

- **标签基数受控**：HTTP 用 gin 的 `FullPath()`（路由模板），连接按传输类型聚合而非按 uid。
  真实 URL / uid 作标签会让指标时间线随 beacon 数线性增长，最终把 Prometheus 撑爆。
- **现算而非推送**：在线数、队列积压等在抓取时求值（默认 15s 一次），不需要后台任务维护一致性，
  也不会出现"指标与真实状态不一致"的窗口。
- **独立注册表**：不用 `prometheus.DefaultRegisterer`，避免间接依赖顺手注册的指标泄漏实现细节。

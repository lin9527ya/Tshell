# 存储后端（SQLite / PostgreSQL）

Tshell 默认使用 **SQLite**（单文件、零依赖）；配置 `database.type: postgres` 可切换到
**PostgreSQL**。两种后端跑同一套业务代码，方言差异收敛在 `pkg/database/dialect.go`。

## 1. 配置

```yaml
database:
    type: sqlite3            # sqlite3（默认）| postgres
    path: ./database.db      # SQLite 文件路径（type=sqlite3 时使用）
    dsn: ""                  # PostgreSQL 连接串（type=postgres 时必填）
```

PostgreSQL 示例：

```yaml
database:
    type: postgres
    dsn: "host=127.0.0.1 port=5432 user=tshell password=*** dbname=tshell sslmode=require"
    max_open_conns: 25
    max_idle_conns: 5
    max_lifetime: 5m
```

启动时会打印 `🗄️ [数据库] 使用 PostgreSQL 后端`；`type=postgres` 但 DSN 为空时
直接拒绝启动。

## 2. 首次部署

服务端启动时自动完成建表与迁移：

1. `Sync2` 按结构体 tag 建表/补列（两种后端都支持）；
2. `schema_migrations` 版本表 + 有序迁移（见 `docs/BACKUP_RESTORE.md` 之外的
   `pkg/database/migrations.go`）；
3. 默认管理员由配置初始化流程创建（弱口令会生成随机密码并打印）。

因此 PostgreSQL 侧只需准备好空库与账号，无需手工执行 SQL。

## 3. 方言层做了什么

| 差异点 | SQLite | PostgreSQL | 处理 |
|---|---|---|---|
| 占位符 | `?` | `$1,$2…` | 原生 SQL 统一走 `ExecSQL` / `QueryStringSQL`（内部 `Rebind`） |
| 自增序列重置 | `sqlite_sequence` | `setval(pg_get_serial_sequence(...))` | `ResetAutoIncrement` |
| 列/表内省 | `PRAGMA table_info` | `information_schema` | `tableColumns` / `columnExists` |
| shell 追加/尾部读取 | `substr(x, -n)` | `right(x, n)` | `shellAppendSQL` / `LoadShellTail` |
| 大小写不敏感匹配 | `COLLATE NOCASE` | `ILIKE` | 统一用 `LOWER(col) LIKE LOWER(?)` |
| DDL 时间类型 | `DATETIME` | `TIMESTAMP` | `schema_migrations` 建表按方言选择 |
| 连接调优 | WAL/PRAGMA 调优 | 无对应项 | 仅 SQLite 执行 |
| 旧库结构修复 | 无主键表重建 / 驼峰列名 | 不适用（新库） | 仅 SQLite 执行 |

**约定**：新增原生 SQL 时不要直接 `Engine.Exec("... ?", args)` —— PostgreSQL 会报
语法错误。改用 `database.ExecSQL` / `database.QueryStringSQL`；xorm 的
`Where/Update/Insert` 构建器自己处理方言，不受影响。

## 4. 备份

- SQLite：内置备份/恢复（VACUUM INTO，见 `docs/BACKUP_RESTORE.md`）。
- PostgreSQL：**内置备份不启用**（`VACUUM INTO` 与文件替换是 SQLite 语义），
  `/api/backup/*` 返回 503。请使用 `pg_dump` / `pg_basebackup`，恢复用
  `pg_restore`/`psql` 并重启服务端。

## 5. 迁移与限制

- **单实例约束不变**：切到 PostgreSQL 只是换存储，连接池/在线状态/会话锁/事件流/
  命令队列仍在进程内存，仍然只能单副本（见 `DEPLOYMENT.md`）。多副本需要共享状态
  外置 + leader 选举，未实现。
- **没有数据迁移工具**：SQLite → PostgreSQL 需要手工导出/导入（表结构由 Sync2 重建，
  数据可用 `pgloader` 或脚本搬运）。属后续工作。
- **SQL 仍分散在 pkg/api**：本次完成的是"方言隔离 + 后端可切换"，尚未抽出
  repository 层（`pkg/api` 中的 xorm 调用仍直接使用全局 Engine）。
- 未覆盖：时间类型在 PG 上是 `timestamp`，SQLite 是 `DATETIME` 文本；跨后端备份
  文件不通用。

## 6. 验证

- 单测（默认 SQLite）：`go test ./pkg/database/`
  —— 覆盖 `Rebind`（含字符串字面量里的 `?`）、方言 SQL 选择、列内省、序列重置。
- PostgreSQL 集成测试（默认跳过，需实例）：
  ```bash
  docker run -d --name tshell-pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=tshell \
    -p 55432:5432 postgres:16-alpine
  TSHELL_TEST_PG_DSN="host=127.0.0.1 port=55432 user=postgres password=test dbname=tshell sslmode=disable" \
    go test ./pkg/database/ -run TestPostgresBackend -v
  ```
  覆盖：连接 → Sync2 → 版本化迁移 → xorm CRUD → 原生 SQL 占位符 →
  shell 追加方言语句 → 自增序列重置 → 列内省。
- CI：`.github/workflows/ci.yml` 的 `postgres` job 用 service container 跑同一用例。

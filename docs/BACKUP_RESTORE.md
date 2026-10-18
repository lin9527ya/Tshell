# 数据库备份与恢复

Tshell 的全部持久状态（用户、beacon 会话、shell 历史、凭据/战利品库、任务、审计日志）
都在一个 SQLite 文件里（默认 `./database.db`）。它是唯一的单点，因此备份与恢复是
运维必做项，不是可选项。

## 1. 为什么不是 `cp database.db`

数据库跑在 WAL 模式，最近的写入可能还在 `database.db-wal` 里。直接拷贝主文件会得到
一个"能打开、但少了最近写入"的库 —— 这类备份最危险：它看起来是好的，恢复时才发现
数据对不上。

本项目的备份用 SQLite 的 **`VACUUM INTO`**：在线一致性快照，不需要停服务，会把 WAL
里的内容一并写进目标文件。

## 2. 备份

### 自动（推荐）

`config.yaml`：

```yaml
backup:
    enabled: true        # 关闭后 /api/backup/* 返回 503
    dir: ./backups       # 备份目录（创建时 0700，文件 0600）
    interval: 24h        # 自动备份间隔（duration 写法，如 12h/30m）；设为 0s 只保留手动/CLI 备份
    keep: 7              # 保留份数（按时间倒序，超出部分自动删除）
    on_startup: false    # 启动时先备一份（改配置/升级前建议开启）
```

自动备份失败会在日志里以 `❌ [备份] 定时备份失败` 报出 —— 静默失败的备份系统等于没有备份。

### 手动

```bash
# 服务端运行时：设置 → 备份（仅 admin）点"立即备份"，或直接调接口
curl -X POST -H "Authorization2: Bearer <token>" http://127.0.0.1:8089/api/backup/create

# 或命令行（不依赖服务端是否运行，适合放进 crontab）
./tshell -backup

# 查看现有备份
./tshell -list-backups
```

每份备份生成时都会跑一次 `PRAGMA integrity_check` + 必需表检查，结果记在同名 `.ok`
文件里；列表里的"已校验/未校验"就来自它（文件大小与记录不符时自动降级为未校验，
能发现被截断或改动过的备份）。

### 接口一览（全部仅 admin）

| 接口 | 说明 |
|---|---|
| `GET /api/backup/list` | 备份列表（名称/大小/时间/是否已校验/备注） |
| `POST /api/backup/create` | 立即备份，body 可选 `{"note":"..."}` |
| `GET /api/backup/download/:name` | 下载备份（归档或转移到另一台服务端） |
| `POST /api/backup/import` | 上传外部备份（先校验再落盘，坏文件不占位置） |
| `DELETE /api/backup/:name` | 删除备份 |

## 3. 恢复（必须先停服务端）

恢复要替换整个数据库文件，而运行中的进程持有它以及 `-wal`/`-shm` 兄弟文件。
**因此没有热恢复接口** —— 这是 SQLite 的形态决定的，不是没做。

```bash
# 1) 停止服务端（systemd: systemctl stop tshell / docker: docker stop tshell）
# 2) 恢复（可用备份名或完整路径）
./tshell -restore tshell-backup-20261018-101530.000.db
#    或
./tshell -restore /path/to/tshell-backup-20261018-101530.000.db

# 3) 启动
./tshell -config config.yaml
```

`-restore` 会做这些事，任何一步失败都不会破坏现有数据：

1. **校验备份**（完整性检查 + 必需表）；不可用的备份直接拒绝；
2. 把现有数据库另存为 `database.db.pre-restore-<时间>`（恢复错了还能退回去）；
3. 临时文件 + `rename` 原子替换，并清掉旧库的 `-wal`/`-shm`；
4. 打印恢复前快照路径，确认无误后可自行删除。

若日志提示"检测到 `-wal` 非空，数据库可能正在被服务端使用"，说明服务端没停干净；
确认无误后可以加 `-force` 跳过该检查。

## 4. 故障演练（建议每季度一次）

在测试环境按下面步骤走一遍，验收标准是**数据真的回来了**：

```bash
# 1) 起一个干净实例并造数据
mkdir -p /tmp/drill && cd /tmp/drill
cp /path/to/tshell-server/tshell . && cp /path/to/config.yaml .
./tshell -config config.yaml &          # 等待启动完成
# 通过 Web/API 建一个监听器、上线一个测试 beacon、下发几条命令

# 2) 备份
./tshell -backup
./tshell -list-backups                  # 记下文件名与"已校验"

# 3) 制造"灾难"：停服务，直接删库（模拟磁盘损坏）
pkill -f './tshell -config'             # 或 systemctl stop tshell
rm -f database.db database.db-wal database.db-shm

# 4) 恢复
./tshell -restore <备份名>
./tshell -config config.yaml &

# 5) 验收：登录后能看到第 1 步的监听器、会话、命令历史与凭据；
#    审计日志里也有备份点之前的记录。
```

演练失败最常见的原因是把备份放在**同一块盘**上：备份与数据库同盘时，磁盘故障会
同时带走两者。生产环境请把 `backup.dir` 指向另一块盘或远端挂载点（rsync/对象存储
同步），并把 `GET /api/backup/download/:name` 接进你的归档流程。

## 5. 已知边界

- **恢复是整库替换**：不支持只回滚某张表/某个时间点。需要更细粒度请用外部工具
  （如 `litestream`/`sqlite3 .recover`），本项目不内置。
- **`config.database.path` 目前未生效**：数据库固定为进程工作目录下的 `./database.db`
  （`pkg/database/sqlite3.go` 的 `DatabaseName`）。备份 CLI 用的是这个真实路径，
  因此行为一致；但改配置里的 `path` 不会改变数据库位置。
- **备份不含 `config.yaml`、`jwt_private.pem`、`profiles/`、`scripts/`、`Downloads/`**：
  只备份数据库。迁移整套环境时这些要一并带走（尤其是 JWT 私钥 —— 丢了它所有登录态失效，
  但数据仍在）。

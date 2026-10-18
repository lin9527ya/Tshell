# 可复现构建

同一份源码 + 同一工具链 + 同一参数 → **逐字节相同**的二进制。

对 C2 这种本身就是高价值攻击目标的软件，这是供应链上最要紧的一环：没有可复现性，
"这个产物确实是我审查过的那份源码编译出来的"就无法验证 —— 你只能选择相信交付方。

## 1. 怎么用

```bash
# 构建两遍并比对（默认 server + implant）
./scripts/repro-build.sh

# CI 模式：不可复现则退出码 1
./scripts/repro-build.sh --check

# 只验一个目标 / 指定输出目录
./scripts/repro-build.sh --target implant --out /tmp/repro
```

输出：

```
[ OK ] implant-tcp-linux-amd64 可复现 (sha256 397f60394e5b5f81…, 6054072 字节)
[ OK ] tshell-server 可复现 (sha256 6a03ef154a85e959…, 572704224 字节)
[ OK ] 构建清单: dist/repro/build-manifest.json
[ OK ] SBOM: dist/repro/sbom-{server,implant}.json
```

产物落在 `dist/repro/`：

| 文件 | 内容 |
|---|---|
| `build-manifest.json` | 版本/提交/是否脏工作区、`SOURCE_DATE_EPOCH`、Go 版本、完整构建参数、每个产物的 sha256 与是否可复现 |
| `sbom-server.json` / `sbom-implant.json` | 模块依赖清单（`go list -m -json all`），可直接喂给依赖扫描工具 |
| `a/` `b/` | 两遍构建的产物，用于人工比对 |

## 2. 让构建可复现的四件事

1. **`-trimpath`**：去掉产物里的本机绝对路径（否则编译目录会进二进制）。
2. **`-buildvcs=false`**：不让工具链自动嵌入 VCS 信息 —— 默认嵌入的 `vcs.modified`
   会随工作区是否干净变化，同一个提交在干净/脏工作区下产出不同二进制。
   版本与提交由 `-ldflags -X main.Version/...` 显式注入（`/api/version` 可查）。
3. **构建时间取提交时间，不取"现在"**：
   `SOURCE_DATE_EPOCH` 缺省为 `git log -1 --format=%ct`，`main.BuildTime` 由它派生。
   原来的 `BUILDTIME=$(date -u ...)` 让每次构建都不同，是"不可复现"最直接的来源。
   需要固定到某个时间点时显式传 `SOURCE_DATE_EPOCH=<unix 秒>`。
4. **同一工具链**：`go.mod` 声明了最低版本，CI 用 `go-version-file` 跟随。
   跨大版本的 Go 不保证产出相同二进制。

## 3. 两个目标的可复现性差别

| 目标 | 可复现性 | 说明 |
|---|---|---|
| implant（客户端） | **强**（`CGO_ENABLED=0`，纯 Go） | 只要 Go 版本与参数一致，任何机器上都应产出相同二进制 |
| server（服务端） | **同环境强** | SQLite 驱动需要 cgo，产物含 C 编译器与 libc 的产物。同一镜像/同一工具链下逐字节一致；换发行版或 gcc 版本后不保证 |

CI 的 `reproducible` job 两者都跑（同一 runner 上构建两遍）。

## 4. 验证别人交付的二进制

```bash
# 1) 取到源码并切到 manifest 里记录的提交
git checkout <commit>

# 2) 用 manifest 里记录的参数本地重建
SOURCE_DATE_EPOCH=<manifest.source_date_epoch> ./scripts/repro-build.sh --target server

# 3) 比对 sha256
sha256sum dist/repro/a/server/tshell-server
#    与交付方 manifest 里的 artifacts[].sha256 对比
```

三者一致（源码提交、构建参数、产物哈希）才说明产物与源码对应。

注意：如果交付方的 `dirty_worktree` 是 `yes`，说明它是在**未提交的工作区**上构建的 ——
manifest 里记录的提交无法还原那份源码，这种情况下哈希比对没有意义。要交付就交付干净提交。

## 5. 与 CI 的关系

`.github/workflows/ci.yml` 的 `reproducible` job 会在每次推送时跑一遍 `--check`，
并把 manifest 与 SBOM 作为构建产物上传（`build-manifest`），可直接归档留痕。

服务端 job 里另有一道 `go build -o /dev/null ./main.go`：
权威构建入口 `build.sh` 用的是**文件模式**（Windows+TDM-GCC 下验证过的唯一可用形态），
而包模式 `go build ./...` 对"main 包里多出第二个 .go"是绿的 —— 只有文件模式会炸。
本地对应的是 `TestFileModeBuildWorks`（`go test .`）。

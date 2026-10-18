# 提权（阶段三 · 提权模块）

> 状态：**三条技术路径的服务端接线、客户端实现与单测已完成；真实成功率尚未在授权
> Windows 目标上验证**（见文末"验证状态"）。命令：`getsystem [auto|token|uac|uac_cleanup]`

## 1. 它解决什么问题

拿到一个普通用户会话（medium 完整性 / UAC 过滤令牌）之后，下一步通常是"要 SYSTEM 或
至少高完整性"——读 LSASS、装服务、清日志、动其它用户的目录都需要它。本模块把这件事
做成一条命令，并且**不破坏已有会话**：无论成功失败，当前 beacon 原样保留。

## 2. 三条技术路径

| tech | 做什么 | 前提 | 痕迹 |
|---|---|---|---|
| `already`（自动判定） | 检测当前是否已是 SYSTEM/高完整性 → 只报告 | — | 无 |
| `token` | 从**已提权进程**（winlogon/lsass/services/wininit/smss/explorer）复制 SYSTEM 或高完整性令牌 → `CreateProcessAsUserW` 派生新会话 | `SeDebugPrivilege`；目标机上存在高完整性进程且可打开令牌 | 小：无注册表改动，只多一个派生进程 |
| `uac` | 写 `HKCU\Software\Classes\ms-settings\shell\open\command` 劫持键 → 启动 auto-elevate 宿主 `fodhelper.exe`（失败退 `computerdefaults.exe`）→ 宿主以高完整性拉起新会话 → **立即还原注册表** | 当前用户在 Administrators 组；UAC 开启（`EnableLUA=1`） | 中：短暂改动 HKCU（已还原），多一个宿主进程 |

`auto`（默认）顺序：`already` → `token` → `uac`，每步失败都把原因写进输出，最后汇总。
选这个顺序是因为**痕迹从小到大**：能偷令牌就不要动注册表。

### 为什么 `explorer.exe` 也在候选里

UAC 提升过一次的用户，其 explorer 常常就是高完整性令牌 —— 这是"已经提权过一次"的
机器上最省事的一条路（不需要再触发一次 UAC）。普通（未提权）的同名进程会被
`tokenIsPrivileged` 过滤掉，不会误用。

## 3. 关键设计（为什么这么做）

1. **只派生新进程，不动当前进程的令牌状态**。
   `make_token`/`steal_token` 是"线程级模拟"，而命令在不同 goroutine 上执行、Go 的
   goroutine 又可能在 OS 线程间迁移 —— 依赖"当前线程正在模拟"的提权结果不稳定。
   显式把令牌交给 `CreateProcessAsUserW` 没有这个隐式依赖，失败也不会污染操作员
   已有的模拟上下文。
2. **新会话必须带上启动口令**（`os.Args[1]` 原样透传）。载荷启动强校验口令
   （见 `core/app/init.go`），不带口令的新进程会立刻退出 —— 现场表现为
   **"提权成功但没上线"**，是最难排查的一类假成功。DLL 载荷没有口令参数时，
   命令直接报错而不是拉起一个注定失败的新进程。
3. **注册表劫持必须还原**。劫持键是可被取证/被 EDR 规则命中的持久改动；
   派生后立即恢复原值（原本不存在就删除整棵子树），还原失败会在回执里**显式告警**
   并提示 `getsystem uac_cleanup`。
4. **删不掉键时退化为"清空劫持值"**。极端受限环境会拒绝删除；此时把命令行值清空
   同样能消除劫持 —— 留下可用的劫持比"报告失败"危险得多。回执会说明用的是哪种方式。
5. **前置自检不做无谓动作**：不在管理员组、UAC 已关闭时直接给出可读原因，
   不写注册表、不拉进程。

## 4. 操作入口

```
getsystem                  # auto：按 already → token → uac 依次尝试
getsystem token            # 只走令牌窃取
getsystem uac              # 只走 UAC 劫持
getsystem uac_cleanup      # 清理 UAC 劫持残留（不依赖内存状态，随时可用）
```

输出示例：

```
[+] 提权成功：技术=token，来源进程=winlogon.exe(812)，新会话 PID=7412（CreateProcessAsUserW）
    提示：新会话上线后可用 `rev2self` 之外的方式独立操作；本会话保持不变
```

```
[!] token 路径不可用：当前令牌不持有 SeDebugPrivilege（未分配）
[+] 提权成功：技术=uac，宿主=fodhelper.exe，新会话 PID=7412，注册表已还原
```

```
[!] token 路径失败：没有取得可用的高完整性令牌：lsass.exe(664): OpenProcessToken 拒绝
[!] uac 路径不可用：当前用户不在 Administrators 组，UAC 宿主拉起的进程不会有更高权限
```

服务端侧：前缀表 `getsystem ` → `GETSYSTEM(132)`；不带参数走精确表（等价 `auto`）；
TTP 报告映射为 `提权 / T1134 令牌操纵 + T1548.002 UAC 绕过`。
该命令**跳过节流豁免名单之外**——实际是加入豁免（与 `INJECT_MEM` 同类），
提权对时序敏感，不该被 `WaitTime` 拖住。

## 5. 失败模式与排查

| 现象 | 原因 | 处置 |
|---|---|---|
| `token` 路径报"不持有 SeDebugPrivilege" | 普通用户或权限被剥离 | 用 `uac` 路径，或先 `steal_token` 一个有 SeDebug 的进程令牌 |
| `token` 路径报"没有可用的高完整性令牌" | 目标机没有已提权进程（且 explorer 也是 medium） | 用 `uac` 路径 |
| `uac` 报"当前用户不在 Administrators 组" | 当前账号不是本地管理员 | 换账号；`uac` 不会给非管理员更高权限 |
| `uac` 报"UAC 已关闭" | `EnableLUA=0` | 此时进程本就该是高完整性，先看 `already` 判定；否则说明令牌被降权 |
| 报"提权成功"但**没有新会话上线** | 新进程起来就退了（口令未传/被 AV 拦） | 用新 PID 在 `ps` 里核对；确认载荷是带口令的 EXE 而不是 DLL |
| 回执里出现"[!!] 注册表还原失败" | 受限环境拒绝删键/改值 | 手动执行 `getsystem uac_cleanup` |
| `CreateProcessAsUserW` 失败且 `CreateProcessWithTokenW` 也失败 | 缺 `SeAssignPrimaryToken`/`SeIncreaseQuota`/`SeImpersonatePrivilege` | 看回执里的 LastError；`tokens` 结果决定要不要换来源进程 |

## 6. 单测覆盖（可在任意 Windows 上跑）

| 用例 | 断言 |
|---|---|
| `TestCmdGetSystemValidatesTech` | 非法技术当场报错（不会静默当 auto 跑一遍真实提权） |
| `TestCmdGetSystemAlreadyPrivilegedIsIdempotent` | 已是高完整性时只报告、可反复执行（普通权限下跳过，避免测试触发真实提权） |
| `TestEnablePrivilegeSemantics` | 不存在的权限名报错；`SeShutdownPrivilege` 幂等；**普通进程启用 `SeTcbPrivilege` 必须失败**（守住 `ERROR_NOT_ALL_ASSIGNED` 检查） |
| `TestSelfCommandLineShape` | 命令行是 `"exe" "口令"` 两段（口令缺失走报错分支） |
| `TestUACRegistryRestoreAndCleanup` | 写→还原回原值；原本不存在则清空；`uacCleanup` 幂等 |
| `TestUACCleanupDegradesSafely` | 删不掉键时劫持值必须失效（安全兜底） |
| `TestRegDeleteKeyIsNotRecursive` | 记录 Windows 语义 quirk：`RegDeleteKeyW` 对带子键的键返回 Access denied，我们的封装走递归路径 |
| `TestPidsByNameFiltersRequestedOnly` | 进程枚举不返回未请求的进程名、PID 不为 0 |
| 服务端 `TestGetSystemCommand` | 五种写法都解析成 `GETSYSTEM`，参数原样下发（不落 SHELL 兜底） |

## 7. 验证状态

**已在仓库内验证**：

- 客户端 `go test ./...` 全绿（Windows 目标）+ `go vet` 干净；
- 服务端 `go vet` + `pkg/sendcommand`/`pkg/command`/`pkg/api` 测试全绿；
- 13 种传输标签交叉编译全绿（windows/amd64 真实现 + linux/darwin 桩）；
- 失败路径与安全兜底（注册表还原/退化清空/参数校验）有单测。

**尚未验证（需要授权 Windows 目标机）**：

1. `token` 路径的真实成功率（本机开发环境里 `LookupPrivilegeValueW` 被受限令牌拒绝，
   连 `SeShutdownPrivilege` 都查不到 —— 该路径的端到端行为只能在正常令牌下确认）；
2. `uac` 路径：`fodhelper.exe`/`computerdefaults.exe` 在目标 Windows 版本上是否仍
   读取 `HKCU\...\ms-settings\shell\open\command`（较新版本可能已收紧）；
3. 派生出的新会话是否在 **30s 内**出现在会话列表（回连时间取决于传输类型）；
4. 受管终端上 `reg.exe`/EDR 是否会拦截注册表劫持。

### VM 验证清单（按序执行，逐步核对）

```text
前置：一台授权的 Windows 10/11 测试虚机（默认 UAC），一个普通管理员账号上线 beacon

1. getsystem                 → 期望：token 或 uac 成功，回执给出新会话 PID
2. 会话列表                  → 期望：出现第二条会话，用户名 SYSTEM 或高完整性账号
3. 在新会话上执行 whoami /priv → 期望：SeDebugPrivilege 等已启用（验证"确实提权了"）
4. getsystem                 → 期望：第二次报"无需提权"（幂等）
5. getsystem uac_cleanup     → 期望："无 UAC 劫持残留"
6. reg query "HKCU\Software\Classes\ms-settings" /s
                             → 期望：查不到劫持键（验证还原干净）
7. 原会话连发 shell whoami    → 期望：仍能正常回包（验证"提权不影响已有会话"）
```

## 8. 与其他模块的边界

- **不改** `make_token`/`steal_token`/`rev2self` 的语义（线程级模拟照旧）；
- 本模块**不做** UAC 之外的本地提权（服务配置错误、AlwaysInstallElevated、令牌特权滥用
  等）—— 那些属于后续"提权链"扩展，各自需要独立的前置探测；
- 派生出的新会话与父会话**完全独立**（各自回连、各自 sleep），父子关系不参与调度。

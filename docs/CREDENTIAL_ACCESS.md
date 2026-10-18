# 凭据获取（阶段三 · 模块 1：LSASS 内存转储）

> 状态：**服务端链路与协议已实现并有单测覆盖；Windows 端转储路径已交叉编译通过，
> 但尚未在真实 Windows 目标上验证**（见文末"验证状态"）。

## 1. 链路

```
操作员（会话行 → 凭据获取）
   │  POST /api/cred/dump {uid, kind=lsass, tech}
   ▼
服务端 pkg/api/cred.go
   ├─ 校验：会话存在 / Windows / 类型与技术合法 / 未被其他操作员占用
   ├─ 建任务 kind=cred（谁/何时/对哪台机器/用了什么技术）
   ├─ 登记"等待回传的制品"（uid + 期望文件名 + 任务 + 操作员 + 30 分钟有效期）
   └─ 下发 CRED_DUMP 帧：cmdBuf = "lsass|<tech>|<fileName>"
   ▼
客户端 core/services/cred.go（Windows）
   ├─ 找 lsass.exe 的 PID
   ├─ 按技术选型获取内存转储 → %TEMP%\<fileName>
   ├─ 经**下载通道**回传（复用 v2 下载协议：type22 头 + type26 分块）
   └─ 回传结束后删除目标机上的转储文件
   ▼
服务端下载完成回调 → matchCredArtifact
   ├─ 按文件名精确匹配到那次请求（匹配不上就按普通下载处理，不错认）
   ├─ 校验 minidump（MDMP 签名 / 版本 / 目录边界 / 必需数据流）
   ├─ 成功 → 任务 completed + 登记制品（loot type=dump, source=lsass）+ 审计 + 事件 + 指标
   └─ 失败 → 任务 failed，**不登记制品**（半截文件/空文件绝不当作"已获取"）
```

## 2. 为什么服务端不解析转储

解析 LSASS 转储 = 重新实现 mimikatz 的 sekurlsa（PE 解析、内存描述符、LSA 密钥、
WDigest/Kerberos/DPAPI 结构）。服务端做这件事收益低、维护成本高，而且**没有任何
安全收益** —— 解析结果最终还是要给操作员看。

服务端负责的是三件更有价值的事：
1. **归属**：制品属于哪次请求、哪个任务、哪个操作员；
2. **真实性**：制品是不是一个完整可用的转储（空文件/半截文件必须显式失败）；
3. **可审计**：谁在什么时候对哪台机器发起过凭据获取（`CRED_ACCESS` 审计 + 指标）。

解析在操作员本地做：

```bash
pypykatz lsa minidump lsass-DC01-20261018-120000.dmp
# 或 mimikatz: sekurlsa::minidump <文件>  →  sekurlsa::logonpasswords
```

## 3. 技术选型（PPL 是分水岭）

现代 Windows 上 LSASS 通常以 **PPL（Protected Process Light）** 运行，未签名进程
即使有 SeDebugPrivilege 也读不到它的内存。三条路的差别就在这里：

| tech | 做法 | 对 PPL | 痕迹 |
|---|---|---|---|
| `comsvcs` | 签名进程 `rundll32.exe` 调用签名库 `comsvcs.dll!MiniDump` | **有效**（转储由签名进程完成） | 长：子进程 + 磁盘落盘，EDR 覆盖好 |
| `dbgcore` | 自身进程内调用签名库 `dbgcore.dll!MiniDumpWriteDump` | 被拒（调用方未签名 → ACCESS_DENIED） | 短：无子进程，行为链短 |
| `auto`（默认） | 先 comsvcs，失败回退 dbgcore | 视目标而定 | 由实际走通的那条决定 |

选择建议：目标是普通工作站（LSASS 未受保护）时 `dbgcore` 更安静；域控等开启 PPL 的
目标只能用 `comsvcs`，并接受它的痕迹。

## 4. 失败是显式的

凭据获取最怕"看起来成功了"。这条链路上每一处失败都会显式落到任务终态：

| 失败点 | 表现 |
|---|---|
| 非 Windows 会话 | 接口 400，不下发（不让操作员对着 Linux 会话等一个必然失败的任务） |
| 会话被其他操作员占用 | 409（凭据获取是最不该撞车的操作） |
| 下发通道不可用 | 503 + 任务 failed + 撤销登记（不留"等待中"的幽灵） |
| 客户端权限不足 / PPL 拒绝 | 客户端返回明确错误（非 Windows 桩同样返回错误而非空路径） |
| 转储写出 0 字节 | 客户端本地即拒绝，不发起回传 |
| 回传中断（断线） | 任务 failed |
| 回传的文件不是完整 minidump | 任务 failed + **不登记制品** |
| 30 分钟未回传 | 登记过期，不再把后来的同名文件认作该请求的产物 |

## 5. 操作入口

- **会话行 → 更多 → 凭据获取（LSASS）**：选技术、看"等待回传/已获取制品"、取回制品。
  弹窗明确写着"服务端不做解析"与本地解析命令。
- `POST /api/cred/dump`（admin/operator）
- `GET /api/cred/artifacts?uid=`（admin/operator）
- 制品本体在服务端 `Downloads/<客户端ID>/` 下，用文件下载接口取回。
- 指标：`tshell_cred_artifacts_total{kind}`；审计动作：`CRED_ACCESS`。

## 6. 验证状态（重要）

**已在本仓库验证**（`go test -race ./...` 全绿）：

- 请求登记与匹配：精确匹配、不同 uid 不匹配、同名请求拒绝、过期清理、取消；
- 制品校验：有效转储通过、空文件/短文件/签名不符/零流/缺系统信息流/目录越界/流越界/
  异常目录条数全部拒绝；
- 归属链路：回传完成后任务置 completed、登记 `loot(type=dump, source=lsass)`、
  待回传记录清空；无效制品 → 任务 failed 且不登记；回传中断 → 任务 failed；
  无关文件回传不误认；
- 接口：参数校验（uid 缺失/会话不存在/非 Windows/类型与技术非法）、
  下发失败时回滚登记与任务；
- 客户端：命令参数校验（含路径穿越拒绝）、非 Windows 桩显式报错；
- 交叉编译：windows/amd64、linux/amd64、darwin/amd64 全部通过。

**尚未验证**（需要真实 Windows 目标，本仓库的 CI 只能交叉编译）：

1. `comsvcs` 路径在开启 PPL 的目标上是否真的成功（rundll32 参数形式、
   `full` 参数在新版 Windows 上的行为）；
2. `dbgcore` 路径的返回码判定（`MiniDumpWriteDump` 在部分失败场景下返回 0 但
   仍写出部分内容 —— 目前靠"文件大小为 0 则拒绝 + 服务端 minidump 校验"兜底，
   服务端校验会拦住半截转储，但客户端侧的失败原因不如直接看返回码清晰）；
3. 大转储（数百 MB ~ 1GB）经下载通道回传的耗时与稳定性（分块 2MB，
   轮询型传输下会持续多个回连周期）；
4. 目标机上 `%TEMP%` 的可用空间（转储体积与 LSASS 内存相当）。

建议的首次验证方式：在一台**授权的**测试虚机上按 §1 链路走一遍，
对照 `CRED_ACCESS` 审计与任务终态确认每一步。

## 7. 后续模块（阶段三 · 第 13 项剩余）

按"单个模块 + 独立开关 + 单独测试"推进，每块都复用本模块的制品管线
（登记 → 回传 → 校验 → 归属）：

| 模块 | 说明 | 复用点 |
|---|---|---|
| DPAPI 凭据/主密钥 | 用户主密钥 + 浏览器/凭据管理器 blob | 制品类型加 `dpapi`，客户端新增取法 |
| 浏览器凭据 / Cookie | Chrome/Edge 登录数据与 Cookie 库（需 DPAPI 解密） | 制品类型加 `browser` |
| Kerberos 票据 | 列出/导出/注入/清除票据 | 票据文件即制品 |
| BloodHound 采集 | 域对象与 ACL 采集，产出 JSON | 制品类型加 `bloodhound` |
| 提权模块 | getsystem（命名管道模拟）/ UAC 绕过 | 属"动作"类，不需要制品管线 |
| 端口扫描 | ✅ 已实现（TCP connect 扫描，跨平台）：`scan <目标> <端口> [超时毫秒]` | 结果直接作为命令回显 |

服务端侧每加一个模块只需：`pkg/cred` 的 `Kinds` 加一项、客户端加一个取法、
前端下拉加一项 —— 登记/校验/归属/审计链路完全复用。

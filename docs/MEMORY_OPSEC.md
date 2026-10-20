# 内存 OPSEC（阶段三 · 第 14 项）

> 状态：sleep mask 已修好并扩展到全部传输（**有单测与并发测试覆盖**）；
> 调用栈伪造、全 syscall 化（含注入路径）待做；Ekko/Foliage 式的"加密自身内存镜像"
> 需要 Windows 实机验证（见文末）。

## 1. sleep mask：原来保护的是副本

原实现：

```go
key, _ := encrypt.GlobalClientKeyManager.GetSessionKey()  // 返回的是**拷贝**
sleepmask.SleepWithMask(d, key)                           // 对拷贝就地 XOR
```

`GetSessionKey()` 明确返回拷贝（注释写着"调用方不得污染全局会话密钥"），
所以被 XOR 的是一个**谁也看不到的局部变量**，真正的会话密钥在整个睡眠期间原样
躺在管理器里，而且在 `aeadCache`（以密钥字节为 map key）里还另有一份。
看着有 OPSEC，实际一点没保护 —— 这类"看起来做了但没做"的防护比不做更危险，
因为它会让运维以为已经覆盖了。

现在（`ClientKeyManager.MaskForSleep`）：

- **就地遮蔽真身**：会话密钥、上一把密钥（轮换重叠期）、主密钥与盐（HKDF 的根）；
- **清空 AEAD 缓存**（不清就等于留了一份明文密钥）；
- **擦除 pad**：还原之后 pad 没有任何用途，留着只是多一份"密文 XOR pad 反推密钥"的材料；
- 随机源失败时**整段回滚**（半遮蔽比不遮蔽更危险：一半真一半假，排查时完全对不上）。

## 2. 全传输覆盖

原来只有轮询型传输（http/https/dns/oss）走 mask，tcp/kcp/websocket/forward 走的是
无遮蔽的 `SleepWithJitter` —— 而**长连接的会话密钥驻留时间最长**（数小时不断线），
恰恰是最该保护的一类。现在两类都走 mask。

## 3. 怎么做到不打断活跃会话

遮蔽期间密钥不可用，而隧道/交互 shell 的收发随时可能发生。三条机制：

1. **在途判定**：加解密进入时计数 +1、返回时 -1；只要有在途使用者就跳过本轮遮蔽。
   注意是"此刻在途"而不是"曾经用过"——后者会把 beacon 自己的心跳与命令回执也算进来，
   结果是每轮都判定为忙、遮蔽永不生效（这个坑第一版就踩了）。
2. **按片遮蔽**：睡眠切成不超过 `maskSliceDuration`（默认 2s）的片，片间重新判定。
   空闲时几乎全程遮蔽；一旦有会话开始收发，最多再多等一片就还原。
3. **等待而非报错**：遮蔽期间取密钥的调用会等待还原（`sync.Cond`），不会拿到被 XOR 的
   字节去加密 —— 那种失败在服务端表现为"GCM 认证失败"，排查方向完全对不上。

## 4. 顺带修掉的一个竞态

`DecryptWithSessionKey` 原来在锁内取切片、**带出锁外**使用：

```go
ckm.mu.RLock(); cur := ckm.sessionKey; ckm.mu.RUnlock()   // 切片指向的还是同一块内存
...SecureDecrypt(ciphertext, cur)                          // 遮蔽若发生在这之间 → 用错密钥
```

遮蔽一旦发生在拷贝之后，解密用的就是被 XOR 过的字节。并发用例（`TestMaskUnderConcurrency`）
复现了它，现在改为锁内拷贝。**这类问题只在"遮蔽窗口与解密撞车"时偶发**，靠人工测试基本碰不到。

## 5. 仍未做（需要 Windows 实机）

| 项 | 说明 | 为什么现在不做 |
|---|---|---|
| Ekko / Foliage 式 mask | 用 ROP gadget 链 + `NtContinue`/定时器在睡眠期加密**自身内存镜像**（含 .text） | **已归档为不可做**（2026-10-20）：Go runtime 的 sysmon/GC/信号线程常驻，加密代码段期间它们随时执行被加密字节 → 必崩；`SuspendThread` 方案会因 mheap/mspan 锁与 GC safepoint 死锁；`runtime.stopTheWorld` 不对外暴露 |
| 全 syscall 化 | `CreateProcessW`/`VirtualAllocEx` 这类直调的调用栈仍长在 Go 线程栈上 | 需 `stdcall` 汇编桩 + 独立线程，属另一个量级；注入/镂空的内存原语已 syscall 化（P0） |

**已做（P2，2026-10-20）**：这两项当时未做的，现在有了能安全落地的版本

- **调用栈伪造（direct syscall 路径）**：`evasion.Syscall()` 在 syscall 前 `mov rsp,<伪栈>`，
  伪栈顶 `BaseThreadInitThunk+0x14`、链尾 `RtlUserThreadStart+0x21`，第 5~8 参数复制到
  伪栈 `+0x28`；桩按 SSN 惰性生成 RW→RX。indirect 刻意不套（返回地址本就该在 ntdll 内）。
  边界：只管 `Syscall()` 这一条路，kernel32 直调的回溯仍能看到 Go 帧。
- **睡眠期模块摘链**：`SleepMask` 期间把本模块从 PEB 三条链表摘掉，醒来挂回；
  挂回前校验邻居指针，被改动过就挂表尾。门控 `config.HostEvasionEnabled()`，默认关。
  边界：不动 VAD，按内存区域扫仍可见镜像页。
- **Win11 24H2 解析修复**：函数体里 syscall 在偏移 0x12（老布局 8），原实现写死 8，
  导致 24H2 上所有 Nt\* 解析失败 → syscall 模式静默退回 kernel32。

**已做（P4，2026-10-20）：这些能力现在可以逐项开关**

上面每一项（含 P3 的 ETW/AMSI/unhook/PPID/BlockDLLs）原来都绑在同一个门控
`HostEvasionEnabled()`（ETW∥AMSI∥syscall≠none）上，勾一个带出一串。现在：

- 生成期：9 个编译期占位符（`ETW_PATCH`/`AMSI`/`SYSCALL_MODE`/`AUTO_DELETE` +
  `SLEEP_MASK_HIDE`/`STACK_SPOOF`/`PPID_SPOOF`/`BLOCK_DLLS`/`UNHOOK_ON_START`），
  缺省全关；关掉就是那段代码路径不存在。
- 运行期：`evasion [status|all on|all off|on <key>|off <key>|reset]`，覆盖只存内存，
  重连即回到生成期值；编译期关的能力运行期打不开（回执说明原因）。
- 门控改成细粒度：摘链读 `SleepMaskHideEnabled()`、伪栈读 `SpoofActive()`（还要
  `syscallMode=direct`）、PPID/BlockDLLs 各自独立，不再被 ETW patch 顺带打开。
- 完整论证与遗留见 `docs/OPSEC.md` §3 与 `docs/KNOWN_LIMITATIONS.md` §20。


## 6. 本项已完成的 syscall/trampoline 修复

`core/evasion`：

1. **RWX → W^X**：trampoline 原来直接 `VirtualAlloc(PAGE_EXECUTE_READWRITE)`。
   一块 RWX 内存本身就是 EDR/扫描器最醒目的特征之一，也与本项目"注入走 RW→RX"的
   原则自相矛盾。现在 RW 写入 → `VirtualProtect(PAGE_EXECUTE_READ)`。
2. **`jmp rel32` → `mov rax, imm64; jmp rax`**：rel32 只有 ±2GB 射程，而 trampoline
   是我们自己分配的地址，与 ntdll 的 syscall 指令之间可能隔着远超 2GB（ASLR 会这么放）。
   一旦超出，rel32 溢出成垃圾地址 —— 表现为随机崩溃，且**只在部分机器上复现**。
3. 机器码生成抽成平台无关的 `BuildTrampoline` / `ParseSyscallStub`，
   于是可以在 Linux 上做**字节级**校验（含被 hook 的函数体识别）。

## 7. 验证方式

```bash
cd Tshell-client/implant
go test ./core/encrypt/ ./core/app/ ./core/evasion/ -race -count=1
```

- `TestMaskHidesRealKeyMaterial`：遮蔽期间真身字节必须已改变（变异检验：改回遮蔽副本 → 失败）；
- `TestMaskClearsAEADCache`：缓存必须清空；
- `TestMaskSkippedWhenKeyInUse`：在途使用者存在时跳过，退出后恢复；
- `TestKeyUseWaitsForUnmask`：遮蔽期间取密钥等待还原，不拿错字节；
- `TestMaskUnderConcurrency`：4 个 goroutine 并发加解密 + 20 轮遮蔽，`-race` 无竞态、
  解密结果始终正确（这条用例抓到过上面那个锁外使用切片的竞态）；
- `TestSleepMaskYieldsToActiveKeyUser`：活跃会话的等待被限制在片长量级；
- `TestBuildTrampoline*` / `TestParseSyscallStub*`：机器码逐字节校验（含 24H2 布局）。
- P2 新增（windows/amd64 实机）：
  - `TestPrepareTrampolinesResolvesEveryName`：名单里每个 Nt\* 都必须解析到，
    缺一个就会静默回退 kernel32 —— 这条是 24H2 那个 bug 的回归护栏；
  - `TestSyscallReachesKernel`：真走 trampoline 进内核（`NtQueryInformationProcess`
    读本进程 PEB，NTSTATUS=0）；
  - `TestSpoofAllocatesWithStackArgs`：6 参数 `NtAllocateVirtualMemory` 走伪栈成功，
    `VirtualQuery` 断分配结果确实是 `PAGE_READWRITE`；
  - `TestSpoofMatchesUnspoofed`：伪栈与直连 trampoline 的返回一致；
  - `TestHideModuleDisappearsFromModuleList`：摘链后 `CreateToolhelp32Snapshot`
    (`TH32CS_SNAPMODULE`) 里**没有**本模块，挂回后又有（这是摘链有没有用的直接判据）；
  - `TestAttachFallsBackToTailWhenNeighborsChanged`：邻居被改动时挂表尾，不硬写失效指针。

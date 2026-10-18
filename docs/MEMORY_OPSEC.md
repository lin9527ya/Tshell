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
| Ekko / Foliage 式 mask | 用 ROP gadget 链 + `NtContinue`/定时器在睡眠期加密**自身内存镜像**（含 .text） | Go runtime 无法安全加密自己的代码段（GC/抢占/信号随时会踩到），且这套机制必须在真实 Windows 上调试才能确认稳定 |
| 调用栈伪造 | 睡眠/敏感调用期间伪造调用栈，规避"返回地址不在 ntdll"一类检查 | 需要汇编级栈构造 + Windows 调试 |
| 全 syscall 化 | 注入路径的 `VirtualAllocEx`/`WriteProcessMemory`/`CreateRemoteThread` 仍走 kernel32 | 见下节已做的部分 |

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
- `TestBuildTrampoline*` / `TestParseSyscallStub*`：机器码逐字节校验。

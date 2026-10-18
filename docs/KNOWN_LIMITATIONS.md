# Tshell 已知限制项（Known Limitations）

> 状态约定：✅ 已实现 | ⏳ 待实现（实现时更新）

## 1. inject 隐蔽注入
- 现状：✅ 已实现（阶段三-1）：最小权限句柄 + RW 写入→执行前 RX + 写后校验 + LastError 细节。
- 遗留：间接 syscall（NtCreateThreadEx 直调）未做，CreateRemoteThread 仍可被检测。进一步隐蔽需替换为线程劫持/APC 注入。
- 状态：✅（核心）/ ⏳（深度隐蔽）

## 2. BOF token API
- 现状：✅ 已实现（阶段三-2）：BeaconIsAdmin（TokenElevation）、BeaconUseToken（ImpersonateLoggedOnUser）、BeaconRevertToken（RevertToSelf）。
- 状态：✅

## 3. inline-bin patchExit 对直接 syscall 无效
- 现状：patch 劫持导出函数入口；shellcode 走直接 syscall（绕过 ntdll 导出）时 patch 无效。
- 计划：syscall 级 patch 涉及内核调用表/重定位层，工程量大且稳定性风险高。
- 状态：⏳（接受现有限制，文档化）

## 4. inline-bin 子进程 stdout 句柄继承
- 现状：✅ 已实现（阶段三-4）：CreateFile 传 SECURITY_ATTRIBUTES{bInheritHandle:1}。
- 状态：✅

## 5. keylog 完整 IME/中文输入
- 现状：✅ 已实现（阶段三-5）：ToUnicodeEx + GetKeyboardState + GetKeyboardLayout，支持中文 IME 组合；保留 ASCII 回退。
- 状态：✅

## 6. execute-assembly 单例 CLR
- 现状：✅ 部分实现（阶段三-6）：全局执行互斥消除并发句柄竞争；go-clr 单例 host 限制仍存在（restore 后旧管道写入，输出捕获不受影响）。
- 状态：✅（互斥）/ ⏳（多实例需替换 go-clr）

## 7. 双引擎架构重构
- 现状：✅ 部分实现：currentProc 赋值（中断生效）、去 .bat 落盘（参数传递）、人为延迟移除。
- 遗留：WMI OLE 固定 goroutine + CoInitialize 配对、输出统一 cmdType 36、与 ConPTY 共享会话注册表。
- 状态：✅（核心）/ ⏳（深度收敛）

## 8. 遗留弱加密
- 现状：✅ 已实现（阶段三-8）：`AllowLegacyCommandFallback` 默认 false（新客户端全部使用会话密钥）。
- 遗留：旧加密函数（Encrypt/Decrypt）代码保留用于 FirstBlood 元数据解析；彻底移除需改 FirstBlood 协议。
- 状态：✅（禁用）/ ⏳（代码移除）

## 9. 密钥轮换协议（notifyKeyUpdate）
- 现状：✅ 已实现（2026-10 核对）：服务端 `pkg/encrypt/encrypt.go` 的 `notifyKeyUpdate`
  经 `KeyRotationSender`（`main.go` 注入）下发 `type=5|nonce`，客户端
  `core/communication/transport_utils.go` 消费并回 ACK(121)，`ConfirmKeyRotation`
  完成 Current/Previous/Next 切换（重叠期仍可解旧消息），`derivation_compare_test.go`
  有对应单测。
- 状态：✅

## 10. VKey 密钥口径统一
- 现状：GenerateListenerKeyConfig 返回 base64(32B)，消费方取字符串前 32/16 字符（熵 192bit）；手动设置 VKey 直接按字符串使用。
- 计划：统一"先 base64 解码再取字节"（破坏性，需迁移现有 DB 监听器密钥，部署环境迁移风险高）。
- 状态：⏳（破坏性变更，谨慎评估）

## 11. 前端 GBK 修复映射表
- 现状：encoding.js 硬编码乱码→中文映射表（几十条）。
- 计划：统一为字节级 TextDecoder('gb18030') 流式方案。
- 状态：⏳

## 12. 审计日志落库
- 现状：✅ 已实现（2026-10）：审计**文件 + SQLite 双写**（文件保留人读与归档），
  查询/统计/登录失败/保留策略全部改走 `audit_log_db` 表（索引 + 库侧分页 + 排序）。
  时间范围与排序用数值列 `ts_unix`（SQLite 的 DATETIME 是 NUMERIC 亲和，
  直接比较时间串/时间值不可靠，实测踩过）。
- 遗留：10 万级日志的预聚合统计（当前统计仍按范围聚合查询）与按月分区。
- 状态：✅（落库）/ ⏳（预聚合）

## 13. 级联 C2（B9，二级中继）
- 状态：⏳（重架构，需设计中继协议与链路加密）

## 14. 多协议热切换（B10，单二进制多协议）
- 状态：⏳（需合并各 cmd/*/main.go 为统一入口 + 适配器热切换）

## 15. TLS 证书自动签发（ACME）
- 状态：⏳

## 16. 密钥健康检查（已下线入口）
- 现状：该功能三层都不成立，已摘除入口：服务端 `SendKeyHealthCheck` 把 4 字节 cmdType
  交给面向文本命令的 `SendCommand`，匹配不到任何前缀后被兜底成 `buildCmd(SHELL, ...)`
  —— 客户端收到的是"包着 4 个原始字节的 SHELL 命令"，cmdType 94 根本没发出去；
  客户端也没有 94 的常量与 dispatch 分支；服务端更没有消费回包的处理
  （`KEY_HEALTH_CHECK` 除定义外全仓零引用）。因此点了按钮只会"没反应"。
- 替代：`GET /api/key-management/key-status/:client_id` 已能显示服务端侧的会话密钥状态
  （当前/上一/下一密钥）与轮换参数，密钥管理页保留"强制轮换"与轮换配置。
- 若要恢复"让客户端确认自身密钥状态"：需客户端补常量+case、服务端改用原始命令发送方式、
  并补回包消费与前端展示 —— 属新功能而非修 bug。

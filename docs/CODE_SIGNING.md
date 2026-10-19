# 代码签名（可选）

> 状态：**已实现（外部命令形态）**，用假签名器做过端到端单测；
> **尚未用真实代码签名证书验证过**（本仓库没有证书）。下面区分了"已验证"与"未验证"。

## 为什么

未签名的 EXE 是 EDR/AV 与 Windows SmartScreen 的直接特征。载荷 hash 轮换
（`HashPad`）解决的是"同一模板静态 hash 被标记"，签名解决的是另一件事 ——
让载荷看起来来自某个有证书的主体（以及过 SmartScreen 的"未知发布者"提示）。

## 配置

```yaml
signing:
    enable: false                 # true：生成失败即报错（绝不静默发未签名文件）
    command: osslsigncode         # 需在 PATH 中；Windows 也可换 signtool（配自定义 args）
    args: []                      # 空 = osslsigncode 默认参数（见下）
    pfx: ./codesign.pfx           # PKCS#12（含私钥）
    password: ""
    timeout_seconds: 60
```

默认参数（`pkg/signing`）：

```
sign -pkcs12 {{pfx}} -pass {{password}} -in {{in}} -out {{out}}
```

占位符：`{{in}}`（待签名 PE）、`{{out}}`（签名结果）、`{{pfx}}`、`{{password}}`。
唯一硬约束是"读 `{{in}}`、写 `{{out}}`"，因此换任何签名器都只是改 args。

## 生效范围

- **GenServer 生成的载荷**（exe/dll）与 **WebDelivery 交付的文件**
- 只在 `OS=windows` 且**不是 shellcode 格式**（bin/hex/c）时执行 —— 签完再转
  shellcode 会把签名剥掉，没有意义
- 签名在**所有 patch/HashPad 之后**：签名覆盖最终字节，签完再改就失效
- `inject_mem`/`migrate` 等中间产物不签名

## 失败策略

启用签名后，**签名失败 = 载荷生成失败**（返回错误、不出未签名文件）。
启动时校验：`signing.pfx` 必须可读、`signing.command` 必须在 PATH 中，否则进程拒绝启动
（配置错误不该等到第一次生成载荷才暴露）。

## 安全注意

- **密码走命令行参数**：签名期间会出现在本机进程列表（`ps`/任务管理器）。
  建议使用**专用签名证书**，不要复用主证书；签名机与 C2 服务端最好分离。
- 硬件令牌/PKCS#11（EV 证书常见形态）需要厂商工具签名器，可通过自定义 `args`
  接入；本仓库未验证过该路径。

## 验证方式

```bash
# 1) 自签证书（测试用；真实证书请走 CA）
openssl req -x509 -newkey rsa:2048 -nodes -keyout key.pem -out cert.pem -days 30 -subj "/CN=Test Signer"
openssl pkcs12 -export -out codesign.pfx -inkey key.pem -in cert.pem -passout pass:test123

# 2) config.yaml
#   signing: {enable: true, pfx: ./codesign.pfx, password: test123}

# 3) 生成载荷后验证签名
osslsigncode verify -in client.exe
```

**已验证**（`pkg/signing/signing_test.go`）：未启用原样返回；参数模板/占位符替换；
签名器失败带 stderr 摘要；"命令成功但没写 out"明确报错；配置校验三条失败路径；
默认值回落。
**未验证**：真实证书 + 真实 `osslsigncode` 产出的签名在 Windows 上的
SmartScreen/EDR 表现（需要证书与 Windows 环境）。

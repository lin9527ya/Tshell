# 平台自身安全：MFA/TOTP、HSTS、jwt.algorithm（阶段三 §3.5）

> 状态：**已完成**（服务端 + 前端 + 测试）。实机/真机层面无需额外验证 ——
> 全部逻辑都在服务端与浏览器里，本机可完整验证。

## 1. 为什么做第二因子

运维入口过去只有"口令 + 限流"。C2 平台的口令一旦泄露（复用到别处、被钓鱼、
日志里残留），攻击者拿到的是**全部已控主机的控制权**。加一个第二因子，
把"知道口令"与"能登录"分开。

密码学上要注意的是：TOTP 密钥本身就是"第二因子"的全部秘密，
**它落库必须是密文**。否则拿到一份库备份的人可以直接算出动态码，
第二因子当场作废 —— 这是很多自研 MFA 最容易忽略的一点。

## 1. 为什么做第二因子

运维入口过去只有"口令 + 限流"。C2 平台的口令一旦泄露（复用到别处、被钓鱼、
日志里残留），攻击者拿到的是**全部已控主机的控制权**。加一个第二因子，
把"知道口令"与"能登录"分开。

密码学上要注意的是：TOTP 密钥本身就是"第二因子"的全部秘密，
**它落库必须是密文**。否则拿到一份库备份的人可以直接算出动态码，
第二因子当场作废 —— 这是很多自研 MFA 最容易忽略的一点。

## 2. 算法与参数（`pkg/security/otp`）

| 项 | 取值 | 说明 |
|---|---|---|
| 算法 | HMAC-SHA1（RFC 4226 动态截断） | 认证器 App 的通用默认；不要为了"看起来更现代"改成 SHA-256，很多 App 不支持 |
| 位数 | 6 | 同上 |
| 步长 | 30s | 同上 |
| 容忍窗口 | ±1 步（±30s） | 客户端与服务器时钟不可能严格同步；再放大会线性放大暴破面 |
| 密钥长度 | 20 字节 | HMAC-SHA1 的分组长度；base32 编码、**不带填充**（带填充部分 App 会拒） |

正确性由 **RFC 6238 附录 B 官方向量**钉住（`totp_test.go`）。
注意标准里的坑：附录 B 那 8 位值里 SHA-1 行用的种子是 20 字节的
`12345678901234567890`，SHA-256/512 行换了更长的种子 —— 抄错种子会得到
"测试通过但真实客户端算不出"的结果。

### 防重放

`Verify` 返回**命中的时间步**而不是简单的 true/false，调用方必须记录
"每个用户最后成功使用的时间步"，并拒绝 `<=` 该值的结果。
少了这一步，攻击者在 30 秒窗口里截获一次动态码就能重复使用 ——
这是 TOTP 实现最常见的漏洞。测试 `TestVerifyReportsTimestepForReplayProtection`
钉住"同窗口内多次校验返回同一时间步"这个契约。

## 3. 恢复码（`otp/recovery.go`）

认证器丢了（换手机、清数据、误删）是**必然会遇到**的运维场景。
没有恢复码，用户就永久失去登录能力，只能靠改库。

- 10 个码，每个 11 字符（`ABCDE-FGHJK`），基 32 字符集**去掉易混的 0/O/1/I/L**；
- 只存 **bcrypt 哈希**：与口令同等待遇，库被读走也拿不到可用恢复码；
- 用掉即从列表移除（`UseRecoveryCode` 返回剩余列表，调用方必须落库）；
- 输入宽容：忽略大小写、空格、漏掉的连字符（用户是照着手抄的纸输入的）。

## 4. 密钥的静态保护（`pkg/security/mfa.go`）

```
TOTP 密钥 --(AES-256-GCM)--> "v1:" + base64(nonce‖ct‖tag) --> users.mfa_secret_enc
                    ^
                    └── 密钥 = HKDF-SHA256(PKCS#8(JWT 私钥), info="tshell-mfa-secret-v1")
```

**为什么从 JWT 私钥派生而不是另建一份密钥**

- 私钥已在 `./jwt_private.pem`（0600），已有备份与运维约定；
- 另造一份密钥意味着又多一个"丢了就全员无法登录"的东西要备份；
- HKDF 是确定性的 → 重启、多副本都能算出同一把。

**必须写明白的代价**：JWT 私钥泄露 ⇒ TOTP 密钥随之可解。
这不是额外弱点（私钥本来就等于"能伪造任意管理员令牌"），
但不能让人误以为 MFA 密钥有独立于私钥的保护。

**三条不可退让的行为**

1. 加密器未初始化时 `SealMFASecret` **返回错误**，绝不静默明文落库；
2. `OpenMFASecret("")` 返回 `ErrMFANotEnabled`（正常状态：该用户没开 MFA），
   而"密文解不开"返回**另一个**错误 —— 混为一谈会导致"解不开就当没开启"，
   即静默把 MFA 关掉；
3. `Users` 的三个 MFA 字段都是 `json:"-"`，并有测试
   （`TestUsersMFAFieldsAreNotSerialized`）防止它们出现在任何响应里。

## 5. 两步登录的令牌隔离（`pkg/security/jwt_manager.go`）

```
POST /users/login  口令通过 ┬─ 未开 MFA → 直接签发 access + refresh（与改动前逐字段一致）
                           └─ 已开 MFA → 200 {mfa_required, pending_token}，**不设任何 cookie**
POST /users/login/mfa  pending_token + 动态码 → 校验通过才签发 access + refresh
```

用 `aud` 把两者彻底分开：

- 访问令牌：`aud = ["tshell-access"]`
- 中间态令牌：`aud = ["tshell-mfa-pending"]`，TTL 5 分钟

**两侧都必须检查**（只查一边等于没查）：

| 检查 | 位置 | 少了它会怎样 |
|---|---|---|
| 拒绝 pending 当访问令牌 | `ValidateToken` | **第二因子形同虚设**：拿第一步的返回就能进后台 |
| 只认 pending 打第二步 | `ValidatePendingToken` | 已登录的人可以用访问令牌重走 MFA 换新令牌（语义错误） |

两条都有测试钉住（`jwt_mfa_test.go`、`TestMFAPendingTokenAudienceIsolation`）。

中间态令牌还带上了 `device_id`：第二步要沿用同一设备指纹 ——
`generateDeviceID` 依附请求上下文，第二步是另一个请求，不带上就算不出同一个指纹。

## 6. 已实现的测试

| 用例 | 断言 |
|---|---|
| `TestRFC6238Vectors` / `...EightDigits` | RFC 6208 官方向量（6 位与 8 位两套） |
| `TestVerifyAcceptsCurrentAndSkewedCodes` | ±1 步通过、±61s 拒绝 |
| `TestVerifyReportsTimestepForReplayProtection` | 同窗口返回同一时间步（防重放的契约） |
| `TestVerifyRejectsMalformedInput` | 空码/非数字/长度错/全角数字都不通过，空密钥不 panic |
| `TestSecretRoundTrip` | base32 无填充往返；小写/空格/多余 `=` 宽容解析 |
| `TestProvisioningURI` | `otpauth://` 关键字段齐全；label 做百分号编码且空格不编成 `+` |
| `TestConfigNormalization` | 非法 Digits/Period/Skew 兜底而非 panic |
| `TestQRRoundTrip` | 编码→解码往返（10 种长度，含 UTF-8 与版本边界） |
| `TestDataModuleCapacityPerVersion` | 各版本可用数据模块数 == 标准总码字×8 + 补位 |
| `TestBlockTableMatchesCodewordTotals` | 纠错块表自洽（Σ块数×(数据+纠错) == 总码字） |
| `TestVersionCapacityIsSufficient` | 版本选择不偏大也不偏小（1..200 字节） |
| `TestQRFinderPatterns/Timing/Alignment` | 定位图形内核、分隔符、定时图形、对齐图形 |
| `TestRecoveryCode*` | 形状/去混淆字符、哈希里无明文、一次性、宽容输入、失败不改动列表 |
| `TestMFASecretRoundTrip` | 加解密往返；同明文两次密文不同（nonce 随机） |
| `TestSealRejectsWhenNotInitialized` | 未初始化拒绝写入（不允许明文落库） |
| `TestOpenMFASecretErrors` | 空密文=未开启；篡改/格式错都报错且**不被当成未开启** |
| `TestOpenFailsAfterKeyChange` | 换私钥后旧密文解不开；换回来仍可解（派生确定性） |
| `TestMFAPendingTokenAudienceIsolation` | **pending 不能当访问令牌**；访问令牌不能打第二步 |
| `TestMFAPendingTokenLifetime` | 中间态 TTL 约 5 分钟 |
| `TestPendingTokenHasNoRefreshToken` | 签发中间态不产生刷新令牌 |
| `TestMigrateUsersMFATableIdempotent` | 迁移幂等；老库升级后三列存在 |
| `TestMigrationVersionsUnique` | 版本号不重复、都有名字与实现 |
| `TestUsersMFAFieldsAreNotSerialized` | MFA 三字段不出现在 JSON 里 |

## 7. 实现过程中修掉的真问题（二维码编码器）

二维码编码器是自实现的（避免为一个用途引入图形依赖链），过程中被测试抓出四个
**"矩阵尺寸正常、任何扫描器都读不出来"** 的错误，值得留档：

1. **定位图形画成了 7×7 实心方块** —— 距离 2 的那一圈必须是浅色。
   尺寸对、往返测试（当时还没写对）也不报错，但扫描器认不出定位图形。
2. **格式信息 bit 14 写到了 (8,15)**（应在 (8,14)）。那一格属于数据区，
   症状是"纠错级别被读成别的值"。
3. **zigzag 的列游标写错**：`right = 5` 而不是 `right--`，导致配对数变成
   (3,4)、(1,2) —— **第 0 列永远不被访问**，可用模块数与码字比特数对不上，
   直接越界写穿。
4. **比特读取器以"码字比特 + 补位数"为上限**，而补位没有数据来源，
   多读那 7 位就越界。

另外两处是**表的转录错误**，由"几何 vs 码字表"双向约束抓出：

- V7-M 的块表被我写成 4×27（标准是 4×31，差 16 个码字）；
- 对齐图形的跳过规则误用了"中心格已被占用就跳过"，把 v7 起合法的
  (22,6)、(6,22) 两个图形也跳掉了（少 40 个模块）。

对策不只是修 bug，而是加了**三道互相独立的守卫**，让同类错误无法再溜过去：

- 可用模块数 == 标准总码字×8 + 补位（几何 ↔ 表）
- Σ块数×(数据+纠错) == 总码字（表自洽）
- 遍历计数恰好等于可用模块数（遍历 ↔ 布局）

以及一条结构性约束：**编码与解码共用同一个 `reserveFunctionModules`**。
解码侧曾经另写一套简化版，少了 123 个保留格，结果把功能图形当数据读、
掩码选错、整条链全错 —— 两套规则只要存在，就一定会漂移。

## 8. 已完成（原续做清单）

1. **登录流程接线**（`pkg/api/users.go`、`pkg/api/mfa.go`）
   - `LoginHandler`：请求加可选 `totp_code`；口令通过后按 §5 分流
   - `POST /users/login/mfa`：挂在 `/api` 组内，**共用** `LoginRateLimitMiddleware`
     + `BruteForceProtectionMiddleware`
   - `POST /users/mfa/start|confirm|disable`、`GET /users/mfa`（状态）、
     `GET /users/mfa/qr`（只接受 `otpauth://` 前缀）、
     `POST /users/mfa/reset`（管理员 + 需带管理员自己的口令）
   - `issueSession(c, username, deviceID, role)`：两步成功的响应逐字段一致
2. **服务端集成测试**（`pkg/api/mfa_test.go`，13 项，全部走真实 gin 路由 + 真实库 + 真实签名）
3. **HSTS**（`pkg/security/middleware.go`）：仅 HTTPS 请求下发；`security.force_hsts`
   供反代不传 `X-Forwarded-Proto` 的部署显式开启
4. **`jwt.algorithm` 接线成真约束**（`pkg/config/manager.go`）：非 `RS256`/空值拒绝启动；
   默认值从 `HS256` 改为 `RS256` 并同步修正示例配置与描述文案
5. **前端**：`LoginView.vue` 两步分支（含"中间态过期自动回第一步"）；
   新增 `views/settings/MFAView.vue` 挂到 `account` 页签（**所有角色可见**）；
   `api/user/index.ts` 新增 6 个方法

### 集成测试覆盖（13 项）

| 用例 | 断言 |
|---|---|
| `TestLoginWithoutMFAResponseUnchanged` | 未开 MFA 的响应字段与 Cookie 与改动前一致（回归） |
| `TestLoginWithMFAReturnsOnlyPendingToken` | 只回 pending；**无** access/refresh；**无** Cookie |
| `TestPendingTokenCannotAccessProtectedRoutes` | 中间态令牌打受保护路由必须 401/403 |
| `TestMFASecondStepRejectsWrongCodeWithoutLocking` | 错码拒绝且**不锁号**；对码签发凭据 + Cookie |
| `TestMFACodeCannotBeReplayed` | 同一动态码第二次必须失败 |
| `TestMFARecoveryCodeIsSingleUse` | 恢复码用掉即落库移除，第二次失败 |
| `TestMFADisableReturnsToSingleFactor` | 关闭后登录直接签发凭据 |
| `TestMFAResetByAdmin` | 管理员重置后目标用户可单因子登录 |
| `TestMFAResetRequiresAdminPassword` | 管理员口令错误时不得重置 |
| `TestMFAResetRequiresAdminRole` | 非管理员不得重置 |
| `TestMFAQROnlyAcceptsOtpauth` | 非 `otpauth://` 文本拒绝渲染 |
| `TestMFAEnrollmentFlow` | start→confirm 全流程；密钥密文落库；错码不开启 |
| `TestMFAMeDoesNotLeakSecret` | 状态接口不含密钥材料 |

前端：`views/login/LoginView.test.ts`（5 项：一次落地 / 切第二步 / 第二步成功 /
恢复码分支 / 中间态过期回第一步）。

## 9. 已知限制

1. **防重放状态在内存里**（`mfaLastSteps`）。单实例部署正确；多副本部署下
   同一个码可能在另一个副本上被接受（每个副本各记一份）。
   与 `pkg/collab` 的会话锁同类问题，属 ROADMAP §3.4 水平扩展的范畴。
   服务器重启会清空该表 —— 30 秒窗口内重启才可能被重放，风险可接受。
2. **未做 WebAuthn / 设备信任**（"记住此设备 30 天"）：当前每次登录都要动态码。
3. **恢复码用完不自动补发**：仍可用动态码登录，重新走一次 `start`+`confirm`
   会换一批新恢复码（同时换新密钥）。
4. **`security.enable_ip_whitelist` 默认关闭**：这是部署选择，不改默认值；
   开启后 `whitelist_ips` 才生效。
5. **JWT 私钥变更会使既有 TOTP 密钥解不开**：表现为登录报"动态码校验暂不可用"，
   救援路径是管理员 reset。**绝不静默降级为单因子**（那等于悄悄关掉 MFA）。

### 已定但尚未落地的设计决定

- 用户点错动态码**不计入** `LoginAttempts`/锁号：那是**口令**失败的惩罚机制，
  用在动态码上会因为手滑把账号锁掉（锁定后连恢复码都用不了）
- `start` 之后未 `confirm` 则**不写库**，MFA 保持关闭（半途而废不留残状态）
- 用户在两步之间被删号 → 第二步按"用户不存在"处理，文案与第一步一致（避免用户名枚举）
- 恢复码用完后仍可用动态码登录；`start` 可重新生成一批并替换

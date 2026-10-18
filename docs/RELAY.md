# 多级中继（Relay）

```
beacon → 中继1 → 中继2 → … → 团队服务器
```

中继节点把 beacon 的流量逐跳转发到团队服务器，团队服务器不直接暴露在公网。
与重定向器（`docs/REDIRECTOR.md`，用于 HTTP/HTTPS 的应用层伪装）互补：
中继工作在**传输层**，对 C2 协议透明，因此 TCP / WebSocket / HTTP(S) / KCP 都能承接。

## 1. 为什么中继是"哑"的

中继只搬字节：不解析 C2 协议、不持有任何密钥、不落盘任何内容。

- **端到端加密**：beacon ↔ 服务端的会话加密（HKDF 会话密钥 + AES-GCM）是端到端的。
  中继即使被拿下、被完整取证，也拿不到会话密钥，解不开指令内容。
- **传输透明**：不解析协议意味着传输类型对中继无感，C2 协议演进不需要同步升级中继。
- **取证价值最低**：中继上只有监听地址与上游地址，没有密钥、没有配置、没有日志内容。

能做的与不能做的：

| 中继能隐藏 | 中继不能隐藏 |
|---|---|
| 团队服务器的公网地址 | beacon 的真实来源 IP（在 MetaInfo 里，服务端仍可见） |
| 服务端监听端口的直接暴露 | 流量存在本身（ISP/边界设备能看到 TLS 连接） |
| 服务端的证书/指纹（若中继做 TLS 终止） | 传输时长与包大小特征 |

## 2. 部署

```bash
# 构建（纯标准库，无第三方依赖）
cd Tshell-relay && go build -trimpath -ldflags "-s -w" -o tshell-relay .

# 配置
cp config.example.json relay.json   # 改 listen / upstream / health_listen
./tshell-relay -config relay.json
```

最小配置：

```json
{
  "listen": "0.0.0.0:443",
  "upstream": "10.10.0.5:8089",
  "health_listen": "127.0.0.1:8081"
}
```

| 字段 | 说明 |
|---|---|
| `listen` / `upstream` | TCP 监听与上游地址（上游可以是另一个中继 —— 级联） |
| `udp_listen` / `udp_upstream` | 可选：中继 KCP（UDP）传输 |
| `tls_cert` / `tls_key` | 可选：在中继上终止 TLS，转发到上游为明文。命令内容本身由应用层加密保护，TLS 终止不会暴露指令 |
| `max_conns` / `max_conns_per_ip` | 连接上限（默认 4096 / 256）—— 暴露在公网的组件必须有上限，否则一个扫描器就能吃光连接表 |
| `dial_timeout` | 连接上游超时（默认 10s） |
| `idle_timeout` | 连接空闲关闭（默认 1h；beacon 有心跳，正常连接不会触发） |
| `max_udp_flows` / `udp_idle_timeout` | UDP 流上限与回收（默认 2048 / 60s） |

健康检查：`GET http://127.0.0.1:8081/healthz`（连接数、流数、接受/拒绝计数）。

## 3. 与监听器的配合（关键）

beacon 里被 patch 的是监听器的 **connect_address**。用中继时，监听器的
connect_address 应填**中继的地址**，监听地址仍是服务端自己的：

1. 服务端建监听器：`listenAddress = 0.0.0.0:8089`，`connectAddress = relay.example.com:443`；
2. 中继：`listen = 0.0.0.0:443`，`upstream = 10.10.0.5:8089`（服务端内网地址）；
3. 生成载荷时选择该监听器 —— 客户端连中继，中继转发到服务端。

若 connect_address 与 listen_address 填反，beacon 会直连服务端（绕过中继），
而中继上什么都看不到。

## 4. 级联（多级中继）

把上游指向下一跳即可：

```
中继1: listen 0.0.0.0:443  upstream relay2.internal:8443
中继2: listen 0.0.0.0:8443 upstream 10.10.0.5:8089
```

每一跳都只是 TCP 转发，链路里任意一跳被拿下都不会泄露密钥。
链路越长延迟与故障面越大：建议**不超过两级**，并且只在第一跳做 IP 限制。

## 5. 验证

仓库自带测试覆盖了中继的核心行为（`cd Tshell-relay && go test -race ./...`）：

- 双向转发（120KB 跨多次读）、**两级级联**、连接上限拒绝、空闲超时关闭、
  TLS 终止、UDP 回显与流上限、健康检查。

端到端（真实客户端经中继上线）：

```bash
# 1) 服务端监听 18095，监听器 connect_address 填中继地址 127.0.0.1:18195
# 2) 中继：listen 127.0.0.1:18195 → upstream 127.0.0.1:18095
# 3) 用该监听器生成 TCP 客户端并运行 → 服务端会话列表出现该 beacon
```

## 6. DNS 中继（阶段三 §3.9）

DNS 传输的流量是"查询/响应"配对的报文，**不能像 KCP 那样裸搬 UDP 字节**。
中继在 DNS 上多做三件事，代价是它必须看懂报文头（只看头部 12 字节，不解析问题段/记录段）：

| 动作 | 为什么必须做 |
|---|---|
| 逐查询**改写事务 ID** | 中继同时服务多个 beacon，ID 会撞车（两个客户端都用 0x1234 时，响应会投递给错的一方，表现为"DNS 通道时好时坏"）。中继换成随机 ID 送上游，回程再改回客户端原本的 ID |
| UDP 响应带 **TC 位时改走 TCP** | DNS 标准要求：响应超长时置 TC 位，客户端改用 TCP 重问。中继直接替客户端做了，并清掉 TC 位 |
| 同端口 **UDP + TCP 都监听** | 只做 UDP 会在长 TXT 响应上失败（C2 的 DNS 通道常用 TXT 回传数据） |

配置：

```json
{
  "dns_listen": "0.0.0.0:53",
  "dns_upstream": "10.10.0.5:53",
  "dns_upstreams": ["10.10.0.5:53", "10.10.0.6:53"]
}
```

- `dns_listen` 与 `listen` **互相独立**：只配 `dns_listen` 就是一台纯 DNS 中继，
  不需要 `upstream`。
- `dns_upstream` 留空时会回落到 TCP 的 `upstream`（常见部署是同一个服务端地址，
  让运维少写一遍）。
- 中继不解析问题段内容，因此对 C2 侧用什么域名、TXT 还是 A 记录**完全透明**。

## 7. 多上游与故障转移（阶段三 §3.9）

单上游的中继本身就是单点 —— 而中继多半部署在没人盯着的 VPS 上，上游挂一个整条链路就断。
把上游写成数组即可启用轮询 + 故障转移：

```json
{
  "listen": "0.0.0.0:443",
  "upstreams": ["10.10.0.5:8089", "10.10.0.6:8089", "relay2.example:443"]
}
```

- `upstreams` 非空时**取代** `upstream`（列表优先；两者都写不报错，但只认列表）。
- 选择策略是**轮询 + 跳过不健康**，不做加权/最少连接：中继是哑的，看不到会话归属，
  按连接粒度分散已经够用，更复杂的调度只增加出错面。
- 健康判定是**被动 + 主动**：
  - 被动：拨号/交换失败**连续** 3 次即标记不健康（一次抖动不会打掉一个上游）；
  - 主动：后台每 `upstream_health_interval`（默认 30s）探一次**不健康**的上游，
    成功即恢复。健康上游由真实流量证明，不做额外探测；
  - 全部上游都被标记不健康时仍会轮转着试 —— 否则永远无法被动恢复。
- `/healthz` 返回 `upstreams` / `upstreams_healthy` / `upstreams_picked` /
  `upstreams_skipped`，运维可一眼看出"几个上游还活着"。

## 8. 验证

DNS 中继与多上游都有单测（`dns_test.go`，10 项）：

```
TestDNSRelayForwardsAndRestoresID         # 转发 + 事务 ID 改回客户端原值
TestDNSRelayAvoidsIDCollision             # 4 个客户端同一 ID 不串台（核心动机）
TestDNSRelayFallsBackToTCPOnTruncation    # TC 位 → 自动改走 TCP 并清 TC
TestDNSRelayFailsOverToSecondUpstream     # 首个上游不可用 → 自动转移
TestDNSRelayFailsWhenAllUpstreamsDown     # 全部不可用 → 明确超时（不挂死）
TestUpstreamPoolRotatesAndSkipsUnhealthy  # 轮询覆盖全部 + 跳过不健康 + 恢复
TestUpstreamPoolFailureThreshold          # 未达阈值不移除（抗抖动）
TestUpstreamPoolNeverReturnsEmpty         # 全不健康仍返回候选（保证被动恢复）
TestUpstreamListMergesAndDedups           # 列表优先 + 去重去空白 + DNS 回落
TestConfigValidationDNSOnly               # 纯 DNS 中继配置合法
```

## 9. 已知限制

- **UDP 中继只覆盖 KCP**：DNS 走独立的、懂协议的那条路径（见 §6），不复用裸转发。
- 中继不记录 beacon 的真实 IP 到服务端（MetaInfo 里有，但那是 beacon 自报的）。
- 多上游是**连接级**轮询，不保证同一 beacon 的多次连接落在同一上游；
  C2 的会话是无状态的逐包轮询，因此这不影响正确性。
- 与 `docs/BACKUP_RESTORE.md` 同级别的运维要求：中继是**一次性**基础设施，
  被标记后直接弃用重建，不要在上面留任何持久状态。

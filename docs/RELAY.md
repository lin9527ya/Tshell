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

## 6. 已知限制

- **UDP 中继只覆盖 KCP**；DNS 传输基于 UDP 且需要按域名转发，未纳入中继。
- 中继不做**负载均衡/多上游**：一个中继只指向一个上游（多级链路本身提供了冗余方案）。
- 中继不记录 beacon 的真实 IP 到服务端（MetaInfo 里有，但那是 beacon 自报的）。
- 与 `docs/BACKUP_RESTORE.md` 同级别的运维要求：中继是**一次性**基础设施，
  被标记后直接弃用重建，不要在上面留任何持久状态。

# 重定向器（Redirector）

重定向器是 C2 与目标之间的一层：只把**画像里声明的那几个 URI**转发到真正的服务端，
其余请求一律送一个无害的诱饵站点。这样防守方顺着 beacon 的回连地址摸过来，
看到的是一个正常站点，而 C2 服务端本身不暴露在公网。

```
beacon ──TLS──▶ 重定向器(公网 VPS) ──内网/隧道──▶ C2 服务端
                    │
                    └─ 其他路径 ─▶ 诱饵站点
```

## 1. 为什么配置要生成而不是手写

手写的重定向器最容易出的问题是**与画像漂移**：改了画像里的 `uri`，忘了改 nginx，
beacon 直接失联 —— 而现场只表现为"上线失败"，排查方向完全错。

因此配置由**当前画像**生成，工具在生成时就把画像里的路径写进去：

```bash
# 在服务端所在目录（读 ./profiles/*.yaml）
./tshell -gen-redirector nginx \
  -profile default \
  -backend 10.10.0.5:8089 \
  -domain cdn.example.com \
  -decoy https://www.example.com \
  -full \
  -cert /etc/nginx/certs/cdn.example.com.crt \
  -key  /etc/nginx/certs/cdn.example.com.key \
  -out /etc/nginx/conf.d/tshell-redirector.conf
```

| 参数 | 说明 |
|---|---|
| `-backend` | **真正的 C2 服务端**地址 `host:port`（建议走内网或隧道，不要暴露在公网） |
| `-domain` | 对外域名（TLS 证书名 / `server_name`） |
| `-decoy` | 非画像请求的落点。给一个和目标业务气质相符的真实站点 |
| `-cert` / `-key` | TLS 证书路径（`-full` 时必填） |
| `-full` | 生成完整可用的 nginx 配置；不加则生成可 include 的 server 片段 |
| `-out` | 输出文件；不加则打印到 stdout |

也可以走接口（admin/operator），便于在别的机器上取：

```bash
curl -H "Authorization2: Bearer <token>" \
  "http://127.0.0.1:8089/api/redirector/template?type=nginx&backend=10.10.0.5:8089&domain=cdn.example.com&decoy=https://www.example.com"
```

> **apache 只生成 VirtualHost 片段**，不生成"完整配置"：apache 的模块集与路径按发行版
> 差异极大（Debian 是 `a2enmod` + `/etc/apache2/mods-enabled`，RHEL 是 `conf.modules.d`，
> 还有编译进内核的模块不能再次 `LoadModule`）。生成一份"看起来完整"的 apache 配置
> 只会在别的发行版上装不起来。把它 include 进你已经配好模块的 apache 即可，
> 所需模块在生成的文件头里列出。

## 2. 部署（nginx）

> HTTP/2 用兼容写法 `listen 443 ssl http2;`：nginx 1.9.5~1.30 都能解析（≥1.25.1 只提示
> deprecated）。若你的 nginx ≥ 1.25.1，可换成 `listen 443 ssl;` + `http2 on;` 消掉警告；
> 但反过来不行 —— 独立的 `http2 on;` 在 Ubuntu 24.04(1.24)/22.04(1.18)、Debian 12(1.22)、
> RHEL 9(1.20) 上会 `unknown directive "http2"` 直接启动失败。

```bash
# 1) 生成（见上）
# 2) 校验语法 —— 生成的是完整配置时用 -c；片段则先放进 conf.d 再 nginx -t
nginx -t -c /etc/nginx/conf.d/tshell-redirector.conf

# 3) 起服务
systemctl reload nginx      # 或 nginx -c ... -p ...
```

生成的内容包含这些**必须保留**的部分：

| 片段 | 作用 |
|---|---|
| `location = <画像路径>` | 只精确匹配画像里声明的路径（`http-get` / `http-post` / websocket） |
| `location = /` + `location /` 兜底 | 未声明的路径一律 **302 到诱饵站点**（返回 404 等于告诉对方这个域名上确实有东西） |
| `server_tokens off` / `ServerTokens Prod` | 不暴露服务器版本 |
| `proxy_hide_header X-Powered-By` 等 | 不泄露后端框架特征 |
| websocket 段的 `Upgrade`/`Connection` + 长超时 | 长连接隧道；超时太短会被 nginx 掐断 |

apache 版另有两点需要注意（生成的文件头已注明）：

- `ServerTokens Prod` / `ServerSignature Off` 必须写在**全局配置**里 ——
  写在 `<VirtualHost>` 内 apache 会直接报 `ServerTokens cannot occur within <VirtualHost> section`
  （这一条是生成器的测试用真实 `apache2 -t` 校验时发现的）；
- websocket 需要 `mod_proxy_wstunnel`，且必须显式 `RewriteCond %{HTTP:Upgrade} =websocket` ——
  apache 不会自动升级协议。

## 3. 校验：生成后先跑一遍

生成器自带语法校验（用真实的 `nginx -t` / `apache2 -t`，本机没装则跳过）：

```bash
go test ./pkg/redirector/ -run "TestNginxAccepts|TestApacheAccepts" -v
```

部署后建议手工确认三件事：

```bash
# 1) 画像路径确实转发到后端（后端日志应出现该请求）
curl -sk -H "Host: cdn.example.com" https://<重定向器>/tencent/mcp/pc/pcsearch

# 2) 未声明的路径跳诱饵
curl -sk -o /dev/null -w '%{http_code} %{redirect_url}\n' https://<重定向器>/anything/else

# 3) 后端特征头被隐藏
curl -skI https://<重定向器>/tencent/mcp/pc/pcsearch | grep -i 'x-powered-by'
```

## 4. 运维要点

- **改画像后要重新生成**：画像的 `http.get.uri` / `http.post.uri` / `ws.path` 一旦变化，
  重定向器必须同步，否则 beacon 失联。生成器输出的文件头写着"请勿手改"，就是为了防止
  两边各改一半。
- **`-backend` 不要暴露在公网**：重定向器到服务端这一段建议走内网、WireGuard 或 SSH 隧道。
  重定向器本身被打穿时，至少不要把服务端地址一起交出去。
- **诱饵要像真的**：302 到一个空白页、或跳到与目标业务无关的站点，本身就是可疑信号。
  有条件时让诱饵就是该域名原本的站点内容。
- **证书用真域名签**：自签证书是明显的重定向器特征（浏览器/EDR 都会标记）。
- **多级重定向**：本工具只生成一级。多级中继（重定向器 → 二级中继 → 服务端）尚未实现，
  见 `docs/KNOWN_LIMITATIONS.md` #13。

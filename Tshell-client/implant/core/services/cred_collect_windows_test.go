//go:build windows

package services

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"
)

// Windows 侧的采集入口在"环境不具备"时必须显式报错，而不是产出空制品。
// 这两条在真实 Windows 上跑（开发机/CI 的 windows runner 都适用）。

func TestCollectDPAPIReportsMissingDir(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "dpapi.zip")
	// 指向一个不存在的目录：必须报错（正常环境里 Protect\<SID> 一定存在，
	// 不存在就说明环境异常，此时产出空 ZIP 会让操作员以为"没有主密钥"）
	_, n, err := collectDPAPIMasterKeysIn(dest, filepath.Join(t.TempDir(), "no-such-protect-dir"))
	if err == nil {
		t.Fatalf("目录不存在时应报错，实际返回 %d 把密钥", n)
	}
}

func TestCollectDPAPIProducesZipOnRealProfile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "dpapi.zip")
	path, n, err := collectDPAPIMasterKeys(dest)
	if err != nil {
		// 环境不具备时跳过（不算失败）：没有 APPDATA（服务上下文）、目录为空、
		// 或者本机的主密钥文件全都解不开（开发机上的 Protect 目录常常是这种状态）
		for _, skip := range []string{
			"读取 DPAPI 主密钥目录失败", "没有取到任何主密钥", "DPAPI 解密失败",
		} {
			if strings.Contains(err.Error(), skip) {
				t.Skipf("本机没有可用的用户主密钥: %v", err)
			}
		}
		t.Fatalf("采集失败: %v", err)
	}
	if n < 1 {
		t.Fatalf("应至少取到 1 把主密钥，实际 %d", n)
	}
	// 制品必须是服务端能校验通过的 ZIP：含 manifest.json 且条目可完整读取
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("制品不是合法 ZIP: %v", err)
	}
	defer zr.Close()
	hasManifest := false
	keys := 0
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			hasManifest = true
		}
		if strings.HasPrefix(f.Name, "keys/") {
			keys++
			// 条目必须能完整读出（服务端会逐个读一遍做 CRC 校验）
			if _, err := f.Open(); err != nil {
				t.Fatalf("条目 %s 打不开: %v", f.Name, err)
			}
		}
	}
	if !hasManifest {
		t.Fatal("制品缺少 manifest.json（服务端会拒绝登记）")
	}
	if keys != n {
		t.Fatalf("制品里 keys/ 条目 = %d，与返回的密钥数 %d 不一致", keys, n)
	}
}

// 技术白名单：拼错的值必须报错，而不是"不过滤、把两家都采了"
func TestCollectBrowserCredentialsRejectsUnknownTech(t *testing.T) {
	for _, tech := range []string{"nonexistent-browser", "firefox", "ALL2", ""} {
		art, err := collectBrowserCredentials(tech)
		if tech == "" {
			// 空值由上层补默认（all），这里直接调用应当报错
			if err == nil {
				t.Fatalf("技术 %q 应报错", tech)
			}
			continue
		}
		if err == nil {
			t.Fatalf("技术 %q 应报错，实际返回 %d 个配置", tech, len(art.Browsers))
		}
		if len(art.Browsers) != 0 {
			t.Fatalf("失败时不应带回数据: %+v", art.Browsers)
		}
	}
}

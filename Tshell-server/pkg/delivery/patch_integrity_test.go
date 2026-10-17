package delivery

// 回归测试：patch 必须保持载荷文件等长且 PE 结构完好。
// 背景：替换值超过占位符长度时 bytes.ReplaceAll 会撑大文件，PE 各节文件偏移
// 整体错位，.reloc 读到全零块头（SizeOfBlock=0），Binject/debug/pe 中
// (SizeOfBlock-8)/2 uint32 下溢分配 8GB+ 内存导致服务端 OOM kill。
// URI 占位符已升级为 128 字符（新模板）；testdata 保留旧 31 字符占位符模板，
// 验证服务端对新旧模板的双兼容。
import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Binject/debug/pe"
)

func loadTemplate(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip("template not found:", err)
	}
	if bytes.HasPrefix(raw, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return raw
}

// loadTemplateForTest 当前模板（占位符升级后为 128 字符版本）
func loadTemplateForTest(t *testing.T) []byte {
	return loadTemplate(t, "../api/server/tcp/tcp_windows_amd64.exe")
}

// loadOldTemplateForTest 旧模板（31 字符占位符，升级前备份）
func loadOldTemplateForTest(t *testing.T) []byte {
	return loadTemplate(t, "testdata/old_tcp_windows_amd64.exe.gz")
}

const testLongPostURI = "/api/v2/cdn/static/assets/sensearch/collection/item/check?ver=2"

// 新模板：patch 后文件必须等长且可被 PE 解析器安全处理
func TestPatchClientBinaryKeepsSizeAndPEIntegrity(t *testing.T) {
	tpl := loadTemplateForTest(t)

	patched, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
		Pass:           "",
	})
	if err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	if len(patched) != len(tpl) {
		t.Fatalf("patch changed file size: %d -> %d (must be equal-length)", len(tpl), len(patched))
	}
	if _, err := pe.NewFile(bytes.NewReader(patched)); err != nil {
		t.Fatalf("patched template failed PE parse: %v", err)
	}
}

// 新模板：标准默认 post URI（40 字符）现在可直接注入（占位符 128 字符）
func TestPatchClientBinaryAllowsDefaultPostURIFromProfile(t *testing.T) {
	tpl := loadTemplateForTest(t)
	patched, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
		HTTPPostURI:    "/tencent/sensearch/collection/item/check", // 标准默认值
	})
	if err != nil {
		t.Fatalf("profile 默认 URI 不应报错: %v", err)
	}
	if len(patched) != len(tpl) {
		t.Fatalf("patch changed file size: %d -> %d", len(tpl), len(patched))
	}
	if !bytes.Contains(patched, []byte("/tencent/sensearch/collection/item/check")) {
		t.Fatal("默认 URI 应被直接注入（新占位符可容纳）")
	}
}

// 新模板：自定义长 URI（>31 字符）可注入
func TestPatchClientBinaryInjectsLongCustomURI(t *testing.T) {
	tpl := loadTemplateForTest(t)
	patched, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
		HTTPPostURI:    testLongPostURI, // 62 字符
	})
	if err != nil {
		t.Fatalf("长 URI 注入失败: %v", err)
	}
	if len(patched) != len(tpl) {
		t.Fatalf("patch changed file size: %d -> %d", len(tpl), len(patched))
	}
	if !bytes.Contains(patched, []byte(testLongPostURI)) {
		t.Fatal("长 URI 未出现在 patch 结果中")
	}
	if bytes.Contains(patched, []byte("PLACEHOLDER_1234567890")) {
		t.Fatal("新占位符残留")
	}
	if _, err := pe.NewFile(bytes.NewReader(patched)); err != nil {
		t.Fatalf("长 URI patch 后 PE 解析失败: %v", err)
	}
}

// 用户提供的超长 profile URI（超过新占位符 128 字符）必须报错而不是撑大文件
func TestPatchClientBinaryRejectsOversizeUserValue(t *testing.T) {
	tpl := loadTemplateForTest(t)
	_, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
		HTTPPostURI:    "/" + strings.Repeat("a", 200),
	})
	if err == nil {
		t.Fatal("oversize user URI must be rejected")
	}
}

// 用户提供的超长密码必须报错而不是撑大文件
func TestPatchClientBinaryRejectsOversizePass(t *testing.T) {
	tpl := loadTemplateForTest(t)
	_, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
		Pass:           strings.Repeat("p", 100),
	})
	if err == nil {
		t.Fatal("oversize pass must be rejected")
	}
}

// 旧模板（31 字符占位符）兼容：短 URI 注入旧占位符
func TestPatchClientBinaryOldTemplateShortURI(t *testing.T) {
	tpl := loadOldTemplateForTest(t)
	patched, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
		HTTPPostURI:    "/short/post", // 11 字符 < 31
	})
	if err != nil {
		t.Fatalf("旧模板短 URI patch 失败: %v", err)
	}
	if len(patched) != len(tpl) {
		t.Fatalf("patch changed file size: %d -> %d", len(tpl), len(patched))
	}
	if !bytes.Contains(patched, []byte("/short/post")) {
		t.Fatal("短 URI 未注入旧占位符")
	}
	if bytes.Contains(patched, []byte("HTTP_POST_URI_PLACEHOLDER")) {
		t.Fatal("旧占位符残留")
	}
}

// 旧模板：超过旧占位符（31 字符）的自定义 URI 报错；默认值置空回退
func TestPatchClientBinaryOldTemplateOversizeHandling(t *testing.T) {
	tpl := loadOldTemplateForTest(t)

	// 自定义超长 -> 报错
	if _, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
		HTTPPostURI:    testLongPostURI,
	}); err == nil {
		t.Fatal("旧模板超长自定义 URI 必须报错")
	}

	// 默认值（40 字符）-> 置空回退，不报错、等长、无残留
	patched, err := PatchClientBinary(tpl, PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://192.168.10.5:1111",
		OS:             "windows",
		Arch:           "amd64",
	})
	if err != nil {
		t.Fatalf("旧模板默认 URI 不应报错: %v", err)
	}
	if len(patched) != len(tpl) {
		t.Fatalf("patch changed file size: %d -> %d", len(tpl), len(patched))
	}
	if bytes.Contains(patched, []byte("HTTP_POST_URI_PLACEHOLDER")) {
		t.Fatal("旧占位符残留（应已置空替换）")
	}
	if _, err := pe.NewFile(bytes.NewReader(patched)); err != nil {
		t.Fatalf("旧模板 patch 后 PE 解析失败: %v", err)
	}
}

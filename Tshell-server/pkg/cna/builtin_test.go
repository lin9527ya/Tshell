package cna

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuiltinOpenConfined 固定 open() 的边界：只能读脚本目录内的文件。
// 原实现是裸 os.ReadFile(任意路径)，operator 经 /api/cna/eval 就能读服务端任意文件。
func TestBuiltinOpenConfined(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "scripts")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 指向根目录外的符号链接：os.Root 必须拒绝跟随
	if err := os.Symlink(secret, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		root    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "脚本目录内文件可读", root: root, path: "data.txt", want: "hello"},
		{name: "子路径可读", root: root, path: "./data.txt", want: "hello"},
		{name: "系统文件被拒", root: root, path: "/etc/passwd", wantErr: true},
		{name: "绝对路径不能逃逸根目录", root: root, path: secret, wantErr: true},
		{name: "相对路径不能上跳", root: root, path: "../secret.txt", wantErr: true},
		{name: "符号链接不能逃逸根目录", root: root, path: "escape.txt", wantErr: true},
		{name: "未绑定根目录时拒绝", root: "", path: "data.txt", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := newInterpIn(tc.root)
			v, err := builtinOpen(ip, nil, []Value{tc.path})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("open(%q) 未被拒绝，返回 %q", tc.path, ToString(v))
				}
				if s := ToString(v); strings.Contains(s, "TOP-SECRET") {
					t.Fatalf("open(%q) 泄漏了根目录外的内容", tc.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("open(%q) 失败: %v", tc.path, err)
			}
			if ToString(v) != tc.want {
				t.Fatalf("open(%q) = %q，期望 %q", tc.path, ToString(v), tc.want)
			}
		})
	}
}

// TestBuiltinOpenEngineWiring 端到端确认引擎把脚本目录注入了解释器
// （否则上面的单元测试通过、实际执行时 rootDir 仍为空 → 脚本里 open() 全废）
func TestBuiltinOpenEngineWiring(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "payload.txt"), []byte("from-script"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Engine{scripts: make(map[string]*Script), dir: root}

	res := e.EvalScript(`println(open("payload.txt"))`, "")
	if res.Error != "" {
		t.Fatalf("脚本执行报错: %s", res.Error)
	}
	if len(res.Output) == 0 || !strings.Contains(strings.Join(res.Output, "\n"), "from-script") {
		t.Fatalf("脚本未能读到脚本目录内文件，输出=%v", res.Output)
	}
}

// TestBuiltinGetenvAllowlist getenv 读的是服务端进程环境（含密钥类变量），只放行白名单
func TestBuiltinGetenvAllowlist(t *testing.T) {
	t.Setenv("TSHELL_TEST_SECRET", "leak-me")
	ip := newInterpIn(t.TempDir())

	v, err := builtinGetenv(ip, nil, []Value{"TSHELL_TEST_SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	if ToString(v) != "" {
		t.Fatalf("敏感环境变量被读出: %q", ToString(v))
	}

	// 白名单内的变量照常可读（大小写不敏感）
	if _, err := builtinGetenv(ip, nil, []Value{"PATH"}); err != nil {
		t.Fatalf("白名单变量读取报错: %v", err)
	}
	if _, err := builtinGetenv(ip, nil, []Value{"path"}); err != nil {
		t.Fatalf("白名单变量（小写）读取报错: %v", err)
	}
}

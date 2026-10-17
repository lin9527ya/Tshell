package cna

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestEngine 构造一个只指向临时脚本目录的引擎（不碰全局 Global），
// 并把墙钟上限压到毫秒级，避免测试真的等 30 秒。
func newTestEngine(t *testing.T, evalTimeout time.Duration) *Engine {
	t.Helper()
	dir := t.TempDir()
	return &Engine{
		scripts:      make(map[string]*Script),
		dir:          dir,
		evalTimeout:  evalTimeout,
		eventTimeout: evalTimeout,
	}
}

// TestEvalLoopTimeout 空循环体与"慢循环"都必须被墙钟拦下。
// 修复前 while(true){} 是永久自旋（步数只在 ExecBlock 的语句循环里累加，
// 空循环体一次都不进），会把一个 handler goroutine 永久占住。
func TestEvalLoopTimeout(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"while 空循环体", `while(true){}`},
		{"for 空循环体", `for(;;){}`},
		{"while 空块", `while(1){ }`},
		{"每步都很慢的循环", `while(true){ sleep(100); }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine(t, 150*time.Millisecond)
			done := make(chan *ExecResult, 1)
			go func() { done <- e.EvalScript(tc.src, "") }()

			select {
			case res := <-done:
				if !strings.Contains(res.Error, "超时") {
					t.Fatalf("期望超时终止，实际 error=%q", res.Error)
				}
			case <-time.After(20 * time.Second):
				t.Fatalf("脚本 %q 未被终止（墙钟护栏失效）", tc.src)
			}
		})
	}
}

// TestEvalStepLimit 无墙钟时仍由步数上限兜底（原有语义保留）
func TestEvalStepLimit(t *testing.T) {
	e := newTestEngine(t, 0)
	res := e.EvalScript(`while(true){}`, "")
	if !strings.Contains(res.Error, "执行步数超限") {
		t.Fatalf("期望步数超限，实际 error=%q", res.Error)
	}
}

// TestRecursionDepthGuard 无限递归必须在进入前拦住。
// Go 的栈溢出是 runtime fatal error（goroutine stack exceeds），recover 抓不住、
// 进程直接退出——修复前 sub f(){f();} f(); 能打崩整个服务端。
func TestRecursionDepthGuard(t *testing.T) {
	e := newTestEngine(t, 5*time.Second)
	res := e.EvalScript(`sub f(){ f(); } f();`, "")
	if !strings.Contains(res.Error, "调用层级过深") {
		t.Fatalf("期望递归深度拦截，实际 error=%q", res.Error)
	}
}

// TestRecursionWithinLimit 正常深度的递归不受影响（护栏不能误伤）
func TestRecursionWithinLimit(t *testing.T) {
	e := newTestEngine(t, 5*time.Second)
	res := e.EvalScript(`
sub countdown(n){
    if (n <= 0) { return 0; }
    return countdown(n - 1);
}
println(countdown(50));
`, "")
	if res.Error != "" {
		t.Fatalf("正常递归报错: %s", res.Error)
	}
	if !strings.Contains(strings.Join(res.Output, "\n"), "0") {
		t.Fatalf("正常递归未产出结果: %v", res.Output)
	}
}

// TestPanicRecovered 解释器内的 panic 必须变成脚本错误而不是掀掉进程。
// 用测试专用 builtin 精确制造 panic（真实场景来自类型断言/越界等）。
func TestPanicRecovered(t *testing.T) {
	builtinFuncs["__test_panic"] = func(ip *Interp, env *Env, args []Value) (Value, error) {
		panic("boom")
	}
	defer delete(builtinFuncs, "__test_panic")

	e := newTestEngine(t, 5*time.Second)
	res := e.EvalScript(`println("before"); __test_panic(); println("after");`, "")
	if !strings.Contains(res.Error, "脚本 panic") {
		t.Fatalf("panic 未被转成脚本错误，error=%q", res.Error)
	}
}

// TestEventPanicRecovered 事件分发路径（裸 goroutine，无 Gin Recovery）同样要兜住 panic
func TestEventPanicRecovered(t *testing.T) {
	builtinFuncs["__test_panic"] = func(ip *Interp, env *Env, args []Value) (Value, error) {
		panic("boom-event")
	}
	defer delete(builtinFuncs, "__test_panic")

	e := newTestEngine(t, 5*time.Second)
	if err := os.WriteFile(filepath.Join(e.dir, "t.cna"),
		[]byte("on beacon_initial { __test_panic(); }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Load("t.cna"); err != nil {
		t.Fatalf("加载脚本失败: %v", err)
	}

	// 不 panic 逃逸即为通过（引擎内部已 recover 并记日志）
	e.FireEvent("beacon_initial", map[string]Value{"_beacon_id": "u1"})
}

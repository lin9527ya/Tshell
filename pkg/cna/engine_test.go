package cna

import (
	"os"
	"strings"
	"testing"
)

func runScript(t *testing.T, src string, ctx map[string]Value) ([]string, error) {
	t.Helper()
	stmts, err := Parse(src)
	if err != nil {
		return nil, err
	}
	ip := NewInterp()
	var logs []string
	ip.Log = func(f string, args ...interface{}) {
		logs = append(logs, strings.TrimSpace(sprintf(f, args...)))
	}
	env := NewEnv(ip.globals)
	env.parent = ip.globals
	for k, v := range ctx {
		env.SetLocal(k, v)
	}
	// 先注册 sub 与 on 处理器（模拟引擎加载）
	var remaining []Stmt
	for _, st := range stmts {
		switch s := st.(type) {
		case *SubStmt:
			ip.defineSub(s, env)
		case *OnStmt:
			fn := &SubRef{Name: s.Event, Params: s.Fn.Params, Body: s.Fn.Body, Closure: env}
			ip.subs["__on_"+s.Event] = fn
			if uid, ok := ctx["_beacon_id"]; ok {
				if _, err := ip.CallSub(fn, []Value{uid}, env); err != nil {
					return logs, err
				}
			}
		default:
			remaining = append(remaining, st)
		}
	}
	if _, err := ip.ExecBlock(&Block{Stmts: remaining}, env); err != nil {
		return logs, err
	}
	return logs, nil
}

func sprintf(f string, args ...interface{}) string {
	parts := []string{f}
	for _, a := range args {
		parts = append(parts, ToString(a))
	}
	return strings.Join(parts, " ")
}

// TestBasicPrint 基础 println 与字符串
func TestBasicPrint(t *testing.T) {
	logs, err := runScript(t, `println("hello");`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "hello") {
		t.Fatalf("got %v", logs)
	}
}

// TestVars 变量与拼接
func TestVars(t *testing.T) {
	logs, err := runScript(t, `
$x = "world";
println("hello " . $x);
$n = 3 + 4;
println("n=" . $n);
`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(logs) != 2 || !strings.Contains(logs[0], "hello world") || !strings.Contains(logs[1], "n=7") {
		t.Fatalf("got %v", logs)
	}
}

// TestIfForeach 控制流
func TestIfForeach(t *testing.T) {
	src := `
@arr = @("a", "b", "c");
$out = "";
foreach $x (@arr) {
  if ($x == "b") {
    $out = $out . "[" . $x . "]";
  } else {
    $out = $out . $x;
  }
}
println($out);
$i = 0;
while ($i < 3) { $i = $i + 1; }
println("i=" . $i);
`
	logs, err := runScript(t, src, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(logs[0], "a[b]c") || !strings.Contains(logs[1], "i=3") {
		t.Fatalf("got %v", logs)
	}
}

// TestSubFunction 函数定义与调用（含 $1 参数）
func TestSubFunction(t *testing.T) {
	src := `
sub twice {
  $r = $1;
  return $r . $r;
}
$s = twice("ab");
println($s);
sub add ( $a, $b ) {
  return $a + $b;
}
println(add(2, 3));
`
	logs, err := runScript(t, src, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(logs[0], "abab") || !strings.Contains(logs[1], "5") {
		t.Fatalf("got %v", logs)
	}
}

// TestOnEvent 事件处理器
func TestOnEvent(t *testing.T) {
	src := `
on beacon_initial {
  println("beacon up: " . $1);
}
on beacon_initial {
  println("second handler");
}
`
	ctx := map[string]Value{"_beacon_id": "abc123"}
	logs, err := runScript(t, src, ctx)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(logs) != 2 || !strings.Contains(logs[0], "abc123") || !strings.Contains(logs[1], "second") {
		t.Fatalf("got %v", logs)
	}
}

// TestInterpolation 字符串插值
func TestInterpolation(t *testing.T) {
	logs, err := runScript(t, `
$name = "tsh";
println("name is $name");
println("calc: [1 + 2]");
`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(logs[0], "name is tsh") || !strings.Contains(logs[1], "calc: 3") {
		t.Fatalf("got %v", logs)
	}
}

// TestMethods 方法调用
func TestMethods(t *testing.T) {
	logs, err := runScript(t, `
$s = "  hello,world  ";
$t = $s.trim().split(",");
println("first=" . $t[0] . " len=" . size($t));
println("upper=" . "abc".to_upper());
`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(logs[0], "first=hello len=2") || !strings.Contains(logs[1], "upper=ABC") {
		t.Fatalf("got %v", logs)
	}
}

// TestDict 字典
func TestDict(t *testing.T) {
	logs, err := runScript(t, `
%d = %("k1" => "v1", "k2" => "v2");
println("k1=" . %d["k1"] . " size=" . size(%d));
`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(logs[0], "k1=v1 size=2") {
		t.Fatalf("got %v", logs)
	}
}

// TestComment 注释
func TestComment(t *testing.T) {
	logs, err := runScript(t, `
# 这是注释
$x = 1; # 行尾注释
println("ok");
`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "ok") {
		t.Fatalf("got %v", logs)
	}
}

// TestEngine 引擎级加载/事件
func TestEngine(t *testing.T) {
	dir := t.TempDir()
	eng := &Engine{scripts: make(map[string]*Script), dir: dir}
	// 注入临时 ExecFunc 捕获
	execs := []string{}
	ExecFunc = func(uid, cmd string) {
		execs = append(execs, uid+"|"+cmd)
	}
	defer func() { ExecFunc = nil }()
	scriptPath := dir + "/test.cna"
	writeFile(t, scriptPath, `
sub send_id {
  exec(beacon_id(), "whoami");
}
on beacon_initial {
  send_id();
  alert("beacon up: " . $1);
}
on ready {
  println("ready event");
}
`)
	if _, err := eng.Load("test.cna"); err != nil {
		t.Fatalf("load err: %v", err)
	}
	if !eng.HasEvent("beacon_initial") || !eng.HasEvent("ready") {
		t.Fatalf("events not registered")
	}
	eng.FireClientEvent("beacon_initial", "uid-123", map[string]interface{}{"os": "windows"})
	if len(execs) != 1 || execs[0] != "uid-123|whoami" {
		t.Fatalf("execs: %v", execs)
	}
	if err := eng.Unload("test.cna"); err != nil {
		t.Fatalf("unload err: %v", err)
	}
	if eng.HasEvent("beacon_initial") {
		t.Fatalf("events should be cleared after unload")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestAliasParse alias 解析与注册
func TestAliasParse(t *testing.T) {
	stmts, err := Parse(`
alias mycmd {
  println("alias run");
}
`)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	found := false
	for _, st := range stmts {
		if a, ok := st.(*AliasStmt); ok && a.Name == "mycmd" {
			found = true
		}
	}
	if !found {
		t.Fatalf("alias not parsed")
	}
}

// TestAliasMenu 引擎级 alias/menu 执行
func TestAliasMenu(t *testing.T) {
	dir := t.TempDir()
	eng := &Engine{scripts: make(map[string]*Script), dir: dir}
	ExecFunc = func(uid, cmd string) {}
	defer func() { ExecFunc = nil }()
	scriptPath := dir + "/test.cna"
	writeFile(t, scriptPath, `
alias hello {
  println("hello " . $1);
}
menu beacon {
  item "Run Thing" {
    println("thing ran");
    exec(beacon_id(), "whoami");
  }
  separator;
  menu "Sub" {
    item "Deep" {
      println("deep ran");
    }
  }
}
`)
	if _, err := eng.Load("test.cna"); err != nil {
		t.Fatalf("load err: %v", err)
	}
	// alias 执行
	res, hit := eng.ExecAlias("hello", []string{"world"}, "uid-1")
	if !hit || len(res.Output) != 1 || !strings.Contains(res.Output[0], "hello world") {
		t.Fatalf("alias exec: hit=%v out=%v", hit, res)
	}
	// 不存在的 alias
	if _, hit := eng.ExecAlias("nope", nil, ""); hit {
		t.Fatalf("unknown alias should not hit")
	}
	// menus 注册
	menus := eng.Menus()
	if len(menus) != 1 || menus[0].Kind != "beacon" || len(menus[0].Items) != 3 {
		t.Fatalf("menus: %+v", menus)
	}
	// 菜单执行
	res, hit = eng.ExecMenuItem("beacon", []string{"Run Thing"}, "uid-2")
	if !hit || len(res.Output) != 1 || !strings.Contains(res.Output[0], "thing ran") {
		t.Fatalf("menu exec: hit=%v out=%v", hit, res)
	}
	// 嵌套菜单执行
	res, hit = eng.ExecMenuItem("beacon", []string{"Sub", "Deep"}, "uid-3")
	if !hit || len(res.Output) != 1 || !strings.Contains(res.Output[0], "deep ran") {
		t.Fatalf("nested menu exec: hit=%v out=%v", hit, res)
	}
}

// TestDataQueryBridge data_query 桥接（注入 fake 数据源）
func TestDataQueryBridge(t *testing.T) {
	DataQueryFunc = func(query string) Value {
		return []Value{
			map[string]Value{
				"uid":      "abc123",
				"computer": "WIN-01",
				"arch":     "x64",
				"online":   "1",
			},
		}
	}
	defer func() { DataQueryFunc = nil }()
	logs, err := runScript(t, `
@b = data_query("select .* from beacon");
println("count: " . size(@b));
%r = @b[0];
println("host: " . %r["computer"] . " arch: " . %r["arch"]);
`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(logs[0], "count: 1") || !strings.Contains(logs[1], "host: WIN-01 arch: x64") {
		t.Fatalf("got %v", logs)
	}
}

// TestArtifactPayloadBridge artifact_payload 桥接（注入 fake 生成器）
func TestArtifactPayloadBridge(t *testing.T) {
	ArtifactPayloadFunc = func(format, arch string) ([]byte, error) {
		if format != "exe" || arch != "x64" {
			t.Fatalf("unexpected args: %s %s", format, arch)
		}
		return []byte("MZ-fake-payload-bytes"), nil
	}
	defer func() { ArtifactPayloadFunc = nil }()
	logs, err := runScript(t, `
$p = artifact_payload("exe", "x64");
println("magic: " . substr($p, 0, 2) . " len: " . strlen($p));
println("is PE: " . ($p == "MZ-fake-payload-bytes"));
`, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(logs[0], "magic: MZ len: 21") || !strings.Contains(logs[1], "is PE: true") {
		t.Fatalf("got %v", logs)
	}
}

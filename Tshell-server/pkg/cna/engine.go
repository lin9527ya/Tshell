package cna

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ===== 引擎级回调（由 main/上层注入，避免包依赖环）=====

// ExecFunc 向客户端派发命令（注入 sendcommand.SendCommand）
var ExecFunc func(uid, command string)

// LhostFunc 返回首选监听主机（注入）
var LhostFunc func() string

// ListenersFunc 返回监听器列表（注入）
var ListenersFunc func() []Value

// DataQueryFunc 数据查询（注入）
var DataQueryFunc func(query string) Value

// ArtifactPayloadFunc 生成载荷 artifact（注入，format: exe/dll/svc-exe, arch: x64/x86）
var ArtifactPayloadFunc func(format, arch string) ([]byte, error)

// EngineLog 引擎日志/告警回调（alert 等）
var EngineLog func(kind, message string)

// EventLog 事件执行日志回调（前端可展示）
var EventLog func(script, event string, err error, output []string)

// ===== 脚本管理 =====

// ScriptInfo 已加载脚本信息
type ScriptInfo struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Events  []string `json:"events"`
	Aliases []string `json:"aliases"`
	Loaded  bool     `json:"loaded"`
}

// Script 单个脚本运行时状态
type Script struct {
	Name    string
	Path    string
	ip      *Interp
	events  map[string][]*SubRef // 事件名 -> 处理器
	aliases map[string]*SubRef   // alias 名 -> 处理器
	menus   map[string][]*MenuItem // 菜单类型 -> 项
	execMu  sync.Mutex // alias/menu 执行互斥（保护临时 Log 捕获）
}

// ExecResult 一次 alias/menu 执行结果
type ExecResult struct {
	Output []string // 脚本 println/alert 输出
	Error  string   // 执行错误（空=成功）
}

// Engine CNA 脚本引擎
type Engine struct {
	mu      sync.RWMutex
	scripts map[string]*Script
	order   []string
	dir     string
}

// Global 全局引擎
var Global *Engine

// InitEngine 初始化引擎（幂等）
func InitEngine(dir string) *Engine {
	if Global != nil {
		return Global
	}
	Global = &Engine{
		scripts: make(map[string]*Script),
		dir:     dir,
	}
	_ = os.MkdirAll(dir, 0o755)
	return Global
}

// Dir 脚本目录
func (e *Engine) Dir() string { return e.dir }

// List 已加载脚本列表
func (e *Engine) List() []ScriptInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var out []ScriptInfo
	for _, name := range e.order {
		s := e.scripts[name]
		evts := make([]string, 0, len(s.events))
		for k := range s.events {
			evts = append(evts, k)
		}
		sort.Strings(evts)
		als := make([]string, 0, len(s.aliases))
		for k := range s.aliases {
			als = append(als, k)
		}
		sort.Strings(als)
		out = append(out, ScriptInfo{
			Name:    s.Name,
			Path:    s.Path,
			Events:  evts,
			Aliases: als,
			Loaded:  true,
		})
	}
	return out
}

// Load 加载脚本文件（若已加载则先卸载）
func (e *Engine) Load(name string) (*Script, error) {
	// 校验文件名安全
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("脚本名不合法")
	}
	if filepath.Ext(name) != ".cna" {
		return nil, fmt.Errorf("仅支持 .cna 脚本")
	}
	full := filepath.Join(e.dir, name)
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("读取脚本失败: %v", err)
	}
	src := string(data)

	e.mu.Lock()
	if old, ok := e.scripts[name]; ok {
		// 卸载旧实例
		_ = old
		delete(e.scripts, name)
		for i, n := range e.order {
			if n == name {
				e.order = append(e.order[:i], e.order[i+1:]...)
				break
			}
		}
	}
	e.mu.Unlock()

	// 解析
	stmts, err := Parse(src)
	if err != nil {
		return nil, fmt.Errorf("解析失败: %v", err)
	}

	s := &Script{
		Name:    name,
		Path:    full,
		ip:      newInterpIn(e.dir),
		events:  make(map[string][]*SubRef),
		aliases: make(map[string]*SubRef),
		menus:   make(map[string][]*MenuItem),
	}
	s.ip.Log = func(format string, args ...interface{}) {
		if EngineLog != nil {
			EngineLog("log", fmt.Sprintf("[%s] %s", name, fmt.Sprintf(format, args...)))
		}
	}

	// 注册全局函数与声明
	s.ip.globals.SetLocal("script_path", full)
	for _, st := range stmts {
		switch t := st.(type) {
		case *SubStmt:
			s.ip.defineSub(t, s.ip.globals)
		case *OnStmt:
			fn := &SubRef{Name: t.Event, Params: t.Fn.Params, Body: t.Fn.Body, Closure: s.ip.globals}
			s.events[t.Event] = append(s.events[t.Event], fn)
		case *AliasStmt:
			fn := &SubRef{Name: t.Name, Params: t.Fn.Params, Body: t.Fn.Body, Closure: s.ip.globals}
			s.aliases[t.Name] = fn
		case *MenuStmt:
			s.menus[t.Kind] = append(s.menus[t.Kind], t.Items...)
		case *LocalStmt:
			for _, sub := range t.Stmts {
				if as, ok := sub.(*Assign); ok {
					v, err := s.ip.eval(as.Value, s.ip.globals)
					if err == nil {
						s.ip.globals.SetLocal(as.Name, v)
					}
				}
			}
		case *GlobalStmt:
			for _, sub := range t.Stmts {
				if as, ok := sub.(*Assign); ok {
					v, err := s.ip.eval(as.Value, s.ip.globals)
					if err == nil {
						s.ip.globals.SetLocal(as.Name, v)
					}
				}
			}
		case *ExprStmt:
			// 顶层表达式（可能是函数调用）
			if _, err := s.ip.eval(t.Expr, s.ip.globals); err != nil {
				logScriptError(name, "顶层执行", err)
			}
		}
	}

	e.mu.Lock()
	e.scripts[name] = s
	e.order = append(e.order, name)
	e.mu.Unlock()

	// 脚本加载完成后触发 load / ready 事件（CS 语义：Aggressor 加载脚本时触发）
	e.FireEvent("load", nil)
	e.FireEvent("ready", nil)

	return s, nil
}

// Unload 卸载脚本
func (e *Engine) Unload(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.scripts[name]; !ok {
		return fmt.Errorf("脚本未加载: %s", name)
	}
	delete(e.scripts, name)
	for i, n := range e.order {
		if n == name {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
	return nil
}

// Reload 重载脚本
func (e *Engine) Reload(name string) error {
	_, err := e.Load(name)
	return err
}

// FireEvent 触发事件（跨所有已加载脚本）
// ctx 为 beacon 上下文（uid 等），可为 nil
func (e *Engine) FireEvent(event string, ctx map[string]Value) {
	e.mu.RLock()
	scripts := make([]*Script, 0, len(e.scripts))
	for _, name := range e.order {
		scripts = append(scripts, e.scripts[name])
	}
	e.mu.RUnlock()

	for _, s := range scripts {
		e.mu.RLock()
		handlers := s.events[event]
		e.mu.RUnlock()
		if len(handlers) == 0 {
			continue
		}
		for _, h := range handlers {
			env := NewEnv(s.ip.globals)
			if h.Closure != nil {
				env = NewEnv(h.Closure)
				env.parent = h.Closure
			}
			for k, v := range ctx {
				env.SetLocal(k, v)
			}
			args := make([]Value, 0, 1)
			if ctx != nil {
				if uid, ok := ctx["_beacon_id"]; ok {
					args = append(args, uid)
				}
			}
			_, err := s.ip.ExecSubIn(h, args, env)
			if err != nil {
				logScriptError(s.Name, "事件 "+event, err)
			}
		}
	}
}

// FireClientEvent 客户端事件：beacon_initial 等
// detail 中 uid/arch/os/host/user/ip/listener 可用
func (e *Engine) FireClientEvent(event, uid string, detail map[string]interface{}) {
	if e == nil || uid == "" {
		return
	}
	ctx := map[string]Value{
		"_beacon_id": uid,
		"_beacon_arch": "",
		"_beacon_os":   "",
		"_beacon_host": "",
		"_beacon_user": "",
		"_beacon_ip":   "",
	}
	if detail != nil {
		if v, ok := detail["arch"]; ok {
			ctx["_beacon_arch"] = ToString(v)
		}
		if v, ok := detail["os"]; ok {
			ctx["_beacon_os"] = ToString(v)
		}
		if v, ok := detail["host"]; ok {
			ctx["_beacon_host"] = ToString(v)
		}
		if v, ok := detail["user"]; ok {
			ctx["_beacon_user"] = ToString(v)
		}
		if v, ok := detail["ip"]; ok {
			ctx["_beacon_ip"] = ToString(v)
		}
	}
	e.FireEvent(event, ctx)
}

// HasEvent 是否有脚本注册了该事件
func (e *Engine) HasEvent(event string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, s := range e.scripts {
		if len(s.events[event]) > 0 {
			return true
		}
	}
	return false
}

// ScriptAliases 所有已注册 alias
func (e *Engine) ScriptAliases() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]string)
	for _, s := range e.scripts {
		for name := range s.aliases {
			out[name] = s.Name
		}
	}
	return out
}

// ExecAlias 执行 alias 命令（command 为完整命令字符串，首个词为别名，其余为参数）。
// 返回脚本 println/alert 输出。
func (e *Engine) ExecAlias(name string, args []string, uid string) (*ExecResult, bool) {
	e.mu.RLock()
	var script *Script
	var fn *SubRef
	for _, s := range e.scripts {
		if a, ok := s.aliases[name]; ok {
			script = s
			fn = a
			break
		}
	}
	e.mu.RUnlock()
	if script == nil {
		return nil, false
	}
	return e.execHandler(script, fn, args, uid), true
}

// MenuItemView 菜单项前端视图
type MenuItemView struct {
	Label     string         `json:"label"`
	Separator bool           `json:"separator"`
	SubItems  []*MenuItemView `json:"sub_items,omitempty"`
}

// MenuView 菜单视图
type MenuView struct {
	Script string          `json:"script"`
	Kind   string          `json:"kind"`
	Items  []*MenuItemView `json:"items"`
}

// Menus 所有已注册菜单
func (e *Engine) Menus() []MenuView {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var out []MenuView
	for _, s := range e.scripts {
		for kind, items := range s.menus {
			if len(items) == 0 {
				continue
			}
			out = append(out, MenuView{
				Script: s.Name,
				Kind:   kind,
				Items:  menuItemsView(items),
			})
		}
	}
	return out
}

func menuItemsView(items []*MenuItem) []*MenuItemView {
	var out []*MenuItemView
	for _, it := range items {
		v := &MenuItemView{Label: it.Label, Separator: it.Separator}
		if len(it.SubItems) > 0 {
			v.SubItems = menuItemsView(it.SubItems)
		}
		out = append(out, v)
	}
	return out
}

// ExecMenuItem 执行菜单项（path 为逐级菜单项路径，如 ["工具","Mimikatz"]）
func (e *Engine) ExecMenuItem(kind string, path []string, uid string) (*ExecResult, bool) {
	e.mu.RLock()
	var script *Script
	var sub *SubStmt
	for _, s := range e.scripts {
		items, ok := s.menus[kind]
		if !ok {
			continue
		}
		if f, ok2 := findMenuItem(items, path); ok2 {
			script = s
			sub = f
			break
		}
	}
	e.mu.RUnlock()
	if script == nil {
		return nil, false
	}
	fn := &SubRef{Name: "", Params: sub.Params, Body: sub.Body, Closure: script.ip.globals}
	return e.execHandler(script, fn, nil, uid), true
}

func findMenuItem(items []*MenuItem, path []string) (*SubStmt, bool) {
	if len(path) == 0 {
		return nil, false
	}
	for _, it := range items {
		if it.Separator {
			continue
		}
		if it.Label == path[0] {
			if len(path) == 1 && it.Fn != nil {
				return it.Fn, true
			}
			if len(path) > 1 {
				return findMenuItem(it.SubItems, path[1:])
			}
		}
	}
	return nil, false
}

// execHandler 执行 handler 并捕获输出（互斥保护临时 Log 捕获）
func (e *Engine) execHandler(script *Script, fn *SubRef, args []string, uid string) *ExecResult {
	res := &ExecResult{}
	script.execMu.Lock()
	defer script.execMu.Unlock()

	origLog := script.ip.Log
	var output []string
	script.ip.Log = func(format string, args ...interface{}) {
		output = append(output, fmt.Sprintf(format, args...))
		if EngineLog != nil {
			EngineLog("log", fmt.Sprintf("[%s] %s", script.Name, fmt.Sprintf(format, args...)))
		}
	}
	defer func() { script.ip.Log = origLog }()

	env := NewEnv(script.ip.globals)
	if fn.Closure != nil {
		env = NewEnv(fn.Closure)
		env.parent = fn.Closure
	}
	if uid != "" {
		env.SetLocal("_beacon_id", uid)
	}
	var vals []Value
	for _, a := range args {
		vals = append(vals, a)
	}
	_, err := script.ip.ExecSubIn(fn, vals, env)
	if err != nil {
		res.Error = err.Error()
		logScriptError(script.Name, "命令 "+fn.Name, err)
	}
	res.Output = output
	return res
}

// EvalScript 交互式求值脚本片段（不注册事件/alias，仅执行顶层语句与函数调用）。
// 用于 Aggressor 控制台的 Eval 输入。
func (e *Engine) EvalScript(content, uid string) *ExecResult {
	res := &ExecResult{}
	stmts, err := Parse(content)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	ip := newInterpIn(e.dir)
	var output []string
	ip.Log = func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		output = append(output, msg)
		if EngineLog != nil {
			EngineLog("log", fmt.Sprintf("[eval] %s", msg))
		}
	}
	env := NewEnv(ip.globals)
	if uid != "" {
		env.SetLocal("_beacon_id", uid)
	}
	var remain []Stmt
	for _, st := range stmts {
		if s, ok := st.(*SubStmt); ok {
			ip.defineSub(s, env)
			continue
		}
		remain = append(remain, st)
	}
	if _, err := ip.ExecBlock(&Block{Stmts: remain}, env); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Output = output
	return res
}

func logScriptError(script, where string, err error) {
	if EventLog != nil {
		EventLog(script, where, err, nil)
	}
	if EngineLog != nil {
		EngineLog("error", fmt.Sprintf("[%s] %s 执行出错: %v", script, where, err))
	}
}

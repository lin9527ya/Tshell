package cna

import (
	"fmt"
	"strconv"
	"strings"
)

// Value Sleep 运行时值
type Value interface{}

// 值类型:
//   nil        — 空值
//   string     — 字符串
//   float64    — 数字
//   bool       — 布尔
//   []Value    — 数组
//   map[string]Value — 字典
//   *SubRef    — 函数引用（用户 sub / 匿名块）

// SubRef 函数引用
type SubRef struct {
	Name   string
	Params []string
	Body   *Block
	Closure *Env
}

// IsTruthy Sleep 真值判断
func IsTruthy(v Value) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != ""
	case []Value:
		return len(t) > 0
	case map[string]Value:
		return len(t) > 0
	case *SubRef:
		return true
	}
	return true
}

// ToString 字符串化
func ToString(v Value) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case string:
		return t
	case []Value:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, ToString(e))
		}
		return strings.Join(parts, "")
	case map[string]Value:
		parts := make([]string, 0, len(t))
		for k, v := range t {
			parts = append(parts, k+" => "+ToString(v))
		}
		return strings.Join(parts, " ")
	case *SubRef:
		if t.Name != "" {
			return "<sub " + t.Name + ">"
		}
		return "<sub anonymous>"
	}
	return fmt.Sprintf("%v", v)
}

// ToInt 取整数
func ToInt(v Value) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err == nil {
			return n, true
		}
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// ToBool 布尔化
func ToBool(v Value) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case float64:
		return t != 0, true
	case string:
		return t != "", true
	}
	return false, false
}

// Env 变量作用域（链式查找）
type Env struct {
	vars  map[string]Value
	parent *Env
}

// NewEnv 创建环境
func NewEnv(parent *Env) *Env {
	return &Env{vars: make(map[string]Value), parent: parent}
}

// Get 查找变量（含父链）；ok=false 表示未定义
func (e *Env) Get(name string) (Value, bool) {
	for env := e; env != nil; env = env.parent {
		if v, ok := env.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// Set 设置变量（就近更新，不存在则在当前环境创建）
func (e *Env) Set(name string, v Value) {
	for env := e; env != nil; env = env.parent {
		if _, ok := env.vars[name]; ok {
			env.vars[name] = v
			return
		}
	}
	e.vars[name] = v
}

// SetLocal 强制在当前环境设置
func (e *Env) SetLocal(name string, v Value) {
	e.vars[name] = v
}

// runtimeError 运行时错误（带停止标志）
type runtimeError struct {
	msg string
}

func (r *runtimeError) Error() string { return r.msg }

func runErr(format string, args ...interface{}) *runtimeError {
	return &runtimeError{msg: fmt.Sprintf(format, args...)}
}

// controlFlow 控制流信号
type controlFlow int

// envReaches env 链是否可达 target（父链比较）
func envReaches(env, target *Env) bool {
	for e := env; e != nil; e = e.parent {
		if e == target {
			return true
		}
	}
	return false
}

const (
	flowNone controlFlow = iota
	flowReturn
	flowBreak
	flowContinue
)

// Interp 解释器状态
type Interp struct {
	// Globals 全局变量
	globals *Env
	// subs 用户函数表
	subs map[string]*SubRef
	// flow 当前控制流
	flow controlFlow
	// ret 返回值
	ret Value
	// maxSteps 防死循环步数上限（单次顶层执行）
	steps int
	// Log 引擎日志回调（加载/事件执行信息）
	Log func(format string, args ...interface{})
}

// NewInterp 创建解释器
func NewInterp() *Interp {
	return &Interp{
		globals: NewEnv(nil),
		subs:    make(map[string]*SubRef),
		Log:     func(string, ...interface{}) {},
	}
}

// ExecBlock 执行语句块
func (ip *Interp) ExecBlock(blk *Block, env *Env) (Value, error) {
	if blk == nil {
		return nil, nil
	}
	for _, st := range blk.Stmts {
		ip.steps++
		if ip.steps > 5_000_000 {
			return nil, runErr("执行步数超限（疑似死循环），已终止")
		}
		if err := ip.execStmt(st, env); err != nil {
			return nil, err
		}
		if ip.flow != flowNone {
			break
		}
	}
	return ip.ret, nil
}

// CallSub 调用函数（带参数绑定 $1..$9 与形参）
func (ip *Interp) CallSub(fn *SubRef, args []Value, caller *Env) (Value, error) {
	// 环境链：函数局部环境 -> 调用点环境（事件上下文/全局） -> 闭包（函数定义处环境）
	env := NewEnv(caller)
	env.parent = caller
	if fn.Closure != nil && caller != nil {
		// 保证全局变量可解析：若调用点链未覆盖 closure，则挂接
		if !envReaches(env, fn.Closure) {
			env.parent = fn.Closure
		}
	}
	if fn.Closure == nil {
		env = NewEnv(caller)
		env.parent = caller
	}
	// 参数绑定
	for i, a := range args {
		if i < 9 {
			env.SetLocal(fmt.Sprintf("%d", i+1), a)
		}
	}
	if len(fn.Params) > 0 {
		for i, p := range fn.Params {
			if i < len(args) {
				env.SetLocal(p, args[i])
			} else {
				env.SetLocal(p, nil)
			}
		}
	}
	prevFlow := ip.flow
	ip.flow = flowNone
	prevRet := ip.ret
	ip.ret = nil
	_, err := ip.ExecBlock(fn.Body, env)
	ret := ip.ret
	ip.ret = prevRet
	ip.flow = prevFlow
	return ret, err
}

// ExecSub 顶层执行 sub（供引擎事件触发）
func (ip *Interp) ExecSub(fn *SubRef, args []Value) (Value, error) {
	ip.steps = 0
	return ip.CallSub(fn, args, ip.globals)
}

// ExecSubIn 在指定调用环境（含事件上下文变量）中执行 sub
func (ip *Interp) ExecSubIn(fn *SubRef, args []Value, caller *Env) (Value, error) {
	ip.steps = 0
	return ip.CallSub(fn, args, caller)
}

func (ip *Interp) execStmt(st Stmt, env *Env) error {
	switch s := st.(type) {
	case *ExprStmt:
		_, err := ip.eval(s.Expr, env)
		return err
	case *Block:
		_, err := ip.ExecBlock(s, env)
		return err
	case *IfStmt:
		c, err := ip.eval(s.Cond, env)
		if err != nil {
			return err
		}
		if IsTruthy(c) {
			_, err := ip.ExecBlock(s.Then, env)
			return err
		}
		if s.Else != nil {
			_, err := ip.ExecBlock(s.Else, env)
			return err
		}
		return nil
	case *ForeachStmt:
		iter, err := ip.eval(s.Iter, env)
		if err != nil {
			return err
		}
		switch it := iter.(type) {
		case []Value:
			for _, item := range it {
				env.Set(s.Var, item)
				_, err := ip.ExecBlock(s.Body, env)
				if err != nil {
					return err
				}
				if ip.flow == flowBreak {
					ip.flow = flowNone
					break
				}
				if ip.flow == flowContinue {
					ip.flow = flowNone
					continue
				}
				if ip.flow == flowReturn {
					return nil
				}
			}
		case map[string]Value:
			for k := range it {
				env.Set(s.Var, k)
				_, err := ip.ExecBlock(s.Body, env)
				if err != nil {
					return err
				}
				if ip.flow == flowBreak {
					ip.flow = flowNone
					break
				}
				if ip.flow == flowContinue {
					ip.flow = flowNone
					continue
				}
				if ip.flow == flowReturn {
					return nil
				}
			}
		default:
			if iter == nil {
				return nil
			}
			// 字符串迭代：按字符
			if str, ok := iter.(string); ok {
				for _, r := range str {
					env.Set(s.Var, string(r))
					_, err := ip.ExecBlock(s.Body, env)
					if err != nil {
						return err
					}
					if ip.flow == flowBreak {
						ip.flow = flowNone
						break
					}
					if ip.flow == flowContinue {
						ip.flow = flowNone
						continue
					}
					if ip.flow == flowReturn {
						return nil
					}
				}
			}
		}
		return nil
	case *WhileStmt:
		for {
			c, err := ip.eval(s.Cond, env)
			if err != nil {
				return err
			}
			if !IsTruthy(c) {
				break
			}
			_, err = ip.ExecBlock(s.Body, env)
			if err != nil {
				return err
			}
			if ip.flow == flowBreak {
				ip.flow = flowNone
				break
			}
			if ip.flow == flowContinue {
				ip.flow = flowNone
				continue
			}
			if ip.flow == flowReturn {
				return nil
			}
		}
		return nil
	case *ForStmt:
		if s.Init != nil {
			if err := ip.execStmt(s.Init, env); err != nil {
				return err
			}
		}
		for {
			if s.Cond != nil {
				c, err := ip.eval(s.Cond, env)
				if err != nil {
					return err
				}
				if !IsTruthy(c) {
					break
				}
			}
			_, err := ip.ExecBlock(s.Body, env)
			if err != nil {
				return err
			}
			if ip.flow == flowBreak {
				ip.flow = flowNone
				break
			}
			if ip.flow == flowContinue {
				ip.flow = flowNone
			}
			if ip.flow == flowReturn {
				return nil
			}
			if s.Post != nil {
				if err := ip.execStmt(s.Post, env); err != nil {
					return err
				}
			}
		}
		return nil
	case *ReturnStmt:
		var v Value
		var err error
		if s.Expr != nil {
			v, err = ip.eval(s.Expr, env)
			if err != nil {
				return err
			}
		}
		ip.ret = v
		ip.flow = flowReturn
		return nil
	case *BreakStmt:
		ip.flow = flowBreak
		return nil
	case *ContinueStmt:
		ip.flow = flowContinue
		return nil
	case *LocalStmt:
		// local：新作用域
		for _, sub := range s.Stmts {
			if as, ok := sub.(*Assign); ok {
				v, err := ip.eval(as.Value, env)
				if err != nil {
					return err
				}
				env.SetLocal(as.Name, v)
			}
		}
		return nil
	case *GlobalStmt:
		for _, sub := range s.Stmts {
			if as, ok := sub.(*Assign); ok {
				v, err := ip.eval(as.Value, env)
				if err != nil {
					return err
				}
				ip.globals.SetLocal(as.Name, v)
			}
		}
		return nil
	case *Assign:
		v, err := ip.eval(s.Value, env)
		if err != nil {
			return err
		}
		if s.Op == "+=" {
			old, _ := env.Get(s.Name)
			v = concatValues(old, v)
		}
		if s.Prefix == '%' {
			if m, ok := v.(map[string]Value); ok {
				env.Set(s.Name, m)
			} else {
				env.Set(s.Name, v)
			}
		} else {
			env.Set(s.Name, v)
		}
		return nil
	case *SubStmt:
		// 嵌套 sub 定义（顶层已由引擎注册）
		ip.defineSub(s, env)
		return nil
	case *OnStmt, *AliasStmt, *MenuStmt, *NopStmt:
		return nil // 顶层声明由引擎处理；块内忽略
	default:
		return nil
	}
}

// defineSub 注册函数
func (ip *Interp) defineSub(s *SubStmt, env *Env) {
	ip.subs[s.Name] = &SubRef{Name: s.Name, Params: s.Params, Body: s.Body, Closure: env}
}

func concatValues(a, b Value) Value {
	switch t := a.(type) {
	case float64:
		if n, ok := ToInt(b); ok {
			return t + float64(n)
		}
	case string:
		return t + ToString(b)
	case []Value:
		return append(t, b)
	case map[string]Value:
		if m, ok := b.(map[string]Value); ok {
			for k, v := range m {
				t[k] = v
			}
			return t
		}
	}
	return ToString(a) + ToString(b)
}

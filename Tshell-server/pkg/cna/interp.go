package cna

import (
	"math"
	"strconv"
	"strings"
)

// eval 表达式求值
func (ip *Interp) eval(e Expr, env *Env) (Value, error) {
	switch ex := e.(type) {
	case *NumLit:
		return ex.Val, nil
	case *StrLit:
		return ex.Val, nil
	case *VarRef:
		return ip.evalVar(ex, env)
	case *ArrayLit:
		var arr []Value
		for _, el := range ex.Elems {
			v, err := ip.eval(el, env)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	case *DictLit:
		dict := make(map[string]Value)
		for i := range ex.Keys {
			k, err := ip.eval(ex.Keys[i], env)
			if err != nil {
				return nil, err
			}
			v, err := ip.eval(ex.Vals[i], env)
			if err != nil {
				return nil, err
			}
			dict[ToString(k)] = v
		}
		return dict, nil
	case *Index:
		obj, err := ip.eval(ex.Obj, env)
		if err != nil {
			return nil, err
		}
		idx, err := ip.eval(ex.Idx, env)
		if err != nil {
			return nil, err
		}
		return indexValue(obj, idx)
	case *Call:
		return ip.evalCall(ex, env)
	case *Ref:
		// 引用求值：返回 SubRef
		if ex.Blk != nil {
			return &SubRef{Name: "", Params: nil, Body: ex.Blk, Closure: env}, nil
		}
		if fn, ok := ip.subs[ex.Name]; ok {
			return fn, nil
		}
		// 也可能是内建函数引用
		if bf, ok := builtinRefs[ex.Name]; ok {
			_ = bf
			return &SubRef{Name: ex.Name, Params: nil, Body: nil, Closure: env}, nil
		}
		return nil, runErr("函数 %s 未定义", ex.Name)
	case *BinOp:
		return ip.evalBinOp(ex, env)
	case *Unary:
		return ip.evalUnary(ex, env)
	case *Bracketed:
		return ip.eval(ex.Expr, env)
	case *Assign:
		v, err := ip.eval(ex.Value, env)
		if err != nil {
			return nil, err
		}
		if ex.Op == "+=" {
			old, _ := env.Get(ex.Name)
			v = concatValues(old, v)
		}
		env.Set(ex.Name, v)
		return v, nil
	}
	return nil, runErr("不支持的表达式")
}

// evalVar 变量求值（含特殊变量）
func (ip *Interp) evalVar(v *VarRef, env *Env) (Value, error) {
	// 特殊变量
	switch v.Name {
	case "0":
		if sv, ok := env.Get("script_path"); ok {
			return sv, nil
		}
		return nil, nil
	case "null":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	val, ok := env.Get(v.Name)
	if !ok {
		return nil, nil // Sleep 未定义变量为 null
	}
	return val, nil
}

// indexValue 索引
func indexValue(obj, idx Value) (Value, error) {
	switch o := obj.(type) {
	case []Value:
		n, ok := ToInt(idx)
		if !ok {
			return nil, runErr("数组索引必须为整数")
		}
		i := int(n)
		if i < 0 {
			i += len(o)
		}
		if i < 0 || i >= len(o) {
			return nil, nil
		}
		return o[i], nil
	case map[string]Value:
		if v, ok := o[ToString(idx)]; ok {
			return v, nil
		}
		return nil, nil
	case string:
		n, ok := ToInt(idx)
		if ok {
			i := int(n)
			if i < 0 {
				i += len(o)
			}
			if i >= 0 && i < len(o) {
				return string(o[i]), nil
			}
			return nil, nil
		}
	}
	return nil, nil
}

// evalCall 函数/方法调用
func (ip *Interp) evalCall(c *Call, env *Env) (Value, error) {
	// 求值参数
	args := make([]Value, 0, len(c.Args))
	for _, a := range c.Args {
		v, err := ip.eval(a, env)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}

	// 函数名
	switch fn := c.Fn.(type) {
	case *VarRef:
		if fn.Prefix == 0 {
			// 标识符：优先用户 sub，其次内建
			if sub, ok := ip.subs[fn.Name]; ok {
				return ip.CallSub(sub, args, env)
			}
			if bf, ok := builtinFuncs[fn.Name]; ok {
				return bf(ip, env, args)
			}
			return nil, runErr("未知函数: %s", fn.Name)
		}
		// 变量作为函数调用（引用）
		val, _ := env.Get(fn.Name)
		if sub, ok := val.(*SubRef); ok {
			return ip.CallSub(sub, args, env)
		}
		return nil, runErr("%s 不是可调用函数", fn.Name)
	case *Index:
		// 方法调用：obj.method(args) —— 对象索引后调用? 这里处理 obj.method 形式
		// 实际 AST 中 obj.method 会被解析为 Index(VarRef(obj), StrLit("method"))
		obj, err := ip.eval(fn.Obj, env)
		if err != nil {
			return nil, err
		}
		idx, err := ip.eval(fn.Idx, env)
		if err != nil {
			return nil, err
		}
		return callMethod(obj, ToString(idx), args)
	case *Bracketed:
		// [expr](args)? 罕见
		v, err := ip.eval(fn.Expr, env)
		if err != nil {
			return nil, err
		}
		if sub, ok := v.(*SubRef); ok {
			return ip.CallSub(sub, args, env)
		}
		return nil, runErr("表达式结果不可调用")
	default:
		// 对任意值求值后尝试调用
		v, err := ip.eval(fn, env)
		if err != nil {
			return nil, err
		}
		if sub, ok := v.(*SubRef); ok {
			return ip.CallSub(sub, args, env)
		}
		return nil, runErr("不可调用的表达式")
	}
}

// callMethod 方法调用（sleep 风格）
func callMethod(obj Value, name string, args []Value) (Value, error) {
	switch o := obj.(type) {
	case string:
		return callStringMethod(o, name, args)
	case []Value:
		return callArrayMethod(o, name, args)
	case map[string]Value:
		return callDictMethod(o, name, args)
	}
	return nil, runErr("类型不支持方法: %s", name)
}

func callStringMethod(s, name string, args []Value) (Value, error) {
	switch name {
	case "length", "size":
		return float64(len(s)), nil
	case "to_upper", "to_upper_case", "upper":
		return strings.ToUpper(s), nil
	case "to_lower", "to_lower_case", "lower":
		return strings.ToLower(s), nil
	case "trim":
		return strings.TrimSpace(s), nil
	case "split":
		sep := ","
		if len(args) > 0 {
			sep = ToString(args[0])
		}
		var arr []Value
		for _, part := range strings.Split(s, sep) {
			arr = append(arr, part)
		}
		return arr, nil
	case "replace":
		if len(args) >= 2 {
			return strings.ReplaceAll(s, ToString(args[0]), ToString(args[1])), nil
		}
	case "indexof", "index":
		if len(args) > 0 {
			return float64(strings.Index(s, ToString(args[0]))), nil
		}
	case "substr", "substring":
		if len(args) > 0 {
			start, _ := ToInt(args[0])
			end := int64(len(s))
			if len(args) > 1 {
				if e, ok := ToInt(args[1]); ok {
					end = e
				}
			}
			if start < 0 || start > int64(len(s)) {
				return "", nil
			}
			if end < start {
				end = start
			}
			if end > int64(len(s)) {
				end = int64(len(s))
			}
			return s[start:end], nil
		}
	case "startswith", "starts_with":
		if len(args) > 0 {
			return strings.HasPrefix(s, ToString(args[0])), nil
		}
	case "endswith", "ends_with":
		if len(args) > 0 {
			return strings.HasSuffix(s, ToString(args[0])), nil
		}
	case "contains":
		if len(args) > 0 {
			return strings.Contains(s, ToString(args[0])), nil
		}
	case "charat", "char_at":
		if len(args) > 0 {
			if n, ok := ToInt(args[0]); ok {
				i := int(n)
				if i >= 0 && i < len(s) {
					return string(s[i]), nil
				}
			}
		}
	case "join":
		return s, nil
	}
	return nil, runErr("字符串无方法: %s", name)
}

func callArrayMethod(arr []Value, name string, args []Value) (Value, error) {
	switch name {
	case "length", "size":
		return float64(len(arr)), nil
	case "join":
		sep := " "
		if len(args) > 0 {
			sep = ToString(args[0])
		}
		parts := make([]string, 0, len(arr))
		for _, e := range arr {
			parts = append(parts, ToString(e))
		}
		return strings.Join(parts, sep), nil
	case "push", "add":
		if len(args) > 0 {
			return append(arr, args...), nil
		}
	case "pop":
		if len(arr) > 0 {
			return arr[:len(arr)-1], nil
		}
	case "reverse":
		out := make([]Value, len(arr))
		for i, e := range arr {
			out[len(arr)-1-i] = e
		}
		return out, nil
	case "contains":
		if len(args) > 0 {
			t := ToString(args[0])
			for _, e := range arr {
				if ToString(e) == t {
					return true, nil
				}
			}
			return false, nil
		}
	}
	return nil, runErr("数组无方法: %s", name)
}

func callDictMethod(d map[string]Value, name string, args []Value) (Value, error) {
	switch name {
	case "length", "size":
		return float64(len(d)), nil
	case "keys":
		var arr []Value
		for k := range d {
			arr = append(arr, k)
		}
		return arr, nil
	case "contains", "has":
		if len(args) > 0 {
			_, ok := d[ToString(args[0])]
			return ok, nil
		}
	}
	return nil, runErr("字典无方法: %s", name)
}

// evalBinOp 二元运算
func (ip *Interp) evalBinOp(b *BinOp, env *Env) (Value, error) {
	// 短路
	if b.Op == "&&" {
		l, err := ip.eval(b.Left, env)
		if err != nil {
			return nil, err
		}
		if !IsTruthy(l) {
			return false, nil
		}
		r, err := ip.eval(b.Right, env)
		if err != nil {
			return nil, err
		}
		return IsTruthy(r), nil
	}
	if b.Op == "||" {
		l, err := ip.eval(b.Left, env)
		if err != nil {
			return nil, err
		}
		if IsTruthy(l) {
			return true, nil
		}
		r, err := ip.eval(b.Right, env)
		if err != nil {
			return nil, err
		}
		return IsTruthy(r), nil
	}

	l, err := ip.eval(b.Left, env)
	if err != nil {
		return nil, err
	}
	r, err := ip.eval(b.Right, env)
	if err != nil {
		return nil, err
	}
	return binValues(b.Op, l, r)
}

// binValues 值运算
func binValues(op string, l, r Value) (Value, error) {
	switch op {
	case "+":
		if ln, lok := toNum(l); lok {
			if rn, rok := toNum(r); rok {
				return ln + rn, nil
			}
		}
		if ls, ok := l.(string); ok {
			if rs, ok2 := r.(string); ok2 {
				return ls + rs, nil
			}
		}
		if la, ok := l.([]Value); ok {
			if ra, ok2 := r.([]Value); ok2 {
				return append(append([]Value{}, la...), ra...), nil
			}
		}
		return ToString(l) + ToString(r), nil
	case "-", "*", "/", "%":
		ln, lok := toNum(l)
		rn, rok := toNum(r)
		if !lok || !rok {
			return nil, runErr("算术运算需要数字")
		}
		switch op {
		case "-":
			return ln - rn, nil
		case "*":
			return ln * rn, nil
		case "/":
			if rn == 0 {
				return nil, runErr("除零错误")
			}
			return ln / rn, nil
		case "%":
			if rn == 0 {
				return nil, runErr("模零错误")
			}
			return math.Mod(ln, rn), nil
		}
	case ".":
		return ToString(l) + ToString(r), nil
	case "==":
		return valuesEqual(l, r), nil
	case "!=":
		return !valuesEqual(l, r), nil
	case "<", ">", "<=", ">=":
		return compareValues(op, l, r)
	case "=~":
		// 正则匹配（子集：简单子串/前缀匹配）
		pat := ToString(r)
		s := ToString(l)
		if strings.HasPrefix(pat, "^") && strings.HasSuffix(pat, "$") {
			return s == strings.TrimSuffix(strings.TrimPrefix(pat, "^"), "$"), nil
		}
		return strings.Contains(s, pat), nil
	case "->":
		return indexValue(l, r)
	}
	return nil, runErr("不支持的运算符: %s", op)
}

func toNum(v Value) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func valuesEqual(a, b Value) bool {
	if an, ok := toNum(a); ok {
		if bn, ok2 := toNum(b); ok2 {
			return an == bn
		}
	}
	as := ToString(a)
	bs := ToString(b)
	return as == bs
}

func compareValues(op string, l, r Value) (Value, error) {
	if ln, lok := toNum(l); lok {
		if rn, rok := toNum(r); rok {
			switch op {
			case "<":
				return ln < rn, nil
			case ">":
				return ln > rn, nil
			case "<=":
				return ln <= rn, nil
			case ">=":
				return ln >= rn, nil
			}
		}
	}
	ls := ToString(l)
	rs := ToString(r)
	switch op {
	case "<":
		return ls < rs, nil
	case ">":
		return ls > rs, nil
	case "<=":
		return ls <= rs, nil
	case ">=":
		return ls >= rs, nil
	}
	return nil, runErr("比较失败")
}

func (ip *Interp) evalUnary(u *Unary, env *Env) (Value, error) {
	v, err := ip.eval(u.Expr, env)
	if err != nil {
		return nil, err
	}
	switch u.Op {
	case "!":
		return !IsTruthy(v), nil
	case "-":
		if n, ok := toNum(v); ok {
			return -n, nil
		}
		return nil, runErr("一元负号需要数字")
	}
	return nil, runErr("不支持的一元运算: %s", u.Op)
}

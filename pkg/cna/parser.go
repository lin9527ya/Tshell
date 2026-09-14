package cna

import (
	"fmt"
	"strings"
)

// Parser 递归下降解析器
type Parser struct {
	lex   *Lexer
	tok   Token
	peekT Token // 提前看一个（用于判断 token 类型）
	buf   []Token // 回退缓冲（保存已预取但未消费的 token）
}

// Parse 解析脚本为顶层语句列表
func Parse(src string) ([]Stmt, error) {
	p := &Parser{lex: NewLexer(src)}
	p.next()
	p.next() // 预读两个
	var stmts []Stmt
	for p.tok.Type != TEOF {
		// 跳过空行
		if p.tok.Type == TNewline {
			p.next()
			continue
		}
		s, err := p.parseStmt()
		if err != nil {
			return stmts, err
		}
		if s != nil {
			stmts = append(stmts, s)
		}
		p.skipNewlines()
	}
	return stmts, nil
}

// next 前进一个 token（优先消费缓冲）
func (p *Parser) next() {
	if len(p.buf) > 0 {
		p.tok = p.peekT
		p.peekT = p.buf[0]
		p.buf = p.buf[1:]
		return
	}
	p.tok = p.peekT
	p.peekT = p.lex.Next()
}

// rollback 撤销最近一次 next()：
// 调用方须先保存 next() 前的 (tok, peekT)，再调用 next()，不满足条件时调用 rollback。
func (p *Parser) rollback(save, savePeek Token) {
	// p.next() 后的 peekT 是 lexer 刚预取的新 token，放回缓冲；
	// 恢复 tok/peekT 到 next() 之前
	p.buf = append([]Token{p.peekT}, p.buf...)
	p.tok = save
	p.peekT = savePeek
}

// skipNewlines 跳过换行 token
func (p *Parser) skipNewlines() {
	for p.tok.Type == TNewline {
		p.next()
	}
}

// errf 解析错误
func (p *Parser) errf(format string, args ...interface{}) error {
	return fmt.Errorf("第 %d 行: %s", p.tok.Line, fmt.Sprintf(format, args...))
}

// parseStmt 解析一条语句（调用前已跳过换行）
func (p *Parser) parseStmt() (Stmt, error) {
	t := p.tok
	switch {
	case t.Type == TIdent:
		switch t.Text {
		case "sub":
			return p.parseSub()
		case "on":
			return p.parseOn()
		case "alias":
			return p.parseAlias()
		case "menu", "popup":
			return p.parseMenu()
		case "if":
			return p.parseIf()
		case "foreach":
			return p.parseForeach()
		case "while":
			return p.parseWhile()
		case "for":
			return p.parseFor()
		case "return":
			p.next()
			var e Expr
			var err error
			if !p.isStmtEnd() {
				e, err = p.parseExpr()
				if err != nil {
					return nil, err
				}
			}
			p.consumeStmtEnd()
			return &ReturnStmt{Expr: e}, nil
		case "break":
			p.next()
			p.consumeStmtEnd()
			return &BreakStmt{}, nil
		case "continue":
			p.next()
			p.consumeStmtEnd()
			return &ContinueStmt{}, nil
		case "local", "global":
			return p.parseLocalGlobal()
		}
		// 其他标识符：表达式语句
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.consumeStmtEnd()
		return &ExprStmt{Expr: e}, nil
	case t.Type == TVar, t.Type == TNumber, t.Type == TString, t.Type == TOp:
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.consumeStmtEnd()
		return &ExprStmt{Expr: e}, nil
	case t.Type == TEOF:
		return nil, nil
	default:
		p.next()
		return &NopStmt{}, nil
	}
}

func (p *Parser) isStmtEnd() bool {
	return p.tok.Type == TNewline || p.tok.Type == TEOF || (p.tok.Type == TOp && p.tok.Text == ";") || (p.tok.Type == TOp && p.tok.Text == "}")
}

// consumeStmtEnd 消费语句结束符（; 或 换行；`}` 不消费）
func (p *Parser) consumeStmtEnd() {
	if p.tok.Type == TNewline {
		p.next()
		return
	}
	if p.tok.Type == TOp && p.tok.Text == ";" {
		p.next()
		return
	}
}

// parseSub sub name ( params ) { body }
func (p *Parser) parseSub() (Stmt, error) {
	line := p.tok.Line
	p.next() // sub
	if p.tok.Type != TIdent {
		return nil, p.errf("sub 后需要函数名")
	}
	name := p.tok.Text
	p.next()
	var params []string
	if p.tok.Type == TOp && p.tok.Text == "(" {
		p.next()
		for {
			if p.tok.Type == TOp && p.tok.Text == ")" {
				p.next()
				break
			}
			if p.tok.Type != TVar {
				// 形参必须是变量；容忍跳过
				p.next()
				continue
			}
			params = append(params, strings.TrimLeft(p.tok.Text, "$@%^"))
			p.next()
			if p.tok.Type == TOp && p.tok.Text == "," {
				p.next()
				continue
			}
		}
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &SubStmt{Name: name, Params: params, Body: body, Line: line}, nil
}

// parseOn on event { body } | on event (a, b) { body }
func (p *Parser) parseOn() (Stmt, error) {
	line := p.tok.Line
	p.next() // on
	var event string
	if p.tok.Type == TIdent {
		event = p.tok.Text
		p.next()
	} else if p.tok.Type == TVar {
		event = p.tok.Text
		p.next()
	} else if p.tok.Type == TString {
		event = p.tok.Raw
		p.next()
	} else {
		return nil, p.errf("on 后需要事件名")
	}
	// 事件名允许带路径 a::b 已由标识符覆盖；允许带参数列表
	if p.tok.Type == TOp && p.tok.Text == "(" {
		p.next()
		for p.tok.Type != TEOF {
			if p.tok.Type == TOp && p.tok.Text == ")" {
				p.next()
				break
			}
			p.next()
		}
	}
	fn, err := p.parseSubBody(line)
	if err != nil {
		return nil, err
	}
	return &OnStmt{Event: event, Fn: fn, Line: line}, nil
}

// parseAlias alias name { body } | alias name (args) { body }
func (p *Parser) parseAlias() (Stmt, error) {
	line := p.tok.Line
	p.next() // alias
	name := ""
	if p.tok.Type == TIdent {
		name = p.tok.Text
		p.next()
	} else if p.tok.Type == TString {
		name = p.tok.Raw
		p.next()
	} else {
		return nil, p.errf("alias 后需要命令名")
	}
	fn, err := p.parseSubBody(line)
	if err != nil {
		return nil, err
	}
	return &AliasStmt{Name: name, Fn: fn, Line: line}, nil
}

// parseSubBody 解析 (params) { body } 或 { body }
func (p *Parser) parseSubBody(line int) (*SubStmt, error) {
	var params []string
	if p.tok.Type == TOp && p.tok.Text == "(" {
		p.next()
		for {
			if p.tok.Type == TOp && p.tok.Text == ")" {
				p.next()
				break
			}
			if p.tok.Type == TVar {
				params = append(params, strings.TrimLeft(p.tok.Text, "$@%^"))
			}
			p.next()
			if p.tok.Type == TOp && p.tok.Text == "," {
				p.next()
			}
		}
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &SubStmt{Name: "", Params: params, Body: body, Line: line}, nil
}

// parseMenu menu kind { ... } | popup kind { ... }
func (p *Parser) parseMenu() (Stmt, error) {
	line := p.tok.Line
	p.next() // menu / popup
	kind := ""
	if p.tok.Type == TIdent || p.tok.Type == TVar {
		kind = p.tok.Text
		p.next()
	} else if p.tok.Type == TString {
		kind = unescape(p.tok.Raw, p.tok.Quote)
		p.next()
	}
	items, err := p.parseMenuItems()
	if err != nil {
		return nil, err
	}
	return &MenuStmt{Kind: kind, Items: items, Line: line}, nil
}

// parseMenuItems 解析 menu { ... } 体内的 item/separator/嵌套 menu
func (p *Parser) parseMenuItems() ([]*MenuItem, error) {
	if p.tok.Type != TOp || p.tok.Text != "{" {
		return nil, p.errf("menu 需要 { 开始")
	}
	p.next()
	p.skipNewlines()
	var items []*MenuItem
	for {
		if p.tok.Type == TEOF {
			return nil, p.errf("menu 语句块未闭合")
		}
		if p.tok.Type == TOp && p.tok.Text == "}" {
			p.next()
			break
		}
		if p.tok.Type == TNewline {
			p.next()
			continue
		}
		if p.tok.Type == TOp && p.tok.Text == ";" {
			p.next()
			continue
		}
		if p.tok.Type == TIdent {
			switch p.tok.Text {
			case "item":
				it, err := p.parseMenuItem()
				if err != nil {
					return nil, err
				}
				items = append(items, it)
				continue
			case "separator":
				p.next()
				items = append(items, &MenuItem{Separator: true, Line: p.tok.Line})
				p.consumeStmtEnd()
				continue
			case "menu":
				// 嵌套子菜单：menu "subname" { ... }
				sub, err := p.parseMenu()
				if err != nil {
					return nil, err
				}
				ms := sub.(*MenuStmt)
				items = append(items, &MenuItem{Label: ms.Kind, SubItems: ms.Items, Line: ms.Line})
				continue
			}
		}
		// 其他内容：跳过语句
		if _, err := p.parseStmt(); err != nil {
			return nil, err
		}
		p.skipNewlines()
	}
	return items, nil
}

// parseMenuItem item "Name" (args) { body }
func (p *Parser) parseMenuItem() (*MenuItem, error) {
	line := p.tok.Line
	p.next() // item
	var label string
	if p.tok.Type == TString {
		label = unescape(p.tok.Raw, p.tok.Quote)
		p.next()
	} else if p.tok.Type == TIdent {
		label = p.tok.Text
		p.next()
	} else {
		return nil, p.errf("item 后需要菜单名称")
	}
	fn, err := p.parseSubBody(line)
	if err != nil {
		return nil, err
	}
	return &MenuItem{Label: label, Fn: fn, Line: line}, nil
}

// parseBlock 解析 { ... }
func (p *Parser) parseBlock() (*Block, error) {
	if p.tok.Type != TOp || p.tok.Text != "{" {
		return nil, p.errf("需要 { 开始语句块")
	}
	p.next()
	p.skipNewlines()
	blk := &Block{}
	for {
		if p.tok.Type == TEOF {
			return nil, p.errf("语句块未闭合（缺少 }）")
		}
		if p.tok.Type == TOp && p.tok.Text == "}" {
			p.next()
			break
		}
		if p.tok.Type == TNewline {
			p.next()
			continue
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if s != nil {
			blk.Stmts = append(blk.Stmts, s)
		}
		p.skipNewlines()
	}
	return blk, nil
}

// parseIf if (cond) { } else { }
func (p *Parser) parseIf() (Stmt, error) {
	p.next() // if
	cond, err := p.parseParenExpr()
	if err != nil {
		return nil, err
	}
	p.skipNewlines()
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	var els *Block
	p.skipNewlines()
	if p.tok.Type == TIdent && p.tok.Text == "else" {
		p.next()
		p.skipNewlines()
		if p.tok.Type == TIdent && p.tok.Text == "if" {
			// else if：递归解析为嵌套 if
			inner, err := p.parseIf()
			if err != nil {
				return nil, err
			}
			els = &Block{Stmts: []Stmt{inner}}
		} else {
			p.skipNewlines()
			els, err = p.parseBlock()
			if err != nil {
				return nil, err
			}
		}
	}
	return &IfStmt{Cond: cond, Then: then, Else: els}, nil
}

// parseForeach foreach $x (@arr) {} / foreach $x @arr {}
func (p *Parser) parseForeach() (Stmt, error) {
	p.next() // foreach
	if p.tok.Type != TVar {
		return nil, p.errf("foreach 后需要循环变量")
	}
	v := p.tok
	p.next()
	// ( 可选
	if p.tok.Type == TOp && p.tok.Text == "(" {
		p.next()
	}
	iter, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.tok.Type == TOp && p.tok.Text == ")" {
		p.next()
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ForeachStmt{Prefix: v.Text[0], Var: strings.TrimLeft(v.Text, "$@%^"), Iter: iter, Body: body}, nil
}

// parseWhile while (cond) {}
func (p *Parser) parseWhile() (Stmt, error) {
	p.next()
	cond, err := p.parseParenExpr()
	if err != nil {
		return nil, err
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &WhileStmt{Cond: cond, Body: body}, nil
}

// parseFor for (init; cond; post) {}
func (p *Parser) parseFor() (Stmt, error) {
	p.next() // for
	if p.tok.Type == TOp && p.tok.Text == "(" {
		p.next()
	}
	var init, post Stmt
	var cond Expr
	p.skipNewlines()
	if !(p.tok.Type == TOp && p.tok.Text == ";") {
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		init = s
	} else {
		p.next()
	}
	p.skipNewlines()
	if !(p.tok.Type == TOp && p.tok.Text == ";") {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		cond = e
	}
	if p.tok.Type == TOp && p.tok.Text == ";" {
		p.next()
	}
	p.skipNewlines()
	if !(p.tok.Type == TOp && p.tok.Text == ")") {
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		post = s
	}
	if p.tok.Type == TOp && p.tok.Text == ")" {
		p.next()
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ForStmt{Init: init, Cond: cond, Post: post, Body: body}, nil
}

// parseLocalGlobal local/global $x = ...
func (p *Parser) parseLocalGlobal() (Stmt, error) {
	p.next() // local / global
	var stmts []Stmt
	for {
		p.skipNewlines()
		if p.tok.Type != TVar {
			break
		}
		v := p.tok
		p.next()
		op := "="
		if p.tok.Type == TOp && (p.tok.Text == "=" || p.tok.Text == "+=") {
			op = p.tok.Text
			p.next()
		}
		var val Expr
		var err error
		if !p.isStmtEnd() {
			val, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
		} else {
			val = &StrLit{Val: ""}
		}
		stmts = append(stmts, &Assign{Prefix: v.Text[0], Name: strings.TrimLeft(v.Text, "$@%^"), Op: op, Value: val})
		p.consumeStmtEnd()
		if p.tok.Type == TOp && p.tok.Text == "," {
			p.next()
		} else {
			break
		}
	}
	// 包装：local 语义由解释器特殊处理（新作用域变量）
	if len(stmts) == 0 {
		return &NopStmt{}, nil
	}
	return &LocalStmt{Stmts: stmts}, nil
}

// parseParenExpr ( expr )
func (p *Parser) parseParenExpr() (Expr, error) {
	if p.tok.Type == TOp && p.tok.Text == "(" {
		p.next()
	}
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.tok.Type == TOp && p.tok.Text == ")" {
		p.next()
	}
	return e, nil
}

// ===== 表达式解析（优先级从低到高）=====

func (p *Parser) parseExpr() (Expr, error) {
	return p.parseAssign()
}

// parseAssign 赋值（右结合）
func (p *Parser) parseAssign() (Expr, error) {
	// 赋值左侧必须是变量
	if p.tok.Type == TVar {
		// 提前看下一个 token 是否为 = / +=
		save := p.tok
		savePeek := p.peekT
		p.next()
		if p.tok.Type == TOp && (p.tok.Text == "=" || p.tok.Text == "+=") {
			op := p.tok.Text
			p.next()
			val, err := p.parseAssign()
			if err != nil {
				return nil, err
			}
			return &Assign{Prefix: save.Text[0], Name: strings.TrimLeft(save.Text, "$@%^"), Op: op, Value: val}, nil
		}
		// 不是赋值：回退并继续
		p.rollback(save, savePeek)
	}
	return p.parseOr()
}

func (p *Parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.tok.Type == TOp && p.tok.Text == "||" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinOp{Op: "||", Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseAnd() (Expr, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.tok.Type == TOp && p.tok.Text == "&&" {
		p.next()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &BinOp{Op: "&&", Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseEquality() (Expr, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for p.tok.Type == TOp && (p.tok.Text == "==" || p.tok.Text == "!=" || p.tok.Text == "=~") {
		op := p.tok.Text
		p.next()
		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		left = &BinOp{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseRelational() (Expr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for p.tok.Type == TOp && (p.tok.Text == "<" || p.tok.Text == ">" || p.tok.Text == "<=" || p.tok.Text == ">=") {
		op := p.tok.Text
		p.next()
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &BinOp{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseAdditive() (Expr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.tok.Type == TOp && (p.tok.Text == "+" || p.tok.Text == "-" || p.tok.Text == ".") {
		op := p.tok.Text
		p.next()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &BinOp{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseMultiplicative() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.tok.Type == TOp && (p.tok.Text == "*" || p.tok.Text == "/" || p.tok.Text == "%") {
		op := p.tok.Text
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinOp{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseUnary() (Expr, error) {
	if p.tok.Type == TOp && (p.tok.Text == "!" || p.tok.Text == "-") {
		op := p.tok.Text
		p.next()
		e, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Unary{Op: op, Expr: e}, nil
	}
	return p.parsePostfix()
}

// parsePostfix 后缀：调用、索引、属性访问
func (p *Parser) parsePostfix() (Expr, error) {
	e, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		if p.tok.Type == TOp && p.tok.Text == "(" {
			p.next()
			var args []Expr
			for {
				if p.tok.Type == TOp && p.tok.Text == ")" {
					p.next()
					break
				}
				if p.tok.Type == TEOF || (p.tok.Type == TOp && (p.tok.Text == "}" || p.tok.Text == ";")) {
					break
				}
				arg, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if p.tok.Type == TOp && p.tok.Text == "," {
					p.next()
					continue
				}
				if p.tok.Type == TOp && p.tok.Text == ")" {
					p.next()
					break
				}
			}
			e = &Call{Fn: e, Args: args}
			continue
		}
		if p.tok.Type == TOp && p.tok.Text == "[" {
			p.next()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.tok.Type == TOp && p.tok.Text == "]" {
				p.next()
			}
			e = &Index{Obj: e, Idx: idx}
			continue
		}
		if p.tok.Type == TOp && strings.HasPrefix(p.tok.Text, ".") && len(p.tok.Text) > 1 {
			// 方法访问 obj.method（lexer 已将 ".method" 合并为一个 token）
			e = &Index{Obj: e, Idx: &StrLit{Val: p.tok.Text[1:]}}
			p.next()
			continue
		}
		if p.tok.Type == TOp && p.tok.Text == "->" {
			// 字典/对象属性访问：dict->key => Index("key")
			p.next()
			if p.tok.Type == TIdent {
				e = &Index{Obj: e, Idx: &StrLit{Val: p.tok.Text}}
				p.next()
				continue
			}
			return e, nil
		}
		break
	}
	return e, nil
}

// parsePrimary 基础表达式
func (p *Parser) parsePrimary() (Expr, error) {
	t := p.tok
	switch t.Type {
	case TNumber:
		p.next()
		return &NumLit{Val: t.Val}, nil
	case TString:
		p.next()
		return p.interpolate(t)
	case TVar:
		p.next()
		return &VarRef{Prefix: t.Text[0], Name: strings.TrimLeft(t.Text, "$@%^")}, nil
	case TIdent:
		p.next()
		return &VarRef{Prefix: 0, Name: t.Text}, nil
	case TOp:
		switch t.Text {
		case "(":
			p.next()
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.tok.Type == TOp && p.tok.Text == ")" {
				p.next()
			}
			return e, nil
		case "[":
			p.next()
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.tok.Type == TOp && p.tok.Text == "]" {
				p.next()
			}
			return &Bracketed{Expr: e}, nil
		case "&":
			// 引用：&name 或 &{ ... }
			p.next()
			if p.tok.Type == TOp && p.tok.Text == "{" {
				p.skipNewlines()
				body, err := p.parseBlock()
				if err != nil {
					return nil, err
				}
				return &Ref{Blk: body}, nil
			}
			if p.tok.Type == TVar {
				name := strings.TrimLeft(p.tok.Text, "$@%^")
				p.next()
				return &Ref{Name: name}, nil
			}
			if p.tok.Type == TIdent {
				name := p.tok.Text
				p.next()
				return &Ref{Name: name}, nil
			}
			return nil, p.errf("& 后需要函数名或匿名块")
		case "@":
			// @("a","b") 数组字面量（@ 后面可能直接跟变量名？@arr 是 TVar）
			p.next()
			if p.tok.Type == TOp && p.tok.Text == "(" {
				p.next()
				var elems []Expr
				for {
					if p.tok.Type == TOp && p.tok.Text == ")" {
						p.next()
						break
					}
					if p.tok.Type == TEOF || (p.tok.Type == TOp && (p.tok.Text == "}" || p.tok.Text == ";")) {
						break
					}
					e, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					elems = append(elems, e)
					if p.tok.Type == TOp && p.tok.Text == "," {
						p.next()
						continue
					}
					if p.tok.Type == TOp && p.tok.Text == ")" {
						p.next()
						break
					}
				}
				return &ArrayLit{Elems: elems}, nil
			}
			return nil, p.errf("无效的数组字面量")
		case "%":
			p.next()
			if p.tok.Type == TOp && p.tok.Text == "(" {
				p.next()
				var keys, vals []Expr
				for {
					if p.tok.Type == TOp && p.tok.Text == ")" {
						p.next()
						break
					}
					if p.tok.Type == TEOF || (p.tok.Type == TOp && (p.tok.Text == "}" || p.tok.Text == ";")) {
						break
					}
					k, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					keys = append(keys, k)
					if p.tok.Type == TOp && p.tok.Text == "=>" {
						p.next()
						v, err := p.parseExpr()
						if err != nil {
							return nil, err
						}
						vals = append(vals, v)
					} else {
						vals = append(vals, &StrLit{Val: ""})
					}
					if p.tok.Type == TOp && p.tok.Text == "," {
						p.next()
						continue
					}
					if p.tok.Type == TOp && p.tok.Text == ")" {
						p.next()
						break
					}
				}
				return &DictLit{Keys: keys, Vals: vals}, nil
			}
			return nil, p.errf("无效的字典字面量")
		case "=>":
			// 单独出现时按空返回
			p.next()
			return &StrLit{Val: ""}, nil
		case "}", ";", "]", ",", ")":
			// 语句/表达式终止符：返回空字符串（容忍）
			return &StrLit{Val: ""}, nil
		}
	}
	return nil, p.errf("意外的 token: %q (行 %d)", t.Text, t.Line)
}

// interpolate 字符串插值：双引号支持 $var 与 [expr]
func (p *Parser) interpolate(t Token) (Expr, error) {
	raw := t.Raw
	if t.Quote != '"' {
		return &StrLit{Val: unescape(raw, t.Quote)}, nil
	}
	// 双引号：解析插值片段
	var parts []Expr
	buf := ""
	i := 0
	flush := func() {
		if buf != "" {
			parts = append(parts, &StrLit{Val: unescape(buf, '"')})
			buf = ""
		}
	}
	for i < len(raw) {
		c := raw[i]
		if c == '\\' && i+1 < len(raw) {
			buf += string(raw[i]) + string(raw[i+1])
			i += 2
			continue
		}
		if c == '$' && i+1 < len(raw) && isVarChar(raw[i+1]) {
			flush()
			j := i + 1
			start := j
			for j < len(raw) && isVarChar(raw[j]) {
				j++
			}
			name := raw[start:j]
			parts = append(parts, &VarRef{Prefix: '$', Name: strings.TrimLeft(name, "$@%^")})
			i = j
			continue
		}
		if c == '[' {
			// [expr] 插值：从 raw 中截取表达式文本，递归解析
			depth := 0
			j := i
			for j < len(raw) {
				if raw[j] == '[' {
					depth++
				} else if raw[j] == ']' {
					depth--
					if depth == 0 {
						break
					}
				}
				j++
			}
			if depth == 0 && j > i {
				flush()
				inner := raw[i+1 : j]
				sub, err := ParseExpr(inner)
				if err != nil {
					return nil, err
				}
				parts = append(parts, sub)
				i = j + 1
				continue
			}
		}
		buf += string(c)
		i++
	}
	flush()
	if len(parts) == 0 {
		return &StrLit{Val: ""}, nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	// 拼接
	e := parts[0]
	for _, part := range parts[1:] {
		e = &BinOp{Op: ".", Left: e, Right: part}
	}
	return e, nil
}

// unescape 处理转义序列（保持原始转义由解释器处理?简化：这里统一处理）
func unescape(s string, quote byte) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			n := s[i+1]
			switch n {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			case '\'':
				sb.WriteByte('\'')
			case 'x', '0':
				// 不处理 \x00 十六进制
				sb.WriteByte(n)
			default:
				sb.WriteByte(n)
			}
			i++
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// ParseExpr 解析单个表达式（供插值递归使用）
func ParseExpr(src string) (Expr, error) {
	p := &Parser{lex: NewLexer(src)}
	p.next()
	p.next()
	p.skipNewlines()
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return e, nil
}

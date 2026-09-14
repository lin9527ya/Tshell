package cna

import (
	"fmt"
	"strings"
)

// TokenType 词法单元类型
type TokenType int

const (
	TEOF TokenType = iota
	TNewline
	TIdent    // 标识符 / 关键字（sub/if/on 等）
	TVar      // $x / @arr / %dict
	TNumber
	TString // 已插值的字符串
	TOp     // 运算符或符号
)

// Token 词法单元
type Token struct {
	Type  TokenType
	Text  string  // 原文（字符串 token 为插值后的内容？不，字符串插值在解析期做，这里存原文）
	Raw   string  // 字符串原始内容（未插值）
	Quote byte    // 字符串引号字符：'"' 或 '\''（单引号不插值）
	Val   float64 // 数字值
	Line  int
	Col   int
}

// 运算符文本集合（用于快速判断）
var opChars = "+-*/%=<>!&|.?:,;()[]{}"

// Lexer Sleep 子集词法器
type Lexer struct {
	src  string
	pos  int
	line int
	col  int
}

// NewLexer 创建词法器
func NewLexer(src string) *Lexer {
	return &Lexer{src: src, line: 1, col: 1}
}

func (l *Lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(off int) byte {
	if l.pos+off >= len(l.src) {
		return 0
	}
	return l.src[l.pos+off]
}

func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

// skipSpace 跳过空格、\r、\t；遇 \n 时作为语句分隔由调用方决定。
// 这里直接返回 Token（调用方忽略空白）。
func (l *Lexer) skipInlineSpace() {
	for l.pos < len(l.src) {
		c := l.peek()
		if c == ' ' || c == '\t' || c == '\r' {
			l.advance()
			continue
		}
		break
	}
}

// Next 取下一个 token（自动跳过注释与空白，遇到换行产生 TNewline）
func (l *Lexer) Next() Token {
	for {
		l.skipInlineSpace()
		if l.pos >= len(l.src) {
			return Token{Type: TEOF, Line: l.line, Col: l.col}
		}
		c := l.peek()
		if c == '#' {
			// 行注释：跳至行尾（含换行，即注释行不产生 NEWLINE？产生，便于语句分隔）
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
			if l.pos >= len(l.src) {
				return Token{Type: TEOF, Line: l.line, Col: l.col}
			}
			l.advance() // 吃掉 \n
			return Token{Type: TNewline, Text: "\n", Line: l.line, Col: l.col}
		}
		if c == '\n' {
			l.advance()
			return Token{Type: TNewline, Text: "\n", Line: l.line, Col: l.col}
		}
		break
	}

	startLine, startCol := l.line, l.col
	c := l.peek()

	// 变量：$name @name %name ^name
	if c == '$' || c == '@' || c == '%' || c == '^' {
		l.advance()
		name := string(c)
		// 支持 $name / $name 下划线数字；也支持 $1..$9（数字参数）
		for l.pos < len(l.src) {
			ch := l.peek()
			if ch == '_' || ch == '0' || ch == '1' || ch == '2' || ch == '3' || ch == '4' || ch == '5' || ch == '6' || ch == '7' || ch == '8' || ch == '9' || ch == '-' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				// '-' 仅在紧邻一个字符时按变量名一部分（如 $a-b? 罕见，避免吞掉减法）
				if ch == '-' && !isVarChar(l.peekAt(1)) {
					break
				}
				name += string(l.advance())
				continue
			}
			break
		}
		if len(name) == 1 {
			// @( / %( 等：@ % 是运算符（数组/字典构造），不是变量
			return Token{Type: TOp, Text: name, Line: startLine, Col: startCol}
		}
		return Token{Type: TVar, Text: name, Line: startLine, Col: startCol}
	}

	// 数字
	if c >= '0' && c <= '9' {
		num := ""
		hasDot := false
		for l.pos < len(l.src) {
			ch := l.peek()
			if ch >= '0' && ch <= '9' {
				num += string(l.advance())
			} else if ch == '.' && !hasDot && isDigit(l.peekAt(1)) {
				hasDot = true
				num += string(l.advance())
			} else {
				break
			}
		}
		var f float64
		fmt.Sscanf(num, "%g", &f)
		return Token{Type: TNumber, Text: num, Val: f, Line: startLine, Col: startCol}
	}

	// 字符串
	if c == '"' || c == '\'' {
		quote := c
		l.advance()
		raw := ""
		for l.pos < len(l.src) {
			ch := l.peek()
			if ch == '\\' {
				raw += string(l.advance())
				if l.pos < len(l.src) {
					raw += string(l.advance())
				}
				continue
			}
			if ch == quote {
				l.advance()
				return Token{Type: TString, Raw: raw, Quote: quote, Line: startLine, Col: startCol}
			}
			raw += string(l.advance())
		}
		// 未闭合：容忍（常见于 # 注释截断）
		return Token{Type: TString, Raw: raw, Quote: quote, Line: startLine, Col: startCol}
	}

	// 标识符 / 关键字
	if isIdentStart(c) {
		name := ""
		for l.pos < len(l.src) && isIdentChar(l.peek()) {
			name += string(l.advance())
		}
		return Token{Type: TIdent, Text: name, Line: startLine, Col: startCol}
	}

	// 运算符
	if strings.ContainsRune(opChars, rune(c)) {
		op := string(l.advance())
		// 方法访问：. 紧邻标识符（无空格）时合并为 ".name" token（如 "a".to_upper()）；
		// 与连接运算 "."（后跟空格/变量）区分
		if op == "." && l.pos < len(l.src) && isIdentStart(l.peek()) {
			name := ""
			for l.pos < len(l.src) && isIdentChar(l.peek()) {
				name += string(l.advance())
			}
			return Token{Type: TOp, Text: "." + name, Line: startLine, Col: startCol}
		}
		// 双字符运算符
		if l.pos < len(l.src) {
			next := l.peek()
			two := op + string(next)
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||", "=~", "+=", "-=", "->", "=>":
				l.advance()
				op = two
			}
		}
		if op == "&" && l.peek() == '{' {
			return Token{Type: TOp, Text: "&{", Line: startLine, Col: startCol}
		}
		if op == "&" && l.peek() == '$' {
			return Token{Type: TOp, Text: "&", Line: startLine, Col: startCol}
		}
		return Token{Type: TOp, Text: op, Line: startLine, Col: startCol}
	}

	// 未知字符：跳过（容忍）
	l.advance()
	return Token{Type: TOp, Text: string(c), Line: startLine, Col: startCol}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isVarChar(c byte) bool {
	return c == '_' || isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-'
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

// quoteString 将字符串 token 转为其内容（处理转义）
func quoteString(text string) string { return text }

package cna

// ===== AST 节点 =====

// Expr 表达式接口
type Expr interface{ exprNode() }

// Stmt 语句接口
type Stmt interface{ stmtNode() }

// ---- 表达式 ----

// NumLit 数字字面量
type NumLit struct{ Val float64 }

// StrLit 字符串字面量（插值已拆为拼接）
type StrLit struct{ Val string }

// VarRef 变量引用 $x / @arr / %dict / $1
type VarRef struct {
	Prefix byte // '$' '@' '%' '^'
	Name   string
}

// ArrayLit 数组构造 @(a, b) 或 @a 字面量
type ArrayLit struct{ Elems []Expr }

// DictLit 字典构造 %("k" => v)
type DictLit struct {
	Keys []Expr
	Vals []Expr
}

// Index 索引 expr[idx]
type Index struct {
	Obj Expr
	Idx Expr
}

// Call 函数/方法调用 obj(args)
type Call struct {
	Fn   Expr
	Args []Expr
}

// Ref 引用 &name 或 &{ }
type Ref struct {
	Name string
	Blk  *Block
}

// BinOp 二元运算
type BinOp struct {
	Op    string
	Left  Expr
	Right Expr
}

// Unary 一元运算 !x / -x
type Unary struct {
	Op   string
	Expr Expr
}

// Bracketed 立即求值 [expr]
type Bracketed struct{ Expr Expr }

// Assign 赋值 $x = expr / @a = expr
type Assign struct {
	Prefix byte
	Name   string
	Op     string // "=" / "+="
	Value  Expr
}

// ---- 语句 ----

// Block 语句块
type Block struct{ Stmts []Stmt }

// ExprStmt 表达式语句
type ExprStmt struct{ Expr Expr }

// IfStmt if/else
type IfStmt struct {
	Cond Expr
	Then *Block
	Else *Block // 可 nil
}

// ForeachStmt foreach $x (@arr) {}
type ForeachStmt struct {
	Prefix byte
	Var    string
	Iter   Expr
	Body   *Block
}

// WhileStmt while (cond) {}
type WhileStmt struct {
	Cond Expr
	Body *Block
}

// ForStmt for (init; cond; post) {}
type ForStmt struct {
	Init Stmt
	Cond Expr
	Post Stmt
	Body *Block
}

// ReturnStmt return [expr]
type ReturnStmt struct{ Expr Expr }

// BreakStmt break
type BreakStmt struct{}

// ContinueStmt continue
type ContinueStmt struct{}

// LocalStmt local $x = ...
type LocalStmt struct{ Stmts []Stmt }

// SubStmt sub name (args) { body }
type SubStmt struct {
	Name   string
	Params []string // 形参名（可空，用 $1..$9）
	Body   *Block
	Line   int
}

// OnStmt on event (params) { body } / on event &handler
type OnStmt struct {
	Event string
	Fn    *SubStmt // 处理器（匿名 sub）
	Line  int
}

// AliasStmt alias name { body }
type AliasStmt struct {
	Name string
	Fn   *SubStmt
	Line int
}

// MenuStmt menu kind { ... } / popup
type MenuStmt struct {
	Kind  string
	Items []*MenuItem
	Line  int
}

// MenuItem 菜单项（item "Name" { body } / separator / 嵌套子菜单）
type MenuItem struct {
	Label     string // 非空=菜单项
	Separator bool
	Fn        *SubStmt // 菜单项执行体
	SubItems  []*MenuItem // 嵌套子菜单
	Line      int
}

// GlobalStmt global $x = ...
type GlobalStmt struct{ Stmts []Stmt }

// NopStmt 空语句（如仅注释行）
type NopStmt struct{}

func (NumLit) exprNode()     {}
func (StrLit) exprNode()     {}
func (VarRef) exprNode()     {}
func (ArrayLit) exprNode()   {}
func (DictLit) exprNode()    {}
func (Index) exprNode()      {}
func (Call) exprNode()       {}
func (Ref) exprNode()        {}
func (BinOp) exprNode()      {}
func (Unary) exprNode()      {}
func (Bracketed) exprNode()  {}
func (Assign) stmtNode()     {}
func (Assign) exprNode()     {}
func (ExprStmt) stmtNode()   {}
func (IfStmt) stmtNode()     {}
func (ForeachStmt) stmtNode() {}
func (WhileStmt) stmtNode()  {}
func (ForStmt) stmtNode()    {}
func (ReturnStmt) stmtNode() {}
func (BreakStmt) stmtNode()  {}
func (ContinueStmt) stmtNode() {}
func (LocalStmt) stmtNode()  {}
func (SubStmt) stmtNode()    {}
func (OnStmt) stmtNode()     {}
func (AliasStmt) stmtNode()  {}
func (MenuStmt) stmtNode()   {}
func (GlobalStmt) stmtNode() {}
func (Block) stmtNode()      {}
func (NopStmt) stmtNode()    {}

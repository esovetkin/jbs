// define abstract syntax tree for JBS
package ast

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"

type Node interface {
	GetSpan() diag.Span
}

type Stmt interface {
	Node
	stmtNode()
}

type Program struct {
	File  string
	Stmts []Stmt
	Span  diag.Span
}

func (p Program) GetSpan() diag.Span { return p.Span }

type Comment struct {
	Text string
	Span diag.Span
}

func (c Comment) GetSpan() diag.Span { return c.Span }

type CommentGroup struct {
	Comments []Comment
}

type NodeComments struct {
	Leading  []CommentGroup
	Inline   *Comment
	Trailing []CommentGroup
}

type HeaderElemKind string

const (
	HeaderElemAfter   HeaderElemKind = "after"
	HeaderElemWith    HeaderElemKind = "with"
	HeaderElemFSub    HeaderElemKind = "fsub"
	HeaderElemOption  HeaderElemKind = "option"
	HeaderElemComment HeaderElemKind = "comment"
	HeaderElemBlank   HeaderElemKind = "blank"
	HeaderElemUnknown HeaderElemKind = "unknown"
)

type HeaderElem struct {
	Kind    HeaderElemKind
	Text    string
	Inline  *Comment
	Comment *Comment
	Span    diag.Span
}

func (h HeaderElem) GetSpan() diag.Span { return h.Span }

type UseSourceKind string

const (
	UseSourceBare UseSourceKind = "bare"
	UseSourcePath UseSourceKind = "path"
)

type UseSource struct {
	Kind  UseSourceKind
	Value string
	Span  diag.Span
}

func (u UseSource) GetSpan() diag.Span { return u.Span }

type UseStmt struct {
	Names  []string
	Source UseSource
	Alias  string
	Span   diag.Span
}

func (u UseStmt) stmtNode()          {}
func (u UseStmt) GetSpan() diag.Span { return u.Span }

type WithItem struct {
	Expr      Expr
	Alias     string
	AliasSpan diag.Span
	Span      diag.Span
}

func (w WithItem) GetSpan() diag.Span { return w.Span }

type AssignOp string

const (
	AssignEq      AssignOp = "="
	AssignPlusEq  AssignOp = "+="
	AssignMinusEq AssignOp = "-="
	AssignStarEq  AssignOp = "*="
	AssignSlashEq AssignOp = "/="
	AssignPctEq   AssignOp = "%="
)

type Assignment struct {
	Name string
	Op   AssignOp
	Expr Expr
	Span diag.Span
}

func (a Assignment) GetSpan() diag.Span { return a.Span }

type GlobalAssign struct {
	Name string
	Op   AssignOp
	Expr Expr
	Span diag.Span
}

func (g GlobalAssign) stmtNode()          {}
func (g GlobalAssign) GetSpan() diag.Span { return g.Span }

type ExprStmt struct {
	Expr Expr
	Span diag.Span
}

func (e ExprStmt) stmtNode()          {}
func (e ExprStmt) funcBodyStmtNode()  {}
func (e ExprStmt) GetSpan() diag.Span { return e.Span }

type IfStmt struct {
	Cond  Expr
	Then  []Stmt
	Elifs []ElifBranch
	Else  []Stmt
	Span  diag.Span
}

func (i IfStmt) stmtNode()          {}
func (i IfStmt) GetSpan() diag.Span { return i.Span }

type ElifBranch struct {
	Cond Expr
	Body []Stmt
	Span diag.Span
}

func (b ElifBranch) GetSpan() diag.Span { return b.Span }

type ForStmt struct {
	Target   string
	Iterable Expr
	Body     []Stmt
	Span     diag.Span
}

func (s ForStmt) stmtNode()          {}
func (s ForStmt) GetSpan() diag.Span { return s.Span }

type WhileStmt struct {
	Cond Expr
	Body []Stmt
	Span diag.Span
}

func (s WhileStmt) stmtNode()          {}
func (s WhileStmt) GetSpan() diag.Span { return s.Span }

type BreakStmt struct {
	Span diag.Span
}

func (s BreakStmt) stmtNode()          {}
func (s BreakStmt) funcBodyStmtNode()  {}
func (s BreakStmt) GetSpan() diag.Span { return s.Span }

type ContinueStmt struct {
	Span diag.Span
}

func (s ContinueStmt) stmtNode()          {}
func (s ContinueStmt) funcBodyStmtNode()  {}
func (s ContinueStmt) GetSpan() diag.Span { return s.Span }

type AnalyseBlock struct {
	StepName    string
	WithItems   []WithItem
	Assignments []AnalyseAssign
	Columns     []AnalyseColumn
	HeaderRaw   string
	Header      []HeaderElem
	BodyRaw     string
	Span        diag.Span
	Comments    NodeComments
}

func (a AnalyseBlock) stmtNode()          {}
func (a AnalyseBlock) GetSpan() diag.Span { return a.Span }

type AnalyseAssign struct {
	Name       string
	Op         AssignOp
	Expr       Expr
	File       string
	FileTarget AnalyseFileTarget
	Span       diag.Span
}

func (a AnalyseAssign) GetSpan() diag.Span { return a.Span }

type AnalyseFileKind string

const (
	AnalyseFileNone  AnalyseFileKind = ""
	AnalyseFileExact AnalyseFileKind = "exact"
	AnalyseFileRegex AnalyseFileKind = "regex"
)

type AnalyseFileTarget struct {
	Kind  AnalyseFileKind
	Value string
	Span  diag.Span
}

func (t AnalyseFileTarget) IsSet() bool {
	return t.Kind != AnalyseFileNone
}

func ExactAnalyseFile(value string, span diag.Span) AnalyseFileTarget {
	return AnalyseFileTarget{Kind: AnalyseFileExact, Value: value, Span: span}
}

func RegexAnalyseFile(value string, span diag.Span) AnalyseFileTarget {
	return AnalyseFileTarget{Kind: AnalyseFileRegex, Value: value, Span: span}
}

func (a AnalyseAssign) EffectiveFileTarget() AnalyseFileTarget {
	if a.FileTarget.IsSet() {
		return a.FileTarget
	}
	if a.File != "" {
		return ExactAnalyseFile(a.File, a.Span)
	}
	return AnalyseFileTarget{}
}

type AnalyseColumnKind string

const (
	AnalyseColumnNamed         AnalyseColumnKind = "named"
	AnalyseColumnInlinePattern AnalyseColumnKind = "inline_pattern"
)

type AnalyseColumn struct {
	Kind       AnalyseColumnKind
	Name       string
	Expr       Expr
	File       string
	FileTarget AnalyseFileTarget
	Title      string
	Span       diag.Span
}

func (a AnalyseColumn) GetSpan() diag.Span { return a.Span }

func (a AnalyseColumn) EffectiveFileTarget() AnalyseFileTarget {
	if a.FileTarget.IsSet() {
		return a.FileTarget
	}
	if a.File != "" {
		return ExactAnalyseFile(a.File, a.Span)
	}
	return AnalyseFileTarget{}
}

type DoBlock struct {
	Name      string
	After     []string
	WithItems []WithItem
	NProc     *int
	FSubs     []FileSubstitution
	HeaderRaw string
	Header    []HeaderElem
	Body      string
	BodyStart diag.Position
	Span      diag.Span
	Comments  NodeComments
}

func (d DoBlock) stmtNode()          {}
func (d DoBlock) GetSpan() diag.Span { return d.Span }

type FileSubstitution struct {
	Path      string
	PathSpan  diag.Span
	Rules     []FileSubstitutionRule
	BodyRaw   string
	BodyStart diag.Position
	Span      diag.Span
}

func (f FileSubstitution) GetSpan() diag.Span { return f.Span }

type FileSubstitutionRule struct {
	Pattern     string
	PatternSpan diag.Span
	Expr        Expr
	Span        diag.Span
}

func (r FileSubstitutionRule) GetSpan() diag.Span { return r.Span }

type Expr interface {
	Node
	exprNode()
}

type FuncBodyStmt interface {
	Node
	funcBodyStmtNode()
}

type IdentExpr struct {
	Name string
	Span diag.Span
}

func (e IdentExpr) exprNode()          {}
func (e IdentExpr) GetSpan() diag.Span { return e.Span }

type QualifiedIdentExpr struct {
	Namespace string
	Name      string
	Span      diag.Span
}

func (e QualifiedIdentExpr) exprNode()          {}
func (e QualifiedIdentExpr) GetSpan() diag.Span { return e.Span }

type MemberExpr struct {
	Base Expr
	Name string
	Span diag.Span
}

func (e MemberExpr) exprNode()          {}
func (e MemberExpr) GetSpan() diag.Span { return e.Span }

type IndexExpr struct {
	Base  Expr
	Items []Expr
	Span  diag.Span
}

func (e IndexExpr) exprNode()          {}
func (e IndexExpr) GetSpan() diag.Span { return e.Span }

type StringExpr struct {
	Value string
	Span  diag.Span
}

func (e StringExpr) exprNode()          {}
func (e StringExpr) GetSpan() diag.Span { return e.Span }

type NumberExpr struct {
	Raw        string
	Int        bool
	IntValue   int64
	FloatValue float64
	Span       diag.Span
}

func (e NumberExpr) exprNode()          {}
func (e NumberExpr) GetSpan() diag.Span { return e.Span }

type BoolExpr struct {
	Value bool
	Span  diag.Span
}

func (e BoolExpr) exprNode()          {}
func (e BoolExpr) GetSpan() diag.Span { return e.Span }

type ListExpr struct {
	Items []Expr
	Span  diag.Span
}

func (e ListExpr) exprNode()          {}
func (e ListExpr) GetSpan() diag.Span { return e.Span }

type TupleExpr struct {
	Items []Expr
	Span  diag.Span
}

func (e TupleExpr) exprNode()          {}
func (e TupleExpr) GetSpan() diag.Span { return e.Span }

type DictEntryExpr struct {
	Key   Expr
	Value Expr
	Span  diag.Span
}

func (e DictEntryExpr) GetSpan() diag.Span { return e.Span }

type DictExpr struct {
	Entries []DictEntryExpr
	Span    diag.Span
}

func (e DictExpr) exprNode()          {}
func (e DictExpr) GetSpan() diag.Span { return e.Span }

type RangeExpr struct {
	Start Expr
	Stop  Expr
	Step  Expr
	Span  diag.Span
}

func (e RangeExpr) exprNode()          {}
func (e RangeExpr) GetSpan() diag.Span { return e.Span }

type CallArgKind uint8

const (
	CallArgPositional CallArgKind = iota
	CallArgNamed
	CallArgPositionalSpread
	CallArgKeywordSpread
)

// CallArg represents a positional, named, `*` spread, or `**` spread call
// argument. A zero Kind with a non-empty Name is treated as named for
// compatibility with tests and older AST construction helpers.
type CallArg struct {
	Kind CallArgKind
	Name string
	Expr Expr
	Span diag.Span
}

func (a CallArg) GetSpan() diag.Span { return a.Span }

func (a CallArg) EffectiveKind() CallArgKind {
	if a.Kind != CallArgPositional {
		return a.Kind
	}
	if a.Name != "" {
		return CallArgNamed
	}
	return CallArgPositional
}

func PosCallArg(expr Expr) CallArg {
	span := diag.Span{}
	if expr != nil {
		span = expr.GetSpan()
	}
	return CallArg{Kind: CallArgPositional, Expr: expr, Span: span}
}

func PosCallArgs(exprs ...Expr) []CallArg {
	if len(exprs) == 0 {
		return nil
	}
	out := make([]CallArg, 0, len(exprs))
	for _, expr := range exprs {
		out = append(out, PosCallArg(expr))
	}
	return out
}

type CallExpr struct {
	Callee Expr
	Args   []CallArg
	Span   diag.Span
}

func (e CallExpr) exprNode()          {}
func (e CallExpr) GetSpan() diag.Span { return e.Span }

// FunctionExpr is a first-class expression node. Ordinary assignments such as
// `name = function(...) { ... }` remain plain assignments whose right-hand side
// is this expression value.
type FunctionExpr struct {
	Params []FuncParam
	Body   []FuncBodyStmt
	Span   diag.Span
}

func (e FunctionExpr) exprNode()          {}
func (e FunctionExpr) GetSpan() diag.Span { return e.Span }

type FuncParam struct {
	Kind    FuncParamKind
	Name    string
	Default Expr
	Span    diag.Span
}

func (p FuncParam) GetSpan() diag.Span { return p.Span }

type FuncParamKind uint8

const (
	FuncParamValue FuncParamKind = iota
	FuncParamArgs
	FuncParamKwargs
)

type LocalAssignStmt struct {
	Name string
	Op   AssignOp
	Expr Expr
	Span diag.Span
}

func (s LocalAssignStmt) funcBodyStmtNode()  {}
func (s LocalAssignStmt) GetSpan() diag.Span { return s.Span }

type ReturnStmt struct {
	Expr Expr
	Span diag.Span
}

func (s ReturnStmt) funcBodyStmtNode()  {}
func (s ReturnStmt) GetSpan() diag.Span { return s.Span }

type FuncIfStmt struct {
	Cond  Expr
	Then  []FuncBodyStmt
	Elifs []FuncElifBranch
	Else  []FuncBodyStmt
	Span  diag.Span
}

func (s FuncIfStmt) funcBodyStmtNode()  {}
func (s FuncIfStmt) GetSpan() diag.Span { return s.Span }

type FuncElifBranch struct {
	Cond Expr
	Body []FuncBodyStmt
	Span diag.Span
}

func (b FuncElifBranch) GetSpan() diag.Span { return b.Span }

type FuncForStmt struct {
	Target   string
	Iterable Expr
	Body     []FuncBodyStmt
	Span     diag.Span
}

func (s FuncForStmt) funcBodyStmtNode()  {}
func (s FuncForStmt) GetSpan() diag.Span { return s.Span }

type FuncWhileStmt struct {
	Cond Expr
	Body []FuncBodyStmt
	Span diag.Span
}

func (s FuncWhileStmt) funcBodyStmtNode()  {}
func (s FuncWhileStmt) GetSpan() diag.Span { return s.Span }

type AliasExpr struct {
	Expr  Expr
	Alias string
	Span  diag.Span
}

func (e AliasExpr) exprNode()          {}
func (e AliasExpr) GetSpan() diag.Span { return e.Span }

type UnaryExpr struct {
	Op   string
	Expr Expr
	Span diag.Span
}

func (e UnaryExpr) exprNode()          {}
func (e UnaryExpr) GetSpan() diag.Span { return e.Span }

type BinaryExpr struct {
	Left  Expr
	Op    string
	Right Expr
	Span  diag.Span
}

func (e BinaryExpr) exprNode()          {}
func (e BinaryExpr) GetSpan() diag.Span { return e.Span }

type CompareExpr struct {
	Left  Expr
	Op    string
	Right Expr
	Span  diag.Span
}

func (e CompareExpr) exprNode()          {}
func (e CompareExpr) GetSpan() diag.Span { return e.Span }

type ConditionalExpr struct {
	Then Expr
	Cond Expr
	Else Expr
	Span diag.Span
}

func (e ConditionalExpr) exprNode()          {}
func (e ConditionalExpr) GetSpan() diag.Span { return e.Span }

type CombExpr interface {
	Node
	combNode()
}

type CombIdent struct {
	Name string
	Span diag.Span
}

func (e CombIdent) combNode()          {}
func (e CombIdent) GetSpan() diag.Span { return e.Span }

type CombBinary struct {
	Left   CombExpr
	Op     string
	OpSpan diag.Span
	Right  CombExpr
	Span   diag.Span
}

func (e CombBinary) combNode()          {}
func (e CombBinary) GetSpan() diag.Span { return e.Span }

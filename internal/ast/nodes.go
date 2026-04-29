// define abstract syntax tree for JBS
package ast

import "jbs/internal/diag"

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
	HeaderElemUse     HeaderElemKind = "use"
	HeaderElemWith    HeaderElemKind = "with"
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
	Source    string
	Selectors []string
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
	Name string
	Op   AssignOp
	Expr Expr
	File string
	Span diag.Span
}

func (a AnalyseAssign) GetSpan() diag.Span { return a.Span }

type AnalyseColumn struct {
	Name  string
	Title string
	Span  diag.Span
}

func (a AnalyseColumn) GetSpan() diag.Span { return a.Span }

type DoBlock struct {
	Name       string
	After      []string
	WithItems  []WithItem
	MaxAsync   *int
	Procs      *int
	Iterations *int
	HeaderRaw  string
	Header     []HeaderElem
	Body       string
	BodyStart  diag.Position
	Span       diag.Span
	Comments   NodeComments
}

func (d DoBlock) stmtNode()          {}
func (d DoBlock) GetSpan() diag.Span { return d.Span }

type SubmitBlock struct {
	Name       string
	After      []string
	UseNames   []string
	WithItems  []WithItem
	MaxAsync   *int
	Procs      *int
	Iterations *int
	HeaderRaw  string
	Header     []HeaderElem
	Fields     []SubmitField
	BodyRaw    string
	Span       diag.Span
	Comments   NodeComments
}

func (s SubmitBlock) stmtNode()          {}
func (s SubmitBlock) GetSpan() diag.Span { return s.Span }

type SubmitField struct {
	Name     string
	Op       AssignOp
	Expr     Expr
	Raw      string
	RawStart diag.Position
	IsRaw    bool
	Span     diag.Span
}

func (s SubmitField) GetSpan() diag.Span { return s.Span }

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

// CallArg represents either a positional call argument (`Name == ""`)
// or a named argument placeholder for later parser/evaluator phases.
type CallArg struct {
	Name string
	Expr Expr
	Span diag.Span
}

func (a CallArg) GetSpan() diag.Span { return a.Span }

func PosCallArg(expr Expr) CallArg {
	span := diag.Span{}
	if expr != nil {
		span = expr.GetSpan()
	}
	return CallArg{Expr: expr, Span: span}
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
	Name    string
	Default Expr
	Span    diag.Span
}

func (p FuncParam) GetSpan() diag.Span { return p.Span }

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

type ModeExpr struct {
	Mode string
	Expr Expr
	Span diag.Span
}

func (e ModeExpr) exprNode()          {}
func (e ModeExpr) GetSpan() diag.Span { return e.Span }

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

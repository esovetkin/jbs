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
	Name string
	From string
	Span diag.Span
}

func (w WithItem) GetSpan() diag.Span { return w.Span }

type Assignment struct {
	Name string
	Expr Expr
	Span diag.Span
}

func (a Assignment) GetSpan() diag.Span { return a.Span }

type GlobalAssign struct {
	Name string
	Expr Expr
	Span diag.Span
}

func (g GlobalAssign) stmtNode()          {}
func (g GlobalAssign) GetSpan() diag.Span { return g.Span }

type LetBlock struct {
	Name        string
	Assignments []Assignment
	BodyRaw     string
	Span        diag.Span
}

func (l LetBlock) stmtNode()          {}
func (l LetBlock) GetSpan() diag.Span { return l.Span }

type AnalyseBlock struct {
	StepName    string
	WithItems   []WithItem
	Assignments []AnalyseAssign
	Columns     []AnalyseColumn
	BodyRaw     string
	Span        diag.Span
}

func (a AnalyseBlock) stmtNode()          {}
func (a AnalyseBlock) GetSpan() diag.Span { return a.Span }

type AnalyseAssign struct {
	Name string
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

type ParamBlock struct {
	Name        string
	WithItems   []WithItem
	Assignments []Assignment
	Final       CombExpr
	BodyRaw     string
	Span        diag.Span
}

func (p ParamBlock) stmtNode()          {}
func (p ParamBlock) GetSpan() diag.Span { return p.Span }

type DoBlock struct {
	Name       string
	After      []string
	WithItems  []WithItem
	MaxAsync   *int
	Iterations *int
	Body       string
	BodyStart  diag.Position
	Span       diag.Span
}

func (d DoBlock) stmtNode()          {}
func (d DoBlock) GetSpan() diag.Span { return d.Span }

type SubmitBlock struct {
	Name       string
	After      []string
	UseNames   []string
	WithItems  []WithItem
	MaxAsync   *int
	Iterations *int
	Fields     []SubmitField
	BodyRaw    string
	Span       diag.Span
}

func (s SubmitBlock) stmtNode()          {}
func (s SubmitBlock) GetSpan() diag.Span { return s.Span }

type SubmitField struct {
	Name     string
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

type ConvertExpr struct {
	Target string
	Expr   Expr
	Span   diag.Span
}

func (e ConvertExpr) exprNode()          {}
func (e ConvertExpr) GetSpan() diag.Span { return e.Span }

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

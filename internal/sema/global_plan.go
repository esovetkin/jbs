package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

type globalReadRef struct {
	Name    string
	SeedAlt string
}

type globalInputKind string

const (
	globalInputAssign          globalInputKind = "assign"
	globalInputExpr            globalInputKind = "expr"
	globalInputIf              globalInputKind = "if"
	globalInputFor             globalInputKind = "for"
	globalInputWhile           globalInputKind = "while"
	globalInputBreak           globalInputKind = "break"
	globalInputContinue        globalInputKind = "continue"
	globalInputProjectedImport globalInputKind = "projected_import"
	globalInputNamespaceImport globalInputKind = "namespace_import"
	globalInputDo              globalInputKind = "do"
	globalInputAnalyse         globalInputKind = "analyse"
)

type projectedImport struct {
	LocalName    string
	SourceName   string
	SourceGlobal *GlobalVar
	Span         diag.Span
}

type globalInputStep struct {
	ID             int
	Kind           globalInputKind
	Name           string
	Assign         *ast.GlobalAssign
	ExprStmt       *ast.ExprStmt
	IfStmt         *ast.IfStmt
	ForStmt        *ast.ForStmt
	WhileStmt      *ast.WhileStmt
	Then           []globalInputStep
	Else           []globalInputStep
	Body           []globalInputStep
	BreakStmt      *ast.BreakStmt
	ContinueStmt   *ast.ContinueStmt
	Import         *projectedImport
	NamespaceScope *moduleScope
	DoBlock        *ast.DoBlock
	AnalyseBlock   *ast.AnalyseBlock
	EffectiveExpr  ast.Expr
	Reads          []globalReadRef
	Index          int
	Names          *eval.NameCatalog
	ForwardVisible bool
	BaseDir        string
}

type globalPlan struct {
	Steps      []globalInputStep
	StepByName map[string]int
	// Precomputed before execution so expression name catalogs can expose
	// locals that may be defined later in sequential or control-flow execution.
	LocalVisibleNames []string
	NextID            int
}

type globalPlanContext struct {
	InControlBody bool
	LoopDepth     int
}

func (ctx globalPlanContext) nestedControl() globalPlanContext {
	ctx.InControlBody = true
	return ctx
}

func (ctx globalPlanContext) nestedLoop() globalPlanContext {
	ctx.InControlBody = true
	ctx.LoopDepth++
	return ctx
}

type globalExecResult struct {
	UserGlobals           GlobalState
	UserGlobalVarByName   map[string]*GlobalVar
	UserGlobalOrder       []string
	TopLevelExprs         []TopLevelExprResult
	ScalarGlobals         GlobalState
	SnapshotBindings      []*GlobalBinding
	ScopeSnapshotsByIndex map[int]*ScopeSnapshot
	ScopeSnapshotsByBlock map[string]*ScopeSnapshot
}

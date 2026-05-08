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

type globalIfBranch struct {
	Cond ast.Expr
	Body []globalInputStep
	Span diag.Span
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
	Elifs          []globalIfBranch
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
	OriginIndex   int
}

func (ctx globalPlanContext) nestedControl(originIndex int) globalPlanContext {
	ctx.InControlBody = true
	ctx.OriginIndex = originIndex
	return ctx
}

func (ctx globalPlanContext) nestedLoop(originIndex int) globalPlanContext {
	ctx.InControlBody = true
	ctx.LoopDepth++
	ctx.OriginIndex = originIndex
	return ctx
}

type globalExecResult struct {
	UserGlobals           GlobalState
	UserGlobalVarByName   map[string]*GlobalVar
	UserGlobalOrder       []string
	TopLevelExprs         []TopLevelExprResult
	PrintEvents           []PrintEvent
	ScalarGlobals         GlobalState
	SnapshotBindings      []*GlobalBinding
	ScopeSnapshotsByIndex map[int]*ScopeSnapshot
	ScopeSnapshotsByBlock map[string]*ScopeSnapshot
}

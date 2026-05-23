package sema

import (
	"reflect"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestBuildStepScopePlansDropsSameSourceInheritedOverlap(t *testing.T) {
	span := diag.NewSpan("scope.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	cases := bindingWithOrigins("cases", []string{"col1", "col2"}, map[string][]eval.Value{
		"col1": {eval.Int(1), eval.Int(2)},
		"col2": {eval.String("a"), eval.String("b")},
	}, map[string]diag.Span{
		"col1": span,
		"col2": span,
	})
	res := &Result{
		BindingsByName: map[string]*GlobalBinding{"cases": cases},
		BindingsByKey:  map[BindingVersionKey]*GlobalBinding{BindingVersionKeyForBinding(cases, "cases"): cases},
		DoBlocks: []ast.DoBlock{
			{Name: "s0", WithItems: []ast.WithItem{withIndexStringItem("cases", []string{"col1"}, span)}, Span: span},
			{Name: "s1", After: []string{"s0"}, WithItems: []ast.WithItem{withIdentItem("cases", span)}, Span: span},
			{Name: "orphan", After: []string{"missing"}, Span: span},
		},
		StepOrder: []string{"s0", "s1", "orphan"},
	}

	diags := &diag.Diagnostics{}
	buildStepScopePlans(res, diags)
	if diags.HasErrors() {
		t.Fatalf("did not expect same-source overlap conflict: %s", diags.String())
	}

	s1 := res.StepScopeByName["s1"]
	if s1 == nil {
		t.Fatalf("expected s1 scope plan")
	}
	if got := s1.Inherited["col1"]; got.Source != "cases" || got.ViaStep != "s0" || got.SourceVar != "col1" {
		t.Fatalf("expected col1 inherited from s0/cases, got %#v", got)
	}
	if len(s1.ExplicitDelta) != 1 {
		t.Fatalf("expected one explicit delta for non-overlapping column, got %#v", s1.ExplicitDelta)
	}
	if delta := s1.ExplicitDelta[0]; delta.Full || delta.Visible != "col2" || delta.SourceVar != "col2" {
		t.Fatalf("expected only col2 to remain explicit, got %#v", delta)
	}
	if got := s1.Expansions[0]; got.Full || !reflect.DeepEqual(got.Vars, []ExpandedWithVar{{Visible: "col2", SourceVar: "col2"}}) {
		t.Fatalf("expected expansion to project away inherited col1, got %#v", got)
	}
	if values := s1.EffectiveValues["col1"]; len(values) != 2 || values[1].I != 2 {
		t.Fatalf("expected inherited col1 values to be cloned, got %#v", values)
	}
	if got := res.StepScopeByName["orphan"].InheritedSteps; !reflect.DeepEqual(got, []string{"missing"}) {
		t.Fatalf("expected missing dependency to be recorded and skipped, got %#v", got)
	}
}

func TestBuildStepScopePlansDeduplicatesRepeatedExplicitConflicts(t *testing.T) {
	span := diag.NewSpan("scope.jbs", diag.NewPos(2, 2, 1), diag.NewPos(3, 2, 2))
	res := &Result{
		BindingsByName: map[string]*GlobalBinding{
			"a": {Name: "a", Value: eval.Int(1), Shape: BindingScalar, Order: []string{"a"}, Vars: map[string][]eval.Value{"a": {eval.Int(1)}}},
			"b": {Name: "b", Value: eval.Int(2), Shape: BindingScalar, Order: []string{"b"}, Vars: map[string][]eval.Value{"b": {eval.Int(2)}}},
		},
		DoBlocks: []ast.DoBlock{
			{Name: "dup", WithItems: []ast.WithItem{
				withIdentAliasItem("b", "x", span),
				withIdentAliasItem("b", "x", span),
				withIdentAliasItem("a", "x", span),
				withIdentAliasItem("a", "x", span),
			}, Span: span},
		},
		StepOrder: []string{"dup"},
	}

	diags := &diag.Diagnostics{}
	buildStepScopePlans(res, diags)
	if got := countDiagCode(diags, string(diag.CodeE214)); got != 1 {
		t.Fatalf("expected repeated explicit conflicts to be reported once, got %d: %s", got, diags.String())
	}
	if plan := res.StepScopeByName["dup"]; plan == nil || len(plan.ExplicitDelta) != 1 || plan.ExplicitDelta[0].Source != "b" {
		t.Fatalf("expected first explicit import to remain, got %#v", plan)
	}
}

func TestStepScopeBindingHelpersAndConflictMessages(t *testing.T) {
	if !sameSourceVersion(VisibleBinding{Source: "a"}, VisibleBinding{Source: "a"}) {
		t.Fatalf("same unversioned source should match")
	}
	if sameSourceVersion(VisibleBinding{Source: "a"}, VisibleBinding{Source: "b"}) {
		t.Fatalf("different unversioned sources should not match")
	}
	if !sameSourceVersion(
		VisibleBinding{SourceKey: BindingVersionKey{Public: "p", Version: "v1"}},
		VisibleBinding{SourceKey: BindingVersionKey{Public: "p", Version: "v1"}},
	) {
		t.Fatalf("same source keys should match")
	}
	if sameSourceVersion(
		VisibleBinding{SourceKey: BindingVersionKey{Public: "p", Version: "v1"}},
		VisibleBinding{Source: "p"},
	) {
		t.Fatalf("versioned source should not match unversioned source")
	}
	if got := visibleBindingSourceVar(VisibleBinding{Name: "visible"}); got != "visible" {
		t.Fatalf("expected visible name fallback, got %q", got)
	}
	if got := stepScopeConflictKey(VisibleBinding{Source: "fallback"}); got != "with:fallback:" {
		t.Fatalf("expected conflict key to fall back to source, got %q", got)
	}
	if got := filterExpansionVars(nil, []ExpandedWithVar{{Visible: "x"}}); got != nil {
		t.Fatalf("nil expansion vars should stay nil, got %#v", got)
	}
	filtered := filterExpansionVars(map[string][]eval.Value{
		"x": {eval.Int(1)},
	}, []ExpandedWithVar{{Visible: "x"}})
	if values := filtered["x"]; len(values) != 1 || values[0].I != 1 {
		t.Fatalf("expected filterExpansionVars to use visible fallback, got %#v", filtered)
	}

	message, hint := stepScopeConflictMessage("run", "x",
		VisibleBinding{Source: "left"},
		VisibleBinding{Source: "right", ViaStep: "prep"},
	)
	if !strings.Contains(message, "`with` import from 'left' collides with name inherited via `after prep`") || hint == "" {
		t.Fatalf("unexpected right-inherited conflict message %q hint %q", message, hint)
	}
	message, hint = stepScopeConflictMessage("run", "x",
		VisibleBinding{Source: "left"},
		VisibleBinding{Source: "right"},
	)
	if !strings.Contains(message, "imported via `with` from 'left' and 'right'") || hint == "" {
		t.Fatalf("unexpected explicit conflict message %q hint %q", message, hint)
	}
}

func TestWithResolverNameHelpersAndAnalyseVariants(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	if got, ok := withBareName(ast.QualifiedIdentExpr{Namespace: "lib", Name: "x", Span: span}); !ok || got != "lib.x" {
		t.Fatalf("expected qualified bare name, got %q ok=%v", got, ok)
	}
	if _, ok := withBareName(ast.QualifiedIdentExpr{Namespace: "", Name: "x", Span: span}); ok {
		t.Fatalf("empty qualified namespace should not be accepted")
	}
	if _, ok := withBareName(ast.StringExpr{Value: "x", Span: span}); ok {
		t.Fatalf("string expression should not be a bare with name")
	}

	if got := analyseSourceVar("x", nil); got != "x" {
		t.Fatalf("nil analyse binding should fall back to source, got %q", got)
	}
	if got := analyseSourceVar("x", &GlobalBinding{Order: []string{"a", "b"}, Vars: map[string][]eval.Value{"a": {eval.Int(1)}, "b": {eval.Int(2)}}}); got != "x" {
		t.Fatalf("multi-column analyse source should fall back to source, got %q", got)
	}
	if got := sourceVarNameForScalar("x", nil); got != "x" {
		t.Fatalf("nil scalar binding should fall back to source, got %q", got)
	}
	if got := sourceVarNameForScalar("x", &GlobalBinding{}); got != "x" {
		t.Fatalf("empty scalar binding should fall back to source, got %q", got)
	}

	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"lib.x": {
				Name:  "lib.x",
				Value: eval.String("%d"),
				Shape: BindingScalar,
				Order: []string{"x"},
				Vars:  map[string][]eval.Value{"x": {eval.String("%d")}},
			},
			"pattern": {
				Name:  "pattern",
				Value: eval.String("%f"),
				Shape: BindingScalar,
				Order: []string{"pattern"},
				Vars:  map[string][]eval.Value{"pattern": {eval.String("%f")}},
			},
		},
	}
	diags := &diag.Diagnostics{}
	imports, issues := resolver.ResolveAnalyseWithItems([]ast.WithItem{
		{Expr: ast.QualifiedIdentExpr{Namespace: "lib", Name: "x", Span: span}, Alias: "qualified", AliasSpan: span, Span: span},
		{Expr: ast.IdentExpr{Name: "pattern", Span: span}, Alias: "qualified", AliasSpan: span, Span: span},
	}, diags)
	if len(issues) != 0 {
		t.Fatalf("did not expect analyse resolve issues, got %#v", issues)
	}
	if got := countDiagCode(diags, string(diag.CodeE214)); got != 1 {
		t.Fatalf("expected analyse alias conflict, got %d: %s", got, diags.String())
	}
	if got := imports["qualified"]; got.Source != "lib.x" || got.SourceVar != "x" {
		t.Fatalf("expected qualified analyse import to use source column, got %#v", got)
	}
}

func TestValidateStepVarReferencesDeduplicatesMissingWarningsWithZeroBodyStart(t *testing.T) {
	span := diag.NewSpan("refs.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	params := bindingWithOrigins("params", []string{"x"}, map[string][]eval.Value{
		"x": {eval.Int(1)},
	}, map[string]diag.Span{"x": span})
	res := &Result{
		Bindings:       []*GlobalBinding{params},
		BindingsByName: map[string]*GlobalBinding{"params": params},
		DoBlocks: []ast.DoBlock{
			{Name: "run", Body: "echo $x ${x}", Span: span},
		},
		StepScopeByName: map[string]*StepScopePlan{"run": {Effective: map[string]VisibleBinding{}}},
	}

	diags := &diag.Diagnostics{}
	validateStepVarReferences(res, diags)
	if got := countDiagCode(diags, string(diag.CodeW311)); got != 1 {
		t.Fatalf("expected repeated missing shell refs to warn once, got %d: %s", got, diags.String())
	}
}

func TestGlobalStepSpanCoversAllStepKinds(t *testing.T) {
	span := func(line int) diag.Span {
		return diag.NewSpan("steps.jbs", diag.NewPos(line, line, 1), diag.NewPos(line+1, line, 2))
	}
	assign := ast.GlobalAssign{Span: span(1)}
	expr := ast.ExprStmt{Span: span(2)}
	ifStmt := ast.IfStmt{Span: span(3)}
	forStmt := ast.ForStmt{Span: span(4)}
	whileStmt := ast.WhileStmt{Span: span(5)}
	breakStmt := ast.BreakStmt{Span: span(6)}
	continueStmt := ast.ContinueStmt{Span: span(7)}
	importStmt := projectedImport{Span: span(8)}
	doBlock := ast.DoBlock{Span: span(9)}
	analyseBlock := ast.AnalyseBlock{Span: span(10)}

	tests := []struct {
		name string
		step globalInputStep
		want diag.Span
	}{
		{"assign", globalInputStep{Kind: globalInputAssign, Assign: &assign}, assign.Span},
		{"expr", globalInputStep{Kind: globalInputExpr, ExprStmt: &expr}, expr.Span},
		{"if", globalInputStep{Kind: globalInputIf, IfStmt: &ifStmt}, ifStmt.Span},
		{"for", globalInputStep{Kind: globalInputFor, ForStmt: &forStmt}, forStmt.Span},
		{"while", globalInputStep{Kind: globalInputWhile, WhileStmt: &whileStmt}, whileStmt.Span},
		{"break", globalInputStep{Kind: globalInputBreak, BreakStmt: &breakStmt}, breakStmt.Span},
		{"continue", globalInputStep{Kind: globalInputContinue, ContinueStmt: &continueStmt}, continueStmt.Span},
		{"import", globalInputStep{Kind: globalInputProjectedImport, Import: &importStmt}, importStmt.Span},
		{"do", globalInputStep{Kind: globalInputDo, DoBlock: &doBlock}, doBlock.Span},
		{"analyse", globalInputStep{Kind: globalInputAnalyse, AnalyseBlock: &analyseBlock}, analyseBlock.Span},
		{"nil", globalInputStep{Kind: globalInputAssign}, diag.Span{}},
		{"unknown", globalInputStep{Kind: "unknown"}, diag.Span{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := globalStepSpan(tt.step); got != tt.want {
				t.Fatalf("globalStepSpan() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMergeGlobalVarsIntoStateBranches(t *testing.T) {
	mergeGlobalVarsIntoState(nil, map[string]*GlobalVar{
		"x": {Name: "x", Value: eval.Int(1)},
	})

	span := diag.NewSpan("globals.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	value := eval.List([]eval.Value{eval.Int(1)})
	state := &GlobalState{}
	mergeGlobalVarsIntoState(state, map[string]*GlobalVar{
		"":      {Name: "", Value: eval.Int(9)},
		"nil":   nil,
		"x":     {Name: "x", Value: value, Span: span},
		"other": {Name: "other", Value: eval.String("ok"), Span: span},
	})

	if state.Values == nil || state.Spans == nil {
		t.Fatalf("expected state maps to be initialized")
	}
	if _, ok := state.Values[""]; ok {
		t.Fatalf("empty global name should be skipped")
	}
	if _, ok := state.Values["nil"]; ok {
		t.Fatalf("nil global var should be skipped")
	}
	if got := state.Values["x"]; len(got.L) != 1 || got.L[0].I != 1 || state.Spans["x"] != span {
		t.Fatalf("unexpected merged x value/span: %#v %#v", got, state.Spans["x"])
	}
	value.L[0] = eval.Int(2)
	if got := state.Values["x"]; got.L[0].I != 1 {
		t.Fatalf("expected merge to clone values, got %#v", got)
	}

	mergeGlobalVarsIntoState(state, map[string]*GlobalVar{
		"x": {Name: "x", Value: eval.Int(3), Span: span},
	})
	if got := state.Values["x"]; got.Kind != eval.KindInt || got.I != 3 {
		t.Fatalf("expected later merge to overwrite x, got %#v", got)
	}
}

func TestPublishLoopVariableBranches(t *testing.T) {
	span := diag.NewSpan("loop.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	step := globalInputStep{Kind: globalInputFor, ForStmt: &ast.ForStmt{Span: span}}

	diags := &diag.Diagnostics{}
	engine := newGlobalSeqEngine(&globalPlan{}, nil, nil, globalExecOptions{}, diags)
	if engine.publishLoopVariable("", eval.Int(1), span, nil, step) {
		t.Fatalf("empty loop variable name should not publish")
	}
	if engine.publishLoopVariable("item", eval.List([]eval.Value{eval.List([]eval.Value{eval.Int(1)})}), span, nil, step) {
		t.Fatalf("nested loop value should not publish")
	}
	if countDiagCode(diags, string(diag.CodeE305)) != 1 {
		t.Fatalf("expected one nested-loop-value diagnostic, got %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	engine = newGlobalSeqEngine(&globalPlan{}, nil, map[string]eval.Value{"jbs_nproc": eval.Int(0)}, globalExecOptions{}, diags)
	if engine.publishLoopVariable("jbs_nproc", eval.List([]eval.Value{eval.Int(1)}), span, nil, step) {
		t.Fatalf("loop variable should respect scalar global validation")
	}
	if countDiagCode(diags, string(diag.CodeE304)) != 1 {
		t.Fatalf("expected scalar-global diagnostic, got %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	engine = newGlobalSeqEngine(&globalPlan{}, nil, nil, globalExecOptions{}, diags)
	engine.publishGlobalVar(&GlobalVar{Name: "dep", Value: eval.Int(5), VersionID: "dep:v1"})
	engine.publishGlobalVar(&GlobalVar{Name: "item", Value: eval.Int(99), VersionID: "old"})
	if !engine.publishLoopVariable("item", eval.Int(7), span, []string{"dep", "item"}, step) {
		t.Fatalf("expected scalar loop variable to publish: %s", diags.String())
	}
	if got := engine.values["item"]; got.Kind != eval.KindInt || got.I != 7 {
		t.Fatalf("expected loop variable to shadow previous value, got %#v", got)
	}
	if got := engine.globalVars["item"].DependsOn; !reflect.DeepEqual(got, []string{"dep"}) {
		t.Fatalf("expected self dependency to be filtered, got %#v", got)
	}
	if got := engine.globalVars["item"].DependsOnKeys; len(got) != 1 || got[0].Public != "dep" {
		t.Fatalf("expected dependency key for dep, got %#v", got)
	}
}

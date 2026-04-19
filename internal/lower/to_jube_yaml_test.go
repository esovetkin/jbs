package lower

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func TestToJUBEYAMLBuildsDocumentFromSemanticResult(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	binding := &sema.GlobalBinding{
		Name:    "jobs",
		Shape:   sema.BindingTable,
		Order:   []string{"x"},
		Rows:    []eval.Row{{Values: map[string]eval.Cell{"x": {Value: eval.Int(1)}}}},
		Vars:    map[string][]eval.Value{"x": {eval.Int(1)}},
		Origins: map[string]diag.Span{"x": span},
		Span:    span,
	}
	synthetic := &sema.GlobalBinding{
		Name:            "synthetic_jobs",
		Shape:           sema.BindingTable,
		Order:           []string{"y"},
		Vars:            map[string][]eval.Value{"y": {eval.Int(2)}},
		SyntheticGlobal: true,
		Span:            span,
	}
	doBlock := ast.DoBlock{
		Name:   "run",
		Body:   "echo hi\n",
		Span:   span,
		Header: []ast.HeaderElem{{Kind: ast.HeaderElemWith, Inline: &ast.Comment{Text: "do with"}}},
	}
	submitBlock := ast.SubmitBlock{
		Name:   "submit_run",
		Span:   span,
		Header: []ast.HeaderElem{{Kind: ast.HeaderElemUse, Inline: &ast.Comment{Text: "submit use"}}},
	}
	res := &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name":    eval.String("bench"),
			"jbs_outpath": eval.String("out_dir"),
			"jbs_comment": eval.String("c"),
			"helper_fn":   eval.Function(&eval.FunctionValue{}),
		}},
		Program:         ast.Program{Stmts: []ast.Stmt{doBlock, submitBlock}},
		Bindings:        []*sema.GlobalBinding{binding, nil, synthetic},
		BindingsByName:  map[string]*sema.GlobalBinding{"jobs": binding},
		DoBlocks:        []ast.DoBlock{doBlock},
		Submits:         []ast.SubmitBlock{submitBlock},
		StepOrder:       []string{"run", "submit_run"},
		SubmitByName:    map[string]*sema.SubmitSpec{},
		StepScopeByName: map[string]*sema.StepScopePlan{},
	}

	diags := &diag.Diagnostics{}
	doc := ToJUBEYAML(res, diags)
	if doc.Name != "bench" || doc.Outpath != "out_dir" || doc.Comment != "c" {
		t.Fatalf("unexpected root globals in lowered doc: %#v", doc)
	}
	if len(doc.ParameterSet) != 2 {
		t.Fatalf("expected one table binding plus one submit parameter set, got %#v", doc.ParameterSet)
	}
	for _, ps := range doc.ParameterSet {
		if ps.Name == "helper_fn" {
			t.Fatalf("did not expect function-valued global to be lowered as parameter set: %#v", doc.ParameterSet)
		}
	}
	if doc.ParameterSet[0].Name != "jobs" || doc.ParameterSet[0].Meta.Kind != ParameterSetKindGlobalTable {
		t.Fatalf("unexpected lowered source parameter set: %#v", doc.ParameterSet[0])
	}
	if doc.ParameterSet[1].Meta.Kind != ParameterSetKindSubmitInit || doc.ParameterSet[1].Meta.Source != "submit_run" {
		t.Fatalf("unexpected submit init parameter set: %#v", doc.ParameterSet[1])
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected do and submit lowered steps, got %#v", doc.Step)
	}
	if doc.Step[0].Meta.Kind != StepKindDo || doc.Step[0].Name != "run" {
		t.Fatalf("unexpected lowered do step: %#v", doc.Step[0])
	}
	if doc.Step[1].Meta.Kind != StepKindSubmit || doc.Step[1].Name != "submit_run" {
		t.Fatalf("unexpected lowered submit step: %#v", doc.Step[1])
	}
	if len(doc.Meta.SourceComments) != 2 {
		t.Fatalf("expected projected source comments for do and submit headers, got %#v", doc.Meta.SourceComments)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics during lowering: %s", diags.String())
	}
}

func TestToJUBEYAMLGlobalFallbacks(t *testing.T) {
	res := &sema.Result{
		Globals:         sema.GlobalState{Values: map[string]eval.Value{}},
		Program:         ast.Program{},
		SubmitByName:    map[string]*sema.SubmitSpec{},
		StepScopeByName: map[string]*sema.StepScopePlan{},
	}
	doc := ToJUBEYAML(res, &diag.Diagnostics{})
	if doc.Name != "jbs_benchmark" || doc.Outpath != "out" || doc.Comment != "" {
		t.Fatalf("unexpected fallback global values: %#v", doc)
	}
}

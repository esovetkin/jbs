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

func TestToJUBEYAMLRegroupsInheritedRowsByProjection(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	binding := hiddenProjectionBindingForLower()
	res := &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name":    eval.String("bench"),
			"jbs_outpath": eval.String("out"),
		}},
		Program: ast.Program{Stmts: []ast.Stmt{
			ast.DoBlock{Name: "step0", Body: "echo ${a}\n", Span: span},
			ast.DoBlock{Name: "step1", After: []string{"step0"}, Body: "echo ${b} ${c}\n", Span: span},
		}},
		Bindings:       []*sema.GlobalBinding{binding},
		BindingsByName: map[string]*sema.GlobalBinding{"p0": binding},
		DoBlocks: []ast.DoBlock{
			{Name: "step0", Body: "echo ${a}\n", Span: span},
			{Name: "step1", After: []string{"step0"}, Body: "echo ${b} ${c}\n", Span: span},
		},
		StepOrder:    []string{"step0", "step1"},
		SubmitByName: map[string]*sema.SubmitSpec{},
		StepScopeByName: map[string]*sema.StepScopePlan{
			"step0": {
				StepName: "step0",
				ExplicitDelta: []sema.ScopeImport{
					{Source: "p0", Visible: "a", SourceVar: "a", Span: span},
				},
				Effective: map[string]sema.VisibleBinding{
					"a": {Name: "a", Source: "p0", SourceVar: "a", Span: span},
				},
			},
			"step1": {
				StepName:       "step1",
				InheritedSteps: []string{"step0"},
				Inherited: map[string]sema.VisibleBinding{
					"a": {Name: "a", Source: "p0", SourceVar: "a", ViaStep: "step0", Span: span},
				},
				ExplicitDelta: []sema.ScopeImport{
					{Source: "p0", Visible: "b", SourceVar: "b", Span: span},
					{Source: "p0", Visible: "c", SourceVar: "c", Span: span},
				},
				Effective: map[string]sema.VisibleBinding{
					"a": {Name: "a", Source: "p0", SourceVar: "a", ViaStep: "step0", Span: span},
					"b": {Name: "b", Source: "p0", SourceVar: "b", Span: span},
					"c": {Name: "c", Source: "p0", SourceVar: "c", Span: span},
				},
			},
		},
	}

	diags := &diag.Diagnostics{}
	doc := ToJUBEYAML(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics during lowering: %s", diags.String())
	}

	var step1Set *ParameterSet
	for i := range doc.ParameterSet {
		ps := &doc.ParameterSet[i]
		if ps.Meta.Kind == ParameterSetKindSubset && ps.Meta.Source == "p0" && ps.Meta.Step == "step1" {
			step1Set = ps
			break
		}
	}
	if step1Set == nil {
		t.Fatalf("expected subset parameter set for step1, got %#v", doc.ParameterSet)
	}
	if len(step1Set.Parameter) < 4 {
		t.Fatalf("expected idx, rows, and payload parameters, got %#v", step1Set.Parameter)
	}
	if got, ok := step1Set.Parameter[0].Value.(SingleQuoted); !ok || string(got) != `{"0,1,12,13":"0,12","2,3,14,15":"2,14","4,5,16,17":"4,16","6,7,18,19":"6,18","8,9,20,21":"8,20","10,11,22,23":"10,22"}["${_jr__step0__p0__a}"]` {
		t.Fatalf("unexpected inherited idx mapping: %#v", step1Set.Parameter[0].Value)
	}
	if step1Set.Parameter[1].Separator != ReservedSeparator {
		t.Fatalf("expected grouped inherited rows helper, got %#v", step1Set.Parameter[1])
	}
	if got, ok := step1Set.Parameter[1].Value.(SingleQuoted); !ok || string(got) != `{"0":"0,1","12":"12,13","2":"2,3","14":"14,15","4":"4,5","16":"16,17","6":"6,7","18":"18,19","8":"8,9","20":"20,21","10":"10,11","22":"22,23"}["${_ji__step1__p0__b_c}"]` {
		t.Fatalf("unexpected inherited rows mapping: %#v", step1Set.Parameter[1].Value)
	}
}

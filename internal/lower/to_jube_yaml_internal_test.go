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
	param := &sema.Paramset{
		Name: "p",
		Block: ast.ParamBlock{
			Name: "p",
			Header: []ast.HeaderElem{
				{Kind: ast.HeaderElemComment, Comment: &ast.Comment{Text: "param header"}},
			},
			Span: span,
		},
		Order: []string{"x"},
		Rows: []eval.Row{
			{
				Values: map[string]eval.Cell{
					"x": {Value: eval.Int(1)},
				},
			},
		},
		Vars:    map[string][]eval.Value{"x": {eval.Int(1)}},
		Origins: map[string]diag.Span{"x": span},
		Modes:   map[string]string{},
	}

	doBlock := ast.DoBlock{
		Name:  "run",
		Body:  "echo hi\n",
		Span:  span,
		After: nil,
		Header: []ast.HeaderElem{
			{Kind: ast.HeaderElemWith, Inline: &ast.Comment{Text: "do with"}},
		},
	}
	submitBlock := ast.SubmitBlock{
		Name: "submit_run",
		Span: span,
	}

	res := &sema.Result{
		Globals: sema.GlobalState{
			Values: map[string]eval.Value{
				"jbs_name":    eval.String("bench"),
				"jbs_outpath": eval.String("out_dir"),
				"jbs_comment": eval.String("c"),
			},
		},
		Program: ast.Program{
			Stmts: []ast.Stmt{doBlock, submitBlock},
		},
		Paramsets:          []*sema.Paramset{param, nil, {Name: "g", SyntheticGlobal: true}},
		ParamByName:        map[string]*sema.Paramset{"p": param},
		DoBlocks:           []ast.DoBlock{doBlock},
		Submits:            []ast.SubmitBlock{submitBlock},
		SubmitByName:       map[string]*sema.SubmitSpec{},
		StepImportByName:   map[string]*sema.StepImportPlan{},
		ImportSourceByName: map[string]*sema.ImportSource{},
		LetByName:          map[string]*sema.LetNamespace{},
	}

	diags := &diag.Diagnostics{}
	doc := ToJUBEYAML(res, diags)

	if doc.Name != "bench" || doc.Outpath != "out_dir" || doc.Comment != "c" {
		t.Fatalf("unexpected root globals in lowered doc: %#v", doc)
	}
	if len(doc.ParameterSet) != 2 {
		t.Fatalf("expected one paramset + one submit paramset, got %#v", doc.ParameterSet)
	}
	if doc.ParameterSet[0].Name != "p" || doc.ParameterSet[0].Meta.Kind != ParameterSetKindParam {
		t.Fatalf("unexpected lowered source paramset: %#v", doc.ParameterSet[0])
	}
	if doc.ParameterSet[1].Meta.Kind != ParameterSetKindSubmitInit || doc.ParameterSet[1].Meta.Source != "submit_run" {
		t.Fatalf("unexpected submit init paramset: %#v", doc.ParameterSet[1])
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected do + submit lowered steps, got %#v", doc.Step)
	}
	if doc.Step[0].Meta.Kind != StepKindDo || doc.Step[0].Name != "run" {
		t.Fatalf("unexpected lowered do step: %#v", doc.Step[0])
	}
	if doc.Step[1].Meta.Kind != StepKindSubmit || doc.Step[1].Name != "submit_run" {
		t.Fatalf("unexpected lowered submit step: %#v", doc.Step[1])
	}
	if len(doc.Meta.SourceComments) == 0 {
		t.Fatalf("expected projected source comments to be carried in document meta")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics during lowering: %s", diags.String())
	}
}

func TestToJUBEYAMLGlobalFallbacks(t *testing.T) {
	res := &sema.Result{
		Globals:            sema.GlobalState{Values: map[string]eval.Value{}},
		Program:            ast.Program{},
		Paramsets:          []*sema.Paramset{},
		ParamByName:        map[string]*sema.Paramset{},
		SubmitByName:       map[string]*sema.SubmitSpec{},
		StepImportByName:   map[string]*sema.StepImportPlan{},
		ImportSourceByName: map[string]*sema.ImportSource{},
		LetByName:          map[string]*sema.LetNamespace{},
	}
	doc := ToJUBEYAML(res, &diag.Diagnostics{})
	if doc.Name != "jbs_benchmark" || doc.Outpath != "out" || doc.Comment != "" {
		t.Fatalf("unexpected fallback global values: %#v", doc)
	}
}

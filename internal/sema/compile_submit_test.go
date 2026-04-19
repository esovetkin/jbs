package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func scalarBinding(name, varName string, value eval.Value, span diag.Span) *GlobalBinding {
	return &GlobalBinding{
		Name:    name,
		Value:   value,
		Shape:   BindingScalar,
		Order:   []string{varName},
		Vars:    map[string][]eval.Value{varName: {value}},
		Origins: map[string]diag.Span{varName: span},
		Span:    span,
	}
}

func TestSubmitHelpers(t *testing.T) {
	if got := sanitizeSubmitHelperPart(""); got != "x" {
		t.Fatalf("expected empty helper part to sanitize to x, got %q", got)
	}
	if got := sanitizeSubmitHelperPart("run-step.1"); got != "run_step_1" {
		t.Fatalf("unexpected sanitized helper part: %q", got)
	}
	if got := submitHelperAlias("run-step", "queue.name"); got != "_jk__run_step_queue_name" {
		t.Fatalf("unexpected submit helper alias: %q", got)
	}

	if !evalValueHasEmptyString(eval.String("")) {
		t.Fatalf("expected empty string value to count as empty")
	}
	if !evalValueHasEmptyString(eval.List([]eval.Value{})) {
		t.Fatalf("expected empty list to count as empty")
	}
	if !evalValueHasEmptyString(eval.Tuple([]eval.Value{eval.String(""), eval.String("")})) {
		t.Fatalf("expected tuple of empty strings to count as empty")
	}
	if evalValueHasEmptyString(eval.List([]eval.Value{eval.String(""), eval.Int(1)})) {
		t.Fatalf("did not expect mixed list to count as empty")
	}
	if submitValueHasEmptyString(SubmitValue{IsRaw: true, Raw: "echo hi"}) {
		t.Fatalf("raw submit values must never count as empty")
	}
	if !submitValueHasEmptyString(SubmitValue{Value: eval.String("")}) {
		t.Fatalf("expected empty submit value to count as empty")
	}

	span := diag.NewSpan("submit.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	missing, missingSpan := submitKeyMissingOrEmpty(map[string]SubmitValue{}, "queue", span)
	if !missing || missingSpan != span {
		t.Fatalf("expected missing submit key to fall back to provided span, got missing=%v span=%#v", missing, missingSpan)
	}
	empty, emptySpan := submitKeyMissingOrEmpty(map[string]SubmitValue{
		"queue": {Value: eval.String(""), Span: span},
	}, "queue", diag.Span{})
	if !empty || emptySpan != span {
		t.Fatalf("expected empty submit key to report its own span, got empty=%v span=%#v", empty, emptySpan)
	}

	if got, ok := submitDirectIdentifier(ast.ModeExpr{Mode: "python", Expr: ast.IdentExpr{Name: "nodes", Span: span}, Span: span}); !ok || got != "nodes" {
		t.Fatalf("expected direct identifier through mode wrapper, got %q ok=%v", got, ok)
	}
	if _, ok := submitDirectIdentifier(ast.BinaryExpr{Left: ast.IdentExpr{Name: "a", Span: span}, Op: "+", Right: ast.IdentExpr{Name: "b", Span: span}, Span: span}); ok {
		t.Fatalf("did not expect binary expression to be treated as direct identifier")
	}

	if rows, series := submitSeriesRowCount(eval.List([]eval.Value{eval.Int(1), eval.Int(2)})); !series || rows != 2 {
		t.Fatalf("expected list to report series row count, got rows=%d series=%v", rows, series)
	}
	if rows, series := submitSeriesRowCount(eval.Tuple([]eval.Value{eval.String("one")})); series || rows != 0 {
		t.Fatalf("expected single-element tuple not to report a series, got rows=%d series=%v", rows, series)
	}
	if !isRawSubmitKey("preprocess") || !isRawSubmitKey("postprocess") || isRawSubmitKey("queue") {
		t.Fatalf("unexpected raw-submit-key classification")
	}
}

func TestCompileSubmitBlockUsesBindingsAndReportsDiagnostics(t *testing.T) {
	span := diag.NewSpan("submit.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	bindings := map[string]*GlobalBinding{
		"defaults.nodes":   scalarBinding("defaults.nodes", "nodes", eval.Int(1), span),
		"defaults.helper":  scalarBinding("defaults.helper", "helper", eval.String("first"), span),
		"defaults.pre":     scalarBinding("defaults.pre", "preprocess", eval.String("ignored"), span),
		"defaults2.nodes":  scalarBinding("defaults2.nodes", "nodes", eval.Int(2), span),
		"defaults2.helper": scalarBinding("defaults2.helper", "helper", eval.String("second"), span),
		"table": {
			Name:    "table",
			Shape:   BindingTable,
			Order:   []string{"x"},
			Vars:    map[string][]eval.Value{"x": {eval.Int(1), eval.Int(2)}},
			Origins: map[string]diag.Span{"x": span},
			Span:    span,
		},
		"seriesSource": {
			Name:    "seriesSource",
			Shape:   BindingScalar,
			Order:   []string{"series"},
			Vars:    map[string][]eval.Value{"series": {eval.Int(4), eval.Int(8)}},
			Origins: map[string]diag.Span{"series": span},
			Span:    span,
		},
	}
	effective := map[string]VisibleBinding{
		"series": {Name: "series", Source: "seriesSource", Span: span},
	}
	namespaces := map[string]*Namespace{
		"defaults": {
			Name:     "defaults",
			Bindings: []string{"defaults.nodes", "defaults.helper", "defaults.pre", "defaults.other.deep"},
		},
		"defaults2": {
			Name:     "defaults2",
			Bindings: []string{"defaults2.nodes", "defaults2.helper"},
		},
	}
	block := ast.SubmitBlock{
		Name:     "submit-step",
		UseNames: []string{"defaults", "defaults2", "missing", "table"},
		Fields: []ast.SubmitField{
			{Name: "account", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "", Span: span}, Span: span},
			{Name: "starter", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "srun", Span: span}, Span: span},
			{Name: "nodes", Op: ast.AssignEq, Expr: ast.IdentExpr{Name: "series", Span: span}, Span: span},
			{Name: "gres", Op: ast.AssignEq, Expr: ast.ListExpr{Items: []ast.Expr{ast.ListExpr{Items: []ast.Expr{ast.StringExpr{Value: "gpu", Span: span}}, Span: span}}, Span: span}, Span: span},
			{Name: "preprocess", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "echo hi", Span: span}, Span: span},
			{Name: "queue", IsRaw: true, Raw: "echo bad", Span: span},
			{Name: "queue", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "late", Span: span}, Span: span},
			{Name: "executable", Op: ast.AssignEq, Expr: nil, Span: span},
			{Name: "unknown_key", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "x", Span: span}, Span: span},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	spec := compileSubmitBlock(block, bindings, map[string]eval.Value{}, effective, namespaces, nil, diags)
	if spec == nil {
		t.Fatalf("expected submit spec")
	}

	if countDiagCode(diags, "E078") != 1 {
		t.Fatalf("expected one unknown submit-use source diagnostic, got %d: %s", countDiagCode(diags, "E078"), diags.String())
	}
	if countDiagCode(diags, "E071") != 1 {
		t.Fatalf("expected one non-scalar submit-use diagnostic, got %d: %s", countDiagCode(diags, "E071"), diags.String())
	}
	if countDiagCode(diags, "E072") != 1 {
		t.Fatalf("expected one unknown submit-key diagnostic, got %d: %s", countDiagCode(diags, "E072"), diags.String())
	}
	if countDiagCode(diags, "E073") != 1 {
		t.Fatalf("expected one raw-required diagnostic, got %d: %s", countDiagCode(diags, "E073"), diags.String())
	}
	if countDiagCode(diags, "E074") != 1 {
		t.Fatalf("expected one raw-not-allowed diagnostic, got %d: %s", countDiagCode(diags, "E074"), diags.String())
	}
	if countDiagCode(diags, "E075") != 1 {
		t.Fatalf("expected one duplicate submit-key diagnostic, got %d: %s", countDiagCode(diags, "E075"), diags.String())
	}
	if countDiagCode(diags, "E076") != 1 {
		t.Fatalf("expected one missing-expression diagnostic, got %d: %s", countDiagCode(diags, "E076"), diags.String())
	}
	if countDiagCode(diags, "E305") != 1 {
		t.Fatalf("expected one nested-list diagnostic, got %d: %s", countDiagCode(diags, "E305"), diags.String())
	}
	if countDiagCode(diags, "W071") != 1 {
		t.Fatalf("expected one ignored raw-key default warning, got %d: %s", countDiagCode(diags, "W071"), diags.String())
	}
	if countDiagCode(diags, "W072") != 2 {
		t.Fatalf("expected two duplicate-default warnings, got %d: %s", countDiagCode(diags, "W072"), diags.String())
	}
	if countDiagCode(diags, "W073") != 2 {
		t.Fatalf("expected two missing-or-empty submit-key warnings, got %d: %s", countDiagCode(diags, "W073"), diags.String())
	}
	if countDiagCode(diags, "W074") != 1 {
		t.Fatalf("expected one executable/args warning, got %d: %s", countDiagCode(diags, "W074"), diags.String())
	}
	if countDiagCode(diags, "W075") != 1 {
		t.Fatalf("expected one series-assignment warning, got %d: %s", countDiagCode(diags, "W075"), diags.String())
	}

	if len(spec.Helpers) != 1 {
		t.Fatalf("expected one helper after duplicate helper override, got %#v", spec.Helpers)
	}
	if spec.Helpers[0].Original != "helper" || spec.Helpers[0].UseName != "defaults2" || spec.Helpers[0].Aliased == "" {
		t.Fatalf("unexpected helper metadata: %#v", spec.Helpers[0])
	}
	if !strings.HasPrefix(spec.Helpers[0].Aliased, "_jk__submit_step_helper") {
		t.Fatalf("expected sanitized helper alias prefix, got %q", spec.Helpers[0].Aliased)
	}

	resolved := make(map[string]SubmitValue, len(spec.Values))
	for _, value := range spec.Values {
		resolved[value.Name] = value
	}
	if !eval.Equal(resolved["nodes"].Value, eval.List([]eval.Value{eval.Int(4), eval.Int(8)})) {
		t.Fatalf("expected nodes to use effective series value, got %#v", resolved["nodes"])
	}
	if !eval.Equal(resolved["tasks"].Value, eval.List([]eval.Value{eval.Int(4), eval.Int(8)})) {
		t.Fatalf("expected tasks to be injected from nodes, got %#v", resolved["tasks"])
	}
	if !eval.Equal(resolved["account"].Value, eval.String("")) {
		t.Fatalf("expected explicit empty account value to be preserved, got %#v", resolved["account"])
	}
	if !eval.Equal(resolved["starter"].Value, eval.String("srun")) {
		t.Fatalf("expected starter value to be preserved, got %#v", resolved["starter"])
	}
}

func TestCompileSubmitBlockSupportsNamesBuiltin(t *testing.T) {
	span := diag.NewSpan("submit.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	bindings := map[string]*GlobalBinding{
		"stepSource": scalarBinding("stepSource", "stepv", eval.Int(1), span),
	}
	effective := map[string]VisibleBinding{
		"stepv": {Name: "stepv", Source: "stepSource", Span: span},
	}
	namespaces := map[string]*Namespace{
		"defaults": {
			Name:     "defaults",
			Bindings: []string{"defaults.alpha", "defaults.beta", "defaults.child.gamma"},
		},
	}
	block := ast.SubmitBlock{
		Name: "submit-step",
		Fields: []ast.SubmitField{
			{
				Name: "nodes",
				Op:   ast.AssignEq,
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "len", Span: span},
					Args: []ast.Expr{
						ast.CallExpr{Callee: ast.IdentExpr{Name: "names", Span: span}, Span: span},
					},
					Span: span,
				},
				Span: span,
			},
			{
				Name: "tasks",
				Op:   ast.AssignEq,
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "len", Span: span},
					Args: []ast.Expr{
						ast.CallExpr{
							Callee: ast.IdentExpr{Name: "names", Span: span},
							Args:   []ast.Expr{ast.IdentExpr{Name: "defaults", Span: span}},
							Span:   span,
						},
					},
					Span: span,
				},
				Span: span,
			},
			{Name: "account", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "a", Span: span}, Span: span},
			{Name: "queue", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "q", Span: span}, Span: span},
			{Name: "starter", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "srun", Span: span}, Span: span},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	spec := compileSubmitBlock(block, bindings, map[string]eval.Value{"visible": eval.Int(2)}, effective, namespaces, nil, diags)
	if spec == nil {
		t.Fatalf("expected submit spec")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	resolved := make(map[string]SubmitValue, len(spec.Values))
	for _, value := range spec.Values {
		resolved[value.Name] = value
	}
	if !eval.Equal(resolved["nodes"].Value, eval.Int(2)) {
		t.Fatalf("expected names() to count visible submit values, got %#v", resolved["nodes"])
	}
	if !eval.Equal(resolved["tasks"].Value, eval.Int(2)) {
		t.Fatalf("expected names(defaults) to count direct namespace members, got %#v", resolved["tasks"])
	}
}

func TestCompileSubmitBlockSupportsReadCSVBuiltin(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "cases.csv"), []byte("x,y\n1,2\n3,4\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	submitFile := filepath.Join(cwd, "submit.jbs")
	span := diag.NewSpan(submitFile, diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	block := ast.SubmitBlock{
		Name: "submit-step",
		Fields: []ast.SubmitField{
			{
				Name: "nodes",
				Op:   ast.AssignEq,
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "len", Span: span},
					Args: []ast.Expr{
						ast.CallExpr{
							Callee: ast.IdentExpr{Name: "read_csv", Span: span},
							Args:   []ast.Expr{ast.StringExpr{Value: "./cases.csv", Span: span}},
							Span:   span,
						},
					},
					Span: span,
				},
				Span: span,
			},
			{Name: "account", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "a", Span: span}, Span: span},
			{Name: "queue", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "q", Span: span}, Span: span},
			{Name: "starter", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "srun", Span: span}, Span: span},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	spec := compileSubmitBlock(block, map[string]*GlobalBinding{}, map[string]eval.Value{}, map[string]VisibleBinding{}, map[string]*Namespace{}, map[string]string{submitFile: cwd}, diags)
	if spec == nil {
		t.Fatalf("expected submit spec")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	resolved := make(map[string]SubmitValue, len(spec.Values))
	for _, value := range spec.Values {
		resolved[value.Name] = value
	}
	if !eval.Equal(resolved["nodes"].Value, eval.Int(2)) {
		t.Fatalf("expected nodes=len(read_csv(...)) == 2, got %#v", resolved["nodes"])
	}
	if !eval.Equal(resolved["tasks"].Value, eval.Int(2)) {
		t.Fatalf("expected injected tasks from nodes, got %#v", resolved["tasks"])
	}
}

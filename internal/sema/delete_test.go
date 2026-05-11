package sema

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func analyzeDeleteSource(src string) (*Result, *diag.Diagnostics) {
	diags := &diag.Diagnostics{}
	prog := parser.Parse("delete_test.jbs", src, diags)
	res := Analyze(prog, BuiltinGlobalValues(), diags)
	return res, diags
}

func TestAnalyzeDeleteRemovesTopLevelGlobal(t *testing.T) {
	res, diags := analyzeDeleteSource(strings.Join([]string{
		"x = 1",
		"delete(x)",
		"visible = names()",
	}, "\n"))
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if _, ok := res.Globals.Values["x"]; ok {
		t.Fatalf("deleted global x remains in Globals.Values")
	}
	if _, ok := res.GlobalVarByName["x"]; ok {
		t.Fatalf("deleted global x remains in GlobalVarByName")
	}
	if _, ok := res.BindingsByName["x"]; ok {
		t.Fatalf("deleted global x remains in BindingsByName")
	}
	visible := res.GlobalVarByName["visible"]
	if visible == nil {
		t.Fatalf("missing visible global")
	}
	if listContainsString(visible.Value, "x") {
		t.Fatalf("deleted global x is still visible through names(): %#v", visible.Value)
	}
}

func TestAnalyzeDeleteRejectsProtectedAndMissingNames(t *testing.T) {
	_, diags := analyzeDeleteSource(strings.Join([]string{
		"delete(jbs_name)",
		"delete(range)",
		"delete(missing)",
	}, "\n"))
	text := diags.String()
	if countDiagCode(diags, "E106") < 2 {
		t.Fatalf("expected protected global and builtin errors, got: %s", text)
	}
	if countDiagCode(diags, "E100") != 1 {
		t.Fatalf("expected missing-variable error, got: %s", text)
	}
	if !strings.Contains(text, "cannot delete global variable 'jbs_name'") {
		t.Fatalf("missing protected global diagnostic: %s", text)
	}
	if !strings.Contains(text, "cannot delete built-in function 'range'") {
		t.Fatalf("missing protected builtin diagnostic: %s", text)
	}
	if !strings.Contains(text, "unknown variable 'missing'") {
		t.Fatalf("missing unknown variable diagnostic: %s", text)
	}
}

func TestAnalyzeDeleteAffectsSnapshotsAndImports(t *testing.T) {
	_, diags := analyzeDeleteSource(strings.Join([]string{
		"x = 1",
		"delete(x)",
		"do s with x {",
		"echo \"$x\"",
		"}",
	}, "\n"))
	if !diags.HasErrors() {
		t.Fatalf("expected deleted import source to fail")
	}
	if !strings.Contains(diags.String(), "x") {
		t.Fatalf("expected diagnostic to mention x, got: %s", diags.String())
	}
}

func TestDeleteCallTargetsDoNotCountAsReads(t *testing.T) {
	expr := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "delete"},
		Args:   ast.PosCallArgs(ast.IdentExpr{Name: "x"}),
	}
	for _, ref := range globalExprReadRefs(expr) {
		if ref.Name == "x" || ref.SeedAlt == "x" {
			t.Fatalf("delete target counted as global read: %#v", ref)
		}
	}
	for _, dep := range globalExprDependencies(expr, "") {
		if dep == "x" {
			t.Fatalf("delete target counted as global dependency: %#v", dep)
		}
	}
	locals := map[string]struct{}{}
	collectExprLocalIdentDeps(expr, locals)
	if _, ok := locals["x"]; ok {
		t.Fatalf("delete target counted as local dependency: %#v", locals)
	}
	for _, ref := range collectExprIdentRefs(expr) {
		if ref.Name == "x" {
			t.Fatalf("delete target counted as identifier reference: %#v", ref)
		}
	}
}

func listContainsString(value eval.Value, target string) bool {
	if value.Kind != eval.KindList {
		return false
	}
	for _, item := range value.L {
		if item.Kind == eval.KindString && item.S == target {
			return true
		}
	}
	return false
}

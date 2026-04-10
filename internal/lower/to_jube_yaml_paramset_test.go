package lower_test

import (
	"strings"
	"testing"

	"jbs/internal/lower"
)

func TestPureOuterProductLowering(t *testing.T) {
	src := `
param p {
  a = ("a","b","c")
  b = ("1","2")
  c = (1,2)
  a * b * c
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.ParameterSet) != 1 {
		t.Fatalf("expected one parameterset")
	}
	ps := doc.ParameterSet[0]
	if ps.Name != "p" {
		t.Fatalf("unexpected parameterset name: %s", ps.Name)
	}
	if len(ps.Parameter) != 4 {
		t.Fatalf("expected i + 3 params, got %d", len(ps.Parameter))
	}
	if ps.Parameter[0].Name != "_ji_p" || ps.Parameter[0].Type != "int" || ps.Parameter[0].Mode != "text" {
		t.Fatalf("expected indexed lowering via context index parameter, got %#v", ps.Parameter[0])
	}
	if ps.Parameter[0].Value != "0,1,2,3,4,5,6,7,8,9,10,11" {
		t.Fatalf("unexpected i range: %#v", ps.Parameter[0].Value)
	}
	for _, p := range ps.Parameter[1:] {
		if p.Mode != "python" {
			t.Fatalf("expected python mode payload parameter, got %#v", p)
		}
	}
}

func TestGroupedDirectSumLowering(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y","z")
  a + b
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	ps := doc.ParameterSet[0]
	if ps.Parameter[0].Name != "_ji_p" {
		t.Fatalf("expected grouped index parameter _ji_p, got %s", ps.Parameter[0].Name)
	}
	if ps.Parameter[0].Value != "0,1,2" {
		t.Fatalf("expected grouped indices 0,1,2 got %#v", ps.Parameter[0].Value)
	}
	if ps.Parameter[1].Mode != "python" || ps.Parameter[2].Mode != "python" {
		t.Fatalf("expected python-indexed grouped payload parameters")
	}
}

func TestSubsetSingleVarUsesIndexMask(t *testing.T) {
	src := `
param param {
  a = ("a","b","c")
  b = ("1","2")
  c = (1,2)
  a * b + c
}

do task with a from param {
  echo ${a}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		if strings.HasPrefix(doc.ParameterSet[i].Name, "_js__task__param__a") {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected subset parameterset for a import")
	}
	if len(subset.Parameter) != 3 {
		t.Fatalf("expected i + rows + a in subset, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "_ji__task__param__a" || subset.Parameter[0].Value != "0,2,4" {
		t.Fatalf("expected masked context index for subset, got %#v", subset.Parameter[0])
	}
	if subset.Parameter[1].Name != "_jr__task__param__a" || subset.Parameter[1].Mode != "python" {
		t.Fatalf("expected python rows context param, got %#v", subset.Parameter[1])
	}
	if subset.Parameter[1].Separator != "####" {
		t.Fatalf("expected rows context separator ####, got %#v", subset.Parameter[1].Separator)
	}
	rowsExpr, ok := subset.Parameter[1].Value.(lower.SingleQuoted)
	if !ok {
		t.Fatalf("expected single-quoted rows lookup expression, got %T", subset.Parameter[1].Value)
	}
	if string(rowsExpr) != "{\"0\":\"0,1\",\"2\":\"2,3\",\"4\":\"4,5\"}[\"${_ji__task__param__a}\"]" {
		t.Fatalf("unexpected rows context payload: %q", string(rowsExpr))
	}
	if subset.Parameter[2].Name != "a" || subset.Parameter[2].Mode != "python" {
		t.Fatalf("expected python indexed a param, got %#v", subset.Parameter[2])
	}
	gotExpr, ok := subset.Parameter[2].Value.(lower.SingleQuoted)
	if !ok {
		t.Fatalf("expected single-quoted python expression, got %T", subset.Parameter[2].Value)
	}
	if string(gotExpr) != "[\"a\",\"a\",\"b\",\"b\",\"c\",\"c\"][$_ji__task__param__a]" {
		t.Fatalf("unexpected subset a expression: %q", string(gotExpr))
	}
}

func TestSubsetTupleFromSameParam(t *testing.T) {
	src := `
param param {
  a = ("a","b","c")
  b = ("1","2")
  c = (1,2)
  a * b + c
}

do task with (a,b) from param {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		if strings.HasPrefix(doc.ParameterSet[i].Name, "_js__task__param__a_b") {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected subset parameterset for (a,b) import")
	}
	if len(subset.Parameter) != 4 {
		t.Fatalf("expected i + rows + a + b in subset, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "_ji__task__param__a_b" || subset.Parameter[0].Value != "0,1,2,3,4,5" {
		t.Fatalf("unexpected tuple subset i mask: %#v", subset.Parameter[0])
	}
	if subset.Parameter[1].Name != "_jr__task__param__a_b" || subset.Parameter[1].Mode != "python" {
		t.Fatalf("expected rows helper for tuple subset, got %#v", subset.Parameter[1])
	}
	if subset.Parameter[1].Separator != "####" {
		t.Fatalf("expected tuple rows helper separator ####, got %#v", subset.Parameter[1].Separator)
	}
	tupleRowsExpr, okTupleRows := subset.Parameter[1].Value.(lower.SingleQuoted)
	if !okTupleRows {
		t.Fatalf("expected single-quoted tuple rows lookup expression, got %T", subset.Parameter[1].Value)
	}
	if string(tupleRowsExpr) != "{\"0\":\"0\",\"1\":\"1\",\"2\":\"2\",\"3\":\"3\",\"4\":\"4\",\"5\":\"5\"}[\"${_ji__task__param__a_b}\"]" {
		t.Fatalf("unexpected tuple rows helper payload: %q", string(tupleRowsExpr))
	}
	if subset.Parameter[2].Mode != "python" || subset.Parameter[3].Mode != "python" {
		t.Fatalf("expected python indexed tuple subset params, got %#v", subset.Parameter)
	}
	aExpr, okA := subset.Parameter[2].Value.(lower.SingleQuoted)
	bExpr, okB := subset.Parameter[3].Value.(lower.SingleQuoted)
	if !okA || !okB {
		t.Fatalf("expected single-quoted tuple expressions, got %T %T", subset.Parameter[2].Value, subset.Parameter[3].Value)
	}
	if string(aExpr) != "[\"a\",\"a\",\"b\",\"b\",\"c\",\"c\"][$_ji__task__param__a_b]" {
		t.Fatalf("unexpected a expression: %q", string(aExpr))
	}
	if string(bExpr) != "[\"1\",\"2\",\"1\",\"2\",\"1\",\"2\"][$_ji__task__param__a_b]" {
		t.Fatalf("unexpected b expression: %q", string(bExpr))
	}
}

func TestMixedWithVariableAndWholeParamsetLowering(t *testing.T) {
	src := `
param p1 {
  a = (1,2)
  a
}
param p2 {
  b = ("x","y")
  b
}
do setup with a from p1, p2 {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one step")
	}
	use := doc.Step[0].Use
	hasP2 := false
	hasSubsetP1 := false
	for _, u := range use {
		if s, ok := u.(string); ok {
			if s == "p2" {
				hasP2 = true
			}
			if strings.HasPrefix(s, "_js__setup__p1__") {
				hasSubsetP1 = true
			}
		}
	}
	if !hasP2 {
		t.Fatalf("expected direct import of full parameterset p2 in step use list: %#v", use)
	}
	if !hasSubsetP1 {
		t.Fatalf("expected subset import for a from p1 in step use list: %#v", use)
	}
}

func TestMixedWithTupleVariableAndWholeParamsetLowering(t *testing.T) {
	src := `
param p1 {
  a = (1,2)
  b = ("m","n")
  a * b
}
param p2 {
  c = ("x","y")
  c
}
do setup with (a,b) from p1, p2 {
  echo ${a} ${b} ${c}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one step")
	}
	use := doc.Step[0].Use
	hasP2 := false
	hasSubsetP1 := false
	for _, u := range use {
		if s, ok := u.(string); ok {
			if s == "p2" {
				hasP2 = true
			}
			if strings.HasPrefix(s, "_js__setup__p1__") {
				hasSubsetP1 = true
			}
		}
	}
	if !hasP2 {
		t.Fatalf("expected direct import of full parameterset p2 in step use list: %#v", use)
	}
	if !hasSubsetP1 {
		t.Fatalf("expected subset import for (a,b) from p1 in step use list: %#v", use)
	}
}

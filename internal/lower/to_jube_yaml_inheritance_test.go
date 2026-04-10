package lower_test

import (
	"strings"
	"testing"

	"jbs/internal/lower"
)

func TestAfterInheritanceWithParamsetUsesOnlyDeltaSubset(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with p {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var hasSubsetB bool
	for _, ps := range doc.ParameterSet {
		if strings.Contains(ps.Name, "_js__s1__p__b") {
			hasSubsetB = true
			break
		}
	}
	if !hasSubsetB {
		t.Fatalf("expected synthetic subset for non-inherited delta variable b, got %#v", doc.ParameterSet)
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected two steps, got %d", len(doc.Step))
	}
	use := doc.Step[1].Use
	hasSubset := false
	hasWhole := false
	for _, u := range use {
		s, ok := u.(string)
		if !ok {
			continue
		}
		if s == "p" {
			hasWhole = true
		}
		if strings.Contains(s, "_js__s1__p__b") {
			hasSubset = true
		}
	}
	if hasWhole {
		t.Fatalf("did not expect full paramset import p for inherited+delta case: %#v", use)
	}
	if !hasSubset {
		t.Fatalf("expected subset import for only variable b, got %#v", use)
	}
}

func TestAfterInheritanceWithOnlyInheritedVarsHasNoUseEntries(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with a from p {
  echo ${a}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected two steps, got %d", len(doc.Step))
	}
	if got := len(doc.Step[1].Use); got != 0 {
		t.Fatalf("expected no explicit use entries for fully inherited imports, got %#v", doc.Step[1].Use)
	}
}

func TestAfterInheritanceSubsetUsesInheritedRowsContext(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with b from p {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		if doc.ParameterSet[i].Name == "_js__s1__p__b" {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected contextual subset for step s1")
	}
	if len(subset.Parameter) != 3 {
		t.Fatalf("expected contextual subset params i + rows + b, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "_ji__s1__p__b" || subset.Parameter[0].Separator != "," {
		t.Fatalf("expected contextual index with separator ',', got %#v", subset.Parameter[0])
	}
	if subset.Parameter[0].Value != "$_jr__s0__p__a" {
		t.Fatalf("expected inherited rows context reference, got %#v", subset.Parameter[0].Value)
	}
	if subset.Parameter[1].Name != "_jr__s1__p__b" || subset.Parameter[1].Mode != "text" {
		t.Fatalf("expected contextual rows helper, got %#v", subset.Parameter[1])
	}
	if subset.Parameter[1].Value != "${_ji__s1__p__b}" {
		t.Fatalf("expected contextual rows helper to mirror idx, got %#v", subset.Parameter[1].Value)
	}
}

func TestAfterInheritanceTransitiveContextPropagation(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y","z")
  c = ("u","v","w")
  a * (b + c)
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with b from p {
  echo ${a} ${b}
}
do s2 after s1 with c from p {
  echo ${a} ${b} ${c}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var s1Subset, s2Subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		switch doc.ParameterSet[i].Name {
		case "_js__s1__p__b":
			s1Subset = &doc.ParameterSet[i]
		case "_js__s2__p__c":
			s2Subset = &doc.ParameterSet[i]
		}
	}
	if s1Subset == nil || s2Subset == nil {
		t.Fatalf("expected contextual subsets for s1 and s2, got %#v", doc.ParameterSet)
	}
	if s1Subset.Parameter[0].Value != "$_jr__s0__p__a" {
		t.Fatalf("expected s1 contextual source from s0 rows helper, got %#v", s1Subset.Parameter[0].Value)
	}
	if s2Subset.Parameter[0].Value != "$_jr__s1__p__b" {
		t.Fatalf("expected s2 contextual source from s1 rows helper, got %#v", s2Subset.Parameter[0].Value)
	}
}

func TestAfterInheritanceConflictingRowContextsRaiseError(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  c = ("u","v")
  a + b + c
}
do s0 with a from p {
  echo ${a}
}
do s1 with b from p {
  echo ${b}
}
do s2 after s0,s1 with c from p {
  echo ${a} ${b} ${c}
}
`
	_, diags := compileDoc(t, src)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E232" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E232 for conflicting inherited row contexts, got: %s", diags.String())
	}
}

package workplan

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestBuildPreservesImmediateDependenciesOnly(t *testing.T) {
	src := `
cases = table(x=[1, 2])

do step1 with cases {
echo "$x"
}

do step2 after step1 {
echo step2
}

do step3 after step2 {
echo step3
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("x.jbs", src, diags)
	res := sema.Analyze(prog, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	plan := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected workplan diagnostics: %s", diags.String())
	}
	var step3 []WorkPackage
	for _, work := range plan.Work {
		if work.StepName == "step3" {
			step3 = append(step3, work)
		}
	}
	if len(step3) != 2 {
		t.Fatalf("expected two step3 workpackages, got %d", len(step3))
	}
	for _, work := range step3 {
		if len(work.Deps) != 1 {
			t.Fatalf("expected one direct dependency for %#v, got %#v", work.ID, work.Deps)
		}
		if work.Deps[0].Step != "step2" {
			t.Fatalf("expected step3 to depend only on step2, got %#v", work.Deps)
		}
	}
}

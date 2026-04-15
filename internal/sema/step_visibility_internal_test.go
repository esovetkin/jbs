package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestResolveImportedVars(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 10))
	sources := map[string]*ImportSource{
		"p": {
			Name:  "p",
			Kind:  SourceKindParam,
			Order: []string{"a", "b"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1)},
				"b": {eval.Int(2)},
			},
		},
		"x": {
			Name:  "x",
			Kind:  SourceKindParam,
			Order: []string{"v"},
			Vars: map[string][]eval.Value{
				"v": {eval.String("fallback")},
			},
		},
	}

	items := []ast.WithItem{
		{Name: "p", Span: span},                         // full source import
		{Name: "p", Span: span},                         // dedupe full import
		{Name: "a", From: "p", Span: span},              // single var import
		{Name: "a", From: "p", Alias: "aa", Span: span}, // alias var import
		{Name: "x", From: "p", Span: span},              // fallback to source named x
		{Name: "no_fallback", From: "p", Span: span},    // unknown var without fallback
		{Name: "z", From: "missing", Span: span},        // unknown source
		{Name: "missing_full", Span: span},              // unknown full source
	}

	got := resolveImportedVars(items, sources)

	if _, ok := got["a"]; !ok {
		t.Fatalf("expected imported key 'a' from full/explicit import, got %#v", got)
	}
	if _, ok := got["b"]; !ok {
		t.Fatalf("expected imported key 'b' from full import, got %#v", got)
	}
	if _, ok := got["aa"]; !ok {
		t.Fatalf("expected imported alias key 'aa', got %#v", got)
	}
	if _, ok := got["v"]; !ok {
		t.Fatalf("expected fallback import key 'v' from source 'x', got %#v", got)
	}
	if _, ok := got["no_fallback"]; ok {
		t.Fatalf("did not expect unknown variable without fallback to be imported, got %#v", got["no_fallback"])
	}
	if _, ok := got["z"]; ok {
		t.Fatalf("did not expect unknown source variable to be imported, got %#v", got["z"])
	}
	if len(got["a"]) != 1 {
		t.Fatalf("expected dedup for repeated full/single import of 'a', got %#v", got["a"])
	}
}

func TestStepVisibleVariables(t *testing.T) {
	originA := diag.NewSpan("param.jbs", diag.NewPos(0, 2, 1), diag.NewPos(0, 2, 5))
	itemSpanB := diag.NewSpan("step.jbs", diag.NewPos(0, 6, 1), diag.NewPos(0, 6, 10))
	itemSpan := diag.NewSpan("step.jbs", diag.NewPos(0, 5, 1), diag.NewPos(0, 5, 10))

	// key "alias" points to source named "real" intentionally to hit src==nil branch
	sources := map[string]*ImportSource{
		"p": {
			Name: "p",
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1)},
				"b": {eval.Int(2)},
			},
			Origins: map[string]diag.Span{
				"a": originA,
			},
		},
		"alias": {
			Name: "real",
			Vars: map[string][]eval.Value{
				"w": {eval.String("x")},
			},
			Origins: map[string]diag.Span{
				"w": diag.NewSpan("param.jbs", diag.NewPos(0, 3, 1), diag.NewPos(0, 3, 5)),
			},
		},
		"e": {
			Name: "e",
			Vars: map[string][]eval.Value{
				"": {eval.String("empty")},
			},
			Order: []string{""},
			Origins: map[string]diag.Span{
				"": itemSpanB,
			},
		},
	}
	items := []ast.WithItem{
		{Name: "a", From: "p", Alias: "aa", Span: itemSpan},
		{Name: "b", From: "p", Span: itemSpanB},
		{Name: "alias", Span: itemSpan},
		{Name: "e", Span: itemSpan},
	}

	imports := resolveImportedVars(items, sources)
	plan := &StepImportPlan{Effective: map[string]VarOrigin{}}
	for name, origins := range imports {
		if len(origins) == 0 {
			continue
		}
		origin := origins[0]
		plan.Effective[name] = VarOrigin{
			Name:      name,
			SourceVar: origin.SourceVar,
			Paramset:  origin.Paramset,
			Kind:      origin.Kind,
			Span:      origin.Span,
		}
	}

	got := visibleSpansFromStepPlan(plan, sources)
	if got["aa"] != originA {
		t.Fatalf("expected visible alias span from source origin, got %#v", got["aa"])
	}
	if got["b"] != itemSpanB {
		t.Fatalf("expected fallback to with-item span when source origin is missing, got %#v", got["b"])
	}
	if got["w"] != itemSpan {
		t.Fatalf("expected fallback to with-item span when source lookup by origin.Paramset fails, got %#v", got["w"])
	}
	if got[""] != itemSpanB {
		t.Fatalf("expected empty source-var name to map via source origin lookup, got %#v", got[""])
	}
}

func TestStepVisibleVariablesFromPlan(t *testing.T) {
	originX := diag.NewSpan("param.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	originY := diag.NewSpan("param.jbs", diag.NewPos(0, 2, 1), diag.NewPos(0, 2, 2))
	fallbackSpan := diag.NewSpan("step.jbs", diag.NewPos(0, 7, 1), diag.NewPos(0, 7, 4))
	plan := &StepImportPlan{
		Effective: map[string]VarOrigin{
			"x": {Paramset: "p", SourceVar: "", Span: fallbackSpan},
			"y": {Paramset: "p", SourceVar: "srcY", Span: fallbackSpan},
			"z": {Paramset: "missing", Span: fallbackSpan},
			"w": {Paramset: "p", SourceVar: "missing", Span: fallbackSpan},
		},
	}
	sources := map[string]*ImportSource{
		"p": {
			Name: "p",
			Origins: map[string]diag.Span{
				"x":    originX,
				"srcY": originY,
			},
		},
	}

	got := visibleSpansFromStepPlan(plan, sources)
	if got["x"] != originX {
		t.Fatalf("expected name-fallback source var span for x, got %#v", got["x"])
	}
	if got["y"] != originY {
		t.Fatalf("expected explicit source var span for y, got %#v", got["y"])
	}
	if got["z"] != fallbackSpan {
		t.Fatalf("expected fallback origin span for missing source, got %#v", got["z"])
	}
	if got["w"] != fallbackSpan {
		t.Fatalf("expected fallback origin span for missing source-var origin, got %#v", got["w"])
	}
}

func TestAddStepValuesToEnvFromPlan(t *testing.T) {
	env := map[string]eval.Value{"pre": eval.String("keep")}
	addEnvFromStepPlan(env, nil, nil)
	if env["pre"].S != "keep" {
		t.Fatalf("nil plan must not mutate env, got %#v", env)
	}

	plan := &StepImportPlan{
		Effective: map[string]VarOrigin{
			"a": {Paramset: "p", SourceVar: ""},
			"b": {Paramset: "p", SourceVar: "srcB"},
			"c": {Paramset: "p", SourceVar: "missing"},
			"d": {Paramset: "missing", SourceVar: "d"},
		},
	}
	sources := map[string]*ImportSource{
		"p": {
			Name: "p",
			Vars: map[string][]eval.Value{
				"a":    {eval.Int(1), eval.Int(2)},
				"srcB": {eval.String("one")},
			},
		},
	}
	addEnvFromStepPlan(env, plan, sources)

	wantA := eval.List([]eval.Value{eval.Int(1), eval.Int(2)})
	if !eval.Equal(env["a"], wantA) {
		t.Fatalf("expected list series value for a, got %#v", env["a"])
	}
	if !eval.Equal(env["b"], eval.String("one")) {
		t.Fatalf("expected scalar series value for b, got %#v", env["b"])
	}
	if _, ok := env["c"]; ok {
		t.Fatalf("did not expect missing source var to be added, got %#v", env["c"])
	}
	if _, ok := env["d"]; ok {
		t.Fatalf("did not expect missing source to be added, got %#v", env["d"])
	}
}

func TestAddStepValuesToEnvFromWithItems(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 8))
	sources := map[string]*ImportSource{
		"p": {
			Name:  "p",
			Order: []string{"a", "b"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1), eval.Int(2)},
				"b": {eval.String("x")},
			},
		},
		"x": {
			Name:  "x",
			Order: []string{"q"},
			Vars: map[string][]eval.Value{
				"q": {eval.Bool(true)},
			},
		},
		"alias": {
			Name:  "real",
			Order: []string{"r"},
			Vars: map[string][]eval.Value{
				"r": {eval.Int(9)},
			},
		},
		"e": {
			Name:  "e",
			Order: []string{""},
			Vars: map[string][]eval.Value{
				"": {eval.Int(7)},
			},
		},
	}
	items := []ast.WithItem{
		{Name: "p", Span: span},
		{Name: "a", From: "p", Alias: "aa", Span: span},
		{Name: "x", From: "p", Span: span}, // fallback to source x
		{Name: "alias", Span: span},        // src key/name mismatch => src nil at env injection stage
		{Name: "e", Span: span},            // empty source var name => sourceVar=="" path
		{Name: "missing", From: "p", Span: span},
		{Name: "z", From: "unknown", Span: span},
	}
	env := map[string]eval.Value{}
	imports := resolveImportedVars(items, sources)
	plan := &StepImportPlan{Effective: map[string]VarOrigin{}}
	for name, origins := range imports {
		if len(origins) == 0 {
			continue
		}
		origin := origins[0]
		plan.Effective[name] = VarOrigin{
			Name:      name,
			SourceVar: origin.SourceVar,
			Paramset:  origin.Paramset,
			Kind:      origin.Kind,
			Span:      origin.Span,
		}
	}
	addEnvFromStepPlan(env, plan, sources)

	if !eval.Equal(env["a"], eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("expected a from full import, got %#v", env["a"])
	}
	if !eval.Equal(env["b"], eval.String("x")) {
		t.Fatalf("expected b from full import, got %#v", env["b"])
	}
	if !eval.Equal(env["aa"], eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("expected aliased aa from explicit import, got %#v", env["aa"])
	}
	if !eval.Equal(env["q"], eval.Bool(true)) {
		t.Fatalf("expected fallback-imported q from source x, got %#v", env["q"])
	}
	if !eval.Equal(env[""], eval.Int(7)) {
		t.Fatalf("expected empty-name import value from source e, got %#v", env[""])
	}
	if _, ok := env["r"]; ok {
		t.Fatalf("did not expect key-name mismatched source alias to be resolved in env, got %#v", env["r"])
	}
	if _, ok := env["missing"]; ok {
		t.Fatalf("did not expect unresolved variable entry, got %#v", env["missing"])
	}
}

func TestResolveImportedVarsFromPlan(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 5))
	plan := &StepImportPlan{
		Effective: map[string]VarOrigin{
			"a": {SourceVar: "srcA", Paramset: "p", Kind: SourceKindParam, Span: span},
			"b": {SourceVar: "srcB", Paramset: "l", Kind: SourceKindLet, Span: span},
		},
	}
	got := importsFromStepPlan(plan)
	if len(got) != 2 {
		t.Fatalf("expected two imported variables, got %#v", got)
	}
	if len(got["a"]) != 1 || len(got["b"]) != 1 {
		t.Fatalf("expected one origin per visible variable, got %#v", got)
	}
	if got["a"][0].SourceVar != "srcA" || got["a"][0].Paramset != "p" || got["a"][0].Kind != SourceKindParam {
		t.Fatalf("unexpected imported var projection for a: %#v", got["a"][0])
	}
	if got["b"][0].SourceVar != "srcB" || got["b"][0].Paramset != "l" || got["b"][0].Kind != SourceKindLet {
		t.Fatalf("unexpected imported var projection for b: %#v", got["b"][0])
	}
	if !reflect.DeepEqual(got["a"][0].Span, span) || !reflect.DeepEqual(got["b"][0].Span, span) {
		t.Fatalf("expected span propagation from plan effective origins, got %#v", got)
	}
}

func TestExplicitImportsFromStepPlan(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 5))
	plan := &StepImportPlan{
		ExplicitDelta: []PlannedImport{
			{Source: "p", Kind: SourceKindParam, Full: true, Span: span},
			{Source: "l", Kind: SourceKindLet, Visible: "vv", SourceVar: "sv", Span: span},
			{Source: "missing", Kind: SourceKindParam, Full: true, Span: span},
		},
	}
	sources := map[string]*ImportSource{
		"p": {
			Name:  "p",
			Kind:  SourceKindParam,
			Order: []string{"a", "b"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1)},
				"b": {eval.Int(2)},
			},
		},
	}

	got := explicitImportsFromStepPlan(plan, sources)
	if len(got["a"]) != 1 || got["a"][0].Paramset != "p" || got["a"][0].SourceVar != "a" {
		t.Fatalf("unexpected explicit full import expansion for a: %#v", got["a"])
	}
	if len(got["b"]) != 1 || got["b"][0].Paramset != "p" || got["b"][0].SourceVar != "b" {
		t.Fatalf("unexpected explicit full import expansion for b: %#v", got["b"])
	}
	if len(got["vv"]) != 1 || got["vv"][0].Paramset != "l" || got["vv"][0].SourceVar != "sv" || got["vv"][0].Kind != SourceKindLet {
		t.Fatalf("unexpected explicit visible/source mapping: %#v", got["vv"])
	}
	if _, ok := got["missing"]; ok {
		t.Fatalf("did not expect missing full import source to expand, got %#v", got["missing"])
	}
}

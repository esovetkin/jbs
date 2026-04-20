package lower

import (
	"reflect"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func TestEnsureSubsetParameterSetForStepCacheAndMissingSource(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{}})

	name, rows := ctx.ensureSubsetParameterSetForStep("run", "missing", []subsetVarSpec{{Visible: "x"}}, false, sourceRowContext{})
	if name != "" || rows.VarName != "" || len(rows.Groups) != 0 {
		t.Fatalf("expected empty subset result for missing source, got name=%q rows=%#v", name, rows)
	}
	if len(ctx.doc.ParameterSet) != 0 {
		t.Fatalf("did not expect emitted parameter sets for missing source, got %#v", ctx.doc.ParameterSet)
	}

	inherited := sourceRowContext{VarName: "rows_prev", Groups: []string{"0,1"}}
	cachedKey := subsetKey{Step: "run", Source: "p", Vars: "x=src=>emit", Full: false, InheritedRows: sourceRowContextKey(inherited)}
	ctx.subsetNames[cachedKey] = subsetInfo{Name: "cached_subset", RowContext: sourceRowContext{VarName: "cached_rows", Groups: []string{"2,3"}}}
	name, rows = ctx.ensureSubsetParameterSetForStep("run", "p", []subsetVarSpec{{Visible: "x", SourceVar: "src", Emitted: "emit"}}, false, inherited)
	if name != "cached_subset" || !reflect.DeepEqual(rows, sourceRowContext{VarName: "cached_rows", Groups: []string{"2,3"}}) {
		t.Fatalf("expected cached subset tuple, got name=%q rows=%#v", name, rows)
	}
}

func TestEnsureSubsetParameterSetForStepWithoutInheritedRows(t *testing.T) {
	srcSpan := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
		"p": {
			Name:  "p",
			Shape: sema.BindingTable,
			Vars: map[string][]eval.Value{
				"a":     {},
				"src_b": {},
			},
			Modes: map[string]string{"src_b": "python"},
			Order: []string{"a", "src_b"},
			Span:  srcSpan,
		},
	}})

	name, rows := ctx.ensureSubsetParameterSetForStep("run", "p", []subsetVarSpec{
		{Visible: "a", Emitted: "_alias_a"},
		{Visible: "b", SourceVar: "src_b"},
	}, false, sourceRowContext{})
	if name == "" || rows.VarName == "" {
		t.Fatalf("expected generated subset identifiers, got name=%q rows=%#v", name, rows)
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one emitted subset parameterset, got %#v", ctx.doc.ParameterSet)
	}
	ps := ctx.doc.ParameterSet[0]
	if ps.Meta.Kind != ParameterSetKindSubset || ps.Meta.Source != "p" || ps.Meta.Step != "run" {
		t.Fatalf("unexpected subset metadata: %#v", ps.Meta)
	}
	if len(ps.Parameter) < 4 {
		t.Fatalf("expected idx, rows, and payload entries, got %#v", ps.Parameter)
	}
	if ps.Parameter[0].Value != "0" {
		t.Fatalf("expected row-count fallback to produce single representative index, got %#v", ps.Parameter[0].Value)
	}
	if ps.Parameter[1].Separator != ReservedSeparator {
		t.Fatalf("expected reserved separator for row groups, got %q", ps.Parameter[1].Separator)
	}
	if !reflect.DeepEqual(rows, sourceRowContext{VarName: ps.Parameter[1].Name, Groups: []string{"0"}}) {
		t.Fatalf("unexpected row context for direct subset: %#v", rows)
	}

	foundAliasedA := false
	foundBPython := false
	for _, param := range ps.Parameter {
		if param.Name == "_alias_a" {
			foundAliasedA = true
		}
		if param.Name == "b" {
			foundBPython = true
			if param.Mode != "python" {
				t.Fatalf("expected mode from source-var mapping for b, got %q", param.Mode)
			}
		}
	}
	if !foundAliasedA || !foundBPython {
		t.Fatalf("missing expected payload params (alias or mode), got %#v", ps.Parameter)
	}
}

func TestEnsureSubsetParameterSetForStepOriginSelection(t *testing.T) {
	srcSpan := diag.NewSpan("src.jbs", diag.NewPos(1, 1, 1), diag.NewPos(1, 5, 5))
	bSpan := diag.NewSpan("src.jbs", diag.NewPos(3, 1, 1), diag.NewPos(3, 5, 5))
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
		"p": {
			Name:  "p",
			Shape: sema.BindingTable,
			Vars: map[string][]eval.Value{
				"a":     {eval.String("x"), eval.String("y")},
				"src_b": {eval.String("u"), eval.String("v")},
			},
			Modes:   map[string]string{"a": "shell", "src_b": "shell"},
			Origins: map[string]diag.Span{"src_b": bSpan},
			Order:   []string{"a", "src_b"},
			Span:    srcSpan,
		},
	}})

	_, _ = ctx.ensureSubsetParameterSetForStep("run", "p", []subsetVarSpec{{Visible: "a"}, {Visible: "b", SourceVar: "src_b"}}, false, sourceRowContext{})
	if got := countLowerDiag(ctx.diags, diag.CodeE231); got != 2 {
		t.Fatalf("expected two shell-varying diagnostics, got %d: %s", got, ctx.diags.String())
	}
	hasSrcFallback := false
	hasOrigin := false
	for _, item := range ctx.diags.Items {
		if item.Code != string(diag.CodeE231) {
			continue
		}
		if item.Span == srcSpan {
			hasSrcFallback = true
		}
		if item.Span == bSpan {
			hasOrigin = true
		}
	}
	if !hasSrcFallback || !hasOrigin {
		t.Fatalf("expected both fallback and explicit origin spans, got %#v", ctx.diags.Items)
	}
}

func TestEnsureSubsetParameterSetForStepWithInheritedRows(t *testing.T) {
	srcSpan := diag.NewSpan("src.jbs", diag.NewPos(2, 1, 1), diag.NewPos(2, 5, 5))
	bSpan := diag.NewSpan("src.jbs", diag.NewPos(4, 1, 1), diag.NewPos(4, 5, 5))
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
		"p": {
			Name:  "p",
			Shape: sema.BindingTable,
			Vars: map[string][]eval.Value{
				"a":     {eval.String("x"), eval.String("y")},
				"src_b": {eval.String("u"), eval.String("v")},
			},
			Modes:   map[string]string{"a": "shell", "src_b": "shell"},
			Origins: map[string]diag.Span{"src_b": bSpan},
			Order:   []string{"a", "src_b"},
			Span:    srcSpan,
		},
	}})

	name, rows := ctx.ensureSubsetParameterSetForStep("run", "p", []subsetVarSpec{{Visible: "a"}, {Visible: "b", SourceVar: "src_b"}}, false, sourceRowContext{
		VarName: "rows_prev",
		Groups:  []string{"0,1"},
	})
	if name == "" || rows.VarName == "" {
		t.Fatalf("expected emitted subset with rows variable in inherited context, got name=%q rows=%#v", name, rows)
	}
	ps := ctx.doc.ParameterSet[0]
	if len(ps.Parameter) < 4 {
		t.Fatalf("expected inherited subset idx, rows, and payload, got %#v", ps.Parameter)
	}
	if ps.Parameter[0].Mode != "python" || ps.Parameter[0].Separator != "," {
		t.Fatalf("expected inherited idx parameter to regroup incoming rows, got %#v", ps.Parameter[0])
	}
	if value, ok := ps.Parameter[0].Value.(SingleQuoted); !ok || string(value) != `{"0,1":"0,1"}["${rows_prev}"]` {
		t.Fatalf("unexpected inherited idx mapping: %#v", ps.Parameter[0].Value)
	}
	if ps.Parameter[1].Mode != "python" || ps.Parameter[1].Separator != ReservedSeparator {
		t.Fatalf("unexpected inherited rows helper parameter: %#v", ps.Parameter[1])
	}
	if value, ok := ps.Parameter[1].Value.(SingleQuoted); !ok || string(value) != `{"0":"0","1":"1"}["${`+ps.Parameter[0].Name+`}"]` {
		t.Fatalf("unexpected inherited rows helper mapping: %#v", ps.Parameter[1].Value)
	}
	if !reflect.DeepEqual(rows, sourceRowContext{VarName: ps.Parameter[1].Name, Groups: []string{"0", "1"}}) {
		t.Fatalf("unexpected inherited row context: %#v", rows)
	}
	if got := countLowerDiag(ctx.diags, diag.CodeE231); got != 2 {
		t.Fatalf("expected two shell-varying diagnostics in contextual payload, got %d: %s", got, ctx.diags.String())
	}
}

func TestEnsureSubsetParameterSetForStepRegroupsInheritedRows(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
		"p0": hiddenProjectionBindingForLower(),
	}})

	name, rows := ctx.ensureSubsetParameterSetForStep("step1", "p0", []subsetVarSpec{
		{Visible: "b"},
		{Visible: "c"},
	}, false, sourceRowContext{
		VarName: "rows_prev",
		Groups:  []string{"0,1,12,13", "2,3,14,15"},
	})
	if name == "" || rows.VarName == "" {
		t.Fatalf("expected inherited projection subset identifiers, got name=%q rows=%#v", name, rows)
	}
	if !reflect.DeepEqual(rows.Groups, []string{"0,1", "12,13", "2,3", "14,15"}) {
		t.Fatalf("unexpected regrouped row context: %#v", rows)
	}
	ps := ctx.doc.ParameterSet[0]
	if got, ok := ps.Parameter[0].Value.(SingleQuoted); !ok || string(got) != `{"0,1,12,13":"0,12","2,3,14,15":"2,14"}["${rows_prev}"]` {
		t.Fatalf("unexpected inherited projection idx mapping: %#v", ps.Parameter[0].Value)
	}
	if got, ok := ps.Parameter[1].Value.(SingleQuoted); !ok || string(got) != `{"0":"0,1","12":"12,13","2":"2,3","14":"14,15"}["${`+ps.Parameter[0].Name+`}"]` {
		t.Fatalf("unexpected inherited projection rows mapping: %#v", ps.Parameter[1].Value)
	}
}

func TestEnsureSubsetParameterSetForStepPreservesRowsForInheritedFullImport(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
		"p0": hiddenProjectionBindingForLower(),
	}})

	name, rows := ctx.ensureSubsetParameterSetForStep("step1", "p0", []subsetVarSpec{
		{Visible: "a"},
		{Visible: "b"},
		{Visible: "c"},
		{Visible: "d"},
	}, true, sourceRowContext{
		VarName: "rows_prev",
		Groups:  []string{"0,1,12,13"},
	})
	if name == "" || rows.VarName == "" {
		t.Fatalf("expected inherited full subset identifiers, got name=%q rows=%#v", name, rows)
	}
	if !reflect.DeepEqual(rows.Groups, []string{"0", "1", "12", "13"}) {
		t.Fatalf("expected inherited full import to preserve row-level groups, got %#v", rows)
	}
	ps := ctx.doc.ParameterSet[0]
	if got, ok := ps.Parameter[0].Value.(SingleQuoted); !ok || string(got) != `{"0,1,12,13":"0,1,12,13"}["${rows_prev}"]` {
		t.Fatalf("unexpected inherited full idx mapping: %#v", ps.Parameter[0].Value)
	}
	if got, ok := ps.Parameter[1].Value.(SingleQuoted); !ok || string(got) != `{"0":"0","1":"1","12":"12","13":"13"}["${`+ps.Parameter[0].Name+`}"]` {
		t.Fatalf("unexpected inherited full rows mapping: %#v", ps.Parameter[1].Value)
	}
}

func TestEnsureSubsetParameterSetForStepNameSuffixOnCollision(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
		"p": {
			Name:  "p",
			Shape: sema.BindingTable,
			Vars:  map[string][]eval.Value{"a": {eval.Int(1)}},
			Order: []string{"a"},
		},
	}})
	ctx.names["_js__run__p__a"] = struct{}{}

	name, rows := ctx.ensureSubsetParameterSetForStep("run", "p", []subsetVarSpec{{Visible: "a"}}, false, sourceRowContext{})
	if name != "_js__run__p__a_1" {
		t.Fatalf("expected unique subset name suffix on collision, got %q", name)
	}
	if rows.VarName != "_jr__run__p__a_1" {
		t.Fatalf("expected rows var to inherit collision suffix, got %q", rows.VarName)
	}
	if len(ctx.doc.ParameterSet) != 1 || ctx.doc.ParameterSet[0].Name != name {
		t.Fatalf("expected emitted subset to use unique name, got %#v", ctx.doc.ParameterSet)
	}
}

func TestEnsureScalarLetSubsetParameterSetForStepCacheAndMissingSource(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{}})

	name, rows := ctx.ensureScalarLetSubsetParameterSetForStep("run", "missing", []subsetVarSpec{{Visible: "x"}})
	if name != "" || rows.VarName != "" || len(rows.Groups) != 0 {
		t.Fatalf("expected empty result for missing scalar source, got name=%q rows=%#v", name, rows)
	}

	cachedKey := subsetKey{Step: "run", Source: "l", Vars: "x=src=>emit", Full: false, InheritedRows: ""}
	ctx.subsetNames[cachedKey] = subsetInfo{Name: "cached_scalar_subset"}
	name, rows = ctx.ensureScalarLetSubsetParameterSetForStep("run", "l", []subsetVarSpec{{Visible: "x", SourceVar: "src", Emitted: "emit"}})
	if name != "cached_scalar_subset" || rows.VarName != "" || len(rows.Groups) != 0 {
		t.Fatalf("expected cached scalar subset, got name=%q rows=%#v", name, rows)
	}
}

func TestEnsureScalarLetSubsetParameterSetForStepSourceVarFallbackAndModes(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
		"l": {
			Name:  "l",
			Shape: sema.BindingScalar,
			Vars: map[string][]eval.Value{
				"a":      {eval.Int(3)},
				"src_py": {eval.String("${q}")},
				"src_sh": {eval.String("$q")},
				"empty":  {},
			},
			Modes: map[string]string{"src_py": "python", "src_sh": "shell"},
		},
	}})

	name, rows := ctx.ensureScalarLetSubsetParameterSetForStep("run", "l", []subsetVarSpec{
		{Visible: "a"},
		{Visible: "b", SourceVar: "src_py", Emitted: "emit_b"},
		{Visible: "c", SourceVar: "src_sh"},
		{Visible: "d", SourceVar: "empty"},
	})
	if name == "" || rows.VarName != "" || len(rows.Groups) != 0 {
		t.Fatalf("expected scalar subset name and empty rows var, got name=%q rows=%#v", name, rows)
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one emitted scalar subset, got %#v", ctx.doc.ParameterSet)
	}
	params := map[string]Parameter{}
	for _, param := range ctx.doc.ParameterSet[0].Parameter {
		params[param.Name] = param
	}
	if p, ok := params["a"]; !ok || p.Mode != "text" || p.Value != "3" {
		t.Fatalf("expected fallback source-var plus default text mode for a, got %#v", p)
	}
	if p, ok := params["emit_b"]; !ok || p.Mode != "python" {
		t.Fatalf("expected emitted python parameter for b, got %#v", p)
	} else if _, ok := p.Value.(SingleQuoted); !ok {
		t.Fatalf("expected python mode to single-quote payload, got %T", p.Value)
	}
	if p, ok := params["c"]; !ok || p.Mode != "shell" || p.Value != "$q" {
		t.Fatalf("expected shell-mode scalar parameter for c, got %#v", p)
	}
	if p, ok := params["d"]; !ok || p.Mode != "text" || p.Value != "" {
		t.Fatalf("expected empty source to lower as empty text value, got %#v", p)
	}
}

func hiddenProjectionBindingForLower() *sema.GlobalBinding {
	aVals := make([]eval.Value, 0, 24)
	bVals := make([]eval.Value, 0, 24)
	cVals := make([]eval.Value, 0, 24)
	dVals := make([]eval.Value, 0, 24)
	pairs := []struct {
		a int64
		b string
	}{
		{a: 0, b: "a"},
		{a: 1, b: "b"},
		{a: 2, b: "c"},
		{a: 3, b: "a"},
		{a: 4, b: "b"},
		{a: 5, b: "c"},
	}
	for _, c := range []string{"x", "z"} {
		for _, pair := range pairs {
			for _, d := range []bool{true, false} {
				aVals = append(aVals, eval.Int(pair.a))
				bVals = append(bVals, eval.String(pair.b))
				cVals = append(cVals, eval.String(c))
				dVals = append(dVals, eval.Bool(d))
			}
		}
	}
	return &sema.GlobalBinding{
		Name:  "p0",
		Shape: sema.BindingTable,
		Order: []string{"a", "b", "c", "d"},
		Vars: map[string][]eval.Value{
			"a": aVals,
			"b": bVals,
			"c": cVals,
			"d": dVals,
		},
	}
}

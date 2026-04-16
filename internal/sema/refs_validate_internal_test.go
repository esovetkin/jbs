package sema

import (
	"reflect"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestCollectExprIdentRefs(t *testing.T) {
	sp := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))

	if got := collectExprIdentRefs(nil); got != nil {
		t.Fatalf("expected nil refs for nil expr, got %#v", got)
	}

	expr := ast.BinaryExpr{
		Left: ast.IdentExpr{Name: "a", Span: sp},
		Op:   "+",
		Right: ast.TupleExpr{
			Items: []ast.Expr{
				ast.CallExpr{
					Callee: ast.IdentExpr{Name: "list", Span: sp},
					Args: []ast.Expr{
						ast.ListExpr{
							Items: []ast.Expr{
								ast.IdentExpr{Name: "b", Span: sp},
								ast.QualifiedIdentExpr{Namespace: "ns", Name: "q", Span: sp},
							},
							Span: sp,
						},
					},
					Span: sp,
				},
				ast.UnaryExpr{
					Op: "-",
					Expr: ast.CompareExpr{
						Left: ast.IdentExpr{Name: "c", Span: sp},
						Op:   "==",
						Right: ast.ConditionalExpr{
							Then: ast.IdentExpr{Name: "d", Span: sp},
							Cond: ast.BoolExpr{Value: true, Span: sp},
							Else: ast.ModeExpr{
								Mode: "python",
								Expr: ast.IdentExpr{Name: "e", Span: sp},
								Span: sp,
							},
							Span: sp,
						},
						Span: sp,
					},
					Span: sp,
				},
			},
			Span: sp,
		},
		Span: sp,
	}

	refs := collectExprIdentRefs(expr)
	got := make([]string, 0, len(refs))
	for _, ref := range refs {
		got = append(got, ref.Name)
	}
	want := []string{"a", "list", "b", "ns", "c", "d", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ident refs: got=%#v want=%#v", got, want)
	}
}

func TestCollectExprStringRefsWith(t *testing.T) {
	sp0 := diag.NewSpan("in.jbs", diag.NewPos(10, 3, 5), diag.NewPos(20, 3, 15))
	sp1 := diag.NewSpan("in.jbs", diag.NewPos(30, 4, 2), diag.NewPos(40, 4, 12))
	sp2 := diag.NewSpan("in.jbs", diag.NewPos(50, 5, 7), diag.NewPos(60, 5, 17))

	if got := collectExprStringRefsWith(nil, collectShellLikeRefs); got != nil {
		t.Fatalf("expected nil for nil expr, got %#v", got)
	}
	expr := ast.TupleExpr{
		Items: []ast.Expr{
			ast.StringExpr{Value: "$a", Span: sp0},
			ast.ModeExpr{
				Mode: "python",
				Expr: ast.ListExpr{
					Items: []ast.Expr{
						ast.StringExpr{Value: "${b}", Span: sp1},
						ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: sp1},
					},
					Span: sp1,
				},
				Span: sp1,
			},
			ast.ConditionalExpr{
				Then: ast.StringExpr{Value: "$c", Span: sp2},
				Cond: ast.BoolExpr{Value: true, Span: sp2},
				Else: ast.IdentExpr{Name: "x", Span: sp2},
				Span: sp2,
			},
		},
		Span: sp2,
	}
	if got := collectExprStringRefsWith(expr, nil); got != nil {
		t.Fatalf("expected nil for nil collector, got %#v", got)
	}

	type call struct {
		text string
		base diag.Position
		file string
	}
	calls := make([]call, 0)
	collector := func(text string, base diag.Position, file string) []varRef {
		calls = append(calls, call{text: text, base: base, file: file})
		return []varRef{{Name: text, Span: diag.NewSpan(file, base, base)}}
	}

	refs := collectExprStringRefsWith(expr, collector)
	gotNames := make([]string, 0, len(refs))
	for _, ref := range refs {
		gotNames = append(gotNames, ref.Name)
	}
	wantNames := []string{"$a", "${b}", "$c"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("unexpected string refs: got=%#v want=%#v", gotNames, wantNames)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 collector calls, got %d", len(calls))
	}
	if calls[0].base.Offset != sp0.Start.Offset+1 || calls[0].base.Column != sp0.Start.Column+1 {
		t.Fatalf("unexpected base for first string: got=%+v span_start=%+v", calls[0].base, sp0.Start)
	}
	if calls[1].base.Offset != sp1.Start.Offset+1 || calls[1].base.Column != sp1.Start.Column+1 {
		t.Fatalf("unexpected base for second string: got=%+v span_start=%+v", calls[1].base, sp1.Start)
	}
	if calls[2].base.Offset != sp2.Start.Offset+1 || calls[2].base.Column != sp2.Start.Column+1 {
		t.Fatalf("unexpected base for third string: got=%+v span_start=%+v", calls[2].base, sp2.Start)
	}
}

func TestCollectEvalStringRefsWith(t *testing.T) {
	value := eval.Tuple([]eval.Value{
		eval.String("$a"),
		eval.List([]eval.Value{
			eval.String("${b}"),
			eval.Int(1),
		}),
	})
	span := diag.NewSpan("vals.jbs", diag.NewPos(0, 0, 0), diag.NewPos(0, 0, 0))

	if got := collectEvalStringRefsWith(value, span, nil); got != nil {
		t.Fatalf("expected nil for nil collector, got %#v", got)
	}

	calls := make([]diag.Position, 0)
	collector := func(text string, base diag.Position, file string) []varRef {
		calls = append(calls, base)
		return []varRef{{Name: text, Span: diag.NewSpan(file, base, base)}}
	}
	refs := collectEvalStringRefsWith(value, span, collector)
	gotNames := make([]string, 0, len(refs))
	for _, ref := range refs {
		gotNames = append(gotNames, ref.Name)
	}
	wantNames := []string{"$a", "${b}"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("unexpected eval string refs: got=%#v want=%#v", gotNames, wantNames)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 collector calls, got %d", len(calls))
	}
	for i, base := range calls {
		if base != (diag.NewPos(0, 1, 1)) {
			t.Fatalf("call %d expected default base position, got %+v", i, base)
		}
	}
}

func TestParseBracedVarRef(t *testing.T) {
	tests := []struct {
		expr     string
		start    int
		wantName string
		wantEnd  int
		wantOK   bool
	}{
		{expr: "${x}", start: 2, wantName: "x", wantEnd: 3, wantOK: true},
		{expr: "${#x}", start: 2, wantName: "x", wantEnd: 4, wantOK: true},
		{expr: "${!x}", start: 2, wantName: "x", wantEnd: 4, wantOK: true},
		{expr: "${x:-${y}}", start: 2, wantName: "x", wantEnd: 9, wantOK: true},
		{expr: "${x\\}}", start: 2, wantName: "x", wantEnd: 5, wantOK: true},
		{expr: "${}", start: 2, wantName: "", wantEnd: 0, wantOK: false},
		{expr: "${#1}", start: 2, wantName: "", wantEnd: 0, wantOK: false},
		{expr: "${x", start: 2, wantName: "", wantEnd: 0, wantOK: false},
		{expr: "$", start: 2, wantName: "", wantEnd: 0, wantOK: false},
	}
	for _, tt := range tests {
		runes := []rune(tt.expr)
		gotName, gotEnd, gotOK := parseBracedVarRef(runes, tt.start)
		if gotName != tt.wantName || gotEnd != tt.wantEnd || gotOK != tt.wantOK {
			t.Fatalf("parseBracedVarRef(%q,start=%d)=(%q,%d,%v), want (%q,%d,%v)",
				tt.expr, tt.start, gotName, gotEnd, gotOK, tt.wantName, tt.wantEnd, tt.wantOK)
		}
	}
}

func TestCommentBoundaryAndSanitizeHelpers(t *testing.T) {
	testsCommentStart := []struct {
		text string
		idx  int
		want bool
	}{
		{text: "#x", idx: 0, want: true},
		{text: "a#x", idx: 1, want: false},
		{text: " #x", idx: 1, want: true},
		{text: ";#x", idx: 1, want: true},
		{text: "x", idx: 0, want: false},
		{text: "#", idx: -1, want: false},
		{text: "#", idx: 2, want: false},
	}
	for _, tt := range testsCommentStart {
		if got := isCommentStart([]rune(tt.text), tt.idx); got != tt.want {
			t.Fatalf("isCommentStart(%q,%d)=%v, want %v", tt.text, tt.idx, got, tt.want)
		}
	}

	testsBoundary := []struct {
		r    rune
		want bool
	}{
		{r: ' ', want: true},
		{r: '\t', want: true},
		{r: '\n', want: true},
		{r: '\r', want: true},
		{r: ';', want: true},
		{r: '|', want: true},
		{r: '&', want: true},
		{r: '(', want: true},
		{r: ')', want: true},
		{r: '{', want: true},
		{r: '}', want: true},
		{r: 'a', want: false},
		{r: '_', want: false},
		{r: '.', want: false},
	}
	for _, tt := range testsBoundary {
		if got := isShellCommentBoundary(tt.r); got != tt.want {
			t.Fatalf("isShellCommentBoundary(%q)=%v, want %v", tt.r, got, tt.want)
		}
	}

	testsSanitize := []struct {
		in   string
		want string
	}{
		{in: "", want: "x"},
		{in: "run_step_1", want: "run_step_1"},
		{in: "run-step.1", want: "run_step_1"},
		{in: "   ", want: "___"},
		{in: "äöß", want: "äöß"},
	}
	for _, tt := range testsSanitize {
		if got := sanitizeStepName(tt.in); got != tt.want {
			t.Fatalf("sanitizeStepName(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildWarningSourcesIncludesParamAndLetWithSameName(t *testing.T) {
	paramBlockSpan := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(10, 1, 10))
	paramVarSpan := diag.NewSpan("in.jbs", diag.NewPos(2, 2, 3), diag.NewPos(3, 2, 4))
	letSpan := diag.NewSpan("in.jbs", diag.NewPos(20, 3, 1), diag.NewPos(30, 3, 11))
	letVarSpan := diag.NewSpan("in.jbs", diag.NewPos(21, 4, 3), diag.NewPos(22, 4, 4))

	res := &Result{
		Paramsets: []*Paramset{
			{
				Name:  "p",
				Block: ast.ParamBlock{Span: paramBlockSpan},
				Vars: map[string][]eval.Value{
					"x": {eval.Int(1)},
				},
				Origins: map[string]diag.Span{
					"x": paramVarSpan,
				},
				Order: []string{"x"},
			},
		},
		LetNamespaces: []*LetNamespace{
			{
				Name: "p",
				Vars: map[string]eval.Value{
					"y": eval.String("z"),
				},
				Origins: map[string]diag.Span{
					"y": letVarSpan,
				},
				Span: letSpan,
			},
		},
	}

	sources := buildWarningSources(res)
	if len(sources) != 2 {
		t.Fatalf("expected 2 warning sources, got %d", len(sources))
	}

	seen := map[sourceKey]warningSource{}
	for _, src := range sources {
		seen[src.Key] = src
	}

	paramKey := sourceKey{Kind: SourceKindParam, Name: "p"}
	paramSource, ok := seen[paramKey]
	if !ok {
		t.Fatalf("missing param warning source for key %+v", paramKey)
	}
	if !reflect.DeepEqual(paramSource.Order, []string{"x"}) {
		t.Fatalf("unexpected param order: got=%v want=%v", paramSource.Order, []string{"x"})
	}
	if got := paramSource.VarOrigins["x"]; got != paramVarSpan {
		t.Fatalf("unexpected param origin span: got=%+v want=%+v", got, paramVarSpan)
	}

	letKey := sourceKey{Kind: SourceKindLet, Name: "p"}
	letSource, ok := seen[letKey]
	if !ok {
		t.Fatalf("missing let warning source for key %+v", letKey)
	}
	if !reflect.DeepEqual(letSource.Order, []string{"y"}) {
		t.Fatalf("unexpected let order: got=%v want=%v", letSource.Order, []string{"y"})
	}
	if got := letSource.VarOrigins["y"]; got != letVarSpan {
		t.Fatalf("unexpected let origin span: got=%+v want=%+v", got, letVarSpan)
	}
}

func TestSourceKeyResolutionHelpers(t *testing.T) {
	exposed := map[sourceKey]map[string]diag.Span{
		{Kind: SourceKindParam, Name: "p"}: {
			"x": diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2)),
		},
		{Kind: SourceKindLet, Name: "p"}: {
			"y": diag.NewSpan("in.jbs", diag.NewPos(3, 1, 1), diag.NewPos(4, 1, 2)),
		},
		{Kind: SourceKindParam, Name: "q"}: {
			"a": diag.NewSpan("in.jbs", diag.NewPos(5, 1, 1), diag.NewPos(6, 1, 2)),
		},
	}
	sources := map[string]*ImportSource{
		"p": {Name: "p", Kind: SourceKindLet},
	}

	if got := resolveSourceKey(SourceKindParam, "p", sources, exposed); got != (sourceKey{Kind: SourceKindParam, Name: "p"}) {
		t.Fatalf("resolveSourceKey explicit param mismatch: got=%+v", got)
	}
	if got := resolveSourceKey("", "p", sources, exposed); got != (sourceKey{Kind: SourceKindLet, Name: "p"}) {
		t.Fatalf("resolveSourceKey fallback from sources mismatch: got=%+v", got)
	}
	if got := resolveSourceKey("", "q", nil, exposed); got != (sourceKey{Kind: SourceKindParam, Name: "q"}) {
		t.Fatalf("resolveSourceKey fallback from exposed mismatch: got=%+v", got)
	}
	if got := resolveSourceKey("", "unknown", nil, nil); got != (sourceKey{Name: "unknown"}) {
		t.Fatalf("resolveSourceKey unknown fallback mismatch: got=%+v", got)
	}

	originNoKind := importedVar{Paramset: "p"}
	if got := sourceKeyFromImportedVar(originNoKind, sources); got != (sourceKey{Kind: SourceKindLet, Name: "p"}) {
		t.Fatalf("sourceKeyFromImportedVar source fallback mismatch: got=%+v", got)
	}
	originWithKind := importedVar{Paramset: "p", Kind: SourceKindParam}
	if got := sourceKeyFromImportedVar(originWithKind, sources); got != (sourceKey{Kind: SourceKindParam, Name: "p"}) {
		t.Fatalf("sourceKeyFromImportedVar explicit kind mismatch: got=%+v", got)
	}
}

func TestBuildWarningSourcesFallbackAndSkips(t *testing.T) {
	paramSpan := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	letSpan := diag.NewSpan("in.jbs", diag.NewPos(3, 1, 1), diag.NewPos(4, 1, 2))

	res := &Result{
		Paramsets: []*Paramset{
			nil, // skipped
			{
				Name:  "empty",
				Block: ast.ParamBlock{Span: paramSpan},
				Vars:  map[string][]eval.Value{}, // skipped because no exposed vars
			},
			{
				Name:  "p",
				Block: ast.ParamBlock{Span: paramSpan},
				Vars: map[string][]eval.Value{
					"x": {eval.Int(1)},
				},
				Origins: map[string]diag.Span{}, // fallback to block span
				Order:   []string{"x"},
			},
		},
		LetNamespaces: []*LetNamespace{
			nil, // skipped
			{
				Name: "l_empty",
				Vars: map[string]eval.Value{}, // skipped because no vars
				Span: letSpan,
			},
			{
				Name: "l",
				Vars: map[string]eval.Value{
					"y": eval.String("v"),
				},
				Origins: map[string]diag.Span{}, // fallback to let span
				Span:    letSpan,
			},
		},
	}

	got := buildWarningSources(res)
	if len(got) != 2 {
		t.Fatalf("expected only non-empty param/let warning sources, got %#v", got)
	}
	seen := map[sourceKey]warningSource{}
	for _, src := range got {
		seen[src.Key] = src
	}

	p, ok := seen[sourceKey{Kind: SourceKindParam, Name: "p"}]
	if !ok {
		t.Fatalf("expected param warning source for p, got %#v", seen)
	}
	if p.VarOrigins["x"] != paramSpan {
		t.Fatalf("expected param origin fallback to block span, got=%+v want=%+v", p.VarOrigins["x"], paramSpan)
	}

	l, ok := seen[sourceKey{Kind: SourceKindLet, Name: "l"}]
	if !ok {
		t.Fatalf("expected let warning source for l, got %#v", seen)
	}
	if l.VarOrigins["y"] != letSpan {
		t.Fatalf("expected let origin fallback to let span, got=%+v want=%+v", l.VarOrigins["y"], letSpan)
	}
}

func TestResolveSourceKeyAdditionalBranches(t *testing.T) {
	exposed := map[sourceKey]map[string]diag.Span{
		{Kind: SourceKindParam, Name: "q"}: {"x": {}},
		{Kind: SourceKindLet, Name: "p"}:   {"y": {}},
	}
	sources := map[string]*ImportSource{
		"s": {Name: "s", Kind: SourceKindParam},
		"p": {Name: "p", Kind: ""}, // force exposed fallback path
	}

	if got := resolveSourceKey("", "", sources, exposed); got != (sourceKey{}) {
		t.Fatalf("expected zero key for empty name, got %+v", got)
	}
	if got := resolveSourceKey(SourceKindLet, "x", sources, exposed); got != (sourceKey{Kind: SourceKindLet, Name: "x"}) {
		t.Fatalf("expected explicit kind resolution, got %+v", got)
	}
	if got := resolveSourceKey("", "s", sources, exposed); got != (sourceKey{Kind: SourceKindParam, Name: "s"}) {
		t.Fatalf("expected source-kind resolution, got %+v", got)
	}
	if got := resolveSourceKey("", "q", nil, exposed); got != (sourceKey{Kind: SourceKindParam, Name: "q"}) {
		t.Fatalf("expected param exposed fallback, got %+v", got)
	}
	if got := resolveSourceKey("", "p", sources, exposed); got != (sourceKey{Kind: SourceKindLet, Name: "p"}) {
		t.Fatalf("expected let exposed fallback when source kind missing, got %+v", got)
	}
	if got := resolveSourceKey("", "unknown", nil, exposed); got != (sourceKey{Name: "unknown"}) {
		t.Fatalf("expected name-only fallback for unknown source, got %+v", got)
	}
}

func TestCollectSubmitStringRefsInvalidAndEscapedCases(t *testing.T) {
	base := diag.NewPos(10, 3, 5)
	text := `\$skip ${} $ ${ok}
${also:-${nested}} ${bad`

	refs := collectSubmitStringRefs(text, base, "in.jbs")
	got := make([]string, 0, len(refs))
	for _, ref := range refs {
		got = append(got, ref.Name)
	}
	want := []string{"ok", "also"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected submit string refs: got=%#v want=%#v", got, want)
	}
	if len(refs) != 2 {
		t.Fatalf("expected exactly 2 refs, got %#v", refs)
	}
	if refs[0].Span.Start.Line != 3 || refs[1].Span.Start.Line != 4 {
		t.Fatalf("expected line tracking across newlines, got refs=%#v", refs)
	}
}

func TestCollectExprStringRefsWithAdditionalWrappers(t *testing.T) {
	sp := diag.NewSpan("in.jbs", diag.NewPos(50, 10, 7), diag.NewPos(60, 10, 17))
	expr := ast.ConvertExpr{
		Target: "tuple",
		Expr: ast.UnaryExpr{
			Op: "-",
			Expr: ast.BinaryExpr{
				Left: ast.StringExpr{Value: "$a", Span: sp},
				Op:   "+",
				Right: ast.CompareExpr{
					Left: ast.StringExpr{Value: "${b}", Span: sp},
					Op:   "==",
					Right: ast.ConditionalExpr{
						Then: ast.StringExpr{Value: "$c", Span: sp},
						Cond: ast.BoolExpr{Value: true, Span: sp},
						Else: ast.CallExpr{
							Callee: ast.IdentExpr{Name: "f", Span: sp},
							Args: []ast.Expr{
								ast.StringExpr{Value: "$d", Span: sp},
							},
							Span: sp,
						},
						Span: sp,
					},
					Span: sp,
				},
				Span: sp,
			},
			Span: sp,
		},
		Span: sp,
	}

	refs := collectExprStringRefsWith(expr, func(text string, base diag.Position, file string) []varRef {
		return []varRef{{Name: text, Span: diag.NewSpan(file, base, base)}}
	})
	got := make([]string, 0, len(refs))
	for _, ref := range refs {
		got = append(got, ref.Name)
	}
	want := []string{"$a", "${b}", "$c", "$d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected wrapped string refs: got=%#v want=%#v", got, want)
	}
}

func TestValidateStepVarReferencesW313ImportSpanFallback(t *testing.T) {
	originSpan := diag.NewSpan("in.jbs", diag.NewPos(1, 2, 3), diag.NewPos(2, 2, 4))
	stepSpan := diag.NewSpan("in.jbs", diag.NewPos(10, 5, 1), diag.NewPos(20, 5, 11))
	ps := &Paramset{
		Name: "p",
		Block: ast.ParamBlock{
			Span: originSpan,
		},
		Vars: map[string][]eval.Value{
			"x": {eval.Int(1)},
		},
		Origins: map[string]diag.Span{
			"x": originSpan,
		},
		Order: []string{"x"},
	}
	res := &Result{
		Program: ast.Program{},
		Paramsets: []*Paramset{
			ps,
		},
		ParamByName: map[string]*Paramset{
			"p": ps,
		},
		DoBlocks: []ast.DoBlock{
			{
				Name:      "s0",
				Body:      "echo $x",
				BodyStart: stepSpan.Start,
				Span:      stepSpan,
			},
			{
				Name:      "s1",
				Body:      "echo nothing",
				BodyStart: stepSpan.Start,
				Span:      stepSpan,
			},
		},
		StepImportByName: map[string]*StepImportPlan{
			"s0": {
				Effective: map[string]VarOrigin{
					"x": {Paramset: "p", Kind: SourceKindParam, SourceVar: "x", Span: stepSpan},
				},
				ExplicitDelta: []PlannedImport{
					{Source: "p", Kind: SourceKindParam, Visible: "x", SourceVar: "x", Span: stepSpan},
				},
			},
			"s1": {
				Effective: map[string]VarOrigin{
					"x": {Paramset: "p", Kind: SourceKindParam, SourceVar: "x", Span: stepSpan},
				},
				// zero span to trigger fallback to source span in W313 emission
				ExplicitDelta: []PlannedImport{
					{Source: "p", Kind: SourceKindParam, Visible: "x", SourceVar: "x"},
				},
			},
		},
	}
	buildImportSources(res)
	diags := &diag.Diagnostics{}
	validateStepVarReferences(res, diags)

	if got := countDiagCode(diags, "W313"); got != 1 {
		t.Fatalf("expected one W313 warning, got %d: %s", got, diags.String())
	}
	for _, d := range diags.Items {
		if d.Code != "W313" {
			continue
		}
		if d.Span != originSpan {
			t.Fatalf("expected W313 span fallback to source origin span, got=%+v want=%+v", d.Span, originSpan)
		}
		return
	}
	t.Fatalf("expected W313 diagnostic, got: %s", diags.String())
}

func TestCompareSourceKeyOrdering(t *testing.T) {
	paramA := sourceKey{Kind: SourceKindParam, Name: "a"}
	paramB := sourceKey{Kind: SourceKindParam, Name: "b"}
	letA := sourceKey{Kind: SourceKindLet, Name: "a"}

	if got := compareSourceKey(paramA, paramA); got != 0 {
		t.Fatalf("expected equal comparison result 0, got %d", got)
	}
	if got := compareSourceKey(paramA, paramB); got >= 0 {
		t.Fatalf("expected paramA < paramB, got %d", got)
	}
	if got := compareSourceKey(paramB, paramA); got <= 0 {
		t.Fatalf("expected paramB > paramA, got %d", got)
	}
	forward := compareSourceKey(letA, paramA)
	reverse := compareSourceKey(paramA, letA)
	if forward == 0 || reverse == 0 || forward != -reverse {
		t.Fatalf("expected strict, symmetric ordering between kinds; forward=%d reverse=%d", forward, reverse)
	}
}

func TestCloneUsedBySourceDeepCopyAndEmptyMap(t *testing.T) {
	keyA := sourceKey{Kind: SourceKindParam, Name: "a"}
	keyB := sourceKey{Kind: SourceKindLet, Name: "b"}
	original := map[sourceKey]map[string]bool{
		keyA: {"x": true},
		keyB: {},
	}

	cloned := cloneUsedBySource(original)
	if len(cloned) != len(original) {
		t.Fatalf("expected clone size %d, got %d", len(original), len(cloned))
	}
	if _, ok := cloned[keyB]; !ok {
		t.Fatalf("expected clone to include empty key %v", keyB)
	}
	cloned[keyA]["x"] = false
	cloned[keyA]["y"] = true
	if original[keyA]["x"] != true {
		t.Fatalf("expected original map to stay unchanged, got %#v", original[keyA])
	}
	if _, ok := original[keyA]["y"]; ok {
		t.Fatalf("expected original map not to receive cloned mutation, got %#v", original[keyA])
	}
}

func TestValidateStepVarReferencesSubmitUseMarksAllowedKeyAsUsed(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	let := &LetNamespace{
		Name: "defaults",
		Vars: map[string]eval.Value{
			"queue":      eval.String("batch"),
			"preprocess": eval.String("echo setup"),
		},
		Origins: map[string]diag.Span{
			"queue":      span,
			"preprocess": span,
		},
		Span: span,
	}
	res := &Result{
		Program: ast.Program{},
		LetNamespaces: []*LetNamespace{
			let,
		},
		ImportSourceByName: map[string]*ImportSource{
			"defaults": {
				Name:  "defaults",
				Kind:  SourceKindLet,
				Vars:  map[string][]eval.Value{"queue": {eval.String("batch")}, "preprocess": {eval.String("echo setup")}},
				Order: []string{"queue", "preprocess"},
				Span:  span,
			},
		},
		Submits: []ast.SubmitBlock{
			{
				Name:     "run",
				UseNames: []string{"defaults"},
				Span:     span,
			},
		},
		StepImportByName: map[string]*StepImportPlan{
			"run": {StepName: "run"},
		},
	}
	diags := &diag.Diagnostics{}
	validateStepVarReferences(res, diags)

	hasQueueW310 := false
	hasPreprocessW310 := false
	for _, item := range diags.Items {
		if item.Code != string(diag.CodeW310) {
			continue
		}
		if reflect.DeepEqual(item.Span, span) && strings.Contains(item.Message, "queue") && strings.Contains(item.Message, "defaults") {
			hasQueueW310 = true
		}
		if reflect.DeepEqual(item.Span, span) && strings.Contains(item.Message, "preprocess") && strings.Contains(item.Message, "defaults") {
			hasPreprocessW310 = true
		}
	}
	if hasQueueW310 {
		t.Fatalf("did not expect W310 for queue imported via submit use defaults: %s", diags.String())
	}
	if !hasPreprocessW310 {
		t.Fatalf("expected W310 for raw submit key preprocess (not auto-marked used), got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesSubmitHelperRefCountsAsImportedUsage(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(10, 4, 1), diag.NewPos(20, 4, 11))
	let := &LetNamespace{
		Name: "defs",
		Vars: map[string]eval.Value{
			"helper_var": eval.String("x"),
		},
		Origins: map[string]diag.Span{
			"helper_var": span,
		},
		Span: span,
	}
	res := &Result{
		Program: ast.Program{},
		LetNamespaces: []*LetNamespace{
			let,
		},
		ImportSourceByName: map[string]*ImportSource{
			"defs": {
				Name:  "defs",
				Kind:  SourceKindLet,
				Vars:  map[string][]eval.Value{"helper_var": {eval.String("x")}},
				Order: []string{"helper_var"},
				Span:  span,
			},
		},
		Submits: []ast.SubmitBlock{
			{
				Name: "run",
				Fields: []ast.SubmitField{
					{
						Name: "args_exec",
						Expr: ast.StringExpr{
							Value: "-lc 'echo ${helper_var}'",
							Span:  span,
						},
						Span: span,
					},
				},
				Span: span,
			},
		},
		SubmitByName: map[string]*SubmitSpec{
			"run": {
				Name: "run",
				Helpers: []SubmitHelper{
					{
						Original: "helper_var",
						UseName:  "defs",
						Span:     span,
					},
				},
			},
		},
		StepImportByName: map[string]*StepImportPlan{
			"run": {StepName: "run"},
		},
	}
	diags := &diag.Diagnostics{}
	validateStepVarReferences(res, diags)

	for _, item := range diags.Items {
		if item.Code == string(diag.CodeW311) && strings.Contains(item.Message, "helper_var") {
			t.Fatalf("did not expect W311 for helper_var when submit helper import exists: %s", diags.String())
		}
		if item.Code == string(diag.CodeW310) && strings.Contains(item.Message, "helper_var") && strings.Contains(item.Message, "defs") {
			t.Fatalf("did not expect W310 for helper_var when referenced via helper import: %s", diags.String())
		}
	}
}

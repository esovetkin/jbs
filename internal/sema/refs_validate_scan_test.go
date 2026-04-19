package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestCollectShellLikeRefsRespectsQuotesCommentsAndSpans(t *testing.T) {
	base := diag.NewPos(40, 3, 7)
	text := "echo '$skip' \"$keep ${braced} \\$escaped\" # $comment\n" +
		"next ${after} \\$ignored"

	refs := collectShellLikeRefs(text, base, "scan.jbs")
	got := refNames(refs)
	want := []string{"keep", "braced", "after"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refs: got=%#v want=%#v", got, want)
	}
	if len(refs) != 3 {
		t.Fatalf("unexpected ref count: got %d", len(refs))
	}
	if refs[0].Span.Start.Line != 3 || refs[0].Span.Start.Column <= base.Column {
		t.Fatalf("expected first ref span to originate on first line after base, got %+v", refs[0].Span.Start)
	}
	if refs[2].Span.Start.Line != 4 {
		t.Fatalf("expected multiline scan to advance line count, got %+v", refs[2].Span.Start)
	}
}

func TestCollectSubmitStringRefsCapturesQuotedAndCommentText(t *testing.T) {
	base := diag.NewPos(10, 2, 3)
	text := "-lc 'echo ${inside_single}' # ${inside_comment} \"$double\" \\$ignored"

	refs := collectSubmitStringRefs(text, base, "submit.jbs")
	got := refNames(refs)
	want := []string{"inside_single", "inside_comment", "double"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected submit refs: got=%#v want=%#v", got, want)
	}
	if refs[0].Span.Start.Line != 2 || refs[0].Span.File != "submit.jbs" {
		t.Fatalf("unexpected first submit ref span: %+v", refs[0].Span)
	}
}

func TestCollectExprStringRefsWrapperAndWalker(t *testing.T) {
	sp0 := diag.NewSpan("expr.jbs", diag.NewPos(0, 1, 1), diag.NewPos(11, 1, 12))
	sp1 := diag.NewSpan("expr.jbs", diag.NewPos(20, 2, 4), diag.NewPos(31, 2, 15))
	sp2 := diag.NewSpan("expr.jbs", diag.NewPos(40, 3, 2), diag.NewPos(52, 3, 14))

	if got := collectExprStringRefs(nil); got != nil {
		t.Fatalf("expected nil for nil expr, got %#v", got)
	}
	if got := collectExprStringRefsWith(nil, collectShellLikeRefs); got != nil {
		t.Fatalf("expected nil for nil expr in walker, got %#v", got)
	}
	if got := collectExprStringRefsWith(ast.StringExpr{Value: "$x", Span: sp0}, nil); got != nil {
		t.Fatalf("expected nil for nil collector, got %#v", got)
	}

	expr := ast.TupleExpr{
		Items: []ast.Expr{
			ast.StringExpr{Value: "${left}", Span: sp0},
			ast.ModeExpr{
				Mode: "python",
				Expr: ast.ListExpr{Items: []ast.Expr{
					ast.StringExpr{Value: "$right", Span: sp1},
					ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: sp1},
				}, Span: sp1},
				Span: sp1,
			},
			ast.ConditionalExpr{
				Then: ast.StringExpr{Value: "$then", Span: sp2},
				Cond: ast.BoolExpr{Value: true, Span: sp2},
				Else: ast.IdentExpr{Name: "skip", Span: sp2},
				Span: sp2,
			},
		},
		Span: sp2,
	}

	got := refNames(collectExprStringRefs(expr))
	want := []string{"left", "right", "then"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected wrapper refs: got=%#v want=%#v", got, want)
	}

	type call struct {
		text string
		base diag.Position
		file string
	}
	calls := make([]call, 0, 3)
	collector := func(text string, base diag.Position, file string) []varRef {
		calls = append(calls, call{text: text, base: base, file: file})
		return []varRef{{Name: text, Span: diag.NewSpan(file, base, base)}}
	}
	gotNames := make([]string, 0, 3)
	for _, ref := range collectExprStringRefsWith(expr, collector) {
		gotNames = append(gotNames, ref.Name)
	}
	wantNames := []string{"${left}", "$right", "$then"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("unexpected collector refs: got=%#v want=%#v", gotNames, wantNames)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 collector calls, got %d", len(calls))
	}
	if calls[0].base.Offset != sp0.Start.Offset+1 || calls[1].base.Column != sp1.Start.Column+1 || calls[2].file != "expr.jbs" {
		t.Fatalf("unexpected collector call metadata: %#v", calls)
	}
}

func TestCollectExprIdentRefsWalksCurrentNodeTypes(t *testing.T) {
	sp := diag.NewSpan("idents.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))

	if got := collectExprIdentRefs(nil); got != nil {
		t.Fatalf("expected nil refs for nil expr, got %#v", got)
	}

	expr := ast.BinaryExpr{
		Left: ast.IdentExpr{Name: "left", Span: sp},
		Op:   "+",
		Right: ast.TupleExpr{Items: []ast.Expr{
			ast.CallExpr{
				Callee: ast.IdentExpr{Name: "call", Span: sp},
				Args: []ast.Expr{
					ast.ListExpr{Items: []ast.Expr{
						ast.QualifiedIdentExpr{Namespace: "ns", Name: "item", Span: sp},
						ast.MemberExpr{
							Base: ast.IndexExpr{
								Base:  ast.IdentExpr{Name: "member_base", Span: sp},
								Items: []ast.Expr{ast.IdentExpr{Name: "member_index", Span: sp}},
								Span:  sp,
							},
							Name: "member_name",
							Span: sp,
						},
						ast.ConvertExpr{Expr: ast.IdentExpr{Name: "convert", Span: sp}, Span: sp},
					}, Span: sp},
				},
				Span: sp,
			},
			ast.AliasExpr{Expr: ast.IndexExpr{Base: ast.IdentExpr{Name: "base", Span: sp}, Items: []ast.Expr{ast.IdentExpr{Name: "index", Span: sp}}, Span: sp}, Alias: "alias", Span: sp},
			ast.UnaryExpr{Op: "-", Expr: ast.CompareExpr{Left: ast.IdentExpr{Name: "compare_left", Span: sp}, Op: "==", Right: ast.ConditionalExpr{Then: ast.IdentExpr{Name: "then", Span: sp}, Cond: ast.BoolExpr{Value: true, Span: sp}, Else: ast.ModeExpr{Mode: "python", Expr: ast.IdentExpr{Name: "mode", Span: sp}, Span: sp}, Span: sp}, Span: sp}, Span: sp},
		}, Span: sp},
		Span: sp,
	}

	got := refNames(collectExprIdentRefs(expr))
	want := []string{"left", "call", "ns", "member_base", "member_index", "convert", "base", "index", "compare_left", "then", "mode"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ident refs: got=%#v want=%#v", got, want)
	}
}

func TestCollectEvalStringRefsWithTraversesListsAndDefaultBase(t *testing.T) {
	value := eval.Tuple([]eval.Value{
		eval.String("$top"),
		eval.List([]eval.Value{eval.Int(1), eval.String("${nested}")}),
	})
	zeroSpan := diag.NewSpan("vals.jbs", diag.Position{}, diag.Position{})
	setSpan := diag.NewSpan("vals.jbs", diag.NewPos(30, 4, 8), diag.NewPos(40, 4, 18))

	if got := collectEvalStringRefsWith(value, zeroSpan, nil); got != nil {
		t.Fatalf("expected nil for nil collector, got %#v", got)
	}

	type call struct {
		text string
		base diag.Position
	}
	calls := make([]call, 0, 2)
	collector := func(text string, base diag.Position, file string) []varRef {
		calls = append(calls, call{text: text, base: base})
		return []varRef{{Name: text, Span: diag.NewSpan(file, base, base)}}
	}

	got := refNames(collectEvalStringRefsWith(value, zeroSpan, collector))
	want := []string{"$top", "${nested}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected eval refs: got=%#v want=%#v", got, want)
	}
	for i, call := range calls {
		if call.base != (diag.NewPos(0, 1, 1)) {
			t.Fatalf("call %d expected default base, got %+v", i, call.base)
		}
	}

	calls = calls[:0]
	_ = collectEvalStringRefsWith(eval.String("$explicit"), setSpan, collector)
	if len(calls) != 1 || calls[0].base != setSpan.Start {
		t.Fatalf("expected explicit span start to be used, got %#v", calls)
	}
}

func TestParseHelpersAndSanitizeStepName(t *testing.T) {
	if end, ok := parseBareVarName([]rune("abc123 rest"), 0); !ok || end != 6 {
		t.Fatalf("parseBareVarName valid case = (%d,%v), want (6,true)", end, ok)
	}
	if _, ok := parseBareVarName([]rune("1abc"), 0); ok {
		t.Fatalf("parseBareVarName should reject digit start")
	}

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
		{expr: "${}", start: 2, wantOK: false},
		{expr: "${#1}", start: 2, wantOK: false},
		{expr: "${x", start: 2, wantOK: false},
	}
	for _, tc := range tests {
		gotName, gotEnd, gotOK := parseBracedVarRef([]rune(tc.expr), tc.start)
		if gotName != tc.wantName || gotEnd != tc.wantEnd || gotOK != tc.wantOK {
			t.Fatalf("parseBracedVarRef(%q,%d) = (%q,%d,%v), want (%q,%d,%v)", tc.expr, tc.start, gotName, gotEnd, gotOK, tc.wantName, tc.wantEnd, tc.wantOK)
		}
	}

	commentTests := []struct {
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
	for _, tc := range commentTests {
		if got := isCommentStart([]rune(tc.text), tc.idx); got != tc.want {
			t.Fatalf("isCommentStart(%q,%d) = %v, want %v", tc.text, tc.idx, got, tc.want)
		}
	}

	boundaryTests := []struct {
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
	for _, tc := range boundaryTests {
		if got := isShellCommentBoundary(tc.r); got != tc.want {
			t.Fatalf("isShellCommentBoundary(%q) = %v, want %v", tc.r, got, tc.want)
		}
	}

	sanitizeTests := []struct {
		in   string
		want string
	}{
		{in: "", want: "x"},
		{in: "run_step_1", want: "run_step_1"},
		{in: "run-step.1", want: "run_step_1"},
		{in: "***", want: "___"},
	}
	for _, tc := range sanitizeTests {
		if got := sanitizeStepName(tc.in); got != tc.want {
			t.Fatalf("sanitizeStepName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

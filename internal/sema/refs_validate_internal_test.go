package sema

import (
	"reflect"
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
	want := []string{"a", "b", "c", "d", "e"}
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

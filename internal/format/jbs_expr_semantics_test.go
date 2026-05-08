package format

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func formatJBSForTest(t *testing.T, name, src string) string {
	t.Helper()
	var diags diag.Diagnostics
	got, err := JBS(name, src, &diags)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return got
}

func TestJBSFormatsFunctionDefaultsWithoutChangingExpressionSemantics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "single tuple default",
			src:  "f=function(x=(1,)){x}\n",
			want: "f = function(x = (1,)) {\n    x\n}\n",
		},
		{
			name: "grouped binary default",
			src:  "f=function(x=(1 + 2) * 3){x}\n",
			want: "f = function(x = (1 + 2) * 3) {\n    x\n}\n",
		},
		{
			name: "unary grouped default",
			src:  "f=function(x=-(a + b)){x}\n",
			want: "f = function(x = -(a + b)) {\n    x\n}\n",
		},
		{
			name: "conditional grouped default",
			src:  "f=function(x=(a if b else c) + d){x}\n",
			want: "f = function(x = (a if b else c) + d) {\n    x\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatJBSForTest(t, "defaults.jbs", tc.src)
			if got != tc.want {
				t.Fatalf("unexpected formatted output\n--- got ---\n%s--- want ---\n%s", got, tc.want)
			}
		})
	}
}

func TestJBSFormatsNamedCallArgumentsWithoutChangingExpressionSemantics(t *testing.T) {
	src := "f(x=(1,), y=(1 + 2) * 3, z=-(a + b))\n"
	want := "f(x = (1,), y = (1 + 2) * 3, z = -(a + b))\n"
	got := formatJBSForTest(t, "named_args_semantics.jbs", src)
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestJBSFormatsStructuredContainersWithoutChangingSiblingExpressions(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "list grouped sibling",
			src:  "x=[(1 + 2) * 3, f(a=1)]\n",
			want: "x = [(1 + 2) * 3, f(a = 1)]\n",
		},
		{
			name: "tuple grouped sibling",
			src:  "x=((1,), f(a=1))\n",
			want: "x = ((1,), f(a = 1))\n",
		},
		{
			name: "dictionary grouped sibling",
			src:  `x={"a":(1 + 2) * 3, 2:f(a=1)}` + "\n",
			want: "x = {\"a\": (1 + 2) * 3, 2: f(a = 1)}\n",
		},
		{
			name: "dictionary function value",
			src:  `d={"a":function(x){x}}` + "\n",
			want: "d = {\"a\": function(x) { x }}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatJBSForTest(t, "containers.jbs", tc.src)
			if got != tc.want {
				t.Fatalf("unexpected formatted output\n--- got ---\n%s--- want ---\n%s", got, tc.want)
			}
		})
	}
}

func TestFormatExprInlineSpanlessFallbackPreservesSemantics(t *testing.T) {
	t.Run("single tuple", func(t *testing.T) {
		expr := ast.TupleExpr{Items: []ast.Expr{ast.IdentExpr{Name: "x"}}}
		if got := formatExprInline(expr, nil); got != "(x,)" {
			t.Fatalf("single tuple formatted as %q", got)
		}
	})

	t.Run("unary grouped binary", func(t *testing.T) {
		expr := ast.UnaryExpr{
			Op: "-",
			Expr: ast.BinaryExpr{
				Left:  ast.IdentExpr{Name: "a"},
				Op:    "+",
				Right: ast.IdentExpr{Name: "b"},
			},
		}
		if got := formatExprInline(expr, nil); got != "-(a + b)" {
			t.Fatalf("unary expression formatted as %q", got)
		}
	})

	t.Run("conditional grouped under binary", func(t *testing.T) {
		expr := ast.BinaryExpr{
			Left: ast.ConditionalExpr{
				Then: ast.IdentExpr{Name: "a"},
				Cond: ast.IdentExpr{Name: "b"},
				Else: ast.IdentExpr{Name: "c"},
			},
			Op:    "+",
			Right: ast.IdentExpr{Name: "d"},
		}
		if got := formatExprInline(expr, nil); got != "(a if b else c) + d" {
			t.Fatalf("conditional expression formatted as %q", got)
		}
	})

	t.Run("dictionary default", func(t *testing.T) {
		src := `f=function(d={"a": (1,)}){d}` + "\n"
		want := "f = function(d = {\"a\": (1,)}) {\n    d\n}\n"
		if got := formatJBSForTest(t, "dict_default.jbs", src); got != want {
			t.Fatalf("unexpected formatted output\n--- got ---\n%s--- want ---\n%s", got, want)
		}
	})

	t.Run("spanless dictionary", func(t *testing.T) {
		expr := ast.DictExpr{Entries: []ast.DictEntryExpr{{
			Key:   ast.StringExpr{Value: "a"},
			Value: ast.TupleExpr{Items: []ast.Expr{ast.IdentExpr{Name: "x"}}},
		}}}
		if got := formatExprInline(expr, nil); got != "{\"a\": (x,)}" {
			t.Fatalf("dictionary formatted as %q", got)
		}
	})
}

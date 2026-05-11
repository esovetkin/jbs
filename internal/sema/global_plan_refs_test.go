package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
)

func TestGlobalExprReadRefsTracksIndexItems(t *testing.T) {
	expr := ast.IndexExpr{
		Base:  ast.IdentExpr{Name: "cfg"},
		Items: []ast.Expr{ast.IdentExpr{Name: "key"}},
	}
	got := globalExprReadNames(expr)
	want := []string{"cfg", "key"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

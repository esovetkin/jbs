package sema

import (
	"maps"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
)

type globalDependencySnapshot struct {
	Names []string
	Keys  []BindingVersionKey
}

func uniqueSortedBindingVersionKeys(keys []BindingVersionKey) []BindingVersionKey {
	if len(keys) == 0 {
		return nil
	}
	seen := make(map[BindingVersionKey]struct{}, len(keys))
	for _, key := range keys {
		if key == (BindingVersionKey{}) {
			continue
		}
		seen[key] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	out := slices.Collect(maps.Keys(seen))
	slices.SortFunc(out, compareBindingVersionKey)
	return out
}

func (e *globalSeqEngine) previousBindingDependencySnapshot(name string) globalDependencySnapshot {
	if e == nil || name == "" {
		return globalDependencySnapshot{}
	}
	out := globalDependencySnapshot{}
	if key, ok := e.bindingKeyForCurrentName(name); ok {
		out.Keys = append(out.Keys, key)
	}
	if gv := e.globalVars[name]; gv != nil {
		out.Names = append(out.Names, gv.DependsOn...)
		out.Keys = append(out.Keys, gv.DependsOnKeys...)
		out.Keys = append(out.Keys, e.expandGlobalDepKeys(gv.DependsOn, name)...)
	}
	out.Names = uniqueSortedNamesExcept(out.Names, name)
	out.Keys = uniqueSortedBindingVersionKeys(out.Keys)
	return out
}

func exprEvalTimeReadsName(expr ast.Expr, name string) bool {
	if expr == nil || name == "" {
		return false
	}
	switch node := expr.(type) {
	case ast.IdentExpr:
		return node.Name == name
	case ast.QualifiedIdentExpr:
		return node.Namespace == name
	case ast.MemberExpr:
		return exprEvalTimeReadsName(node.Base, name)
	case ast.IndexExpr:
		if exprEvalTimeReadsName(node.Base, name) {
			return true
		}
		return exprListEvalTimeReadsName(node.Items, name)
	case ast.StringExpr, ast.NumberExpr, ast.BoolExpr:
		return false
	case ast.ListExpr:
		return exprListEvalTimeReadsName(node.Items, name)
	case ast.TupleExpr:
		return exprListEvalTimeReadsName(node.Items, name)
	case ast.DictExpr:
		for _, entry := range node.Entries {
			if exprEvalTimeReadsName(entry.Key, name) || exprEvalTimeReadsName(entry.Value, name) {
				return true
			}
		}
		return false
	case ast.RangeExpr:
		return exprEvalTimeReadsName(node.Start, name) ||
			exprEvalTimeReadsName(node.Stop, name) ||
			exprEvalTimeReadsName(node.Step, name)
	case ast.CallExpr:
		if exprEvalTimeReadsName(node.Callee, name) {
			return true
		}
		for _, arg := range node.Args {
			if exprEvalTimeReadsName(arg.Expr, name) {
				return true
			}
		}
		return false
	case ast.FunctionExpr:
		for _, param := range node.Params {
			if exprEvalTimeReadsName(param.Default, name) {
				return true
			}
		}
		return false
	case ast.AliasExpr:
		return exprEvalTimeReadsName(node.Expr, name)
	case ast.UnaryExpr:
		return exprEvalTimeReadsName(node.Expr, name)
	case ast.BinaryExpr:
		return exprEvalTimeReadsName(node.Left, name) || exprEvalTimeReadsName(node.Right, name)
	case ast.CompareExpr:
		return exprEvalTimeReadsName(node.Left, name) || exprEvalTimeReadsName(node.Right, name)
	case ast.ConditionalExpr:
		return exprEvalTimeReadsName(node.Then, name) ||
			exprEvalTimeReadsName(node.Cond, name) ||
			exprEvalTimeReadsName(node.Else, name)
	default:
		return false
	}
}

func exprListEvalTimeReadsName(exprs []ast.Expr, name string) bool {
	for _, expr := range exprs {
		if exprEvalTimeReadsName(expr, name) {
			return true
		}
	}
	return false
}

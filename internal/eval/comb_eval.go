// implement combination algebra logic
//
// i.e. in
// ```jbs
// a = [1,2,3] * 2   # [2,4,6]
// b = ("a","b") * 2 # ("a","b","a","b")
// a */+ b
// ```
// implement only the last row logic
//
// + row-wise merge, * product
//
// where combinations are "rows" carrying variable values and their
// names. This implements cyclic-broadcast logic, inspired by vector
// arithmetic in R
package eval

import (
	"fmt"
	"sort"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type Cell struct {
	Value    Value
	Origin   diag.Span
	Assigned bool
}

type Row struct {
	Values map[string]Cell
}

type CombEvalOptions struct {
	SourceRows map[string][]Row
}

func (r Row) clone() Row {
	m := make(map[string]Cell, len(r.Values))
	for k, v := range r.Values {
		m[k] = v
	}
	return Row{Values: m}
}

func EvalCombination(expr ast.CombExpr, series map[string][]Value, origins map[string]diag.Span, diags *diag.Diagnostics) []Row {
	return EvalCombinationWithOptions(expr, series, origins, CombEvalOptions{}, diags)
}

func EvalCombinationWithOptions(expr ast.CombExpr, series map[string][]Value, origins map[string]diag.Span, opts CombEvalOptions, diags *diag.Diagnostics) []Row {
	if expr == nil {
		diags.AddError(diag.CodeE113, "missing combination expression", diag.Span{}, "add a combination expression to the global assignment")
		return nil
	}
	checkRepeatedIdentifiers(expr, diags)
	return evalComb(expr, series, origins, opts, diags)
}

func checkRepeatedIdentifiers(expr ast.CombExpr, diags *diag.Diagnostics) {
	first := make(map[string]diag.Span)
	walkComb(expr, func(id ast.CombIdent) {
		if id.Name == "" {
			return
		}
		if prev, ok := first[id.Name]; ok {
			diags.AddError(diag.CodeE036,
				fmt.Sprintf("repeated identifier '%s' is not allowed in combination expression", id.Name),
				id.Span,
				"use each identifier at most once in a combination expression",
				diag.RelatedSpan{Message: "first occurrence", Span: prev},
			)
			return
		}
		first[id.Name] = id.Span
	})
}

func walkComb(expr ast.CombExpr, fn func(ast.CombIdent)) {
	switch e := expr.(type) {
	case ast.CombIdent:
		fn(e)
	case ast.CombBinary:
		walkComb(e.Left, fn)
		walkComb(e.Right, fn)
	}
}

func evalComb(expr ast.CombExpr, series map[string][]Value, origins map[string]diag.Span, opts CombEvalOptions, diags *diag.Diagnostics) []Row {
	switch e := expr.(type) {
	case ast.CombIdent:
		if e.Name == "" {
			return nil
		}
		if srcRows, ok := opts.SourceRows[e.Name]; ok {
			return cloneRows(srcRows)
		}
		vals, ok := series[e.Name]
		if ok {
			rows := make([]Row, 0, len(vals))
			origin := e.Span
			if o, exists := origins[e.Name]; exists && !o.IsZero() {
				origin = o
			}
			for _, v := range vals {
				rows = append(rows, Row{Values: map[string]Cell{e.Name: {Value: v, Origin: origin}}})
			}
			return rows
		}
		diags.AddError(diag.CodeE111, fmt.Sprintf("unknown combination identifier '%s'", e.Name), e.Span, "define the variable before final expression")
		return nil
	case ast.CombBinary:
		left := evalComb(e.Left, series, origins, opts, diags)
		right := evalComb(e.Right, series, origins, opts, diags)
		if e.Op == "+" {
			return rowWiseMergeRows(left, right, e, diags)
		}
		if e.Op == "*" {
			return productRows(left, right, e, diags)
		}
		diags.AddError(diag.CodeE112, fmt.Sprintf("unsupported combination operator '%s'", e.Op), e.OpSpan, "use '+' or '*' only")
		return nil
	default:
		diags.AddError(diag.CodeE113, "unsupported combination node", expr.GetSpan(), "check final expression syntax")
		return nil
	}
}

func cloneRows(in []Row) []Row {
	if len(in) == 0 {
		return nil
	}
	out := make([]Row, len(in))
	for i, row := range in {
		out[i] = row.clone()
	}
	return out
}

func rowWiseMergeRows(left, right []Row, op ast.CombBinary, diags *diag.Diagnostics) []Row {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	m := len(left)
	n := len(right)
	k := m
	if n > k {
		k = n
	}
	if m != n {
		lo := m
		hi := n
		if lo > hi {
			lo, hi = hi, lo
		}
		shouldWarn := hi%lo != 0
		if shouldWarn {
			diags.AddWarning(diag.CodeW101,
				fmt.Sprintf("length mismatch in '+': left=%d right=%d; cyclic broadcast to length %d", m, n, k),
				op.OpSpan,
				"align lengths to avoid cyclic broadcast",
			)
		}
	}
	rows := make([]Row, 0, k)
	for i := 0; i < k; i++ {
		merged, ok := mergeRows(left[i%m], right[i%n], op.OpSpan, diags)
		if !ok {
			continue
		}
		rows = append(rows, merged)
	}
	return rows
}

func productRows(left, right []Row, op ast.CombBinary, diags *diag.Diagnostics) []Row {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rows := make([]Row, 0, len(left)*len(right))
	for _, l := range left {
		for _, r := range right {
			merged, ok := mergeRows(l, r, op.OpSpan, diags)
			if !ok {
				continue
			}
			rows = append(rows, merged)
		}
	}
	return rows
}

func mergeRows(a, b Row, at diag.Span, diags *diag.Diagnostics) (Row, bool) {
	out := a.clone()
	for name, cell := range b.Values {
		if existing, ok := out.Values[name]; ok {
			if !Equal(existing.Value, cell.Value) {
				diags.AddError(diag.CodeE042,
					fmt.Sprintf("conflicting values for '%s' during row merge", name),
					at,
					"avoid conflicting assignments in combined expressions",
					diag.RelatedSpan{Message: "left value origin", Span: existing.Origin},
					diag.RelatedSpan{Message: "right value origin", Span: cell.Origin},
				)
				return Row{}, false
			}
			continue
		}
		out.Values[name] = cell
	}
	return out, true
}

func RowVariableNames(rows []Row) []string {
	if len(rows) == 0 {
		return nil
	}
	set := make(map[string]struct{})
	for _, row := range rows {
		for name := range row.Values {
			set[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

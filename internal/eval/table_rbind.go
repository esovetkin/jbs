package eval

import (
	"fmt"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalRbindValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	bound, ok := bindBuiltinArgs("rbind", args, builtinSignature{
		Name:          "rbind",
		Varargs:       "tables",
		NamedVarargs:  true,
		AllowNoArgs:   true,
		MinPositional: 1,
	}, at, diags)
	if !ok {
		return Null()
	}
	return rbindTables(bound.Varargs, at, diags)
}

func rbindTables(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	tables := make([]Value, 0, len(args))
	spans := make([]diag.Span, 0, len(args))
	totalRows := 0
	for i, arg := range args {
		if !IsComb(arg.Value) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("rbind() argument %d must be a table value", i+1), nonZeroSpan(arg.Span, at), "pass table values only")
			return Null()
		}
		tables = append(tables, arg.Value)
		spans = append(spans, nonZeroSpan(arg.Span, at))
		totalRows += len(arg.Value.C.Rows)
	}

	baseOrder := CombNames(tables[0])
	rows := make([]Row, 0, totalRows)
	for i, table := range tables {
		names := CombNames(table)
		if !sameColumnSet(baseOrder, names) {
			missing, extra := columnSetDiff(baseOrder, names)
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("rbind() argument %d columns do not match argument 1", i+1),
				spans[i],
				fmt.Sprintf("missing: [%s]; extra: [%s]", strings.Join(missing, ", "), strings.Join(extra, ", ")),
			)
			return Null()
		}
		for _, row := range table.C.Rows {
			out, ok := cloneRowForOrder("rbind", row, baseOrder, spans[i], diags)
			if !ok {
				return Null()
			}
			rows = append(rows, out)
		}
	}

	return CombValue(&Comb{
		Order: append([]string(nil), baseOrder...),
		Rows:  rows,
	})
}

func sameColumnSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := stringSet(a)
	for _, name := range b {
		if _, ok := seen[name]; !ok {
			return false
		}
	}
	return true
}

func columnSetDiff(base, got []string) (missing, extra []string) {
	baseSet := stringSet(base)
	gotSet := stringSet(got)
	for _, name := range base {
		if _, ok := gotSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	for _, name := range got {
		if _, ok := baseSet[name]; !ok {
			extra = append(extra, name)
		}
	}
	return missing, extra
}

func stringSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

func cloneRowForOrder(caller string, row Row, order []string, at diag.Span, diags *diag.Diagnostics) (Row, bool) {
	out := Row{Values: make(map[string]Cell, len(order))}
	for _, name := range order {
		cell, ok := row.Values[name]
		if !ok {
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() could not read table column '%s'", caller, name), at, "use well-formed table values")
			return Row{}, false
		}
		cell.Value = CloneValue(cell.Value)
		out.Values[name] = cell
	}
	return out, true
}

package sema

import (
	"maps"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func resolveTopLevelGlobals(prog ast.Program, defaults map[string]eval.Value, diags *diag.Diagnostics) GlobalState {
	exec := execGlobalPlan(buildGlobalPlan(prog, defaults, baseDirForProgramFile(prog.File), diags), defaults, defaults, diags)
	return GlobalState{
		Values: maps.Clone(exec.ScalarGlobals.Values),
		Modes:  maps.Clone(exec.ScalarGlobals.Modes),
		Spans:  maps.Clone(exec.ScalarGlobals.Spans),
	}
}

func isScalarGlobalValue(v eval.Value) bool {
	switch v.Kind {
	case eval.KindString, eval.KindInt, eval.KindFloat, eval.KindBool, eval.KindNull:
		return true
	default:
		return false
	}
}

func hasNestedList(v eval.Value) bool {
	if v.Kind != eval.KindList && v.Kind != eval.KindTuple {
		return false
	}
	for _, item := range v.L {
		if item.Kind == eval.KindList || item.Kind == eval.KindTuple {
			return true
		}
	}
	return false
}

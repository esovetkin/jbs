package sema

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func compileGlobalsForTest(t *testing.T, prog ast.Program, builtins map[string]eval.Value, diags *diag.Diagnostics) (map[string]*GlobalVar, []string) {
	t.Helper()
	exec := execGlobalPlan(
		buildGlobalPlan(prog, builtins, baseDirForProgramFile(prog.File)),
		builtins,
		builtins,
		diags,
	)
	return globalVarsFromExec(exec)
}

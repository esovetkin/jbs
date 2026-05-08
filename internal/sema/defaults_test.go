package sema

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestBuiltinGlobalValuesIncludesAnalyseDatabase(t *testing.T) {
	defaults := BuiltinGlobalValues()
	value, ok := defaults["jbs_database"]
	if !ok {
		t.Fatalf("missing jbs_database default")
	}
	if !eval.Equal(value, eval.String("")) {
		t.Fatalf("jbs_database default = %#v, want empty string", value)
	}
	if !isBuiltinGlobalName("jbs_database") {
		t.Fatalf("jbs_database should be a built-in global")
	}
}

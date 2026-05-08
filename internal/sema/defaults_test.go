package sema

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestBuiltinGlobalValuesIncludesRuntimeDefaults(t *testing.T) {
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
	value, ok = defaults["jbs_benchmarks"]
	if !ok {
		t.Fatalf("missing jbs_benchmarks default")
	}
	if value.Kind != eval.KindDict || value.D == nil || len(value.D.Order) != 0 {
		t.Fatalf("jbs_benchmarks default = %#v, want empty dictionary", value)
	}
	if !isBuiltinGlobalName("jbs_benchmarks") {
		t.Fatalf("jbs_benchmarks should be a built-in global")
	}
}

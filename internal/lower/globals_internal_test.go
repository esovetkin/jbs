package lower

import (
	"testing"

	"jbs/internal/eval"
)

func TestBuiltinGlobalValues(t *testing.T) {
	got := BuiltinGlobalValues()
	if len(got) != 3 {
		t.Fatalf("expected 3 builtin globals, got %d (%#v)", len(got), got)
	}
	if !eval.Equal(got["jbs_name"], eval.String("jbs_benchmark")) {
		t.Fatalf("unexpected jbs_name default: %#v", got["jbs_name"])
	}
	if !eval.Equal(got["jbs_outpath"], eval.String("out")) {
		t.Fatalf("unexpected jbs_outpath default: %#v", got["jbs_outpath"])
	}
	if !eval.Equal(got["jbs_comment"], eval.String("")) {
		t.Fatalf("unexpected jbs_comment default: %#v", got["jbs_comment"])
	}

	// Ensure callers get a fresh map each time.
	got["jbs_name"] = eval.String("mutated")
	again := BuiltinGlobalValues()
	if !eval.Equal(again["jbs_name"], eval.String("jbs_benchmark")) {
		t.Fatalf("expected BuiltinGlobalValues to return fresh defaults, got %#v", again["jbs_name"])
	}
}

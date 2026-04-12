package lower

import (
	"testing"

	"jbs/internal/eval"
	"jbs/internal/sema"
)

func TestGlobalString(t *testing.T) {
	tests := []struct {
		name     string
		globals  sema.GlobalState
		key      string
		fallback string
		want     string
	}{
		{
			name:     "missing key uses fallback",
			globals:  sema.GlobalState{Values: map[string]eval.Value{}},
			key:      "jbs_name",
			fallback: "jbs_benchmark",
			want:     "jbs_benchmark",
		},
		{
			name:     "nil map uses fallback",
			globals:  sema.GlobalState{},
			key:      "jbs_outpath",
			fallback: "out",
			want:     "out",
		},
		{
			name: "string value returns raw string",
			globals: sema.GlobalState{Values: map[string]eval.Value{
				"jbs_name": eval.String("demo"),
			}},
			key:      "jbs_name",
			fallback: "jbs_benchmark",
			want:     "demo",
		},
		{
			name: "empty string value does not fallback",
			globals: sema.GlobalState{Values: map[string]eval.Value{
				"jbs_comment": eval.String(""),
			}},
			key:      "jbs_comment",
			fallback: "fallback-comment",
			want:     "",
		},
		{
			name: "non-string scalar stringifies",
			globals: sema.GlobalState{Values: map[string]eval.Value{
				"jbs_outpath": eval.Int(42),
			}},
			key:      "jbs_outpath",
			fallback: "out",
			want:     "42",
		},
		{
			name: "non-string blank stringifies to fallback",
			globals: sema.GlobalState{Values: map[string]eval.Value{
				"jbs_comment": eval.Null(),
			}},
			key:      "jbs_comment",
			fallback: "comment-fallback",
			want:     "comment-fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := globalString(tt.globals, tt.key, tt.fallback); got != tt.want {
				t.Fatalf("globalString(..., %q, %q)=%q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}

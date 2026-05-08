package benchmarks

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestFromValueDefaultEmptyConfig(t *testing.T) {
	cfg, problems := FromValue(eval.Value{}, nil)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %#v", problems)
	}
	if cfg.Configured || len(cfg.Specs) != 0 || len(cfg.ByName) != 0 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestFromValueStringAndListValues(t *testing.T) {
	value := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "small"}, Value: eval.String("analyse_small")},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "large"}, Value: eval.List([]eval.Value{
			eval.String("analyse_large"),
			eval.String("summary"),
			eval.String("summary"),
		})},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "tuple"}, Value: eval.Tuple([]eval.Value{eval.String("analyse_tuple")})},
	})
	cfg, problems := FromValue(value, nil)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %#v", problems)
	}
	if !cfg.Configured || len(cfg.Specs) != 3 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if got := cfg.ByName["small"].Analyses; len(got) != 1 || got[0] != "analyse_small" {
		t.Fatalf("small analyses = %#v", got)
	}
	if got := cfg.ByName["large"].Analyses; len(got) != 2 || got[0] != "analyse_large" || got[1] != "summary" {
		t.Fatalf("large analyses = %#v", got)
	}
	if got := cfg.ByName["tuple"].Analyses; len(got) != 1 || got[0] != "analyse_tuple" {
		t.Fatalf("tuple analyses = %#v", got)
	}
}

func TestFromValueRejectsInvalidShapes(t *testing.T) {
	cases := []struct {
		name string
		in   eval.Value
		want string
	}{
		{
			name: "non_dictionary",
			in:   eval.String("bad"),
			want: "must be a dictionary",
		},
		{
			name: "non_string_key",
			in: eval.DictValue([]eval.DictEntry{
				{Key: eval.DictKey{Kind: eval.DictKeyInt, I: 1}, Value: eval.String("analyse")},
			}),
			want: "key must be a string",
		},
		{
			name: "non_string_value",
			in: eval.DictValue([]eval.DictEntry{
				{Key: eval.DictKey{Kind: eval.DictKeyString, S: "bench"}, Value: eval.Int(1)},
			}),
			want: "must be a string or a list of strings",
		},
		{
			name: "non_string_list_item",
			in: eval.DictValue([]eval.DictEntry{
				{Key: eval.DictKey{Kind: eval.DictKeyString, S: "bench"}, Value: eval.List([]eval.Value{eval.Int(1)})},
			}),
			want: "analyse names must be strings",
		},
		{
			name: "empty_list",
			in: eval.DictValue([]eval.DictEntry{
				{Key: eval.DictKey{Kind: eval.DictKeyString, S: "bench"}, Value: eval.List(nil)},
			}),
			want: "must list at least one analyse block",
		},
		{
			name: "empty_analyse_name",
			in: eval.DictValue([]eval.DictEntry{
				{Key: eval.DictKey{Kind: eval.DictKeyString, S: "bench"}, Value: eval.String(" ")},
			}),
			want: "empty analyse name",
		},
		{
			name: "invalid_name",
			in: eval.DictValue([]eval.DictEntry{
				{Key: eval.DictKey{Kind: eval.DictKeyString, S: "..."},
					Value: eval.String("analyse")},
			}),
			want: "valid directory name",
		},
		{
			name: "duplicate_dir",
			in: eval.DictValue([]eval.DictEntry{
				{Key: eval.DictKey{Kind: eval.DictKeyString, S: "a b"}, Value: eval.String("a")},
				{Key: eval.DictKey{Kind: eval.DictKeyString, S: "a_b"}, Value: eval.String("b")},
			}),
			want: "both map to directory",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, problems := FromValue(tc.in, nil)
			if len(problems) == 0 {
				t.Fatalf("expected problem")
			}
			if !strings.Contains(problems[0].Message, tc.want) {
				t.Fatalf("problem = %#v, want substring %q", problems, tc.want)
			}
		})
	}
}

func TestSafeComponent(t *testing.T) {
	cases := map[string]string{
		"a b":       "a_b",
		"../bad":    "bad",
		"case.name": "case.name",
		"---":       "",
	}
	for in, want := range cases {
		if got := SafeComponent(in); got != want {
			t.Fatalf("SafeComponent(%q) = %q, want %q", in, got, want)
		}
	}
}

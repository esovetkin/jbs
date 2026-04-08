package cli

import "testing"

func mustParseFlags(t *testing.T, args []string) Flags {
	t.Helper()
	f, err := ParseFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v (args=%v)", err, args)
	}
	return f
}

func TestParseFlagsCompileModeCases(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantInput  string
		wantOutput string
		wantCheck  bool
	}{
		{
			name:       "defaults",
			args:       []string{"input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantCheck:  false,
		},
		{
			name:       "check_and_output",
			args:       []string{"--check", "-o", "JUBE.yaml", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "JUBE.yaml",
			wantCheck:  true,
		},
		{
			name:       "short_check_and_output",
			args:       []string{"-c", "-o", "JUBE.yaml", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "JUBE.yaml",
			wantCheck:  true,
		},
		{
			name:       "output_after_input",
			args:       []string{"input.jbs", "-o", "JUBE.yaml"},
			wantInput:  "input.jbs",
			wantOutput: "JUBE.yaml",
			wantCheck:  false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := mustParseFlags(t, tc.args)
			if f.Input != tc.wantInput {
				t.Fatalf("unexpected input: got=%q want=%q", f.Input, tc.wantInput)
			}
			if f.Output != tc.wantOutput {
				t.Fatalf("unexpected output: got=%q want=%q", f.Output, tc.wantOutput)
			}
			if f.Check != tc.wantCheck {
				t.Fatalf("unexpected check flag: got=%v want=%v", f.Check, tc.wantCheck)
			}
		})
	}
}

func TestParseFlagsNoArgMode(t *testing.T) {
	f := mustParseFlags(t, nil)
	if !f.Help || f.HelpGlobals {
		t.Fatalf("expected no-arg mode to select general help")
	}
}

func TestParseFlagsHelpTopics(t *testing.T) {
	cases := []struct {
		topic string
		check func(Flags) bool
	}{
		{topic: "globals", check: func(f Flags) bool { return f.HelpGlobals }},
		{topic: "do", check: func(f Flags) bool { return f.HelpDo }},
		{topic: "analyse", check: func(f Flags) bool { return f.HelpAnalyse }},
		{topic: "let", check: func(f Flags) bool { return f.HelpLet }},
		{topic: "submit", check: func(f Flags) bool { return f.HelpSubmit }},
		{topic: "use", check: func(f Flags) bool { return f.HelpUse }},
		{topic: "param", check: func(f Flags) bool { return f.HelpParam }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.topic, func(t *testing.T) {
			f := mustParseFlags(t, []string{"help", tc.topic})
			if !f.Help || !tc.check(f) {
				t.Fatalf("expected help mode for topic %q", tc.topic)
			}
		})
	}
}

func TestParseFlagsEmbedModes(t *testing.T) {
	cases := []struct {
		name          string
		args          []string
		wantEmbed     bool
		wantEmbedName string
	}{
		{
			name:          "embed_list",
			args:          []string{"embed"},
			wantEmbed:     true,
			wantEmbedName: "",
		},
		{
			name:          "embed_name",
			args:          []string{"embed", "jsc"},
			wantEmbed:     true,
			wantEmbedName: "jsc",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := mustParseFlags(t, tc.args)
			if f.Embed != tc.wantEmbed || f.EmbedName != tc.wantEmbedName {
				t.Fatalf("unexpected embed flags: got=%#v", f)
			}
		})
	}
}

func TestParseFlagsFmtMode(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantInput  string
		wantStrict bool
	}{
		{
			name:       "default",
			args:       []string{"fmt", "input.jbs"},
			wantInput:  "input.jbs",
			wantStrict: false,
		},
		{
			name:       "long_before",
			args:       []string{"fmt", "--strict", "input.jbs"},
			wantInput:  "input.jbs",
			wantStrict: true,
		},
		{
			name:       "long_after",
			args:       []string{"fmt", "input.jbs", "--strict"},
			wantInput:  "input.jbs",
			wantStrict: true,
		},
		{
			name:       "short_before",
			args:       []string{"fmt", "-s", "input.jbs"},
			wantInput:  "input.jbs",
			wantStrict: true,
		},
		{
			name:       "short_after",
			args:       []string{"fmt", "input.jbs", "-s"},
			wantInput:  "input.jbs",
			wantStrict: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := mustParseFlags(t, tc.args)
			if !f.Fmt {
				t.Fatalf("expected fmt mode")
			}
			if f.Input != tc.wantInput {
				t.Fatalf("unexpected input: got=%q want=%q", f.Input, tc.wantInput)
			}
			if f.FmtStrict != tc.wantStrict {
				t.Fatalf("unexpected strict flag: got=%v want=%v", f.FmtStrict, tc.wantStrict)
			}
		})
	}
}

func TestParseFlagsPrintParamModes(t *testing.T) {
	cases := []struct {
		name          string
		args          []string
		wantType      string
		wantOutput    string
		wantInput     string
		wantPrintMode bool
	}{
		{
			name:          "defaults",
			args:          []string{"printparam", "input.jbs"},
			wantType:      "pretty",
			wantOutput:    "-",
			wantInput:     "input.jbs",
			wantPrintMode: true,
		},
		{
			name:          "explicit_pretty",
			args:          []string{"printparam", "--type", "pretty", "input.jbs"},
			wantType:      "pretty",
			wantOutput:    "-",
			wantInput:     "input.jbs",
			wantPrintMode: true,
		},
		{
			name:          "csv",
			args:          []string{"printparam", "-t=csv", "input.jbs"},
			wantType:      "csv",
			wantOutput:    "-",
			wantInput:     "input.jbs",
			wantPrintMode: true,
		},
		{
			name:          "custom_output",
			args:          []string{"printparam", "--output", "out.txt", "input.jbs"},
			wantType:      "pretty",
			wantOutput:    "out.txt",
			wantInput:     "input.jbs",
			wantPrintMode: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := mustParseFlags(t, tc.args)
			if f.PrintParam != tc.wantPrintMode {
				t.Fatalf("unexpected printparam mode: got=%v want=%v", f.PrintParam, tc.wantPrintMode)
			}
			if f.PrintType != tc.wantType {
				t.Fatalf("unexpected print type: got=%q want=%q", f.PrintType, tc.wantType)
			}
			if f.Output != tc.wantOutput {
				t.Fatalf("unexpected output: got=%q want=%q", f.Output, tc.wantOutput)
			}
			if f.Input != tc.wantInput {
				t.Fatalf("unexpected input: got=%q want=%q", f.Input, tc.wantInput)
			}
		})
	}
}

func TestParseFlagsErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "help_unknown_topic", args: []string{"help", "badtopic"}},
		{name: "fmt_missing_file", args: []string{"fmt"}},
		{name: "fmt_rejects_option", args: []string{"fmt", "-o", "x.jbs"}},
		{name: "fmt_rejects_check", args: []string{"fmt", "-c"}},
		{name: "fmt_duplicate_long_strict", args: []string{"fmt", "--strict", "--strict", "input.jbs"}},
		{name: "fmt_duplicate_short_strict", args: []string{"fmt", "-s", "-s", "input.jbs"}},
		{name: "fmt_duplicate_mixed_strict", args: []string{"fmt", "--strict", "-s", "input.jbs"}},
		{name: "printparam_rejects_check", args: []string{"printparam", "--check", "input.jbs"}},
		{name: "printparam_bad_type", args: []string{"printparam", "-t", "json", "input.jbs"}},
		{name: "printparam_missing_input", args: []string{"printparam", "-t", "pretty"}},
		{name: "too_many_args", args: []string{"a.jbs", "b.jbs"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseFlags(tc.args); err == nil {
				t.Fatalf("expected usage error for args=%v", tc.args)
			}
		})
	}
}

package cli

import (
	"testing"

	helpdocs "jbs/docs"
)

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
	if !f.Repl || f.Help || f.HelpTopic != "" {
		t.Fatalf("expected no-arg mode to select repl")
	}
}

func TestParseFlagsReplMode(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "repl_command", args: []string{"repl"}},
		{name: "repl_with_extra", args: []string{"repl", "extra"}, wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f, err := ParseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected usage error for args=%v", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !f.Repl {
				t.Fatalf("expected repl mode")
			}
		})
	}
}

func TestParseFlagsHelpTopics(t *testing.T) {
	for _, topic := range knownHelpTopics {
		topic := topic
		t.Run(topic, func(t *testing.T) {
			f := mustParseFlags(t, []string{"help", topic})
			if !f.Help {
				t.Fatalf("expected help mode for topic %q", topic)
			}
			if f.HelpTopic != topic {
				t.Fatalf("unexpected help topic: got=%q want=%q", f.HelpTopic, topic)
			}
		})
	}
}

func TestParseFlagsHelpCommandForms(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantHelp  bool
		wantTopic string
		wantErr   bool
	}{
		{
			name:      "help_without_topic",
			args:      []string{"help"},
			wantHelp:  true,
			wantTopic: "",
		},
		{
			name:      "help_with_valid_topic",
			args:      []string{"help", "do"},
			wantHelp:  true,
			wantTopic: "do",
		},
		{
			name:    "help_with_unknown_topic",
			args:    []string{"help", "bad"},
			wantErr: true,
		},
		{
			name:    "help_with_extra_argument",
			args:    []string{"help", "do", "extra"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f, err := ParseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected usage error for args=%v", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Help != tc.wantHelp {
				t.Fatalf("unexpected help flag: got=%v want=%v", f.Help, tc.wantHelp)
			}
			if f.HelpTopic != tc.wantTopic {
				t.Fatalf("unexpected help topic: got=%q want=%q", f.HelpTopic, tc.wantTopic)
			}
		})
	}
}

func TestKnownHelpTopicsExistInDocs(t *testing.T) {
	for _, topic := range knownHelpTopics {
		if _, err := helpdocs.Page(topic); err != nil {
			t.Fatalf("help topic %q has no docs page: %v", topic, err)
		}
	}
}

func TestHelpUsageTextMatchesTopicRegistry(t *testing.T) {
	want := "usage: jbs help [" + helpUsageTopics() + "]"
	if got := helpUsageMessage(); got != want {
		t.Fatalf("unexpected help usage message: got=%q want=%q", got, want)
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
		{name: "repl_extra_argument", args: []string{"repl", "extra"}},
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

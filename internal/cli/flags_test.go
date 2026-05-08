package cli

import (
	"slices"
	"testing"

	helpdocs "gitlab.jsc.fz-juelich.de/sdlaml/jbs/docs"
)

func mustParseFlags(t *testing.T, args []string) Flags {
	t.Helper()
	f, err := ParseFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v (args=%v)", err, args)
	}
	return f
}

func TestParseFlagsDefaultRunAndCheckCases(t *testing.T) {
	cases := []struct {
		name                  string
		args                  []string
		wantInput             string
		wantOutput            string
		wantCheck             bool
		wantRun               bool
		wantArchive           bool
		wantFWait             bool
		wantFWaitExitExisting bool
		wantFWaitPaths        []string
		wantDryRun            bool
		wantNoStrict          bool
		wantBenchmark         string
	}{
		{
			name:       "defaults",
			args:       []string{"input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantCheck:  false,
			wantRun:    true,
		},
		{
			name:       "check",
			args:       []string{"--check", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantCheck:  true,
		},
		{
			name:       "short_check",
			args:       []string{"-c", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantCheck:  true,
		},
		{
			name:       "run_command",
			args:       []string{"run", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantCheck:  false,
			wantRun:    true,
		},
		{
			name:        "archive_command",
			args:        []string{"archive", "input.jbs"},
			wantInput:   "input.jbs",
			wantOutput:  "-",
			wantArchive: true,
		},
		{
			name:           "fwait_command",
			args:           []string{"fwait", "done.flag"},
			wantOutput:     "-",
			wantFWait:      true,
			wantFWaitPaths: []string{"done.flag"},
		},
		{
			name:           "fwait_multiple_files",
			args:           []string{"fwait", "a", "b"},
			wantOutput:     "-",
			wantFWait:      true,
			wantFWaitPaths: []string{"a", "b"},
		},
		{
			name:                  "fwait_exit_existing",
			args:                  []string{"fwait", "-e", "a"},
			wantOutput:            "-",
			wantFWait:             true,
			wantFWaitExitExisting: true,
			wantFWaitPaths:        []string{"a"},
		},
		{
			name:                  "fwait_exit_existing_after_path",
			args:                  []string{"fwait", "a", "-e", "b"},
			wantOutput:            "-",
			wantFWait:             true,
			wantFWaitExitExisting: true,
			wantFWaitPaths:        []string{"a", "b"},
		},
		{
			name:       "run_command_dry_run_long",
			args:       []string{"run", "--dry-run", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantRun:    true,
			wantDryRun: true,
		},
		{
			name:       "run_command_dry_run_short",
			args:       []string{"run", "-n", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantRun:    true,
			wantDryRun: true,
		},
		{
			name:       "run_command_dry_run_short_after_input",
			args:       []string{"run", "input.jbs", "-n"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantRun:    true,
			wantDryRun: true,
		},
		{
			name:         "run_command_dry_run_no_strict",
			args:         []string{"run", "--no-strict", "-n", "input.jbs"},
			wantInput:    "input.jbs",
			wantOutput:   "-",
			wantRun:      true,
			wantDryRun:   true,
			wantNoStrict: true,
		},
		{
			name:         "run_command_no_strict_after_input",
			args:         []string{"run", "input.jbs", "--no-strict"},
			wantInput:    "input.jbs",
			wantOutput:   "-",
			wantRun:      true,
			wantNoStrict: true,
		},
		{
			name:         "run_command_no_strict_before_input",
			args:         []string{"run", "--no-strict", "input.jbs"},
			wantInput:    "input.jbs",
			wantOutput:   "-",
			wantRun:      true,
			wantNoStrict: true,
		},
		{
			name:          "run_command_benchmark_long",
			args:          []string{"run", "--benchmark", "small", "input.jbs"},
			wantInput:     "input.jbs",
			wantOutput:    "-",
			wantRun:       true,
			wantBenchmark: "small",
		},
		{
			name:          "run_command_benchmark_long_equals",
			args:          []string{"run", "--benchmark=small", "input.jbs"},
			wantInput:     "input.jbs",
			wantOutput:    "-",
			wantRun:       true,
			wantBenchmark: "small",
		},
		{
			name:          "run_command_benchmark_short",
			args:          []string{"run", "-b", "small", "input.jbs"},
			wantInput:     "input.jbs",
			wantOutput:    "-",
			wantRun:       true,
			wantBenchmark: "small",
		},
		{
			name:          "run_command_benchmark_short_equals",
			args:          []string{"run", "-b=small", "input.jbs"},
			wantInput:     "input.jbs",
			wantOutput:    "-",
			wantRun:       true,
			wantBenchmark: "small",
		},
		{
			name:          "continue_benchmark",
			args:          []string{"continue", "-b", "small", "input.jbs"},
			wantInput:     "input.jbs",
			wantOutput:    "-",
			wantBenchmark: "small",
		},
		{
			name:       "default_run_dry_run_short_before_input",
			args:       []string{"-n", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantRun:    true,
			wantDryRun: true,
		},
		{
			name:       "default_run_dry_run_short_after_input",
			args:       []string{"input.jbs", "-n"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantRun:    true,
			wantDryRun: true,
		},
		{
			name:       "default_run_dry_run_long_before_input",
			args:       []string{"--dry-run", "input.jbs"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantRun:    true,
			wantDryRun: true,
		},
		{
			name:       "default_run_dry_run_long_after_input",
			args:       []string{"input.jbs", "--dry-run"},
			wantInput:  "input.jbs",
			wantOutput: "-",
			wantRun:    true,
			wantDryRun: true,
		},
		{
			name:         "default_run_no_strict_after_input",
			args:         []string{"input.jbs", "--no-strict"},
			wantInput:    "input.jbs",
			wantOutput:   "-",
			wantRun:      true,
			wantNoStrict: true,
		},
		{
			name:         "default_run_no_strict_before_input",
			args:         []string{"--no-strict", "input.jbs"},
			wantInput:    "input.jbs",
			wantOutput:   "-",
			wantRun:      true,
			wantNoStrict: true,
		},
		{
			name:          "default_run_benchmark_before_input",
			args:          []string{"--benchmark", "small", "input.jbs"},
			wantInput:     "input.jbs",
			wantOutput:    "-",
			wantRun:       true,
			wantBenchmark: "small",
		},
		{
			name:          "default_run_benchmark_after_input",
			args:          []string{"input.jbs", "-b=small"},
			wantInput:     "input.jbs",
			wantOutput:    "-",
			wantRun:       true,
			wantBenchmark: "small",
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
			if f.Run != tc.wantRun {
				t.Fatalf("unexpected run flag: got=%v want=%v", f.Run, tc.wantRun)
			}
			if f.Archive != tc.wantArchive {
				t.Fatalf("unexpected archive flag: got=%v want=%v", f.Archive, tc.wantArchive)
			}
			if f.FWait != tc.wantFWait {
				t.Fatalf("unexpected fwait flag: got=%v want=%v", f.FWait, tc.wantFWait)
			}
			if f.FWaitExitExisting != tc.wantFWaitExitExisting {
				t.Fatalf("unexpected fwait exit-existing flag: got=%v want=%v", f.FWaitExitExisting, tc.wantFWaitExitExisting)
			}
			if !slices.Equal(f.FWaitPaths, tc.wantFWaitPaths) {
				t.Fatalf("unexpected fwait paths: got=%v want=%v", f.FWaitPaths, tc.wantFWaitPaths)
			}
			if f.DryRun != tc.wantDryRun {
				t.Fatalf("unexpected dry-run flag: got=%v want=%v", f.DryRun, tc.wantDryRun)
			}
			if f.NoStrict != tc.wantNoStrict {
				t.Fatalf("unexpected no-strict flag: got=%v want=%v", f.NoStrict, tc.wantNoStrict)
			}
			if f.Benchmark != tc.wantBenchmark {
				t.Fatalf("unexpected benchmark flag: got=%q want=%q", f.Benchmark, tc.wantBenchmark)
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
		{name: "top_level_output_removed", args: []string{"-o", "out.yaml", "input.jbs"}},
		{name: "run_duplicate_dry_run", args: []string{"run", "-n", "--dry-run", "input.jbs"}},
		{name: "run_dry_run_missing_input", args: []string{"run", "--dry-run"}},
		{name: "run_duplicate_no_strict", args: []string{"run", "--no-strict", "--no-strict", "input.jbs"}},
		{name: "run_no_strict_missing_input", args: []string{"run", "--no-strict"}},
		{name: "run_duplicate_benchmark", args: []string{"run", "-b", "small", "--benchmark", "large", "input.jbs"}},
		{name: "run_benchmark_missing_value", args: []string{"run", "-b"}},
		{name: "run_benchmark_empty_value", args: []string{"run", "--benchmark=", "input.jbs"}},
		{name: "run_rejects_option", args: []string{"run", "-o", "out.yaml", "input.jbs"}},
		{name: "continue_rejects_no_strict", args: []string{"continue", "input.jbs", "--no-strict"}},
		{name: "continue_rejects_dry_run", args: []string{"continue", "input.jbs", "-n"}},
		{name: "continue_rejects_option", args: []string{"continue", "-o", "out.yaml", "input.jbs"}},
		{name: "continue_duplicate_benchmark", args: []string{"continue", "-b", "small", "-b", "large", "input.jbs"}},
		{name: "continue_benchmark_missing_value", args: []string{"continue", "-b"}},
		{name: "archive_missing_input", args: []string{"archive"}},
		{name: "archive_extra_argument", args: []string{"archive", "input.jbs", "extra"}},
		{name: "archive_rejects_option", args: []string{"archive", "-o", "out.tar.gz", "input.jbs"}},
		{name: "fwait_missing_input", args: []string{"fwait"}},
		{name: "fwait_exit_existing_missing_input", args: []string{"fwait", "-e"}},
		{name: "fwait_rejects_option", args: []string{"fwait", "--timeout", "1", "a"}},
		{name: "check_rejects_no_strict", args: []string{"--check", "input.jbs", "--no-strict"}},
		{name: "check_rejects_dry_run", args: []string{"--check", "-n", "input.jbs"}},
		{name: "check_rejects_benchmark", args: []string{"--check", "-b", "small", "input.jbs"}},
		{name: "help_rejects_no_strict", args: []string{"--help", "--no-strict"}},
		{name: "help_rejects_dry_run", args: []string{"--help", "-n"}},
		{name: "help_rejects_benchmark", args: []string{"--help", "-b", "small"}},
		{name: "default_duplicate_dry_run", args: []string{"-n", "--dry-run", "input.jbs"}},
		{name: "default_dry_run_missing_input", args: []string{"-n"}},
		{name: "default_duplicate_no_strict", args: []string{"--no-strict", "--no-strict", "input.jbs"}},
		{name: "default_no_strict_missing_input", args: []string{"--no-strict"}},
		{name: "default_duplicate_benchmark", args: []string{"-b", "small", "-b", "large", "input.jbs"}},
		{name: "default_benchmark_missing_input", args: []string{"-b", "small"}},
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

package cli

import "testing"

func TestParseFlagsDefaults(t *testing.T) {
	f, err := ParseFlags([]string{"input.jbs"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Input != "input.jbs" {
		t.Fatalf("unexpected input: %s", f.Input)
	}
	if f.Output != "-" {
		t.Fatalf("expected default output '-', got %q", f.Output)
	}
	if f.Check {
		t.Fatalf("expected check=false by default")
	}
}

func TestParseFlagsCheckAndOutput(t *testing.T) {
	f, err := ParseFlags([]string{"--check", "-o", "JUBE.yaml", "input.jbs"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Check {
		t.Fatalf("expected check mode")
	}
	if f.Output != "JUBE.yaml" {
		t.Fatalf("unexpected output %q", f.Output)
	}
}

func TestParseFlagsOutputAfterInput(t *testing.T) {
	f, err := ParseFlags([]string{"input.jbs", "-o", "JUBE.yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Output != "JUBE.yaml" {
		t.Fatalf("expected output from trailing -o, got %q", f.Output)
	}
}

func TestParseFlagsNoArgMode(t *testing.T) {
	f, err := ParseFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Help || f.HelpGlobals {
		t.Fatalf("expected no-arg mode to select general help")
	}
}

func TestParseFlagsHelpGlobals(t *testing.T) {
	f, err := ParseFlags([]string{"help", "globals"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Help || !f.HelpGlobals {
		t.Fatalf("expected help globals mode")
	}
}

func TestParseFlagsHelpUnknownSubcommand(t *testing.T) {
	if _, err := ParseFlags([]string{"help", "badtopic"}); err == nil {
		t.Fatalf("expected usage error for unknown help subcommand")
	}
}

func TestParseFlagsTooManyArgs(t *testing.T) {
	_, err := ParseFlags([]string{"a.jbs", "b.jbs"})
	if err == nil {
		t.Fatalf("expected usage error")
	}
}

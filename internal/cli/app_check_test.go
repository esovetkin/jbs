package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckParserOnlyValidSource(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "bench.jbs")
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`  echo ok`,
		`}`,
		``,
	}, "\n")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("check failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("expected quiet successful check, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "bench")); !os.IsNotExist(err) {
		t.Fatalf("check should not create a run directory, stat err=%v", err)
	}
}

func TestRunCheckReportsParserErrors(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "bad.jbs")
	if err := os.WriteFile(input, []byte("@\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"-c", input}, &stdout, &stderr); code != 1 {
		t.Fatalf("expected parser failure, got code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	errText := stderr.String()
	if !strings.Contains(errText, "ERROR") || !strings.Contains(errText, "bad.jbs") {
		t.Fatalf("expected formatted parser diagnostic, got %q", errText)
	}
}

func TestRunCheckReportsStrayClosingBrace(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "stray_close.jbs")
	if err := os.WriteFile(input, []byte("}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"-c", input}, &stdout, &stderr); code != 1 {
		t.Fatalf("expected parser failure, got code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unexpected closing brace") {
		t.Fatalf("expected stray brace diagnostic, got %q", stderr.String())
	}
}

func TestRunCheckAcceptsDoHereDocWithBrace(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "heredoc.jbs")
	src := "do run {\ncat > out <<EOF\n}\nEOF\necho after\n}\n"
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"-c", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected parser success, got code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("expected quiet parser success, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunCheckAcceptsDoParameterExpansionWithHash(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "param_expansion.jbs")
	src := "do run {\nfile=name.txt\necho ${file#*.}\n}\n"
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"-c", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected parser success, got code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("expected quiet parser success, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunCheckReportsReadFailure(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "missing.jbs")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", input}, &stdout, &stderr); code != 1 {
		t.Fatalf("expected read failure, got code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "failed to load input") {
		t.Fatalf("expected load error, got %q", stderr.String())
	}
}

func TestRunCheckDoesNotRunSemanticAnalysis(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "semantic.jbs")
	src := strings.Join([]string{
		`do s after missing_step with missing_value {`,
		`  echo "$missing_value"`,
		`}`,
		``,
	}, "\n")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("parser-only check should ignore semantic errors, code=%d stderr=%s", code, stderr.String())
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("expected quiet semantic-blind check, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunCheckDoesNotExecuteShell(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	marker := filepath.Join(dir, "marker")
	input := filepath.Join(dir, "bench.jbs")
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`value = shell("touch marker; printf hi; exit 7")`,
		`sh = shell`,
		`other = sh("touch marker")`,
		`do s with value {`,
		`  echo "$value"`,
		`}`,
		``,
	}, "\n")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected successful parser-only check, code=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected marker not to exist, stat err=%v", err)
	}
}

func TestRunCheckDoesNotResolveImports(t *testing.T) {
	dir := t.TempDir()
	chdirCLITest(t, dir)
	input := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`use value from "./missing_or_bad.jbs"`,
		`do s {`,
		`  echo ok`,
		`}`,
		``,
	}, "\n")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("parser-only check should not resolve imports, code=%d stderr=%s", code, stderr.String())
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("expected quiet import-blind check, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

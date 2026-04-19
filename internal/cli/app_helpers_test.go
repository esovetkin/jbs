package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEmbedListSpecificAndUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runEmbed("", &stdout, &stderr); code != 0 {
		t.Fatalf("expected successful embedded file listing, code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "jsc") {
		t.Fatalf("expected embedded file list to mention jsc, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runEmbed("jsc", &stdout, &stderr); code != 0 {
		t.Fatalf("expected successful embedded file read, code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "systemname") {
		t.Fatalf("expected embedded jsc content, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runEmbed("missing-embed", &stdout, &stderr); code != 1 {
		t.Fatalf("expected unknown embed to fail, code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown embedded file") {
		t.Fatalf("expected unknown embed error message, got %q", stderr.String())
	}
}

func TestWriteFileAtomicAndRunFmtNoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	original := []byte("x = 1\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := writeFileAtomic(path, []byte("y = 2\n"), 0o640); err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read atomic output: %v", err)
	}
	if string(data) != "y = 2\n" {
		t.Fatalf("unexpected atomic write content: %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat atomic output: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("expected atomic write permissions 0640, got %o", info.Mode().Perm())
	}

	if err := os.WriteFile(path, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatalf("rewrite fmt input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := runFmt(path, false, &stdout, &stderr); code != 0 {
		t.Fatalf("expected runFmt to succeed on already formatted file, code=%d stderr=%s", code, stderr.String())
	}
	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read formatted file: %v", err)
	}
	if string(formatted) != "x = 1\n" {
		t.Fatalf("expected runFmt no-op for already formatted file, got %q", string(formatted))
	}
}

func TestPrintHelpTopic(t *testing.T) {
	var out bytes.Buffer
	if err := printHelpTopic(&out, "use"); err != nil {
		t.Fatalf("expected help topic to render, got %v", err)
	}
	if !strings.Contains(out.String(), "use <module>") {
		t.Fatalf("expected use help content, got %q", out.String())
	}

	out.Reset()
	if err := printHelpTopic(&out, "missing-topic"); err == nil {
		t.Fatalf("expected unknown help topic to fail")
	}
}

func TestRunCheckWithTopLevelExprLinesProducesNoExprOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		"use jsc",
		"jsc.systemname",
		"x = (1, 2)",
		"do run with x {",
		"  echo ${x}",
		"}",
		"x",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful check, code=%d stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected no stdout output from top-level expr lines in check mode, got %q", stdout.String())
	}
}

func TestRunYAMLIgnoresTopLevelExprOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		"use jsc",
		"jsc.systemname",
		"x = (1, 2)",
		"do run with x {",
		"  echo ${x}",
		"}",
		"x",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--output", "-", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful yaml run, code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "name:") {
		t.Fatalf("expected yaml output, got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "juwelsbooster") {
		t.Fatalf("did not expect bare expr output to leak into yaml, got %q", stdout.String())
	}
}

func TestRunCheckWithFunctionValuedGlobals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		"base = 40",
		"mk = function(delta) {",
		"  function(x) {",
		"    x + delta + base",
		"  }",
		"}",
		"inc = mk(1)",
		"x = inc(1)",
		"do run with x {",
		"  echo ${x}",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful check, code=%d stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected no stdout output in check mode, got %q", stdout.String())
	}
}

func TestRunCheckWithMapReduceExprLinesProducesNoExprOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		"inc = function(x) {",
		"  x + 1",
		"}",
		"sum2 = function(acc, x) {",
		"  acc + x",
		"}",
		"map(inc, [1,2,3])",
		"reduce(sum2, [1,2,3])",
		"x = (1, 2)",
		"do run with x {",
		"  echo ${x}",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful check, code=%d stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected no stdout output from map/reduce expr lines in check mode, got %q", stdout.String())
	}
}

func TestRunYAMLIgnoresTopLevelFunctionCallOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		"marker = function() {",
		"  \"TOPLEVEL_FUNCTION_OUTPUT_SHOULD_NOT_APPEAR\"",
		"}",
		"marker()",
		"x = (1, 2)",
		"do run with x {",
		"  echo ${x}",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--output", "-", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful yaml run, code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "name:") {
		t.Fatalf("expected yaml output, got %q", out)
	}
	if strings.Contains(out, "TOPLEVEL_FUNCTION_OUTPUT_SHOULD_NOT_APPEAR") {
		t.Fatalf("did not expect top-level function call result to leak into yaml, got %q", out)
	}
}

func TestRunYAMLIgnoresTopLevelMapReduceOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		"marker = function(x) {",
		"  \"MAP_EXPR_SHOULD_NOT_APPEAR\"",
		"}",
		"sum2 = function(acc, x) {",
		"  acc + x",
		"}",
		"map(marker, [1])",
		"reduce(sum2, [1,2])",
		"x = (1, 2)",
		"do run with x {",
		"  echo ${x}",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--output", "-", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful yaml run, code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "name:") {
		t.Fatalf("expected yaml output, got %q", out)
	}
	if strings.Contains(out, "MAP_EXPR_SHOULD_NOT_APPEAR") {
		t.Fatalf("did not expect top-level map result to leak into yaml, got %q", out)
	}
}

func TestRunCheckRejectsFunctionValuedWithImport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		"add = function(x) {",
		"  x + 1",
		"}",
		"do run with add {",
		"  echo hi",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", path}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failing check for function-valued with import, code=%d stderr=%s", code, stderr.String())
	}
	errText := stderr.String()
	if !strings.Contains(errText, "ERROR E420") || !strings.Contains(errText, "not a data binding") {
		t.Fatalf("expected data-binding error for function-valued with import, got %q", errText)
	}
}

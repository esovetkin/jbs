package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCompileToStdout(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a * b
}

do prep with p {
  echo prep
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "in.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "parameterset:") {
		t.Fatalf("expected yaml output, got: %s", out.String())
	}
}

func TestRunCompileSemicolonFixture(t *testing.T) {
	in := filepath.Join("..", "..", "tests", "semicolon.jbs")
	expectedPath := filepath.Join("..", "..", "tests", "semicolon.yaml")
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected fixture: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	if out.String() != string(expected) {
		t.Fatalf("semicolon fixture mismatch\n--- got ---\n%s\n--- expected ---\n%s", out.String(), string(expected))
	}
}

func TestRunNoArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run(nil, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected usage text, got: %s", out.String())
	}
}

func TestRunHelpTopics(t *testing.T) {
	topics := []string{"globals", "param", "do", "let", "analyse", "submit"}
	for _, topic := range topics {
		var out bytes.Buffer
		var errBuf bytes.Buffer
		code := Run([]string{"help", topic}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("topic=%s expected exit 0, got %d stderr=%s", topic, code, errBuf.String())
		}
		if errBuf.Len() != 0 {
			t.Fatalf("topic=%s expected empty stderr, got: %s", topic, errBuf.String())
		}
		if out.Len() == 0 {
			t.Fatalf("topic=%s expected non-empty help output", topic)
		}
	}
}

func TestRunHelpTemplateRejected(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"help", "template"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "usage: jbs help [analyse|do|globals|let|param|submit]") {
		t.Fatalf("expected help usage error, got: %s", errBuf.String())
	}
}

func TestRunShowsSourceExcerptOnError(t *testing.T) {
	src := `
param p {
  a = @
  a
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	errText := errBuf.String()
	if !strings.Contains(errText, "a = @") {
		t.Fatalf("expected failing source line in diagnostics, got: %s", errText)
	}
	if !strings.Contains(errText, "^") {
		t.Fatalf("expected caret marker in diagnostics, got: %s", errText)
	}
}

func TestRunNoDuplicateUnknownParamsetDiagnostic(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

do setup with x from missing_set {
  echo setup
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "bad_unknown.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	errText := errBuf.String()
	if got := strings.Count(errText, "unknown parameterset 'missing_set'"); got != 1 {
		t.Fatalf("expected exactly one unknown-paramset diagnostic, got %d\n%s", got, errText)
	}
}

func TestRunFmtInPlace(t *testing.T) {
	src := `jbs_name="test"
jbs_outpath ="test"
param paramset{
  a=["a","b","c"]
  b=["1","2"]
  c=(1,2)

  a*b*c
}
do task with a from paramset{
  echo ${a} ${b} ${c}
}
submit benchmark with paramset after task{
preprocess={
 export X=1
}
args_exec="python main.py --case ${a} --nnodes ${c}"
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "fmt.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"fmt", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}

	got, err := os.ReadFile(in)
	if err != nil {
		t.Fatalf("read formatted: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "do task\n        with a from paramset\n{") {
		t.Fatalf("expected canonical do header formatting, got:\n%s", text)
	}
	if !strings.Contains(text, "submit benchmark\n        after task\n        with paramset\n{") {
		t.Fatalf("expected canonical submit header formatting, got:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatalf("expected trailing newline in formatted file")
	}
}

func TestRunFmtParseErrorLeavesFileUnchanged(t *testing.T) {
	src := `param p {
  a = @
  a
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "bad_fmt.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"fmt", in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	got, err := os.ReadFile(in)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != src {
		t.Fatalf("expected file unchanged on format error")
	}
}

func TestRunFmtMissingPathUsageError(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"fmt"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2 for usage error, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "usage: jbs fmt <file.jbs>") {
		t.Fatalf("expected fmt usage error, got: %s", errBuf.String())
	}
}

func TestRunFmtPreservesFilePermissions(t *testing.T) {
	src := `jbs_name="test"
param p{
  a=(1,2)
  a
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "perm.jbs")
	if err := os.WriteFile(in, []byte(src), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"fmt", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	info, err := os.Stat(in)
	if err != nil {
		t.Fatalf("stat formatted file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected permissions 0600, got %#o", info.Mode().Perm())
	}
}

func TestRunFmtNoRewriteWhenUnchanged(t *testing.T) {
	src := "jbs_name = \"test\"\n"
	dir := t.TempDir()
	in := filepath.Join(dir, "same.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	before, err := os.Stat(in)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"fmt", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	after, err := os.Stat(in)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("expected unchanged file not to be rewritten")
	}
}

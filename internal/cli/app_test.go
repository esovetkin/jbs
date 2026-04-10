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

func TestRunCompileBackslashContinuationFixture(t *testing.T) {
	in := filepath.Join("..", "..", "tests", "backslash_continuation.jbs")
	expectedPath := filepath.Join("..", "..", "tests", "backslash_continuation.yaml")
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
		t.Fatalf("backslash continuation fixture mismatch\n--- got ---\n%s\n--- expected ---\n%s", out.String(), string(expected))
	}
}

func TestRunCompileStepOptionsFixture(t *testing.T) {
	in := filepath.Join("..", "..", "tests", "step_options.jbs")
	expectedPath := filepath.Join("..", "..", "tests", "step_options.yaml")
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
		t.Fatalf("step options fixture mismatch\n--- got ---\n%s\n--- expected ---\n%s", out.String(), string(expected))
	}
}

func TestRunCheckStepOptionsValidAndInvalid(t *testing.T) {
	valid := filepath.Join("..", "..", "tests", "step_options.jbs")
	{
		var out bytes.Buffer
		var errBuf bytes.Buffer
		code := Run([]string{"-c", valid}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("expected -c valid to exit 0, got %d stderr=%s", code, errBuf.String())
		}
	}

	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid_step_options.jbs")
	src := `
do run max_async=-1 procs=-1 iterations=0 {
  echo hi
}
`
	if err := os.WriteFile(invalid, []byte(src), 0o644); err != nil {
		t.Fatalf("write invalid fixture: %v", err)
	}
	{
		var out bytes.Buffer
		var errBuf bytes.Buffer
		code := Run([]string{"-c", invalid}, &out, &errBuf)
		if code != 1 {
			t.Fatalf("expected -c invalid to exit 1, got %d stderr=%s", code, errBuf.String())
		}
		text := errBuf.String()
		if !strings.Contains(text, "E216") || !strings.Contains(text, "E217") || !strings.Contains(text, "E219") {
			t.Fatalf("expected E216/E217/E219 in diagnostics, got: %s", text)
		}
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
	topics := []string{"globals", "param", "do", "let", "analyse", "submit", "use"}
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
	if !strings.Contains(errBuf.String(), "usage: jbs help [analyse|do|globals|let|param|submit|use]") {
		t.Fatalf("expected help usage error, got: %s", errBuf.String())
	}
}

func TestRunEmbedList(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"embed"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "jsc.jbs") {
		t.Fatalf("expected jsc.jbs in embed list, got: %s", out.String())
	}
}

func TestRunEmbedContent(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"embed", "jsc"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "let submit_defaults") {
		t.Fatalf("expected embedded jsc content, got: %s", out.String())
	}
}

func TestRunEmbedUnknown(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"embed", "does_not_exist"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "unknown embedded file") {
		t.Fatalf("expected unknown embedded file error, got: %s", errBuf.String())
	}
}

func TestRunPrintParamPrettyStdout(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}

do s0 with a from p {
  echo ${a}
}

do s1 after s0 with b from p {
  echo ${a} ${b}
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "pp.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"printparam", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	expected := strings.Join([]string{
		"| p.a | p.b | step   |",
		"|-----|-----|--------|",
		"| 1   |     | do: s0 |",
		"| 2   |     | do: s0 |",
		"| 1   | x   | do: s1 |",
		"| 2   | y   | do: s1 |",
		"",
	}, "\n")
	if out.String() != expected {
		t.Fatalf("unexpected pretty printparam output\n--- got ---\n%s\n--- expected ---\n%s", out.String(), expected)
	}
}

func TestRunPrintParamCSVStdout(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}

do s0 with a from p {
  echo ${a}
}

do s1 after s0 with b from p {
  echo ${a} ${b}
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "pp_csv.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"printparam", "-t", "csv", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	expected := strings.Join([]string{
		"p.a,p.b,step",
		"1,,do: s0",
		"2,,do: s0",
		"1,x,do: s1",
		"2,y,do: s1",
		"",
	}, "\n")
	if out.String() != expected {
		t.Fatalf("unexpected csv printparam output\n--- got ---\n%s\n--- expected ---\n%s", out.String(), expected)
	}
}

func TestRunPrintParamOutputFile(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

do s with p {
  echo ${a}
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "pp_out.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	outPath := filepath.Join(dir, "printparam.csv")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"printparam", "-t", "csv", "-o", outPath, in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout for file output, got: %s", out.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.HasPrefix(string(data), "p.a,step\n1,do: s\n2,do: s\n") {
		t.Fatalf("unexpected file output: %s", string(data))
	}
}

func TestRunPrintParamUsageError(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"printparam"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "usage: jbs printparam [-t pretty|csv] [-o <outputfile>] <file.jbs>") {
		t.Fatalf("expected printparam usage error, got: %s", errBuf.String())
	}
}

func TestRunPrintParamExcludesSubmitUseDefaultsOnlyColumns(t *testing.T) {
	src := `
let submit_defaults {
  queue = "batch"
  gres = "gpu:4"
}

submit run
  use submit_defaults
{
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "pp_submit_defaults_only.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"printparam", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	text := out.String()
	if !strings.Contains(text, "step") || !strings.Contains(text, "submit: run") {
		t.Fatalf("expected step-only printparam output, got: %s", text)
	}
	if strings.Contains(text, "submit_defaults.") {
		t.Fatalf("unexpected submit_defaults columns in printparam output: %s", text)
	}
}

func TestRunPrintParamExcludesSubmitUseDefaultsButKeepsWithImportedLetAndParam(t *testing.T) {
	src := `
use submit_defaults from jsc

let l {
  x = 1
}

param p {
  a = (1,2,3)
  b = ("a","b")
  a*b
}

submit run
  use submit_defaults
  with p, l
{
  preprocess = {
    echo ${a} $b $x
  }
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "pp_submit_mixed.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"printparam", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	text := out.String()
	for _, want := range []string{"l.x", "p.a", "p.b"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected printparam output to include %q, got: %s", want, text)
		}
	}
	if strings.Contains(text, "submit_defaults.") {
		t.Fatalf("unexpected submit_defaults columns in printparam output: %s", text)
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

func TestRunShowsImportedFileExcerptOnError(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.jbs")
	if err := os.WriteFile(libPath, []byte(`
param p {
  a = @
  a
}
`), 0o644); err != nil {
		t.Fatalf("write imported file: %v", err)
	}
	entry := filepath.Join(dir, "main.jbs")
	if err := os.WriteFile(entry, []byte(`use p from "./lib.jbs"`+"\n"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", entry}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	errText := errBuf.String()
	if !strings.Contains(errText, "lib.jbs") {
		t.Fatalf("expected imported filename in diagnostics, got: %s", errText)
	}
	if !strings.Contains(errText, "a = @") || !strings.Contains(errText, "^") {
		t.Fatalf("expected imported source excerpt and caret, got: %s", errText)
	}
}

func TestRunCheckAbsoluteInputUsesEntryDirectoryForQuotedImports(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	otherDir := filepath.Join(root, "other")
	if err := os.MkdirAll(filepath.Join(projectDir, "lib"), 0o755); err != nil {
		t.Fatalf("mkdir lib dir: %v", err)
	}
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir other dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "lib", "mod.jbs"), []byte(`
param p {
  x = (1,2)
  x
}
`), 0o644); err != nil {
		t.Fatalf("write imported file: %v", err)
	}
	entry := filepath.Join(projectDir, "main.jbs")
	if err := os.WriteFile(entry, []byte(`
use p from "./lib/mod.jbs"
do s with p {
  echo ${x}
}
`), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(otherDir); err != nil {
		t.Fatalf("chdir foreign cwd: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", entry}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "E531") {
		t.Fatalf("did not expect E531, got: %s", errBuf.String())
	}
}

func TestRunFmtAbsoluteInputUsesEntryDirectoryForQuotedImports(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	otherDir := filepath.Join(root, "other")
	if err := os.MkdirAll(filepath.Join(projectDir, "lib"), 0o755); err != nil {
		t.Fatalf("mkdir lib dir: %v", err)
	}
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir other dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "lib", "mod.jbs"), []byte(`
param p {
  x = (1,2)
  x
}
`), 0o644); err != nil {
		t.Fatalf("write imported file: %v", err)
	}
	entry := filepath.Join(projectDir, "main.jbs")
	if err := os.WriteFile(entry, []byte(`
use p from "./lib/mod.jbs"

do s
        with p
{
        echo ${x}
}
`), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(otherDir); err != nil {
		t.Fatalf("chdir foreign cwd: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"fmt", "--strict", entry}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "E531") {
		t.Fatalf("did not expect E531, got: %s", errBuf.String())
	}
}

func TestRunRepeatedSubmitUseClauseAllowedAndWarnsOnCollision(t *testing.T) {
	src := `
let defaults {
  queue = "batch"
}
let gpu_defaults {
  queue = "devel"
}
submit run
  use defaults
  use gpu_defaults
{
  args_exec = "-lc hostname"
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "bad_submit_use.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "W072") {
		t.Fatalf("expected W072 in diagnostics, got: %s", errBuf.String())
	}
}

func TestRunCheckUseCollisionLocalImportFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "use_collision_local_import.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "E534") {
		t.Fatalf("expected E534 in diagnostics, got: %s", errBuf.String())
	}
}

func TestRunCheckUseCollisionImportImportFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "use_collision_import_import.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "E534") {
		t.Fatalf("expected E534 in diagnostics, got: %s", errBuf.String())
	}
}

func TestRunCheckUseSubmitDoubleHeaderFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "use_submit_double_use_header.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "W072") {
		t.Fatalf("did not expect W072 for disjoint submit defaults, got: %s", errBuf.String())
	}
}

func TestRunCheckSubmitUseMultiOkFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "submit_use_multi_ok.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "W072") {
		t.Fatalf("did not expect W072 for disjoint submit defaults, got: %s", errBuf.String())
	}
}

func TestRunCheckSubmitUseMultiCollisionWarnFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "submit_use_multi_collision_warn.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "W072") {
		t.Fatalf("expected W072 in diagnostics, got: %s", errBuf.String())
	}
}

func TestRunCheckQualifiedWithFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "qualified_with_main.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
}

func TestRunCheckQualifiedWithMissingSymbolFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "qualified_with_missing_symbol.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "E532") {
		t.Fatalf("expected E532 in diagnostics, got: %s", errBuf.String())
	}
}

func TestRunCheckSubmitUseParamInvalidFixture(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWD, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	in := filepath.Join("tests", "submit_use_param_invalid.jbs")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"-c", in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "E071") {
		t.Fatalf("expected E071 in diagnostics, got: %s", errBuf.String())
	}
}

func TestRunQualifiedWithFormsFromModuleAlias(t *testing.T) {
	dir := t.TempDir()
	lib := `
param p
{
        x = 1 if true else 0
        x
}

let l
{
        x = 1
}
`
	main := `
use test_lib
use p from test_lib
use l from test_lib

do s0 with p {
    echo ${x}
}

do s1 with test_lib.p {
    echo ${x}
}

do s2 with test_lib.l {
    echo ${x}
}

do s3 with l {
    echo ${x}
}
`
	libPath := filepath.Join(dir, "test_lib.jbs")
	mainPath := filepath.Join(dir, "test.jbs")
	if err := os.WriteFile(libPath, []byte(lib), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatalf("write main: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	commands := [][]string{
		{"-c", "test.jbs"},
		{"fmt", "test.jbs"},
	}
	for _, args := range commands {
		var out bytes.Buffer
		var errBuf bytes.Buffer
		code := Run(args, &out, &errBuf)
		if code != 0 {
			t.Fatalf("args=%v expected exit code 0, got %d stderr=%s", args, code, errBuf.String())
		}
	}

	formatted, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read formatted file: %v", err)
	}
	if !strings.Contains(string(formatted), "with test_lib.p") {
		t.Fatalf("expected formatted file to keep qualified with for param import, got:\n%s", string(formatted))
	}
	if !strings.Contains(string(formatted), "with test_lib.l") {
		t.Fatalf("expected formatted file to keep qualified with for let import, got:\n%s", string(formatted))
	}
}

func TestRunFmtAndCheckParityForQualifiedWithCases(t *testing.T) {
	cases := []struct {
		name         string
		lib          string
		main         string
		wantCheck    int
		wantFmt      int
		wantStrict   int
		wantDiagPart string
	}{
		{
			name: "valid",
			lib: `
param p
{
        x = 1 if true else 0
        x
}

let l
{
        x = 1
}
`,
			main: `
use test_lib

do s0 with test_lib.p {
    echo ${x}
}

	do s1 with test_lib.l {
	    echo ${x}
	}
	`,
			wantCheck:  0,
			wantFmt:    0,
			wantStrict: 0,
		},
		{
			name: "unknown_alias",
			lib: `
param p
{
        x = 1
        x
}
`,
			main: `
do s0 with unknown_alias.p {
	    echo ${x}
}
`,
			wantCheck:    1,
			wantFmt:      0,
			wantStrict:   1,
			wantDiagPart: "E537",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			libPath := filepath.Join(dir, "test_lib.jbs")
			mainPath := filepath.Join(dir, "test.jbs")
			if err := os.WriteFile(libPath, []byte(tc.lib), 0o644); err != nil {
				t.Fatalf("write lib: %v", err)
			}
			if err := os.WriteFile(mainPath, []byte(tc.main), 0o644); err != nil {
				t.Fatalf("write main: %v", err)
			}

			origWD, err := os.Getwd()
			if err != nil {
				t.Fatalf("getwd: %v", err)
			}
			t.Cleanup(func() { _ = os.Chdir(origWD) })
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("chdir temp dir: %v", err)
			}

			commands := []struct {
				args []string
				want int
			}{
				{args: []string{"-c", "test.jbs"}, want: tc.wantCheck},
				{args: []string{"fmt", "test.jbs"}, want: tc.wantFmt},
				{args: []string{"fmt", "--strict", "test.jbs"}, want: tc.wantStrict},
				{args: []string{"fmt", "-s", "test.jbs"}, want: tc.wantStrict},
			}
			for _, cmd := range commands {
				var out bytes.Buffer
				var errBuf bytes.Buffer
				code := Run(cmd.args, &out, &errBuf)
				if code != cmd.want {
					t.Fatalf("args=%v expected exit %d, got %d stderr=%s", cmd.args, cmd.want, code, errBuf.String())
				}
				if tc.wantDiagPart != "" && cmd.want != 0 && !strings.Contains(errBuf.String(), tc.wantDiagPart) {
					t.Fatalf("args=%v expected diagnostics to contain %q, got: %s", cmd.args, tc.wantDiagPart, errBuf.String())
				}
			}
		})
	}
}

func TestRunFmtSyntaxOnlyIgnoresImportedFileDiagnostics(t *testing.T) {
	dir := t.TempDir()
	lib := `
param p
{
        x = @
        x
}
`
	main := `
use test_lib
use p from test_lib

do s0 with p {
    echo ${x}
}
`
	libPath := filepath.Join(dir, "test_lib.jbs")
	mainPath := filepath.Join(dir, "test.jbs")
	if err := os.WriteFile(libPath, []byte(lib), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatalf("write main: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"fmt", "test.jbs"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%s", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "test_lib.jbs") {
		t.Fatalf("did not expect imported-file diagnostics in syntax-only fmt, got: %s", errBuf.String())
	}
}

func TestRunFmtStrictShowsImportedFileDiagnostics(t *testing.T) {
	dir := t.TempDir()
	lib := `
param p
{
        x = @
        x
}
`
	main := `
use test_lib
use p from test_lib

do s0 with p {
    echo ${x}
}
`
	libPath := filepath.Join(dir, "test_lib.jbs")
	mainPath := filepath.Join(dir, "test.jbs")
	if err := os.WriteFile(libPath, []byte(lib), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatalf("write main: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	for _, args := range [][]string{
		{"fmt", "--strict", "test.jbs"},
		{"fmt", "-s", "test.jbs"},
	} {
		var out bytes.Buffer
		var errBuf bytes.Buffer
		code := Run(args, &out, &errBuf)
		if code != 1 {
			t.Fatalf("args=%v expected exit code 1, got %d", args, code)
		}
		if !strings.Contains(errBuf.String(), "test_lib.jbs") {
			t.Fatalf("args=%v expected diagnostics to reference imported file, got: %s", args, errBuf.String())
		}
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
	if !strings.Contains(errBuf.String(), "usage: jbs fmt [-s|--strict] <file.jbs>") {
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

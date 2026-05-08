package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunFmtNoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatalf("write fmt input: %v", err)
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

func TestDefaultFileRunMatchesExplicitRunTree(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	runCase := func(args func(string) []string) []string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "main.jbs")
		src := strings.Join([]string{
			`jbs_name = "bench"`,
			`do step {`,
			`        echo ok`,
			`}`,
			"",
		}, "\n")
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write input: %v", err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		var stdout, stderr bytes.Buffer
		if code := Run(args(path), &stdout, &stderr); code != 0 {
			t.Fatalf("run failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		return collectRunTree(t, filepath.Join(dir, "bench", "000000"))
	}

	gotDefault := runCase(func(path string) []string { return []string{path} })
	gotExplicit := runCase(func(path string) []string { return []string{"run", path} })
	if !reflect.DeepEqual(gotDefault, gotExplicit) {
		t.Fatalf("default and explicit run trees differ:\ndefault=%#v\nexplicit=%#v", gotDefault, gotExplicit)
	}
}

func collectRunTree(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel += "/"
		}
		entries = append(entries, rel)
		return nil
	}); err != nil {
		t.Fatalf("walk run tree: %v", err)
	}
	return entries
}

func TestRunFmtPreservesDoRawHeredoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := "do write_sbatch with cases {\n    cat > run.sbatch <<EOF  \n#!/bin/bash\n\nEOF\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runFmt(path, false, &stdout, &stderr); code != 0 {
		t.Fatalf("expected runFmt to succeed, code=%d stderr=%s", code, stderr.String())
	}
	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read formatted file: %v", err)
	}
	if !strings.Contains(string(formatted), "    cat > run.sbatch <<EOF  \n#!/bin/bash\n\nEOF\n") {
		t.Fatalf("raw heredoc payload was changed:\n%s", string(formatted))
	}
}

func TestRunFmtStrictPreservesDoRawHeredoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do write_sbatch {`,
		`    cat > run.sbatch <<EOF  `,
		`#!/bin/bash`,
		`EOF`,
		`}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runFmt(path, true, &stdout, &stderr); code != 0 {
		t.Fatalf("expected strict runFmt to succeed, code=%d stderr=%s", code, stderr.String())
	}
	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read formatted file: %v", err)
	}
	if !strings.Contains(string(formatted), "    cat > run.sbatch <<EOF  \n#!/bin/bash\nEOF\n") {
		t.Fatalf("strict fmt changed raw heredoc payload:\n%s", string(formatted))
	}
}

func TestRunFmtStrictAcceptsCanonicalSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`x = (1, 2)`,
		`cases = table(x = x)`,
		`do run`,
		`        with cases`,
		`{`,
		`        echo "${x}"`,
		`}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runFmt(path, true, &stdout, &stderr); code != 0 {
		t.Fatalf("expected strict formatter to accept canonical syntax, code=%d stderr=%s", code, stderr.String())
	}
}

func TestRunFmtStrictRejectsFormerTopLevelParamBlockAsGenericSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`param cases {`,
		`  x = (1, 2)`,
		`  x`,
		`}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runFmt(path, true, &stdout, &stderr); code != 1 {
		t.Fatalf("expected strict formatter to reject invalid syntax, code=%d stderr=%s", code, stderr.String())
	}
	errText := stderr.String()
	if !strings.Contains(errText, "ERROR E061") || !strings.Contains(errText, "unexpected trailing tokens after expression") {
		t.Fatalf("expected generic expression diagnostic, got %q", errText)
	}
}

func TestRunFmtStrictAcceptsTopLevelCompoundAssignment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`seed = 1`,
		`seed += 1`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runFmt(path, true, &stdout, &stderr); code != 0 {
		t.Fatalf("expected strict formatter to accept top-level compound assignment, code=%d stderr=%s", code, stderr.String())
	}
	errText := stderr.String()
	if strings.Contains(errText, "ERROR E307") {
		t.Fatalf("did not expect top-level compound-assignment diagnostic, got %q", errText)
	}
}

func TestRunFmtStrictRejectsUnsupportedWithSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`x = (1, 2)`,
		`cases = table(x = x)`,
		`do run`,
		`        with x from cases`,
		`{`,
		`        echo "${x}"`,
		`}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runFmt(path, true, &stdout, &stderr); code != 1 {
		t.Fatalf("expected strict formatter to reject invalid with syntax, code=%d stderr=%s", code, stderr.String())
	}
	errText := stderr.String()
	if !strings.Contains(errText, "ERROR E023") || !strings.Contains(errText, "invalid with-clause syntax") {
		t.Fatalf("expected generic with-clause syntax diagnostic, got %q", errText)
	}
	if strings.Contains(errText, "rewrite") || strings.Contains(errText, "old with") {
		t.Fatalf("did not expect rewrite diagnostic, got %q", errText)
	}
}

func TestRunFmtStrictRejectsInheritAsHeaderClause(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`x = (1, 2)`,
		`cases = table(x = x)`,
		`do prep`,
		`        with cases[x]`,
		`{`,
		`        echo "${x}"`,
		`}`,
		`do run`,
		`        inherit prep`,
		`{`,
		`        echo hi`,
		`}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runFmt(path, true, &stdout, &stderr); code != 1 {
		t.Fatalf("expected strict formatter to reject invalid header clause, code=%d stderr=%s", code, stderr.String())
	}
	errText := stderr.String()
	if !strings.Contains(errText, "ERROR E031") || !strings.Contains(errText, "expected '{' to start do block body") {
		t.Fatalf("expected generic do-body diagnostic, got %q", errText)
	}
}

func TestPrintHelpTopic(t *testing.T) {
	var out bytes.Buffer
	if err := printHelpTopic(&out, "use"); err != nil {
		t.Fatalf("expected help topic to render, got %v", err)
	}
	if !strings.Contains(out.String(), "use value from") {
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

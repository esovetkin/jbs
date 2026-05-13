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

func TestRunCheckRejectsAnalyseWithTableBindingPrecisely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := strings.Join([]string{
		`cases = table(x=[1])`,
		`do run with cases {`,
		`  echo "$x"`,
		`}`,
		`analyse run with cases {`,
		`  (x)`,
		`}`,
		``,
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", path}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failing check for table-valued analyse import, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	errText := stderr.String()
	want := "analyse with-clause requires a bare string scalar variable; 'cases' is a table"
	if !strings.Contains(errText, "ERROR E420") || !strings.Contains(errText, want) {
		t.Fatalf("expected precise analyse with diagnostic %q, got %q", want, errText)
	}
}

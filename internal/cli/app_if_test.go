package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckAcceptsIfAssignments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := `
jbs_name = "if_demo"
flag = true
if flag {
	cases = t(x = range(2))
} else {
	cases = t(x = range(5))
}
do run with cases {
	echo $x
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected successful check, code=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no check output, got %q", stdout.String())
	}
}

func TestRunRejectsDeclarationInsideIf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	if err := os.WriteFile(path, []byte("if true { do run { echo bad } }\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{path}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected run failure, stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout on parser error, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "ERROR E080") {
		t.Fatalf("expected E080, got %q", stderr.String())
	}
}

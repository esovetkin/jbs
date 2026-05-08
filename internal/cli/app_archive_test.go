package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jbsrun "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/run"
)

func TestArchiveCommandArchivesDryRunBenchmarkDirectory(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeArchiveInput(t, cwd, "bench.jbs", []string{
		`jbs_name = "bench_out"`,
		`cases = table(x=[1])`,
		`do s with cases {`,
		`echo "$x"`,
		`}`,
	})

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"archive", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("archive failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench.tar.gz")); err != nil {
		t.Fatalf("expected archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench_out")); !os.IsNotExist(err) {
		t.Fatalf("expected benchmark directory removal, stat error: %v", err)
	}
	names := readCLITarGzNames(t, filepath.Join(cwd, "bench.tar.gz"))
	if !tarNameHasSuffix(names, "/bench_out/000000/status") {
		t.Fatalf("archive missing benchmark status; names=%v", names)
	}
	out := stdout.String()
	if !strings.Contains(out, "archived bench_out to bench.tar.gz as ") || !strings.Contains(out, " and removed bench_out") {
		t.Fatalf("unexpected archive stdout: %q", out)
	}
}

func TestArchiveCommandArchivesCompletedBenchmarkDirectory(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeArchiveInput(t, cwd, "completed.jbs", []string{
		`jbs_name = "completed_out"`,
		`do s {`,
		`echo done`,
		`}`,
	})

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"archive", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("archive failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "completed_out")); !os.IsNotExist(err) {
		t.Fatalf("expected completed benchmark directory removal, stat error: %v", err)
	}
	names := readCLITarGzNames(t, filepath.Join(cwd, "completed.tar.gz"))
	if !tarNameHasSuffix(names, "/completed_out/000000/s/000000/stdout") {
		t.Fatalf("archive missing completed stdout; names=%v", names)
	}
}

func TestArchiveCommandAppendsSnapshots(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeArchiveInput(t, cwd, "append.jbs", []string{
		`jbs_name = "append_out"`,
		`do s { echo hi }`,
	})

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("first dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"archive", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("first archive failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("second dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"archive", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("second archive failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	tops := tarTopLevelNames(readCLITarGzNames(t, filepath.Join(cwd, "append.tar.gz")))
	if len(tops) != 2 {
		t.Fatalf("top-level snapshots = %v, want 2", tops)
	}
}

func TestArchiveCommandRejectsMissingBenchmarkRoot(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeArchiveInput(t, cwd, "missing.jbs", []string{
		`jbs_name = "missing_out"`,
		`do s { echo hi }`,
	})

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"archive", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected archive failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "cannot lock benchmark root missing_out") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "missing.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("expected no archive, stat error: %v", err)
	}
}

func TestArchiveCommandRejectsRunningBenchmark(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeArchiveInput(t, cwd, "running.jbs", []string{
		`jbs_name = "running_out"`,
		`do s { echo hi }`,
	})

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	statusPath := filepath.Join(cwd, "running_out", "000000", "status")
	status := readRootStatus(t, statusPath)
	status.Status = jbsrun.StatusRunning
	writeRootStatus(t, statusPath, status)

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"archive", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected archive failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "status is RUNNING") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "running_out")); err != nil {
		t.Fatalf("expected benchmark directory to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "running.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("expected no archive, stat error: %v", err)
	}
}

func TestArchiveCommandDoesNotEmitCompileTimePrintOutput(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeArchiveInput(t, cwd, "print.jbs", []string{
		`jbs_name = "print_out"`,
		`print("archive-print")`,
		`do s { echo hi }`,
	})

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"archive", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("archive failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "archive-print") {
		t.Fatalf("archive replayed print output: %q", stdout.String())
	}
}

func writeArchiveInput(t *testing.T, cwd, name string, lines []string) string {
	t.Helper()
	input := filepath.Join(cwd, name)
	if err := os.WriteFile(input, []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return input
}

func readCLITarGzNames(t *testing.T, archivePath string) []string {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	names := make([]string, 0)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

func tarNameHasSuffix(names []string, suffix string) bool {
	for _, name := range names {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func tarTopLevelNames(names []string) map[string]struct{} {
	tops := make(map[string]struct{})
	for _, name := range names {
		trimmed := strings.Trim(name, "/")
		if trimmed == "" {
			continue
		}
		top, _, _ := strings.Cut(trimmed, "/")
		tops[top] = struct{}{}
	}
	return tops
}

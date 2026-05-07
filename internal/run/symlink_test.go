package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentOSSupportsSymbolicLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")

	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", link); err != nil {
		t.Fatalf("current OS does not support symbolic links required by jbs run: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symbolic link, mode is %s", link, info.Mode())
	}
	dest, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if dest != "target.txt" {
		t.Fatalf("unexpected symbolic link target: got %q want %q", dest, "target.txt")
	}
	data, err := os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok\n" {
		t.Fatalf("symbolic link did not resolve to target content: %q", string(data))
	}
}

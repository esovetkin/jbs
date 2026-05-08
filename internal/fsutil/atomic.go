package fsutil

import (
	"encoding/csv"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

type AtomicWriteOptions struct {
	SyncDir    bool
	TempSuffix string
}

func WriteFileAtomic(path string, data []byte, perm fs.FileMode, opts AtomicWriteOptions) error {
	return writeAtomic(path, perm, opts, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

func WriteJSONAtomic(path string, value any, perm fs.FileMode, opts AtomicWriteOptions) error {
	return writeAtomic(path, perm, opts, func(f *os.File) error {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		return enc.Encode(value)
	})
}

func WriteCSVAtomic(path string, rows [][]string, perm fs.FileMode, opts AtomicWriteOptions) error {
	return writeAtomic(path, perm, opts, func(f *os.File) error {
		w := csv.NewWriter(f)
		if err := w.WriteAll(rows); err != nil {
			return err
		}
		return w.Error()
	})
}

func ReadJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func SyncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func writeAtomic(path string, perm fs.FileMode, opts AtomicWriteOptions, fill func(*os.File) error) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, tempPattern(path, opts))
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := fill(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	renamed = true
	if opts.SyncDir {
		return SyncDir(dir)
	}
	return nil
}

func tempPattern(path string, opts AtomicWriteOptions) string {
	suffix := opts.TempSuffix
	if suffix == "" {
		suffix = "tmp"
	}
	return "." + filepath.Base(path) + "." + suffix + "-*"
}

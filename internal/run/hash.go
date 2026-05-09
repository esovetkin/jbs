package run

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"maps"
	"slices"
)

// SourceBundleHash returns the durable source identity hash for a run.
//
// The hash includes each source label and its text. File labels are the
// cleaned absolute paths used by the loader, not symlink-resolved physical
// paths. Labels are intentionally part of the identity because relative
// imports and file reads are evaluated relative to those loaded source paths.
func SourceBundleHash(sources map[string]string) string {
	keys := slices.Sorted(maps.Keys(sources))
	h := sha256.New()
	for _, key := range keys {
		io.WriteString(h, key)
		h.Write([]byte{0})
		io.WriteString(h, sources[key])
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

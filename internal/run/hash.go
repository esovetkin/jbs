package run

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"maps"
	"slices"
)

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

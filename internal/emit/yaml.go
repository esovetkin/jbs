package emit

import (
	"bytes"

	"gopkg.in/yaml.v3"

	"jbs/internal/lower"
)

func YAML(doc lower.Document) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

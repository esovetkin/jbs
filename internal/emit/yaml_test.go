package emit

import (
	"testing"

	"gopkg.in/yaml.v3"

	"jbs/internal/lower"
)

func TestYAMLRoundTripParses(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		ParameterSet: []lower.ParameterSet{
			{
				Name: "p",
				Parameter: []lower.Parameter{
					{Name: "a", Mode: "text", Separator: "####", Value: "1####2"},
				},
			},
		},
	}
	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("encoded yaml does not parse: %v", err)
	}
}

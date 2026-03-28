package emit

import (
	"strings"
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

func TestYAMLIncludesSectionAndRoleComments(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		ParameterSet: []lower.ParameterSet{
			{
				Name:      "matrix",
				Parameter: []lower.Parameter{{Name: "a", Value: "1"}},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindParam,
					Source: "matrix",
				},
			},
			{
				Name:     "run__submit_params",
				InitWith: "platform.xml:systemParameter",
				Parameter: []lower.Parameter{
					{Name: "queue", Mode: "python", Value: lower.SingleQuoted("__import__(\"os\")")},
				},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindSubmitInit,
					Source: "run",
				},
			},
		},
		Step: []lower.Step{
			{
				Name: "setup",
				Do:   []interface{}{lower.Literal("echo setup\n")},
				Meta: lower.StepMeta{
					Kind:   lower.StepKindDo,
					Source: "setup",
				},
			},
			{
				Name: "run",
				Use:  []interface{}{"run__submit_params"},
				Meta: lower.StepMeta{
					Kind:   lower.StepKindSubmit,
					Source: "run",
				},
			},
		},
	}

	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# From jbs_name") {
		t.Fatalf("missing name comment: %s", text)
	}
	if !strings.Contains(text, "# From jbs_outpath") {
		t.Fatalf("missing outpath comment: %s", text)
	}
	if !strings.Contains(text, "# Parameter sets used to create workpackage combinations") {
		t.Fatalf("missing parameterset section comment: %s", text)
	}
	if !strings.Contains(text, "# Param block 'matrix'") {
		t.Fatalf("missing param block role comment: %s", text)
	}
	if !strings.Contains(text, "# Parameters for submit block 'run'") {
		t.Fatalf("missing submit parameterset role comment: %s", text)
	}
	if !strings.Contains(text, "# From jbs_queue") {
		t.Fatalf("missing submit parameter to global annotation for queue: %s", text)
	}
	if !strings.Contains(text, "# Step generated from do block 'setup'") {
		t.Fatalf("missing do step comment: %s", text)
	}
	if !strings.Contains(text, "# Step generated from submit block 'run'") {
		t.Fatalf("missing submit step comment: %s", text)
	}
	if !strings.Contains(text, "\n\n# From jbs_outpath\noutpath: out\n\n# Parameter sets used to create workpackage combinations") {
		t.Fatalf("missing blank-line separation between root fields/sections: %s", text)
	}
	if !strings.Contains(text, "\n\n  # Parameters for submit block 'run'") {
		t.Fatalf("missing blank-line separation between parameterset blocks: %s", text)
	}
	if !strings.Contains(text, "\n\n  # Step generated from submit block 'run'") {
		t.Fatalf("missing blank-line separation between step blocks: %s", text)
	}

	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("encoded yaml with comments does not parse: %v", err)
	}
}

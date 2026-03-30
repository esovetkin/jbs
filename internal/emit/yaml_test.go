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
		PatternSet: []lower.PatternSet{
			{
				Name: "p",
				Pattern: []lower.Pattern{
					{
						Name:  "_jbs_pattern__p_number__write__p0",
						Type:  "int",
						Value: lower.SingleQuoted("Number: $jube_pat_int"),
						Meta: lower.PatternMeta{
							IsAnalyseAlias: true,
							AnalyseStep:    "write",
							AliasName:      "p0",
							PatternRef:     "p.number",
						},
					},
				},
				Meta: lower.PatternSetMeta{
					Kind:   lower.PatternSetKindBase,
					Source: "p",
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
		Analyser: []lower.Analyser{
			{
				Name: "analyser_write",
				Use:  "p",
				Analyse: []lower.AnalyseItem{
					{
						Step: "write",
						File: []lower.AnalyseFile{
							{Use: "p", Value: "en"},
						},
					},
				},
				Meta: lower.AnalyserMeta{Source: "write"},
			},
		},
		Result: &lower.ResultObject{
			Use: []string{"analyser_write"},
			Table: []lower.ResultTable{
				{
					Name:  "result_write",
					Style: "csv",
					Column: []lower.ResultColumn{
						{Title: "a", Expr: "a"},
						{Title: "p0", Expr: "_jbs_pattern__p_number__write__p0"},
					},
					Meta: lower.ResultTableMeta{Source: "write"},
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
	if !strings.Contains(text, "# Pattern sets used for result extraction") {
		t.Fatalf("missing patternset section comment: %s", text)
	}
	if !strings.Contains(text, "# Analyser definitions for parsing step output files") {
		t.Fatalf("missing analyser section comment: %s", text)
	}
	if !strings.Contains(text, "# Result tables generated from analyser output") {
		t.Fatalf("missing result section comment: %s", text)
	}
	if !strings.Contains(text, "# Param block 'matrix'") {
		t.Fatalf("missing param block role comment: %s", text)
	}
	if !strings.Contains(text, "# Patterns block 'p'") {
		t.Fatalf("missing patternset role comment: %s", text)
	}
	if !strings.Contains(text, "# From analyse 'write': alias 'p0' for pattern 'p.number'") {
		t.Fatalf("missing alias pattern entry comment: %s", text)
	}
	if !strings.Contains(text, "# Parameters for submit block 'run'") {
		t.Fatalf("missing submit parameterset role comment: %s", text)
	}
	if !strings.Contains(text, "# Step generated from do block 'setup'") {
		t.Fatalf("missing do step comment: %s", text)
	}
	if !strings.Contains(text, "# Step generated from submit block 'run'") {
		t.Fatalf("missing submit step comment: %s", text)
	}
	if !strings.Contains(text, "# Analyser generated from analyse block 'write'") {
		t.Fatalf("missing analyser item comment: %s", text)
	}
	if !strings.Contains(text, "# Result table generated from analyse block 'write'") {
		t.Fatalf("missing result table comment: %s", text)
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
	if !strings.Contains(text, "\n\n# Pattern sets used for result extraction") {
		t.Fatalf("missing blank-line separation before patternset section: %s", text)
	}
	if !strings.Contains(text, "\n\n# Analyser definitions for parsing step output files") {
		t.Fatalf("missing blank-line separation before analyser section: %s", text)
	}
	if !strings.Contains(text, "\n\n# Result tables generated from analyser output") {
		t.Fatalf("missing blank-line separation before result section: %s", text)
	}
	if !strings.Contains(text, "\n  - name: analyser_write\n    use: p\n    analyse:\n") {
		t.Fatalf("missing compact analyser use scalar output: %s", text)
	}

	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("encoded yaml with comments does not parse: %v", err)
	}
}

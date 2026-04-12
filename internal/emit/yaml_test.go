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
		Comment: "hello",
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
						Name:  "_jp__p_number__write__p0",
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
					Kind:   lower.PatternSetKindLet,
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
					Kind:          lower.StepKindSubmit,
					Source:        "run",
					InheritsFrom:  []string{"setup"},
					InheritedVars: []string{"a", "b"},
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
						{Title: "p0", Expr: "_jp__p_number__write__p0"},
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
	if !strings.Contains(text, "# benchmark comment\ncomment: hello") {
		t.Fatalf("missing benchmark comment root field annotation: %s", text)
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
	if !strings.Contains(text, "# Let namespace 'p' used for analyse extraction") {
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
	if !strings.Contains(text, "inherits from setup:") {
		t.Fatalf("missing inheritance note in step comment: %s", text)
	}
	if !strings.Contains(text, "\n  # - a\n  # - b\n") {
		t.Fatalf("missing inherited variable list in step comment: %s", text)
	}
	if !strings.Contains(text, "# Analyser generated from analyse block 'write'") {
		t.Fatalf("missing analyser item comment: %s", text)
	}
	if !strings.Contains(text, "# Result table generated from analyse block 'write'") {
		t.Fatalf("missing result table comment: %s", text)
	}
	if !strings.Contains(text, "\n\n# From jbs_outpath\noutpath: out\n\n# benchmark comment\ncomment: hello\n\n# Parameter sets used to create workpackage combinations") {
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

func TestYAMLRawSubmitBlocksOmitSeparatorKey(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		ParameterSet: []lower.ParameterSet{
			{
				Name:     "run__submit_params",
				InitWith: "platform.xml:systemParameter",
				Parameter: []lower.Parameter{
					{Name: "preprocess", Mode: "text", Value: lower.Literal("export X=1\n")},
					{Name: "postprocess", Mode: "text", Value: lower.Literal("export Y=2\n")},
				},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindSubmitInit,
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
	if strings.Contains(text, "- name: preprocess\n        mode: text\n        separator:") {
		t.Fatalf("preprocess must not emit separator key: %s", text)
	}
	if strings.Contains(text, "- name: postprocess\n        mode: text\n        separator:") {
		t.Fatalf("postprocess must not emit separator key: %s", text)
	}
}

func TestYAMLAnnotatesJRReservedSeparatorHelper(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		ParameterSet: []lower.ParameterSet{
			{
				Name: "_js__s0__p__a",
				Parameter: []lower.Parameter{
					{Name: "_ji__s0__p__a", Type: "int", Mode: "text", Value: "0,2"},
					{
						Name:      "_jr__s0__p__a",
						Mode:      "python",
						Separator: lower.ReservedSeparator,
						Value:     lower.SingleQuoted("{\"0\":\"0,1\",\"2\":\"2,3\"}[\"${_ji__s0__p__a}\"]"),
					},
					{Name: "a", Mode: "python", Value: lower.SingleQuoted("[1,1,2,2][$_ji__s0__p__a]")},
				},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindSubset,
					Source: "p",
					Step:   "s0",
				},
			},
		},
	}

	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "Internal helper: grouped source row IDs stay opaque with separator #### for after-step narrowing") {
		t.Fatalf("missing _jr helper comment: %s", text)
	}
	if !strings.Contains(text, "separator: '####'") {
		t.Fatalf("missing reserved separator output: %s", text)
	}
}

func TestYAMLAnnotatesLetSubsetParameterSetComment(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		ParameterSet: []lower.ParameterSet{
			{
				Name: "_js__s0__l__systemname",
				Parameter: []lower.Parameter{
					{Name: "_ji__s0__l__systemname", Type: "int", Mode: "text", Value: "0"},
					{Name: "systemname", Mode: "shell", Value: "hostname"},
				},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindSubset,
					Source: "l",
					Step:   "s0",
				},
			},
		},
	}

	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "Synthetic subset parameterset for step 's0' derived from 'l' for variable-only imports") {
		t.Fatalf("missing let subset comment: %s", text)
	}
}

func TestYAMLEmitsProjectedSourceComments(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		Step: []lower.Step{
			{
				Name: "run",
				Use:  []interface{}{"p"},
				Do:   []interface{}{lower.Literal("echo hi\n")},
				Meta: lower.StepMeta{
					Kind:   lower.StepKindDo,
					Source: "run",
				},
			},
		},
		Meta: lower.DocumentMeta{
			SourceComments: []lower.CommentProjection{
				{Target: "do:run.header.with", Text: "from source with clause"},
				{Target: "do:run.header", Text: "from source header"},
			},
		},
	}
	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "from source header") {
		t.Fatalf("missing projected header comment: %s", text)
	}
	if !strings.Contains(text, "from source with clause") {
		t.Fatalf("missing projected clause comment: %s", text)
	}
}

func TestYAMLEmitsProjectedCommentsForParamLetAnalyse(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		ParameterSet: []lower.ParameterSet{
			{
				Name:      "p",
				Parameter: []lower.Parameter{{Name: "a", Value: "1"}},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindParam,
					Source: "p",
				},
			},
		},
		PatternSet: []lower.PatternSet{
			{
				Name: "l",
				Pattern: []lower.Pattern{
					{Name: "number", Value: lower.SingleQuoted("Number: $jube_pat_int")},
				},
				Meta: lower.PatternSetMeta{
					Kind:   lower.PatternSetKindLet,
					Source: "l",
				},
			},
		},
		Analyser: []lower.Analyser{
			{
				Name: "analyser_write",
				Analyse: []lower.AnalyseItem{
					{Step: "write"},
				},
				Meta: lower.AnalyserMeta{Source: "write"},
			},
		},
		Meta: lower.DocumentMeta{
			SourceComments: []lower.CommentProjection{
				{Target: "param:p.header", Text: "param header comment"},
				{Target: "let:l.header", Text: "let header comment"},
				{Target: "analyse:write.header", Text: "analyse header comment"},
			},
		},
	}
	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "param header comment") {
		t.Fatalf("missing projected param comment: %s", text)
	}
	if !strings.Contains(text, "let header comment") {
		t.Fatalf("missing projected let comment: %s", text)
	}
	if !strings.Contains(text, "analyse header comment") {
		t.Fatalf("missing projected analyse comment: %s", text)
	}
}

func TestYAMLEmitsProjectedCommentsUnconditionally(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		Step: []lower.Step{
			{
				Name: "run",
				Meta: lower.StepMeta{
					Kind:   lower.StepKindDo,
					Source: "run",
				},
			},
		},
		Meta: lower.DocumentMeta{
			SourceComments: []lower.CommentProjection{
				{Target: "do:run.header", Text: "must appear"},
			},
		},
	}
	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "must appear") {
		t.Fatalf("projected comments must always be emitted: %s", text)
	}
}

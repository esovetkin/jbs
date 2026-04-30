package emit

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"jbs/internal/lower"
)

func findParameterSetNode(root *yaml.Node, name string) *yaml.Node {
	seq := mapValueNode(rootMap(root), "parameterset")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range seq.Content {
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		if value := mapValueNode(item, "name"); value != nil && value.Value == name {
			return item
		}
	}
	return nil
}

func findParameterNode(setNode *yaml.Node, name string) *yaml.Node {
	seq := mapValueNode(setNode, "parameter")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range seq.Content {
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		if value := mapValueNode(item, "name"); value != nil && value.Value == name {
			return item
		}
	}
	return nil
}

func TestYAMLRoundTripIncludesCommentsAndOmitRawSeparator(t *testing.T) {
	doc := lower.Document{
		Name:    "demo",
		Outpath: "out",
		Comment: "hello",
		ParameterSet: []lower.ParameterSet{
			{
				Name: "matrix",
				Parameter: []lower.Parameter{
					{Name: "a", Value: "1"},
					{Name: "label", Mode: "text", Separator: lower.ReservedSeparator, Value: "test,with,comma"},
				},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindGlobalTable,
					Source: "matrix",
				},
			},
			{
				Name: "_js__setup__matrix__a",
				Parameter: []lower.Parameter{
					{Name: "_ji__setup__matrix__a", Type: "int", Mode: "text", Value: "0,2"},
					{Name: "_jr__setup__matrix__a", Mode: "python", Separator: lower.ReservedSeparator, Value: lower.SingleQuoted(`{"0":"0,1","2":"2,3"}["${_ji__setup__matrix__a}"]`)},
					{Name: "a", Mode: "python", Value: lower.SingleQuoted("[1,1,2,2][$_ji__setup__matrix__a]")},
				},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindSubset,
					Source: "matrix",
					Step:   "setup",
				},
			},
			{
				Name:     "run__submit_params",
				InitWith: "platform.xml:systemParameter",
				Parameter: []lower.Parameter{
					{Name: "preprocess", Mode: "text", Value: lower.Literal("export X=1\n")},
					{Name: "postprocess", Mode: "text", Value: lower.Literal("export Y=2\n")},
					{Name: "queue", Mode: "python", Value: lower.SingleQuoted("${queue}")},
				},
				Meta: lower.ParameterSetMeta{
					Kind:   lower.ParameterSetKindSubmitInit,
					Source: "run",
				},
			},
		},
		PatternSet: []lower.PatternSet{
			{
				Name: "patterns",
				Pattern: []lower.Pattern{
					{Name: "_jp__patterns_number__write__p0", Type: "int", Value: lower.SingleQuoted("Number: $jube_pat_int"), Meta: lower.PatternMeta{IsAnalyseAlias: true, AnalyseStep: "write", AliasName: "p0", PatternRef: "patterns.number"}},
				},
				Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindImportedGlobals, Source: "patterns"},
			},
		},
		Step: []lower.Step{
			{Name: "setup", Use: []interface{}{"matrix"}, Do: []interface{}{lower.Literal("echo setup\n")}, Meta: lower.StepMeta{Kind: lower.StepKindDo, Source: "setup"}},
			{Name: "run", Use: []interface{}{"run__submit_params"}, Meta: lower.StepMeta{Kind: lower.StepKindSubmit, Source: "run", InheritsFrom: []string{"setup"}, InheritedVars: []string{"a", "b"}}},
		},
		Analyser: []lower.Analyser{
			{Name: "analyser_write", Use: "patterns", Analyse: []lower.AnalyseItem{{Step: "write", File: []lower.AnalyseFile{{Use: "patterns", Value: "out.log"}}}}, Meta: lower.AnalyserMeta{Source: "write"}},
		},
		Result: &lower.ResultObject{
			Use:   []string{"analyser_write"},
			Table: []lower.ResultTable{{Name: "result_write", Style: "csv", Column: []lower.ResultColumn{{Title: "p0", Expr: "_jp__patterns_number__write__p0"}}, Meta: lower.ResultTableMeta{Source: "write"}}},
		},
		Meta: lower.DocumentMeta{SourceComments: []lower.CommentProjection{{Target: "do:setup.header", Text: "setup header"}, {Target: "do:setup.header.with", Text: "setup with"}, {Target: "analyse:write.header", Text: "analyse header"}}},
	}

	data, err := YAML(doc)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(data)
	checks := []string{
		"# From jbs_name",
		"# From jbs_outpath",
		"# benchmark comment\ncomment: hello",
		"# Parameter sets used to create workpackage combinations",
		"# Pattern sets used for result extraction",
		"# Analyser definitions for parsing step output files",
		"# Result tables generated from analyser output",
		"# Table-valued global 'matrix'",
		"Synthetic subset parameterset for step 'setup' derived from 'matrix' for variable-only imports",
		"Parameters for submit block 'run'",
		"Imported globals from 'patterns' used for analyse extraction",
		"From analyse 'write': alias 'p0' for pattern 'patterns.number'",
		"Step generated from do block 'setup'",
		"Step generated from submit block 'run'",
		"inherits from setup:",
		"# - a",
		"# - b",
		"Analyser generated from analyse block 'write'",
		"Result table generated from analyse block 'write'",
		"Internal helper: grouped source row IDs stay opaque with separator #### for after-step narrowing",
		"setup header",
		"setup with",
		"analyse header",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("missing expected YAML content %q:\n%s", check, text)
		}
	}
	if !strings.Contains(text, "\n\n# From jbs_outpath\noutpath: out\n\n# benchmark comment\ncomment: hello") {
		t.Fatalf("missing root-field block spacing: %s", text)
	}
	if !strings.Contains(text, "\n\n  # Parameters for submit block 'run'") {
		t.Fatalf("missing spacing between parameterset blocks: %s", text)
	}
	if !strings.Contains(text, "\n\n  # Step generated from submit block 'run'") {
		t.Fatalf("missing spacing between step blocks: %s", text)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("encoded YAML does not parse: %v", err)
	}
	submitSet := findParameterSetNode(&root, "run__submit_params")
	if submitSet == nil {
		t.Fatalf("missing submit parameterset in parsed YAML")
	}
	matrixSet := findParameterSetNode(&root, "matrix")
	if matrixSet == nil {
		t.Fatalf("missing matrix parameterset in parsed YAML")
	}
	labelParam := findParameterNode(matrixSet, "label")
	if labelParam == nil {
		t.Fatalf("missing label parameter in parsed YAML")
	}
	if sep := mapValueNode(labelParam, "separator"); sep == nil || sep.Value != lower.ReservedSeparator {
		t.Fatalf("expected parsed scalar separator %q, got %#v", lower.ReservedSeparator, sep)
	}
	if value := mapValueNode(labelParam, "_"); value == nil || value.Value != "test,with,comma" {
		t.Fatalf("expected parsed scalar value to preserve commas, got %#v", value)
	}
	for _, name := range []string{"preprocess", "postprocess"} {
		param := findParameterNode(submitSet, name)
		if param == nil {
			t.Fatalf("missing %s parameter in parsed YAML", name)
		}
		if key := mapKeyNode(param, "separator"); key != nil {
			t.Fatalf("raw submit parameter %s must not emit a separator key", name)
		}
	}
}

func TestSpacingHelpers(t *testing.T) {
	if !shouldInsertSpacer("outpath: out", "name: demo") {
		t.Fatalf("expected spacer between name and outpath")
	}
	if !shouldInsertSpacer("# section", "name: demo") {
		t.Fatalf("expected spacer before root comment block")
	}
	if !shouldInsertSpacer("  # item", "value: x") {
		t.Fatalf("expected spacer before indented item comment")
	}
	if shouldInsertSpacer("  # item", "parameterset:") {
		t.Fatalf("did not expect spacer immediately after section key")
	}
	if shouldInsertSpacer("", "name: demo") {
		t.Fatalf("blank lines should not trigger spacer insertion")
	}

	in := []byte("name: demo\noutpath: out\n# section\nstep:\n  - name: s0\n  # item\n")
	got := string(addBlockSpacing(in))
	if !strings.Contains(got, "name: demo\n\noutpath: out") {
		t.Fatalf("expected spacer after name field, got %q", got)
	}
	if !strings.Contains(got, "outpath: out\n\n# section") {
		t.Fatalf("expected spacer before root comment block, got %q", got)
	}
	if !strings.Contains(got, "- name: s0\n\n  # item") {
		t.Fatalf("expected spacer before indented comment block, got %q", got)
	}
}

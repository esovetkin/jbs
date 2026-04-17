package emit

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"jbs/internal/lower"
)

func TestRootMapVariants(t *testing.T) {
	if got := rootMap(nil); got != nil {
		t.Fatalf("rootMap(nil) = %#v, want nil", got)
	}

	m := &yaml.Node{Kind: yaml.MappingNode}
	if got := rootMap(m); got != m {
		t.Fatalf("rootMap(mapping) should return the mapping node")
	}

	docEmpty := &yaml.Node{Kind: yaml.DocumentNode}
	if got := rootMap(docEmpty); got != nil {
		t.Fatalf("rootMap(empty document) = %#v, want nil", got)
	}

	docScalar := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "x"}}}
	if got := rootMap(docScalar); got != nil {
		t.Fatalf("rootMap(document scalar) = %#v, want nil", got)
	}

	docMap := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{m}}
	if got := rootMap(docMap); got != m {
		t.Fatalf("rootMap(document mapping) should return the first mapping child")
	}

	if got := rootMap(&yaml.Node{Kind: yaml.SequenceNode}); got != nil {
		t.Fatalf("rootMap(non-document non-map) = %#v, want nil", got)
	}
}

func TestMapAndSequenceHelpers(t *testing.T) {
	keyName := &yaml.Node{Kind: yaml.ScalarNode, Value: "name"}
	valName := &yaml.Node{Kind: yaml.ScalarNode, Value: "demo"}
	keyOut := &yaml.Node{Kind: yaml.ScalarNode, Value: "outpath"}
	valOut := &yaml.Node{Kind: yaml.ScalarNode, Value: "out"}
	m := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{keyName, valName, keyOut, valOut}}

	if got := mapKeyNode(nil, "name"); got != nil {
		t.Fatalf("mapKeyNode(nil) = %#v, want nil", got)
	}
	if got := mapKeyNode(&yaml.Node{Kind: yaml.SequenceNode}, "name"); got != nil {
		t.Fatalf("mapKeyNode(non-map) = %#v, want nil", got)
	}
	if got := mapKeyNode(m, "name"); got != keyName {
		t.Fatalf("mapKeyNode did not return the expected key node")
	}
	if got := mapKeyNode(m, "missing"); got != nil {
		t.Fatalf("mapKeyNode(missing) = %#v, want nil", got)
	}

	if got := mapValueNode(nil, "name"); got != nil {
		t.Fatalf("mapValueNode(nil) = %#v, want nil", got)
	}
	if got := mapValueNode(&yaml.Node{Kind: yaml.SequenceNode}, "name"); got != nil {
		t.Fatalf("mapValueNode(non-map) = %#v, want nil", got)
	}
	if got := mapValueNode(m, "outpath"); got != valOut {
		t.Fatalf("mapValueNode did not return the expected value node")
	}
	if got := mapValueNode(m, "missing"); got != nil {
		t.Fatalf("mapValueNode(missing) = %#v, want nil", got)
	}

	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{valName}}
	if got := seqItem(nil, 0); got != nil {
		t.Fatalf("seqItem(nil) = %#v, want nil", got)
	}
	if got := seqItem(m, 0); got != nil {
		t.Fatalf("seqItem(non-sequence) = %#v, want nil", got)
	}
	if got := seqItem(seq, -1); got != nil {
		t.Fatalf("seqItem(negative) = %#v, want nil", got)
	}
	if got := seqItem(seq, 1); got != nil {
		t.Fatalf("seqItem(out of range) = %#v, want nil", got)
	}
	if got := seqItem(seq, 0); got != valName {
		t.Fatalf("seqItem(valid) did not return the expected node")
	}
}

func TestCommentHelpers(t *testing.T) {
	setHeadComment(nil, "x")
	n := &yaml.Node{}
	setHeadComment(n, "")
	if n.HeadComment != "" {
		t.Fatalf("setHeadComment with empty text should not modify the node")
	}
	setHeadComment(n, "hello")
	if n.HeadComment != "hello" {
		t.Fatalf("setHeadComment did not set comment, got %q", n.HeadComment)
	}

	appendHeadComment(nil, "x")
	appendHeadComment(n, "   ")
	if n.HeadComment != "hello" {
		t.Fatalf("appendHeadComment with blank text should not modify the node")
	}
	appendHeadComment(n, "world")
	if n.HeadComment != "hello\nworld" {
		t.Fatalf("appendHeadComment should append with newline, got %q", n.HeadComment)
	}
}

func TestCommentBuilderFunctions(t *testing.T) {
	paramCases := []struct {
		name string
		in   lower.ParameterSet
		want string
	}{
		{name: "global table", in: lower.ParameterSet{Name: "jobs", Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindGlobalTable}}, want: "Table-valued global 'jobs'"},
		{name: "subset step source", in: lower.ParameterSet{Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset, Step: "s0", Source: "p"}}, want: "Synthetic subset parameterset for step 's0' derived from 'p' for variable-only imports"},
		{name: "subset source only", in: lower.ParameterSet{Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset, Source: "p"}}, want: "Synthetic subset parameterset derived from 'p' for variable-only imports"},
		{name: "subset generic", in: lower.ParameterSet{Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset}}, want: "Synthetic subset parameterset for variable-only imports"},
		{name: "submit init fallback", in: lower.ParameterSet{Name: "run__submit_params", Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubmitInit}}, want: "Parameters for submit block 'run__submit_params'"},
		{name: "default", in: lower.ParameterSet{Name: "x", Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKind("other")}}, want: "Generated parameterset 'x'"},
	}
	for _, tc := range paramCases {
		if got := parameterSetComment(tc.in); got != tc.want {
			t.Fatalf("%s: parameterSetComment() = %q, want %q", tc.name, got, tc.want)
		}
	}

	if got := parameterEntryComment(lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset}, lower.Parameter{Name: "_jr__s0__p__a", Separator: lower.ReservedSeparator}); !strings.Contains(got, "separator ####") {
		t.Fatalf("expected reserved-separator helper comment, got %q", got)
	}
	if got := parameterEntryComment(lower.ParameterSetMeta{Kind: lower.ParameterSetKindGlobalTable}, lower.Parameter{Name: "_jr__s0__p__a", Separator: lower.ReservedSeparator}); got != "" {
		t.Fatalf("expected non-subset parameter entry comment to be empty, got %q", got)
	}

	patternCases := []struct {
		name string
		in   lower.PatternSet
		want string
	}{
		{name: "imported globals with source", in: lower.PatternSet{Name: "p", Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindImportedGlobals, Source: "lib.p"}}, want: "Imported globals from 'lib.p' used for analyse extraction"},
		{name: "imported globals fallback", in: lower.PatternSet{Name: "p", Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindImportedGlobals}}, want: "Imported globals from 'p' used for analyse extraction"},
		{name: "inline with source", in: lower.PatternSet{Name: "p", Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindInlineAnalyse, Source: "write"}}, want: "Inline analyse extraction patterns for step 'write'"},
		{name: "inline fallback", in: lower.PatternSet{Name: "p", Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindInlineAnalyse}}, want: "Inline analyse extraction patterns in 'p'"},
		{name: "default", in: lower.PatternSet{Name: "p", Meta: lower.PatternSetMeta{Kind: lower.PatternSetKind("other")}}, want: "Generated pattern set 'p'"},
	}
	for _, tc := range patternCases {
		if got := patternSetComment(tc.in); got != tc.want {
			t.Fatalf("%s: patternSetComment() = %q, want %q", tc.name, got, tc.want)
		}
	}

	if got := patternEntryComment(lower.Pattern{}); got != "" {
		t.Fatalf("expected non-alias pattern comment to be empty, got %q", got)
	}
	aliasComment := patternEntryComment(lower.Pattern{Meta: lower.PatternMeta{IsAnalyseAlias: true, AnalyseStep: "write", AliasName: "p0", PatternRef: "p.number"}})
	if aliasComment != "From analyse 'write': alias 'p0' for pattern 'p.number'" {
		t.Fatalf("unexpected alias pattern comment: %q", aliasComment)
	}

	if got := analyserComment(lower.Analyser{Name: "a"}); got != "Analyser generated from analyse block 'a'" {
		t.Fatalf("unexpected analyser comment fallback: %q", got)
	}
	if got := resultTableComment(lower.ResultTable{Name: "r"}); got != "Result table generated from analyse block 'r'" {
		t.Fatalf("unexpected result table comment fallback: %q", got)
	}

	if got := stepComment(lower.Step{Name: "x"}); got != "Generated step 'x'" {
		t.Fatalf("unexpected generic step comment: %q", got)
	}
	if got := stepComment(lower.Step{Name: "d", Meta: lower.StepMeta{Kind: lower.StepKindDo}}); got != "Step generated from do block 'd'" {
		t.Fatalf("unexpected do step comment fallback: %q", got)
	}
	inherit := stepComment(lower.Step{Name: "s", Meta: lower.StepMeta{Kind: lower.StepKindSubmit, Source: "run", InheritsFrom: []string{"prep"}, InheritedVars: []string{"a"}}})
	if !strings.Contains(inherit, "Step generated from submit block 'run'; inherits from prep:") || !strings.Contains(inherit, "\n- a") {
		t.Fatalf("unexpected inherited step comment: %q", inherit)
	}
}

func TestAnnotateHelpersAndProjectedCommentRouting(t *testing.T) {
	makeStep := func(name string, withDepend, withUse, withMax, withProcs, withIter bool) *yaml.Node {
		n := &yaml.Node{Kind: yaml.MappingNode}
		addKV := func(k, v string) {
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &yaml.Node{Kind: yaml.ScalarNode, Value: v})
		}
		addKV("name", name)
		if withDepend {
			addKV("depend", "prep")
		}
		if withUse {
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "use"}, &yaml.Node{Kind: yaml.SequenceNode})
		}
		if withMax {
			addKV("max_async", "1")
		}
		if withProcs {
			addKV("procs", "1")
		}
		if withIter {
			addKV("iterations", "1")
		}
		return n
	}

	parameterSeq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "parameter"}, {Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}}}}}
	patternSeq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "pattern"}, {Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}}}}}
	stepSeq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{makeStep("s1", true, true, true, false, false), makeStep("s2", false, false, false, true, false), makeStep("s3", false, false, false, false, true)}}
	analyserSeq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}, {Kind: yaml.MappingNode}}}
	resultNode := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "table"}, {Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}}}

	annotateParameterSets(parameterSeq, []lower.ParameterSet{{Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset, Source: "p", Step: "s0"}, Parameter: []lower.Parameter{{Name: "_jr__s0__p__a", Separator: lower.ReservedSeparator}}}})
	annotatePatternSets(patternSeq, []lower.PatternSet{{Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindImportedGlobals, Source: "lib.p"}, Pattern: []lower.Pattern{{Meta: lower.PatternMeta{IsAnalyseAlias: true, AnalyseStep: "write", AliasName: "p0", PatternRef: "lib.p.number"}}}}})
	annotateSteps(stepSeq, []lower.Step{{Name: "s1", Meta: lower.StepMeta{Kind: lower.StepKindSubmit, Source: "s1"}}, {Name: "s2", Meta: lower.StepMeta{Kind: lower.StepKindDo, Source: "s2"}}, {Name: "s3", Meta: lower.StepMeta{Kind: lower.StepKindDo, Source: "s3"}}})
	annotateAnalysers(analyserSeq, []lower.Analyser{{Meta: lower.AnalyserMeta{Source: "skip"}}, {Meta: lower.AnalyserMeta{Source: "a"}}})
	annotateResult(resultNode, &lower.ResultObject{Table: []lower.ResultTable{{Meta: lower.ResultTableMeta{Source: "a"}}}})

	if !strings.Contains(seqItem(parameterSeq, 0).HeadComment, "Synthetic subset parameterset") {
		t.Fatalf("expected parameterset comment, got %#v", seqItem(parameterSeq, 0))
	}
	paramEntries := mapValueNode(seqItem(parameterSeq, 0), "parameter")
	if !strings.Contains(seqItem(paramEntries, 0).HeadComment, "separator ####") {
		t.Fatalf("expected parameter entry helper comment, got %#v", seqItem(paramEntries, 0))
	}
	if !strings.Contains(seqItem(patternSeq, 0).HeadComment, "Imported globals from 'lib.p'") {
		t.Fatalf("expected patternset comment, got %#v", seqItem(patternSeq, 0))
	}
	patternEntries := mapValueNode(seqItem(patternSeq, 0), "pattern")
	if !strings.Contains(seqItem(patternEntries, 0).HeadComment, "alias 'p0'") {
		t.Fatalf("expected pattern entry alias comment, got %#v", seqItem(patternEntries, 0))
	}
	if !strings.Contains(seqItem(analyserSeq, 1).HeadComment, "analyse block 'a'") {
		t.Fatalf("expected analyser comment, got %#v", seqItem(analyserSeq, 1))
	}
	if !strings.Contains(seqItem(mapValueNode(resultNode, "table"), 0).HeadComment, "analyse block 'a'") {
		t.Fatalf("expected result table comment, got %#v", seqItem(mapValueNode(resultNode, "table"), 0))
	}

	annotateProjectedStepComment(stepSeq, "badtarget", "x")
	annotateProjectedStepComment(stepSeq, "do:s1.body", "x")
	annotateProjectedStepComment(stepSeq, "do:missing.header", "x")
	annotateProjectedStepComment(stepSeq, "do:s1.header", "header")
	annotateProjectedStepComment(stepSeq, "do:s1.header.after", "after")
	annotateProjectedStepComment(stepSeq, "do:s1.header.use", "use")
	annotateProjectedStepComment(stepSeq, "do:s1.header.with", "with")
	annotateProjectedStepComment(stepSeq, "do:s1.header.options", "options-max")
	annotateProjectedStepComment(stepSeq, "do:s2.header.options", "options-procs")
	annotateProjectedStepComment(stepSeq, "do:s3.header.options", "options-iter")

	s1 := findStepNodeByName(stepSeq, "s1")
	if s1 == nil || !strings.Contains(s1.HeadComment, "header") {
		t.Fatalf("expected projected header comment on s1, got %#v", s1)
	}
	if key := mapKeyNode(s1, "depend"); key == nil || !strings.Contains(key.HeadComment, "after") {
		t.Fatalf("expected projected after comment on depend key, got %#v", key)
	}
	if key := mapKeyNode(s1, "use"); key == nil || !strings.Contains(key.HeadComment, "use") || !strings.Contains(key.HeadComment, "with") {
		t.Fatalf("expected projected use/with comments on use key, got %#v", key)
	}
	if key := mapKeyNode(s1, "max_async"); key == nil || !strings.Contains(key.HeadComment, "options-max") {
		t.Fatalf("expected options comment on max_async, got %#v", key)
	}
	if key := mapKeyNode(findStepNodeByName(stepSeq, "s2"), "procs"); key == nil || !strings.Contains(key.HeadComment, "options-procs") {
		t.Fatalf("expected options comment on procs, got %#v", key)
	}
	if key := mapKeyNode(findStepNodeByName(stepSeq, "s3"), "iterations"); key == nil || !strings.Contains(key.HeadComment, "options-iter") {
		t.Fatalf("expected options comment on iterations, got %#v", key)
	}

	annotateProjectedAnalyseComment(analyserSeq, []lower.Analyser{{Meta: lower.AnalyserMeta{Source: "skip"}}, {Name: "fallback_name"}}, "analyse:missing.header", "x")
	annotateProjectedAnalyseComment(analyserSeq, []lower.Analyser{{Meta: lower.AnalyserMeta{Source: "skip"}}, {Name: "fallback_name"}}, "analyse:fallback_name.header", "ana-c")
	if !strings.Contains(seqItem(analyserSeq, 1).HeadComment, "ana-c") {
		t.Fatalf("expected projected analyse comment on matching analyser, got %#v", seqItem(analyserSeq, 1))
	}

	root := &yaml.Node{Kind: yaml.MappingNode}
	addKV := func(key string, value *yaml.Node) {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	}
	addKV("step", stepSeq)
	addKV("analyser", analyserSeq)
	annotateProjectedComments(root, lower.Document{Analyser: []lower.Analyser{{Meta: lower.AnalyserMeta{Source: "skip"}}, {Name: "fallback_name"}}, Meta: lower.DocumentMeta{SourceComments: []lower.CommentProjection{{Target: "", Text: "skip-empty"}, {Target: "unknown:target", Text: "skip-unknown"}, {Target: "do:s1.header", Text: "step-route"}, {Target: "analyse:fallback_name.header", Text: "analyse-route"}}}})
	if !strings.Contains(findStepNodeByName(stepSeq, "s1").HeadComment, "step-route") {
		t.Fatalf("expected routed step projection comment")
	}
	if !strings.Contains(seqItem(analyserSeq, 1).HeadComment, "analyse-route") {
		t.Fatalf("expected routed analyse projection comment")
	}
}

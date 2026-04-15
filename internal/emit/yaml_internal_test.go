package emit

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"jbs/internal/lower"
)

func TestRootMapVariants(t *testing.T) {
	if got := rootMap(nil); got != nil {
		t.Fatalf("rootMap(nil)=%#v, want nil", got)
	}

	m := &yaml.Node{Kind: yaml.MappingNode}
	if got := rootMap(m); got != m {
		t.Fatalf("rootMap(mapping) should return mapping node")
	}

	docEmpty := &yaml.Node{Kind: yaml.DocumentNode}
	if got := rootMap(docEmpty); got != nil {
		t.Fatalf("rootMap(empty doc)=%#v, want nil", got)
	}

	docScalar := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "x"}},
	}
	if got := rootMap(docScalar); got != nil {
		t.Fatalf("rootMap(doc scalar)=%#v, want nil", got)
	}

	docMap := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{m},
	}
	if got := rootMap(docMap); got != m {
		t.Fatalf("rootMap(doc map) should return first mapping content node")
	}

	if got := rootMap(&yaml.Node{Kind: yaml.SequenceNode}); got != nil {
		t.Fatalf("rootMap(non-doc non-map)=%#v, want nil", got)
	}
}

func TestMapAndSequenceHelpers(t *testing.T) {
	keyName := &yaml.Node{Kind: yaml.ScalarNode, Value: "name"}
	valName := &yaml.Node{Kind: yaml.ScalarNode, Value: "demo"}
	keyOut := &yaml.Node{Kind: yaml.ScalarNode, Value: "outpath"}
	valOut := &yaml.Node{Kind: yaml.ScalarNode, Value: "out"}
	m := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			keyName, valName, keyOut, valOut,
		},
	}

	if got := mapKeyNode(nil, "name"); got != nil {
		t.Fatalf("mapKeyNode(nil)=%#v, want nil", got)
	}
	if got := mapKeyNode(&yaml.Node{Kind: yaml.SequenceNode}, "name"); got != nil {
		t.Fatalf("mapKeyNode(non-map)=%#v, want nil", got)
	}
	if got := mapKeyNode(m, "name"); got != keyName {
		t.Fatalf("mapKeyNode did not return expected key node")
	}
	if got := mapKeyNode(m, "missing"); got != nil {
		t.Fatalf("mapKeyNode missing=%#v, want nil", got)
	}

	if got := mapValueNode(nil, "name"); got != nil {
		t.Fatalf("mapValueNode(nil)=%#v, want nil", got)
	}
	if got := mapValueNode(&yaml.Node{Kind: yaml.SequenceNode}, "name"); got != nil {
		t.Fatalf("mapValueNode(non-map)=%#v, want nil", got)
	}
	if got := mapValueNode(m, "outpath"); got != valOut {
		t.Fatalf("mapValueNode did not return expected value node")
	}
	if got := mapValueNode(m, "missing"); got != nil {
		t.Fatalf("mapValueNode missing=%#v, want nil", got)
	}

	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{valName}}
	if got := seqItem(nil, 0); got != nil {
		t.Fatalf("seqItem(nil)=%#v, want nil", got)
	}
	if got := seqItem(m, 0); got != nil {
		t.Fatalf("seqItem(non-seq)=%#v, want nil", got)
	}
	if got := seqItem(seq, -1); got != nil {
		t.Fatalf("seqItem(negative index)=%#v, want nil", got)
	}
	if got := seqItem(seq, 1); got != nil {
		t.Fatalf("seqItem(out-of-range)=%#v, want nil", got)
	}
	if got := seqItem(seq, 0); got != valName {
		t.Fatalf("seqItem(valid) did not return expected element")
	}
}

func TestCommentHelpers(t *testing.T) {
	setHeadComment(nil, "x")
	n := &yaml.Node{}
	setHeadComment(n, "")
	if n.HeadComment != "" {
		t.Fatalf("setHeadComment with empty text should not modify node")
	}
	setHeadComment(n, "hello")
	if n.HeadComment != "hello" {
		t.Fatalf("setHeadComment did not set comment, got %q", n.HeadComment)
	}

	appendHeadComment(nil, "x")
	appendHeadComment(n, "   ")
	if n.HeadComment != "hello" {
		t.Fatalf("appendHeadComment with blank text should not modify node")
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
		{
			name: "param source fallback to name",
			in: lower.ParameterSet{
				Name: "p",
				Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindParam},
			},
			want: "Param block 'p'",
		},
		{
			name: "subset with step and source",
			in: lower.ParameterSet{
				Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset, Step: "s0", Source: "p"},
			},
			want: "Synthetic subset parameterset for step 's0' derived from 'p' for variable-only imports",
		},
		{
			name: "subset with source only",
			in: lower.ParameterSet{
				Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset, Source: "p"},
			},
			want: "Synthetic subset parameterset derived from 'p' for variable-only imports",
		},
		{
			name: "subset generic",
			in: lower.ParameterSet{
				Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset},
			},
			want: "Synthetic subset parameterset for variable-only imports",
		},
		{
			name: "submit init source fallback to name",
			in: lower.ParameterSet{
				Name: "run__submit_params",
				Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubmitInit},
			},
			want: "Parameters for submit block 'run__submit_params'",
		},
		{
			name: "default kind",
			in: lower.ParameterSet{
				Name: "x",
				Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKind("other")},
			},
			want: "Generated parameterset 'x'",
		},
	}
	for _, tc := range paramCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parameterSetComment(tc.in); got != tc.want {
				t.Fatalf("parameterSetComment()=%q, want %q", got, tc.want)
			}
		})
	}

	patternCases := []struct {
		name string
		in   lower.PatternSet
		want string
	}{
		{
			name: "let with source",
			in: lower.PatternSet{
				Name: "p",
				Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindLet, Source: "l"},
			},
			want: "Let namespace 'l' used for analyse extraction",
		},
		{
			name: "let source fallback name",
			in: lower.PatternSet{
				Name: "p",
				Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindLet},
			},
			want: "Let namespace 'p' used for analyse extraction",
		},
		{
			name: "inline with source",
			in: lower.PatternSet{
				Name: "p",
				Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindInline, Source: "write"},
			},
			want: "Inline analyse extraction patterns for step 'write'",
		},
		{
			name: "inline source fallback name",
			in: lower.PatternSet{
				Name: "p",
				Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindInline},
			},
			want: "Inline analyse extraction patterns in 'p'",
		},
		{
			name: "default kind",
			in: lower.PatternSet{
				Name: "p",
				Meta: lower.PatternSetMeta{Kind: lower.PatternSetKind("other")},
			},
			want: "Generated pattern set 'p'",
		},
	}
	for _, tc := range patternCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := patternSetComment(tc.in); got != tc.want {
				t.Fatalf("patternSetComment()=%q, want %q", got, tc.want)
			}
		})
	}

	if got := analyserComment(lower.Analyser{Name: "a"}); got != "Analyser generated from analyse block 'a'" {
		t.Fatalf("unexpected analyserComment fallback: %q", got)
	}
	if got := resultTableComment(lower.ResultTable{Name: "r"}); got != "Result table generated from analyse block 'r'" {
		t.Fatalf("unexpected resultTableComment fallback: %q", got)
	}

	if got := stepComment(lower.Step{Name: "x"}); got != "Generated step 'x'" {
		t.Fatalf("unexpected generic step comment: %q", got)
	}
	if got := stepComment(lower.Step{Name: "d", Meta: lower.StepMeta{Kind: lower.StepKindDo}}); got != "Step generated from do block 'd'" {
		t.Fatalf("unexpected do step comment fallback: %q", got)
	}
	inherit := stepComment(lower.Step{
		Name: "s",
		Meta: lower.StepMeta{
			Kind:          lower.StepKindSubmit,
			Source:        "run",
			InheritsFrom:  []string{"prep"},
			InheritedVars: []string{"a"},
		},
	})
	if !strings.Contains(inherit, "Step generated from submit block 'run'; inherits from prep:") ||
		!strings.Contains(inherit, "\n- a") {
		t.Fatalf("unexpected inherited step comment: %q", inherit)
	}
}

func TestProjectedStepCommentsBranches(t *testing.T) {
	makeStep := func(name string, withDepend, withUse bool, withMax, withProcs, withIter bool) *yaml.Node {
		n := &yaml.Node{Kind: yaml.MappingNode}
		addKV := func(k, v string) {
			n.Content = append(n.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: k},
				&yaml.Node{Kind: yaml.ScalarNode, Value: v},
			)
		}
		addKV("name", name)
		if withDepend {
			addKV("depend", "prep")
		}
		if withUse {
			n.Content = append(n.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "use"},
				&yaml.Node{Kind: yaml.SequenceNode},
			)
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

	stepSeq := &yaml.Node{
		Kind: yaml.SequenceNode,
		Content: []*yaml.Node{
			makeStep("s1", true, true, true, false, false),
			makeStep("s2", false, false, false, true, false),
			makeStep("s3", false, false, false, false, true),
		},
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
		t.Fatalf("expected header comment on step s1, got %#v", s1)
	}
	dependKey := mapKeyNode(s1, "depend")
	if dependKey == nil || !strings.Contains(dependKey.HeadComment, "after") {
		t.Fatalf("expected projected after comment on depend key, got %#v", dependKey)
	}
	useKey := mapKeyNode(s1, "use")
	if useKey == nil || !strings.Contains(useKey.HeadComment, "use") || !strings.Contains(useKey.HeadComment, "with") {
		t.Fatalf("expected projected use/with comments on use key, got %#v", useKey)
	}
	if key := mapKeyNode(s1, "max_async"); key == nil || !strings.Contains(key.HeadComment, "options-max") {
		t.Fatalf("expected options comment on max_async, got %#v", key)
	}

	s2 := findStepNodeByName(stepSeq, "s2")
	if key := mapKeyNode(s2, "procs"); key == nil || !strings.Contains(key.HeadComment, "options-procs") {
		t.Fatalf("expected options comment on procs, got %#v", key)
	}
	s3 := findStepNodeByName(stepSeq, "s3")
	if key := mapKeyNode(s3, "iterations"); key == nil || !strings.Contains(key.HeadComment, "options-iter") {
		t.Fatalf("expected options comment on iterations, got %#v", key)
	}
}

func TestProjectedParamLetAnalyseComments(t *testing.T) {
	seqWithItems := func(n int) *yaml.Node {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0, n)}
		for i := 0; i < n; i++ {
			seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.MappingNode})
		}
		return seq
	}

	paramSeq := seqWithItems(2)
	patternSeq := seqWithItems(2)
	analyserSeq := seqWithItems(2)

	paramSets := []lower.ParameterSet{
		{Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindSubset, Source: "skip"}},
		{Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindParam, Source: "p"}},
	}
	patternSets := []lower.PatternSet{
		{Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindInline, Source: "skip"}},
		{Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindLet, Source: "l"}},
	}
	analysers := []lower.Analyser{
		{Meta: lower.AnalyserMeta{Source: "skip"}},
		{Meta: lower.AnalyserMeta{Source: "a"}},
	}

	annotateProjectedParamComment(paramSeq, paramSets, "param:missing.header", "x")
	annotateProjectedParamComment(paramSeq, paramSets, "param:p.header", "param-c")
	if !strings.Contains(seqItem(paramSeq, 1).HeadComment, "param-c") {
		t.Fatalf("expected projected param comment on matching param set")
	}

	annotateProjectedLetComment(patternSeq, patternSets, "let:missing.header", "x")
	annotateProjectedLetComment(patternSeq, patternSets, "let:l.header", "let-c")
	if !strings.Contains(seqItem(patternSeq, 1).HeadComment, "let-c") {
		t.Fatalf("expected projected let comment on matching pattern set")
	}

	annotateProjectedAnalyseComment(analyserSeq, analysers, "analyse:missing.header", "x")
	annotateProjectedAnalyseComment(analyserSeq, analysers, "analyse:a.header", "ana-c")
	if !strings.Contains(seqItem(analyserSeq, 1).HeadComment, "ana-c") {
		t.Fatalf("expected projected analyse comment on matching analyser")
	}
}

func TestAnnotateResultAndProjectedCommentsRouting(t *testing.T) {
	annotateResult(nil, nil)
	annotateResult(&yaml.Node{Kind: yaml.SequenceNode}, nil)
	annotateResult(&yaml.Node{Kind: yaml.MappingNode}, nil)
	annotateResult(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "table"},
		{Kind: yaml.MappingNode},
	}}, &lower.ResultObject{})

	doc := lower.Document{
		ParameterSet: []lower.ParameterSet{
			{Meta: lower.ParameterSetMeta{Kind: lower.ParameterSetKindParam, Source: "p"}},
		},
		PatternSet: []lower.PatternSet{
			{Meta: lower.PatternSetMeta{Kind: lower.PatternSetKindLet, Source: "l"}},
		},
		Step: []lower.Step{
			{Name: "s0"},
		},
		Analyser: []lower.Analyser{
			{Meta: lower.AnalyserMeta{Source: "a"}},
		},
		Meta: lower.DocumentMeta{
			SourceComments: []lower.CommentProjection{
				{Target: "", Text: "skip-empty"},
				{Target: "unknown:target", Text: "skip-unknown"},
				{Target: "param:p.header", Text: "param-route"},
				{Target: "let:l.header", Text: "let-route"},
				{Target: "do:s0.header", Text: "step-route"},
				{Target: "analyse:a.header", Text: "analyse-route"},
			},
		},
	}

	root := &yaml.Node{Kind: yaml.MappingNode}
	addKV := func(key string, value *yaml.Node) {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	}
	addKV("parameterset", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}})
	addKV("patternset", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}})
	addKV("step", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "name"},
		{Kind: yaml.ScalarNode, Value: "s0"},
	}}}})
	addKV("analyser", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}})

	annotateProjectedComments(root, doc)

	if !strings.Contains(seqItem(mapValueNode(root, "parameterset"), 0).HeadComment, "param-route") {
		t.Fatalf("expected routed param projection comment")
	}
	if !strings.Contains(seqItem(mapValueNode(root, "patternset"), 0).HeadComment, "let-route") {
		t.Fatalf("expected routed let projection comment")
	}
	if stepNode := findStepNodeByName(mapValueNode(root, "step"), "s0"); stepNode == nil || !strings.Contains(stepNode.HeadComment, "step-route") {
		t.Fatalf("expected routed step projection comment")
	}
	if !strings.Contains(seqItem(mapValueNode(root, "analyser"), 0).HeadComment, "analyse-route") {
		t.Fatalf("expected routed analyse projection comment")
	}
}

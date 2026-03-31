package emit

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"jbs/internal/lower"
)

func YAML(doc lower.Document) ([]byte, error) {
	var node yaml.Node
	if err := node.Encode(doc); err != nil {
		return nil, err
	}
	annotateComments(&node, doc)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return addBlockSpacing(buf.Bytes()), nil
}

func annotateComments(root *yaml.Node, doc lower.Document) {
	m := rootMap(root)
	if m == nil {
		return
	}

	setHeadComment(mapKeyNode(m, "name"), "From jbs_name")
	setHeadComment(mapKeyNode(m, "outpath"), "From jbs_outpath")
	setHeadComment(mapKeyNode(m, "parameterset"), "Parameter sets used to create workpackage combinations")
	setHeadComment(mapKeyNode(m, "patternset"), "Pattern sets used for result extraction")
	setHeadComment(mapKeyNode(m, "step"), "Steps executed by JUBE")
	setHeadComment(mapKeyNode(m, "analyser"), "Analyser definitions for parsing step output files")
	setHeadComment(mapKeyNode(m, "result"), "Result tables generated from analyser output")

	annotateParameterSets(mapValueNode(m, "parameterset"), doc.ParameterSet)
	annotatePatternSets(mapValueNode(m, "patternset"), doc.PatternSet)
	annotateSteps(mapValueNode(m, "step"), doc.Step)
	annotateAnalysers(mapValueNode(m, "analyser"), doc.Analyser)
	annotateResult(mapValueNode(m, "result"), doc.Result)
}

func annotateParameterSets(seq *yaml.Node, sets []lower.ParameterSet) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for i := 0; i < len(sets) && i < len(seq.Content); i++ {
		item := seqItem(seq, i)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		setHeadComment(item, parameterSetComment(sets[i]))
	}
}

func annotateSteps(seq *yaml.Node, steps []lower.Step) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for i := 0; i < len(steps) && i < len(seq.Content); i++ {
		item := seqItem(seq, i)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		setHeadComment(item, stepComment(steps[i]))
	}
}

func annotatePatternSets(seq *yaml.Node, sets []lower.PatternSet) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for i := 0; i < len(sets) && i < len(seq.Content); i++ {
		item := seqItem(seq, i)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		setHeadComment(item, patternSetComment(sets[i]))
		annotatePatternEntries(mapValueNode(item, "pattern"), sets[i].Pattern)
	}
}

func annotatePatternEntries(seq *yaml.Node, patterns []lower.Pattern) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for i := 0; i < len(patterns) && i < len(seq.Content); i++ {
		item := seqItem(seq, i)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		comment := patternEntryComment(patterns[i])
		if comment == "" {
			continue
		}
		setHeadComment(item, comment)
	}
}

func annotateAnalysers(seq *yaml.Node, analysers []lower.Analyser) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for i := 0; i < len(analysers) && i < len(seq.Content); i++ {
		item := seqItem(seq, i)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		setHeadComment(item, analyserComment(analysers[i]))
	}
}

func annotateResult(node *yaml.Node, result *lower.ResultObject) {
	if node == nil || node.Kind != yaml.MappingNode || result == nil {
		return
	}
	tables := mapValueNode(node, "table")
	if tables == nil || tables.Kind != yaml.SequenceNode {
		return
	}
	for i := 0; i < len(result.Table) && i < len(tables.Content); i++ {
		item := seqItem(tables, i)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		setHeadComment(item, resultTableComment(result.Table[i]))
	}
}

func parameterSetComment(ps lower.ParameterSet) string {
	switch ps.Meta.Kind {
	case lower.ParameterSetKindParam:
		src := ps.Meta.Source
		if src == "" {
			src = ps.Name
		}
		return fmt.Sprintf("Param block '%s'", src)
	case lower.ParameterSetKindSubset:
		if ps.Meta.Source != "" {
			return fmt.Sprintf("Synthetic subset parameterset derived from '%s' for variable-only imports", ps.Meta.Source)
		}
		return "Synthetic subset parameterset for variable-only imports"
	case lower.ParameterSetKindSubmitInit:
		src := ps.Meta.Source
		if src == "" {
			src = ps.Name
		}
		return fmt.Sprintf("Parameters for submit block '%s'", src)
	default:
		return fmt.Sprintf("Generated parameterset '%s'", ps.Name)
	}
}

func stepComment(step lower.Step) string {
	base := fmt.Sprintf("Generated step '%s'", step.Name)
	switch step.Meta.Kind {
	case lower.StepKindDo:
		src := step.Meta.Source
		if src == "" {
			src = step.Name
		}
		base = fmt.Sprintf("Step generated from do block '%s'", src)
	case lower.StepKindSubmit:
		src := step.Meta.Source
		if src == "" {
			src = step.Name
		}
		base = fmt.Sprintf("Step generated from submit block '%s'", src)
	}
	if len(step.Meta.InheritsFrom) == 0 {
		return base
	}
	lines := []string{
		fmt.Sprintf("%s; inherits from %s:", base, strings.Join(step.Meta.InheritsFrom, ", ")),
	}
	for _, name := range step.Meta.InheritedVars {
		lines = append(lines, "- "+name)
	}
	return strings.Join(lines, "\n")
}

func patternSetComment(ps lower.PatternSet) string {
	switch ps.Meta.Kind {
	case lower.PatternSetKindBase:
		if ps.Meta.Source != "" {
			return fmt.Sprintf("Patterns block '%s'", ps.Meta.Source)
		}
		return fmt.Sprintf("Patterns block '%s'", ps.Name)
	default:
		return fmt.Sprintf("Generated pattern set '%s'", ps.Name)
	}
}

func patternEntryComment(p lower.Pattern) string {
	if !p.Meta.IsAnalyseAlias {
		return ""
	}
	return fmt.Sprintf(
		"From analyse '%s': alias '%s' for pattern '%s'",
		p.Meta.AnalyseStep,
		p.Meta.AliasName,
		p.Meta.PatternRef,
	)
}

func analyserComment(an lower.Analyser) string {
	src := an.Meta.Source
	if src == "" {
		src = an.Name
	}
	return fmt.Sprintf("Analyser generated from analyse block '%s'", src)
}

func resultTableComment(table lower.ResultTable) string {
	src := table.Meta.Source
	if src == "" {
		src = table.Name
	}
	return fmt.Sprintf("Result table generated from analyse block '%s'", src)
}

func rootMap(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil
		}
		if root.Content[0].Kind == yaml.MappingNode {
			return root.Content[0]
		}
		return nil
	}
	if root.Kind == yaml.MappingNode {
		return root
	}
	return nil
}

func mapKeyNode(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return k
		}
	}
	return nil
}

func mapValueNode(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return v
		}
	}
	return nil
}

func seqItem(seq *yaml.Node, idx int) *yaml.Node {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	if idx < 0 || idx >= len(seq.Content) {
		return nil
	}
	return seq.Content[idx]
}

func setHeadComment(n *yaml.Node, text string) {
	if n == nil || text == "" {
		return
	}
	n.HeadComment = text
}

func addBlockSpacing(in []byte) []byte {
	lines := strings.Split(string(in), "\n")
	out := make([]string, 0, len(lines)+16)
	prevNonEmpty := ""

	for _, line := range lines {
		if shouldInsertSpacer(line, prevNonEmpty) {
			out = append(out, "")
		}
		out = append(out, line)
		if strings.TrimSpace(line) != "" {
			prevNonEmpty = line
		}
	}
	return []byte(strings.Join(out, "\n"))
}

func shouldInsertSpacer(line, prevNonEmpty string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	prevTrimmed := strings.TrimSpace(prevNonEmpty)
	if strings.HasPrefix(line, "outpath:") && strings.HasPrefix(prevTrimmed, "name:") {
		return true
	}
	if strings.HasPrefix(line, "# ") {
		return prevTrimmed != "" && !strings.HasPrefix(prevTrimmed, "#")
	}
	if strings.HasPrefix(line, "  # ") {
		return prevTrimmed != "" &&
			!strings.HasPrefix(prevTrimmed, "#") &&
			prevTrimmed != "parameterset:" &&
			prevTrimmed != "patternset:" &&
			prevTrimmed != "step:" &&
			prevTrimmed != "analyser:" &&
			prevTrimmed != "table:"
	}
	return false
}

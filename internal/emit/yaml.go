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
	setHeadComment(mapKeyNode(m, "step"), "Steps executed by JUBE")

	annotateParameterSets(mapValueNode(m, "parameterset"), doc.ParameterSet)
	annotateSteps(mapValueNode(m, "step"), doc.Step)
}

func annotateParameterSets(seq *yaml.Node, sets []lower.ParameterSet) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	submitTargetToGlobal := submitParameterGlobalMap()
	for i := 0; i < len(sets) && i < len(seq.Content); i++ {
		item := seqItem(seq, i)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		setHeadComment(item, parameterSetComment(sets[i]))
		annotateSubmitParameterComments(item, sets[i], submitTargetToGlobal)
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
	switch step.Meta.Kind {
	case lower.StepKindDo:
		src := step.Meta.Source
		if src == "" {
			src = step.Name
		}
		return fmt.Sprintf("Step generated from do block '%s'", src)
	case lower.StepKindSubmit:
		src := step.Meta.Source
		if src == "" {
			src = step.Name
		}
		return fmt.Sprintf("Step generated from submit block '%s'", src)
	default:
		return fmt.Sprintf("Generated step '%s'", step.Name)
	}
}

func annotateSubmitParameterComments(psNode *yaml.Node, ps lower.ParameterSet, submitTargetToGlobal map[string]string) {
	if ps.Meta.Kind != lower.ParameterSetKindSubmitInit {
		return
	}
	parameterSeq := mapValueNode(psNode, "parameter")
	if parameterSeq == nil || parameterSeq.Kind != yaml.SequenceNode {
		return
	}
	for i := 0; i < len(parameterSeq.Content); i++ {
		paramNode := seqItem(parameterSeq, i)
		if paramNode == nil || paramNode.Kind != yaml.MappingNode {
			continue
		}
		nameNode := mapValueNode(paramNode, "name")
		if nameNode == nil || nameNode.Kind != yaml.ScalarNode {
			continue
		}
		if globalName, ok := submitTargetToGlobal[nameNode.Value]; ok {
			setHeadComment(paramNode, fmt.Sprintf("From %s", globalName))
		}
	}
}

func submitParameterGlobalMap() map[string]string {
	out := make(map[string]string)
	for _, spec := range lower.BuiltinGlobals() {
		if spec.Target == "" {
			continue
		}
		out[spec.Target] = spec.Name
	}
	return out
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
		return prevTrimmed != ""
	}
	if strings.HasPrefix(line, "  # ") {
		return prevTrimmed != "" && prevTrimmed != "parameterset:" && prevTrimmed != "step:"
	}
	return false
}

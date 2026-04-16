package lower

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLiteralMarshalYAML(t *testing.T) {
	n, err := Literal("line0\nline1\n").MarshalYAML()
	if err != nil {
		t.Fatalf("Literal MarshalYAML returned error: %v", err)
	}
	node, ok := n.(*yaml.Node)
	if !ok {
		t.Fatalf("expected yaml.Node pointer, got %T", n)
	}
	if node.Kind != yaml.ScalarNode || node.Style != yaml.LiteralStyle || node.Tag != "!!str" || node.Value != "line0\nline1\n" {
		t.Fatalf("unexpected literal node shape: %#v", node)
	}
}

func TestSingleQuotedMarshalYAML(t *testing.T) {
	n, err := SingleQuoted(`"${x:-y}"`).MarshalYAML()
	if err != nil {
		t.Fatalf("SingleQuoted MarshalYAML returned error: %v", err)
	}
	node, ok := n.(*yaml.Node)
	if !ok {
		t.Fatalf("expected yaml.Node pointer, got %T", n)
	}
	if node.Kind != yaml.ScalarNode || node.Style != yaml.SingleQuotedStyle || node.Tag != "!!str" || node.Value != `"${x:-y}"` {
		t.Fatalf("unexpected single-quoted node shape: %#v", node)
	}
}

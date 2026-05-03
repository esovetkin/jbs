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

func TestLiteralMarshalYAMLRaggedIndentUsesQuotedStyle(t *testing.T) {
	n, err := Literal("    cat <<EOF\npayload\nEOF\n").MarshalYAML()
	if err != nil {
		t.Fatalf("Literal MarshalYAML returned error: %v", err)
	}
	node, ok := n.(*yaml.Node)
	if !ok {
		t.Fatalf("expected yaml.Node pointer, got %T", n)
	}
	if node.Kind != yaml.ScalarNode || node.Style != yaml.DoubleQuotedStyle || node.Tag != "!!str" || node.Value != "    cat <<EOF\npayload\nEOF\n" {
		t.Fatalf("unexpected ragged literal node shape: %#v", node)
	}
}

func TestLiteralMarshalYAMLUniformIndentKeepsLiteralStyle(t *testing.T) {
	n, err := Literal("echo one\necho two\n").MarshalYAML()
	if err != nil {
		t.Fatalf("Literal MarshalYAML returned error: %v", err)
	}
	node, ok := n.(*yaml.Node)
	if !ok {
		t.Fatalf("expected yaml.Node pointer, got %T", n)
	}
	if node.Kind != yaml.ScalarNode || node.Style != yaml.LiteralStyle || node.Tag != "!!str" || node.Value != "echo one\necho two\n" {
		t.Fatalf("unexpected uniform literal node shape: %#v", node)
	}
}

func TestLiteralMarshalYAMLColumnZeroHeredocKeepsLiteralStyle(t *testing.T) {
	n, err := Literal("cat <<EOF\npayload\nEOF\n").MarshalYAML()
	if err != nil {
		t.Fatalf("Literal MarshalYAML returned error: %v", err)
	}
	node, ok := n.(*yaml.Node)
	if !ok {
		t.Fatalf("expected yaml.Node pointer, got %T", n)
	}
	if node.Kind != yaml.ScalarNode || node.Style != yaml.LiteralStyle || node.Tag != "!!str" || node.Value != "cat <<EOF\npayload\nEOF\n" {
		t.Fatalf("unexpected column-zero literal node shape: %#v", node)
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

package sema

import (
	"testing"

	"jbs/internal/diag"
)

func refNames(refs []varRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Name)
	}
	return out
}

func TestCollectShellLikeRefsMalformedBraces(t *testing.T) {
	text := "echo ${x\n" +
		"echo ${}\n" +
		"echo $x.txt\n"
	refs := collectShellLikeRefs(text, diag.NewPos(0, 1, 1), "in.jbs")
	names := refNames(refs)
	if len(names) != 1 || names[0] != "x" {
		t.Fatalf("expected only $x.txt to be detected, got %#v", names)
	}
}

func TestCollectShellLikeRefsHashAndPatternVariants(t *testing.T) {
	text := "echo ${x##a} ${#x} ${!x}\n"
	refs := collectShellLikeRefs(text, diag.NewPos(0, 1, 1), "in.jbs")
	names := refNames(refs)
	if len(names) != 3 {
		t.Fatalf("expected three refs, got %#v", names)
	}
	for idx, name := range names {
		if name != "x" {
			t.Fatalf("expected ref %d to be x, got %#v (all=%#v)", idx, name, names)
		}
	}
}

func TestCollectSubmitStringRefsCountsSingleQuotedPayloadVars(t *testing.T) {
	text := "-lc 'echo id=${id}; echo label=${label}'"
	refs := collectSubmitStringRefs(text, diag.NewPos(0, 1, 1), "in.jbs")
	names := refNames(refs)
	if len(names) != 2 {
		t.Fatalf("expected two refs, got %#v", names)
	}
	if names[0] != "id" || names[1] != "label" {
		t.Fatalf("unexpected refs: %#v", names)
	}
}

func TestCollectSubmitStringRefsEscapedDollarIgnored(t *testing.T) {
	text := "-lc 'echo \\$x \\${x:-1} ${x}'"
	refs := collectSubmitStringRefs(text, diag.NewPos(0, 1, 1), "in.jbs")
	names := refNames(refs)
	if len(names) != 1 || names[0] != "x" {
		t.Fatalf("expected only unescaped ${x} to be detected, got %#v", names)
	}
}

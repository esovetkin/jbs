package lower

import (
	"testing"

	"jbs/internal/eval"
)

// parity contract:
// odd number of preceding '\' escapes '$' and blocks alias rewrite;
// even number means '$' is active and can be rewritten.
func TestRewriteShellRefsEscapedDollarParity(t *testing.T) {
	aliases := map[string]string{
		"nodes": "_ja__nodes",
		"x":     "_ja__x",
	}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "bareNoEscape", in: `$nodes`, want: `$_ja__nodes`},
		{name: "bareOddEscaped", in: `\$nodes`, want: `\$nodes`},
		{name: "bareEvenActive", in: `\\$nodes`, want: `\\$_ja__nodes`},
		{name: "bareTripleEscaped", in: `\\\$nodes`, want: `\\\$nodes`},
		{name: "bracedNoEscape", in: `${nodes}`, want: `${_ja__nodes}`},
		{name: "bracedOddEscaped", in: `\${nodes}`, want: `\${nodes}`},
		{name: "bracedEvenActive", in: `\\${nodes}`, want: `\\${_ja__nodes}`},
		{name: "bracedTripleEscaped", in: `\\\${nodes}`, want: `\\\${nodes}`},
		{name: "defaultOddEscaped", in: `\${nodes:-0}`, want: `\${nodes:-0}`},
		{name: "defaultEvenActive", in: `\\${nodes:-0}`, want: `\\${_ja__nodes:-0}`},
		{name: "assignEvenActive", in: `\\${nodes:=0}`, want: `\\${_ja__nodes:=0}`},
		{name: "altEvenActive", in: `\\${nodes:+x}`, want: `\\${_ja__nodes:+x}`},
		{name: "suffixEvenActive", in: `\\${nodes%.*}`, want: `\\${_ja__nodes%.*}`},
		{name: "prefixEvenActive", in: `\\${nodes#pre}`, want: `\\${_ja__nodes#pre}`},
		{name: "unknownAliasEvenActive", in: `\\$unknown`, want: `\\$unknown`},
		{name: "mixedLine", in: `\\$nodes \${nodes} ${x} \\\$nodes`, want: `\\$_ja__nodes \${nodes} ${_ja__x} \\\$nodes`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteShellRefs(tc.in, aliases)
			if got != tc.want {
				t.Fatalf("rewriteShellRefs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRewriteShellRefsBracedOperators(t *testing.T) {
	aliases := map[string]string{"nodes": "_ja__nodes"}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default", in: `${nodes:-0}`, want: `${_ja__nodes:-0}`},
		{name: "assign", in: `${nodes:=0}`, want: `${_ja__nodes:=0}`},
		{name: "alternate", in: `${nodes:+x}`, want: `${_ja__nodes:+x}`},
		{name: "suffixShortest", in: `${nodes%.*}`, want: `${_ja__nodes%.*}`},
		{name: "suffixLongest", in: `${nodes%%.*}`, want: `${_ja__nodes%%.*}`},
		{name: "prefixShortest", in: `${nodes#pre}`, want: `${_ja__nodes#pre}`},
		{name: "prefixLongest", in: `${nodes##pre}`, want: `${_ja__nodes##pre}`},
		{name: "slice", in: `${nodes:1}`, want: `${_ja__nodes:1}`},
		{name: "sliceRange", in: `${nodes:1:2}`, want: `${_ja__nodes:1:2}`},
		{name: "length", in: `${#nodes}`, want: `${#_ja__nodes}`},
		{name: "indirect", in: `${!nodes}`, want: `${!_ja__nodes}`},
		{name: "indirectTail", in: `${!nodes[@]}`, want: `${!_ja__nodes[@]}`},
		{name: "nestedTailPreserved", in: `${nodes:-${fallback}}`, want: `${_ja__nodes:-${fallback}}`},
		{name: "mixedLine", in: `$nodes ${nodes} ${nodes:-x}`, want: `$_ja__nodes ${_ja__nodes} ${_ja__nodes:-x}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteShellRefs(tc.in, aliases)
			if got != tc.want {
				t.Fatalf("rewriteShellRefs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRewriteShellRefsSafetyCases(t *testing.T) {
	aliases := map[string]string{"nodes": "_ja__nodes"}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "escapedBare", in: `\$nodes`, want: `\$nodes`},
		{name: "escapedBraced", in: `\${nodes:-x}`, want: `\${nodes:-x}`},
		{name: "specialUnbraced", in: `$$ $? $1`, want: `$$ $? $1`},
		{name: "specialBraced", in: `${1} ${?} ${@} ${*}`, want: `${1} ${?} ${@} ${*}`},
		{name: "nonIdentHead", in: `${1:-x}`, want: `${1:-x}`},
		{name: "unknownAlias", in: `${unknown:-x}`, want: `${unknown:-x}`},
		{name: "malformedOpen", in: `${nodes:-x`, want: `${nodes:-x`},
		{name: "malformedShort", in: `${`, want: `${`},
		{name: "escapedBraceInTail", in: `${nodes#\}}`, want: `${_ja__nodes#\}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteShellRefs(tc.in, aliases)
			if got != tc.want {
				t.Fatalf("rewriteShellRefs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRewriteShellRefsNoOpAliasMap(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		aliases map[string]string
	}{
		{name: "nil", in: `${nodes:-0}`, aliases: nil},
		{name: "empty", in: `${nodes:-0}`, aliases: map[string]string{}},
		{name: "unrelated", in: `${nodes:-0}`, aliases: map[string]string{"queue": "_ja__queue"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteShellRefs(tc.in, tc.aliases)
			if got != tc.in {
				t.Fatalf("rewriteShellRefs(%q) = %q, want unchanged", tc.in, got)
			}
		})
	}
}

func TestRewriteShellRefsInEvalValue(t *testing.T) {
	aliases := map[string]string{
		"nodes": "_ja__nodes",
		"queue": "_ja__queue",
	}
	in := eval.List([]eval.Value{
		eval.String(`$nodes ${queue:-batch}`),
		eval.Int(7),
		eval.Tuple([]eval.Value{
			eval.String(`${nodes}`),
			eval.Bool(true),
		}),
	})
	got := rewriteShellRefsInEvalValue(in, aliases)
	want := eval.List([]eval.Value{
		eval.String(`$_ja__nodes ${_ja__queue:-batch}`),
		eval.Int(7),
		eval.Tuple([]eval.Value{
			eval.String(`${_ja__nodes}`),
			eval.Bool(true),
		}),
	})
	if !eval.Equal(got, want) {
		t.Fatalf("rewriteShellRefsInEvalValue(list) = %#v, want %#v", got, want)
	}

	scalar := eval.Float(3.5)
	gotScalar := rewriteShellRefsInEvalValue(scalar, aliases)
	if !eval.Equal(gotScalar, scalar) {
		t.Fatalf("rewriteShellRefsInEvalValue(scalar) = %#v, want unchanged %#v", gotScalar, scalar)
	}

	noAlias := rewriteShellRefsInEvalValue(in, nil)
	if !eval.Equal(noAlias, in) {
		t.Fatalf("rewriteShellRefsInEvalValue(nil aliases) changed value: got %#v, want %#v", noAlias, in)
	}
}

func TestRewriteShellRefsEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		aliases map[string]string
		want    string
	}{
		{
			name:    "trailingDollar",
			in:      `echo foo$`,
			aliases: map[string]string{"foo": "_ja__foo"},
			want:    `echo foo$`,
		},
		{
			name:    "nonIdentifierAfterDollar",
			in:      `$- $/ $.`,
			aliases: map[string]string{"x": "_ja__x"},
			want:    `$- $/ $.`,
		},
		{
			name:    "emptyAliasIgnoredBareAndBraced",
			in:      `$nodes ${nodes:-0}`,
			aliases: map[string]string{"nodes": ""},
			want:    `$nodes ${nodes:-0}`,
		},
		{
			name:    "emptyInput",
			in:      ``,
			aliases: map[string]string{"nodes": "_ja__nodes"},
			want:    ``,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteShellRefs(tc.in, tc.aliases)
			if got != tc.want {
				t.Fatalf("rewriteShellRefs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

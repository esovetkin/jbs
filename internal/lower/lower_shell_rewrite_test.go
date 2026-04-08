package lower

import "testing"

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

package lower

import "testing"

func TestNormalizeRawLiteral(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty remains empty",
			in:   "",
			want: "",
		},
		{
			name: "existing trailing newline kept",
			in:   "echo ok\n",
			want: "echo ok\n",
		},
		{
			name: "missing trailing newline is appended",
			in:   "echo ok",
			want: "echo ok\n",
		},
		{
			name: "normalization happens before newline append",
			in:   "\n  a\r\n\tb\t \r\n\n",
			want: " a\nb\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRawLiteral(tt.in); got != tt.want {
				t.Fatalf("normalizeRawLiteral(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeRawBlock(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "all blank input collapses to empty",
			in:   " \n\t \n",
			want: "",
		},
		{
			name: "leading and trailing blank lines removed",
			in:   "\n    a\n      b\n\n",
			want: "a\n  b",
		},
		{
			name: "interior blank lines are preserved",
			in:   "\n    a\n\n      b\n",
			want: "a\n\n  b",
		},
		{
			name: "trailing horizontal whitespace per line is trimmed",
			in:   "    a\t \n      b  ",
			want: "a\n  b",
		},
		{
			name: "mixed line endings are normalized",
			in:   "\ra\r\nb\n",
			want: "a\nb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRawBlock(tt.in); got != tt.want {
				t.Fatalf("normalizeRawBlock(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeRawBlockPreservesIndentedHeredocPayloadWhenDelimiterIsColumnZero(t *testing.T) {
	in := "\n    cat > file <<EOF\n    keep spaces\nEOF\n"
	want := "    cat > file <<EOF\n    keep spaces\nEOF"
	if got := normalizeRawBlock(in); got != want {
		t.Fatalf("normalizeRawBlock(%q)=%q, want %q", in, got, want)
	}
}

func TestStripIndent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{
			name: "non-positive indent does nothing",
			in:   "  abc",
			n:    0,
			want: "  abc",
		},
		{
			name: "strips exactly n whitespace runes",
			in:   "\t abc",
			n:    2,
			want: "abc",
		},
		{
			name: "stops at first non-whitespace before n",
			in:   " a",
			n:    2,
			want: "a",
		},
		{
			name: "line without leading whitespace is unchanged",
			in:   "abc",
			n:    3,
			want: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripIndent(tt.in, tt.n); got != tt.want {
				t.Fatalf("stripIndent(%q, %d)=%q, want %q", tt.in, tt.n, got, tt.want)
			}
		})
	}
}

package shellref

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func refNames(refs []Ref) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Name)
	}
	return out
}

func TestCollectMalformedBraces(t *testing.T) {
	text := "echo ${x\n" +
		"echo ${}\n" +
		"echo $x.txt\n"
	refs := Collect(text, diag.NewPos(0, 1, 1), "in.jbs")
	names := refNames(refs)
	if len(names) != 1 || names[0] != "x" {
		t.Fatalf("expected only $x.txt to be detected, got %#v", names)
	}
}

func TestCollectHashAndPatternVariants(t *testing.T) {
	text := "echo ${x##a} ${#x} ${!x}\n"
	refs := Collect(text, diag.NewPos(0, 1, 1), "in.jbs")
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

func TestCollectNestedRefsInBracedExpansions(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "bare default",
			text: "echo ${missing:-$fallback}",
			want: []string{"missing", "fallback"},
		},
		{
			name: "nested braced default",
			text: "echo ${x:-${fallback}}",
			want: []string{"x", "fallback"},
		},
		{
			name: "double quoted default",
			text: `echo "${missing:-$fallback}"`,
			want: []string{"missing", "fallback"},
		},
		{
			name: "recursive defaults",
			text: "echo ${x:-${y:-$z}}",
			want: []string{"x", "y", "z"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refs := Collect(tc.text, diag.NewPos(0, 1, 1), "in.jbs")
			if got := refNames(refs); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected refs: got=%#v want=%#v", got, tc.want)
			}
		})
	}
}

func TestCollectQuotes(t *testing.T) {
	refs := Collect("echo '$single' \"$double\" $plain\n", diag.NewPos(0, 1, 1), "in.jbs")
	got := refNames(refs)
	want := []string{"double", "plain"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refs: got=%#v want=%#v", got, want)
	}
}

func TestNamesUniqueOrder(t *testing.T) {
	got := Names("$b $a $b ${c:-x} '$d'")
	want := []string{"b", "a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected names: got=%#v want=%#v", got, want)
	}
}

func TestNamesDeduplicatesNestedBracedRefs(t *testing.T) {
	got := Names("$fallback ${missing:-$fallback} ${missing:-${other}}")
	want := []string{"fallback", "missing", "other"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected names: got=%#v want=%#v", got, want)
	}
}

func TestIsEscapedDollarParity(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "noEscape", text: "$x", want: false},
		{name: "oddOne", text: "\\$x", want: true},
		{name: "evenTwo", text: "\\\\$x", want: false},
		{name: "oddThree", text: "\\\\\\$x", want: true},
		{name: "evenFour", text: "\\\\\\\\$x", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runes := []rune(tc.text)
			idx := -1
			for i, r := range runes {
				if r == '$' {
					idx = i
					break
				}
			}
			if idx < 0 {
				t.Fatalf("test input %q has no '$'", tc.text)
			}
			if got := isEscapedDollar(runes, idx); got != tc.want {
				t.Fatalf("isEscapedDollar(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestCollectDollarParity(t *testing.T) {
	text := "echo \\$x\n" +
		"echo \\\\$x\n" +
		"echo \\\\\\$x\n" +
		"echo \\\\\\\\$x\n"

	refs := Collect(text, diag.NewPos(0, 1, 1), "in.jbs")
	got := refNames(refs)
	want := []string{"x", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refs for parity scan: got=%#v want=%#v", got, want)
	}
}

func TestCollectSkipsCommentsAndInvalidDollarForms(t *testing.T) {
	text := "# $first\n" +
		"echo $1 $- $\n" +
		"echo ${unterminated\n" +
		"run ;# $second\n" +
		"echo $ok\n"

	refs := Collect(text, diag.NewPos(0, 1, 1), "in.jbs")
	got := refNames(refs)
	want := []string{"ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refs: got=%#v want=%#v", got, want)
	}
}

func TestCollectDoubleQuoteBackslashHandling(t *testing.T) {
	text := `echo "\$skip" "$take" "trail\`

	refs := Collect(text, diag.NewPos(0, 1, 1), "in.jbs")
	got := refNames(refs)
	want := []string{"take"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refs: got=%#v want=%#v", got, want)
	}
}

func TestCollectSpanUsesBasePosition(t *testing.T) {
	refs := Collect("α\n  $name tail", diag.NewPos(20, 7, 5), "script.jbs")
	if len(refs) != 1 {
		t.Fatalf("expected one ref, got %#v", refs)
	}
	want := Ref{
		Name: "name",
		Span: diag.NewSpan(
			"script.jbs",
			diag.NewPos(24, 8, 3),
			diag.NewPos(29, 8, 8),
		),
	}
	if !reflect.DeepEqual(refs[0], want) {
		t.Fatalf("unexpected ref: got=%#v want=%#v", refs[0], want)
	}
}

func TestParseHelpers(t *testing.T) {
	if end, ok := parseBareVarName([]rune("abc123 rest"), 0); !ok || end != 6 {
		t.Fatalf("parseBareVarName valid case = (%d,%v), want (6,true)", end, ok)
	}
	if _, ok := parseBareVarName([]rune("1abc"), 0); ok {
		t.Fatalf("parseBareVarName should reject digit start")
	}

	tests := []struct {
		expr        string
		start       int
		wantName    string
		wantNameEnd int
		wantEnd     int
		wantOK      bool
	}{
		{expr: "${x}", start: 2, wantName: "x", wantNameEnd: 3, wantEnd: 3, wantOK: true},
		{expr: "${#x}", start: 2, wantName: "x", wantNameEnd: 4, wantEnd: 4, wantOK: true},
		{expr: "${!x}", start: 2, wantName: "x", wantNameEnd: 4, wantEnd: 4, wantOK: true},
		{expr: "${x:-${y}}", start: 2, wantName: "x", wantNameEnd: 3, wantEnd: 9, wantOK: true},
		{expr: "${x\\}}", start: 2, wantName: "x", wantNameEnd: 3, wantEnd: 5, wantOK: true},
		{expr: "${}", start: 2, wantOK: false},
		{expr: "${#1}", start: 2, wantOK: false},
		{expr: "${x", start: 2, wantOK: false},
		{expr: "$", start: 1, wantOK: false},
	}
	for _, tc := range tests {
		got, gotOK := parseBracedVarRef([]rune(tc.expr), tc.start)
		if got.Name != tc.wantName || got.NameEnd != tc.wantNameEnd || got.End != tc.wantEnd || gotOK != tc.wantOK {
			t.Fatalf("parseBracedVarRef(%q,%d) = (%q,%d,%d,%v), want (%q,%d,%d,%v)", tc.expr, tc.start, got.Name, got.NameEnd, got.End, gotOK, tc.wantName, tc.wantNameEnd, tc.wantEnd, tc.wantOK)
		}
	}

	commentTests := []struct {
		text string
		idx  int
		want bool
	}{
		{text: "#x", idx: 0, want: true},
		{text: "a#x", idx: 1, want: false},
		{text: " #x", idx: 1, want: true},
		{text: ";#x", idx: 1, want: true},
		{text: "x", idx: 0, want: false},
		{text: "#", idx: -1, want: false},
		{text: "#", idx: 2, want: false},
	}
	for _, tc := range commentTests {
		if got := isCommentStart([]rune(tc.text), tc.idx); got != tc.want {
			t.Fatalf("isCommentStart(%q,%d) = %v, want %v", tc.text, tc.idx, got, tc.want)
		}
	}

	boundaryTests := []struct {
		r    rune
		want bool
	}{
		{r: ' ', want: true},
		{r: '\t', want: true},
		{r: '\n', want: true},
		{r: '\r', want: true},
		{r: ';', want: true},
		{r: '|', want: true},
		{r: '&', want: true},
		{r: '(', want: true},
		{r: ')', want: true},
		{r: '{', want: true},
		{r: '}', want: true},
		{r: 'a', want: false},
		{r: '_', want: false},
		{r: '.', want: false},
	}
	for _, tc := range boundaryTests {
		if got := isShellCommentBoundary(tc.r); got != tc.want {
			t.Fatalf("isShellCommentBoundary(%q) = %v, want %v", tc.r, got, tc.want)
		}
	}
}

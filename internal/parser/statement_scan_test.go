package parser

import "testing"

func TestScanStructuralState(t *testing.T) {
	cases := []struct {
		name           string
		src            string
		wantNeedsMore  bool
		wantBraceDepth int
		wantParenDepth int
		wantBrackDepth int
		wantSingle     bool
		wantDouble     bool
		wantLineCont   bool
		wantHereDoc    bool
	}{
		{name: "simple_complete", src: `x = 1`, wantNeedsMore: false},
		{name: "open_brace", src: "do run {", wantNeedsMore: true, wantBraceDepth: 1},
		{name: "open_paren", src: "x = (1, 2", wantNeedsMore: true, wantParenDepth: 1},
		{name: "open_bracket", src: "x = [1, 2", wantNeedsMore: true, wantBrackDepth: 1},
		{name: "open_single", src: "x = 'abc", wantNeedsMore: true, wantSingle: true},
		{name: "open_double", src: "x = \"abc", wantNeedsMore: true, wantDouble: true},
		{name: "line_continuation", src: "x = 1 \\", wantNeedsMore: true, wantLineCont: true},
		{name: "line_continuation_with_spaces", src: "x = 1 \\   ", wantNeedsMore: true, wantLineCont: true},
		{name: "double_backslash_no_continuation", src: `x = 1 \\\\`, wantNeedsMore: false, wantLineCont: false},
		{name: "comment_ignored_for_continuation", src: "x = 1 # \\", wantNeedsMore: false},
		{name: "delimiters_inside_quotes", src: `x = "{[()]}"`, wantNeedsMore: false},
		{name: "unmatched_closer_does_not_require_more", src: "x = 1}", wantNeedsMore: false},
		{
			name: "raw_block_like_nested",
			src: `do run {
	if true {
	  echo hi
	}
	`,
			wantNeedsMore:  true,
			wantBraceDepth: 1,
		},
		{
			name: "raw_block_like_complete",
			src: `do run {
	if true {
	  echo hi
	}
	}`,
			wantNeedsMore: false,
		},
		{
			name: "multiline_function_literal_needs_closing_brace",
			src: `f = function(x) {
  x`,
			wantNeedsMore:  true,
			wantBraceDepth: 1,
		},
		{
			name: "multiline_if_else_complete",
			src: `if true {
  x = 1
} else {
  x = 2
}`,
			wantNeedsMore: false,
		},
		{
			name: "multiline_if_needs_else_closing_brace",
			src: `if true {
  x = 1
} else {
  x = 2`,
			wantNeedsMore:  true,
			wantBraceDepth: 1,
		},
		{
			name: "multiline_for_needs_closing_brace",
			src: `for x in range(3) {
  x`,
			wantNeedsMore:  true,
			wantBraceDepth: 1,
		},
		{
			name: "multiline_while_complete",
			src: `while false {
  break
}`,
			wantNeedsMore: false,
		},
		{
			name: "anonymous_multiline_call_needs_closing_paren",
			src: `function(x) {
  x
}(`,
			wantNeedsMore:  true,
			wantParenDepth: 1,
		},
		{
			name: "comments_with_braces_do_not_confuse_tracking",
			src: `function(x) {
	  # } ] )
	  x
}`,
			wantNeedsMore: false,
		},
		{
			name: "do_block_heredoc_waits_past_brace",
			src: `do run {
cat <<EOF
}`,
			wantNeedsMore:  true,
			wantBraceDepth: 1,
			wantHereDoc:    true,
		},
		{
			name: "do_block_heredoc_complete",
			src: `do run {
cat <<EOF
}
EOF
}`,
			wantNeedsMore: false,
		},
		{
			name: "do_block_quoted_heredoc_complete",
			src: `do run {
cat <<'JSON'
{"a": {"b": 1}}
JSON
}`,
			wantNeedsMore: false,
		},
		{
			name:          "do_block_strip_tabs_heredoc_complete",
			src:           "do run {\ncat <<-EOF\n\t}\n\tEOF\n}",
			wantNeedsMore: false,
		},
		{
			name:          "do_block_hash_parameter_expansion_complete",
			src:           "do run {\necho ${file#*.}\n}",
			wantNeedsMore: false,
		},
		{
			name:           "do_block_hash_parameter_expansion_waits_for_block_close",
			src:            "do run {\necho ${file#*.}",
			wantNeedsMore:  true,
			wantBraceDepth: 1,
		},
		{
			name:          "do_block_nested_parameter_expansion_complete",
			src:           "do run {\necho ${file:-${fallback#*.}}\n}",
			wantNeedsMore: false,
		},
		{
			name:          "single_quote_escaped_quote_complete",
			src:           `x = '\''`,
			wantNeedsMore: false,
		},
		{
			name:          "double_quote_escaped_quote_complete",
			src:           `x = "\""`,
			wantNeedsMore: false,
		},
		{
			name:          "double_quote_parameter_expansion_brace_is_not_block_close",
			src:           `do run {` + "\n" + `echo "${name:-}}"` + "\n}",
			wantNeedsMore: false,
		},
		{
			name:          "balanced_parens_and_brackets_close",
			src:           `x = ([1])`,
			wantNeedsMore: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := ScanStructuralState(tc.src)
			if got.NeedsMoreInput() != tc.wantNeedsMore {
				t.Fatalf("NeedsMoreInput()=%v want %v; state=%+v", got.NeedsMoreInput(), tc.wantNeedsMore, got)
			}
			if got.BraceDepth != tc.wantBraceDepth {
				t.Fatalf("BraceDepth=%d want %d", got.BraceDepth, tc.wantBraceDepth)
			}
			if got.ParenDepth != tc.wantParenDepth {
				t.Fatalf("ParenDepth=%d want %d", got.ParenDepth, tc.wantParenDepth)
			}
			if got.BracketDepth != tc.wantBrackDepth {
				t.Fatalf("BracketDepth=%d want %d", got.BracketDepth, tc.wantBrackDepth)
			}
			if got.InSingle != tc.wantSingle {
				t.Fatalf("InSingle=%v want %v", got.InSingle, tc.wantSingle)
			}
			if got.InDouble != tc.wantDouble {
				t.Fatalf("InDouble=%v want %v", got.InDouble, tc.wantDouble)
			}
			if got.LineContinue != tc.wantLineCont {
				t.Fatalf("LineContinue=%v want %v", got.LineContinue, tc.wantLineCont)
			}
			if got.HereDocPending != tc.wantHereDoc {
				t.Fatalf("HereDocPending=%v want %v", got.HereDocPending, tc.wantHereDoc)
			}
		})
	}
}

func TestScanTopLevelStatementOffsets(t *testing.T) {
	t.Run("start at eof", func(t *testing.T) {
		src := []rune("x = 1")
		stmtEnd, nextOff := scanTopLevelStatementOffsets(src, len(src))
		if stmtEnd != len(src) || nextOff != len(src) {
			t.Fatalf("expected EOF offsets, got stmtEnd=%d nextOff=%d", stmtEnd, nextOff)
		}
	})

	t.Run("line comment inside grouping does not terminate statement", func(t *testing.T) {
		src := []rune("x = (1 # inner\n + 2)\ny = 3\n")
		stmtEnd, nextOff := scanTopLevelStatementOffsets(src, 0)
		wantStmt := len("x = (1 # inner\n + 2)")
		wantNext := wantStmt + 1
		if stmtEnd != wantStmt || nextOff != wantNext {
			t.Fatalf("unexpected offsets: stmtEnd=%d nextOff=%d want %d/%d", stmtEnd, nextOff, wantStmt, wantNext)
		}
	})

	t.Run("escaped quotes inside strings stay in statement", func(t *testing.T) {
		src := []rune(`x = "\"" + '\''` + "\ny = 1\n")
		stmtEnd, nextOff := scanTopLevelStatementOffsets(src, 0)
		wantStmt := len(`x = "\"" + '\''`)
		wantNext := wantStmt + 1
		if stmtEnd != wantStmt || nextOff != wantNext {
			t.Fatalf("unexpected offsets: stmtEnd=%d nextOff=%d want %d/%d", stmtEnd, nextOff, wantStmt, wantNext)
		}
	})

	t.Run("semicolon inside brackets is ignored", func(t *testing.T) {
		src := []rune("x = [1; 2]; y = 3\n")
		stmtEnd, nextOff := scanTopLevelStatementOffsets(src, 0)
		wantStmt := len("x = [1; 2]")
		wantNext := wantStmt + 1
		if stmtEnd != wantStmt || nextOff != wantNext {
			t.Fatalf("unexpected offsets: stmtEnd=%d nextOff=%d want %d/%d", stmtEnd, nextOff, wantStmt, wantNext)
		}
	})

	t.Run("top level comment at eof", func(t *testing.T) {
		src := []rune("x = 1 # trailing")
		stmtEnd, nextOff := scanTopLevelStatementOffsets(src, 0)
		want := len("x = 1 ")
		if stmtEnd != want || nextOff != len(src) {
			t.Fatalf("unexpected offsets: stmtEnd=%d nextOff=%d want stmtEnd=%d nextOff=%d", stmtEnd, nextOff, want, len(src))
		}
	})
}

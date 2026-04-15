package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func analyzeShellSrc(t *testing.T, src string) *diag.Diagnostics {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	return diags
}

func TestShellSuffixRefStopsAtDotAndDoesNotRaiseE100(t *testing.T) {
	diags := analyzeShellSrc(t, `
param p {
  x = (1,2)
  x
}
do run with p {
  echo $x.txt
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if hasDiagCode(diags, "E100") {
		t.Fatalf("did not expect E100 for $x.txt, got: %s", diags.String())
	}
	if hasW310For(diags, "p", "x") {
		t.Fatalf("did not expect W310 for p.x; $x.txt should count as usage, got: %s", diags.String())
	}
}

func TestShellUsageScanning(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantW310 int
	}{
		{
			name: "braced default counts as usage",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo ${x:-1}
}
`,
			wantW310: 0,
		},
		{
			name: "comment reference ignored",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  # ${x}
  echo done
}
`,
			wantW310: 1,
		},
		{
			name: "single quoted reference ignored",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo '${x}'
}
`,
			wantW310: 1,
		},
		{
			name: "double quoted reference counts",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo "${x}"
}
`,
			wantW310: 0,
		},
		{
			name: "hash inside word is not comment start",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo foo#bar${x}
}
`,
			wantW310: 0,
		},
		{
			name: "escaped dollar ignored",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo \$x
}
`,
			wantW310: 1,
		},
		{
			name: "escaped dollar inside double quotes ignored",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo "\$x"
}
`,
			wantW310: 1,
		},
		{
			name: "even backslashes before dollar count as usage",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo \\$x
}
`,
			wantW310: 0,
		},
		{
			name: "even backslashes before braced ref count as usage",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo \\${x:-1}
}
`,
			wantW310: 0,
		},
		{
			name: "braced variants count",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo ${x:=1}
  echo ${x:+ok}
  echo ${x:?err}
  echo ${x:1}
  echo ${x:1:2}
  echo ${x%a}
  echo ${!x}
}
`,
			wantW310: 0,
		},
		{
			name: "malformed braced expansions do not count as usage",
			src: `
param p {
  x = (1,2)
  x
}
do run with p {
  echo ${}
  echo ${!}
  echo ${1}
}
`,
			wantW310: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := analyzeShellSrc(t, tc.src)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if hasDiagCode(diags, "E100") {
				t.Fatalf("did not expect E100 in shell scanning case, got: %s", diags.String())
			}
			if got := diagCount(diags, "W310"); got != tc.wantW310 {
				t.Fatalf("unexpected W310 count: got=%d want=%d diags=%s", got, tc.wantW310, diags.String())
			}
		})
	}
}

func TestSubmitUsageScanning(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantW310 int
		wantW311 int
	}{
		{
			name: "raw submit blocks follow shell rules",
			src: `
param p {
  x = (1,2)
  x
}
submit run with p {
  preprocess = {
    # ${x}
    echo '${x}'
    echo ${x:-1}
  }
}
`,
			wantW310: 0,
			wantW311: 0,
		},
		{
			name: "submit key expression counts usage",
			src: `
param p {
  queue_name = ("devel")
  queue_name
}
submit run with p {
  queue = "${queue_name}"
}
`,
			wantW310: 0,
			wantW311: 0,
		},
		{
			name: "hash and pattern variants count usage",
			src: `
param p {
  x = ("abc")
  x
}
submit run with p {
  args_exec = "${x##a} ${#x} ${!x}"
}
`,
			wantW310: 0,
			wantW311: 0,
		},
		{
			name: "args_exec single quoted payload counts usage",
			src: `
param jobs {
  id = (1,2)
  label = ("alpha","beta")
  id + label
}
submit run0 with jobs {
  account = "atmlaml"
  queue = "batch"
  executable = "/bin/bash"
  args_exec = "-lc 'echo id=${id} > run.out; echo label=${label} >> run.out'"
}
`,
			wantW310: 0,
			wantW311: 0,
		},
		{
			name: "single quoted braced variants count usage",
			src: `
param p {
  x = ("abc")
  x
}
submit run with p {
  args_exec = "-lc 'echo ${x:-1} ${x%a} ${#x} ${!x}'"
}
`,
			wantW310: 0,
			wantW311: 0,
		},
		{
			name: "escaped dollars do not count usage",
			src: `
param p {
  x = (1,2)
  x
}
submit run with p {
  args_exec = "-lc 'echo \$x \${x:-1}'"
}
`,
			wantW310: 1,
			wantW311: 0,
		},
		{
			name: "even backslashes in submit string count usage",
			src: `
param p {
  x = (1,2)
  x
}
submit run with p {
  args_exec = "-lc 'echo \\\\$x \\\\${x:-1}'"
}
`,
			wantW310: 0,
			wantW311: 0,
		},
		{
			name: "malformed braced expansions do not hard error",
			src: `
param p {
  x = (1,2)
  x
}
submit run with p {
  args_exec = "-lc 'echo ${} ${!} ${1}'"
}
`,
			wantW310: 1,
			wantW311: 0,
		},
		{
			name: "raw preprocess and postprocess count usage",
			src: `
param p {
  x = (1,2)
  x
}
submit run with p {
  preprocess = {
    echo ${x}
  }
  postprocess = {
    echo $x
  }
}
`,
			wantW310: 0,
			wantW311: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := analyzeShellSrc(t, tc.src)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if hasDiagCode(diags, "E100") {
				t.Fatalf("did not expect E100 in submit shell scanning case, got: %s", diags.String())
			}
			if got := diagCount(diags, "W310"); got != tc.wantW310 {
				t.Fatalf("unexpected W310 count: got=%d want=%d diags=%s", got, tc.wantW310, diags.String())
			}
			if got := diagCount(diags, "W311"); got != tc.wantW311 {
				t.Fatalf("unexpected W311 count: got=%d want=%d diags=%s", got, tc.wantW311, diags.String())
			}
		})
	}
}

func TestQualifiedLikeShellReferenceInStepDoesNotRaiseE100(t *testing.T) {
	diags := analyzeShellSrc(t, `
let l {
  systemname = shell("hostname")
}
do s0 {
  echo ${l.systemname}
}
`)
	if diags.HasErrors() {
		t.Fatalf("did not expect hard errors for shell-like qualified token, got: %s", diags.String())
	}
	if hasDiagCode(diags, "E100") {
		t.Fatalf("did not expect E100 from shell text scanning, got: %s", diags.String())
	}
	if !hasW310ForLet(diags, "l", "systemname") {
		t.Fatalf("expected W310 for let.l.systemname because ${l.systemname} must not count as qualified usage, got: %s", diags.String())
	}
}

func TestShellParityAffectsW310AndW311(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantW310 int
		wantW311 int
	}{
		{
			name: "odd escaped ref is ignored",
			src: `
param p {
  x = (1,2)
  x
}
do run {
  echo \$x
}
`,
			wantW310: 1,
			wantW311: 0,
		},
		{
			name: "even parity bare ref is active",
			src: `
param p {
  x = (1,2)
  x
}
do run {
  echo \\$x
}
`,
			wantW310: 0,
			wantW311: 1,
		},
		{
			name: "even parity braced ref is active",
			src: `
param p {
  x = (1,2)
  x
}
do run {
  echo \\${x:-1}
}
`,
			wantW310: 0,
			wantW311: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := analyzeShellSrc(t, tc.src)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got := diagCount(diags, "W310"); got != tc.wantW310 {
				t.Fatalf("unexpected W310 count: got=%d want=%d diags=%s", got, tc.wantW310, diags.String())
			}
			if got := diagCount(diags, "W311"); got != tc.wantW311 {
				t.Fatalf("unexpected W311 count: got=%d want=%d diags=%s", got, tc.wantW311, diags.String())
			}
		})
	}
}

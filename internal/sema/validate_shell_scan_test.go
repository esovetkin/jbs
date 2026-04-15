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

func TestStepLocalUnusedImportWarnsEvenIfUsedInOtherStep(t *testing.T) {
	diags := analyzeShellSrc(t, `
param p {
  nodes = (1)
  nodes
}
do s with p {
  echo "\$nodes"
}
submit run with p {
  account = "a"
  queue = "q"
  args_exec = "\\\\$nodes ${nodes}"
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !hasW313For(diags, "s", "nodes") {
		t.Fatalf("expected W313 for step-local unused import s.nodes, got: %s", diags.String())
	}
	if got := diagCount(diags, "W313"); got != 1 {
		t.Fatalf("expected exactly one W313, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 when nodes is used in another step, got %d: %s", got, diags.String())
	}
}

func TestStepLocalUnusedImportSkippedWhenGlobalW310Applies(t *testing.T) {
	diags := analyzeShellSrc(t, `
param p {
  nodes = (1)
  nodes
}
do s with p {
  echo "\$nodes"
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !hasW310For(diags, "p", "nodes") {
		t.Fatalf("expected W310 for globally unused p.nodes, got: %s", diags.String())
	}
	if got := diagCount(diags, "W313"); got != 0 {
		t.Fatalf("did not expect W313 when W310 already covers global unused variable, got %d: %s", got, diags.String())
	}
}

func TestStepLocalUnusedImportInSubmitStep(t *testing.T) {
	diags := analyzeShellSrc(t, `
param p {
  nodes = (1,2)
  nodes
}
do prep with nodes from p {
  echo ${nodes}
}
submit run with nodes from p {
  account = "a"
  queue = "q"
  tasks = 1
  args_exec = "echo done"
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !hasW313For(diags, "run", "nodes") {
		t.Fatalf("expected W313 for submit step run.nodes, got: %s", diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 because nodes is used in prep, got %d: %s", got, diags.String())
	}
}

func TestStepLocalUnusedImportWithAfterUsesExplicitDeltaOnly(t *testing.T) {
	diags := analyzeShellSrc(t, `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with b from p {
  echo ${a}
}
do s2 after s1 with b from p {
  echo ${b}
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !hasW313For(diags, "s1", "b") {
		t.Fatalf("expected W313 for explicit delta import b in s1, got: %s", diags.String())
	}
	if hasW313For(diags, "s1", "a") {
		t.Fatalf("did not expect W313 for inherited variable a in s1, got: %s", diags.String())
	}
}

func TestStepLocalUnusedImportWarnsForLetSource(t *testing.T) {
	diags := analyzeShellSrc(t, `
let l {
  x = "v"
}
do s0 with l {
  echo hi
}
do s1 with x from l {
  echo ${x}
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !hasW313For(diags, "s0", "x") {
		t.Fatalf("expected W313 for step-local unused let import s0.x, got: %s", diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 for l.x because it is used in s1, got %d: %s", got, diags.String())
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

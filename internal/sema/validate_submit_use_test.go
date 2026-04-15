package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestSubmitKeyHelpers(t *testing.T) {
	if !sema.IsSubmitKey("nodes") {
		t.Fatalf("expected nodes to be recognized as submit key")
	}
	if sema.IsSubmitKey("not_a_submit_key") {
		t.Fatalf("did not expect arbitrary key to be recognized")
	}
	keys := sema.SubmitKeys()
	if len(keys) == 0 {
		t.Fatalf("expected non-empty submit key list")
	}
	if keys[0] > keys[len(keys)-1] {
		t.Fatalf("expected sorted submit keys, got %#v", keys)
	}
}

func TestModeExprInsideTupleListWarns(t *testing.T) {
	src := `
param p {
  a = (python("A"), shell("B"))
  b = [python("C"), "D"]
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected warnings only, got errors: %s", diags.String())
	}
	found := false
	for _, d := range diags.Items {
		if d.Code == "W301" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected W301 for python()/shell() inside tuple/list, got: %s", diags.String())
	}
}

func TestTopLevelModeExprAssignmentNoTupleListWarning(t *testing.T) {
	src := `
param p {
  queue = python("devel")
  host = shell("localhost")
  queue + host
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	for _, d := range diags.Items {
		if d.Code == "W301" {
			t.Fatalf("did not expect W301 for standalone mode assignments, got: %s", diags.String())
		}
	}
}

func TestSubmitUnknownKeyError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  not_allowed = "x"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E072" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E072, got: %s", diags.String())
	}
}

func TestSubmitPreprocessRequiresRawBlock(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  preprocess = "echo hi"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E073" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E073, got: %s", diags.String())
	}
}

func TestSubmitNonRawKeyRejectsRawBlock(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  queue = {
    devel
  }
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E074" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E074, got: %s", diags.String())
	}
}

func TestSubmitDuplicateKeyError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  queue = "q1"
  queue = "q2"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E075" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E075, got: %s", diags.String())
	}
}

func TestSubmitHeaderUseAppliesDefaultsAndExplicitOverride(t *testing.T) {
	src := `
let defaults {
  queue = "batch"
  nodes = 2
  tasks = 2
}
submit run
  use defaults
{
  queue = "devel"
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	spec := res.SubmitByName["run"]
	if spec == nil {
		t.Fatalf("missing submit spec for run")
	}
	queue, ok := submitValueByName(spec, "queue")
	if !ok {
		t.Fatalf("missing queue submit value: %#v", spec.Values)
	}
	if queue.Value.Kind != eval.KindString || queue.Value.S != "devel" {
		t.Fatalf("expected queue override to be 'devel', got %#v", queue.Value)
	}
	if nodes, ok := submitValueByName(spec, "nodes"); !ok || nodes.Value.I != 2 {
		t.Fatalf("expected nodes default=2, got %#v", spec.Values)
	}
	if tasks, ok := submitValueByName(spec, "tasks"); !ok || tasks.Value.I != 2 {
		t.Fatalf("expected tasks default=2, got %#v", spec.Values)
	}
	if _, ok := submitValueByName(spec, "args_exec"); !ok {
		t.Fatalf("expected explicit args_exec in submit values: %#v", spec.Values)
	}
}

func TestSubmitHeaderUseIdentifierExpressions(t *testing.T) {
	cases := []struct {
		name         string
		src          string
		wantE100     bool
		wantW072     int
		wantValueKey string
		wantValue    int64
		noW310       [][2]string
	}{
		{
			name: "helper identifier resolves in submit expression",
			src: `
let defaults {
  mynodes = 4
}
submit run
  use defaults
{
  nodes = mynodes
  args_exec = "-lc hostname"
}
`,
			wantValueKey: "nodes",
			wantValue:    4,
			noW310:       [][2]string{{"defaults", "mynodes"}},
		},
		{
			name: "submit key default identifier resolves in submit expression",
			src: `
let defaults {
  nodes = 6
}
submit run
  use defaults
{
  tasks = nodes
  args_exec = "-lc hostname"
}
`,
			wantValueKey: "tasks",
			wantValue:    6,
			noW310:       [][2]string{{"defaults", "nodes"}},
		},
		{
			name: "helper last wins across use clauses",
			src: `
let d0 {
  mynodes = 1
}
let d1 {
  mynodes = 4
}
submit run
  use d0
  use d1
{
  nodes = mynodes
  args_exec = "-lc hostname"
}
`,
			wantW072:     1,
			wantValueKey: "nodes",
			wantValue:    4,
		},
		{
			name: "mixed with and use resolves with use precedence",
			src: `
let d0 {
  mynodes = 1
}
let d1 {
  mynodes = 4
}
submit run
  with d0
  use d1
{
  nodes = mynodes
  args_exec = "-lc hostname"
}
`,
			wantValueKey: "nodes",
			wantValue:    4,
			noW310:       [][2]string{{"d0", "mynodes"}, {"d1", "mynodes"}},
		},
		{
			name: "unresolved identifier still errors",
			src: `
submit run {
  nodes = missing
  args_exec = "-lc hostname"
}
`,
			wantE100: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			prog := parser.Parse("in.jbs", tc.src, diags)
			res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
			if tc.wantE100 {
				if !hasDiagCode(diags, "E100") {
					t.Fatalf("expected E100, got: %s", diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got := diagCount(diags, "W072"); got != tc.wantW072 {
				t.Fatalf("unexpected W072 count: got=%d want=%d diags=%s", got, tc.wantW072, diags.String())
			}
			spec := res.SubmitByName["run"]
			if spec == nil {
				t.Fatalf("missing submit spec for run")
			}
			value, ok := submitValueByName(spec, tc.wantValueKey)
			if !ok {
				t.Fatalf("missing submit value %q: %#v", tc.wantValueKey, spec.Values)
			}
			if value.Value.Kind != eval.KindInt || value.Value.I != tc.wantValue {
				t.Fatalf("unexpected submit value %q: got=%#v want=%d", tc.wantValueKey, value.Value, tc.wantValue)
			}
			for _, noWarn := range tc.noW310 {
				if hasW310ForLet(diags, noWarn[0], noWarn[1]) {
					t.Fatalf("did not expect W310 for %s.%s, got: %s", noWarn[0], noWarn[1], diags.String())
				}
			}
		})
	}
}

func TestSubmitSeriesIdentifierAssignmentWarning(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantW075  int
		wantList  bool
		wantListN int
		valueKey  string
	}{
		{
			name: "direct identifier with multi-row import warns",
			src: `
param p {
  nodes = (1,2)
  nodes
}
submit run with p {
  account = "a"
  queue = "q"
  nodes = nodes
  args_exec = "-lc hostname"
}
`,
			wantW075:  1,
			wantList:  true,
			wantListN: 2,
			valueKey:  "nodes",
		},
		{
			name: "interpolation string does not warn",
			src: `
param p {
  nodes = (1,2)
  nodes
}
submit run with p {
  account = "a"
  queue = "q"
  nodes = "${nodes}"
  args_exec = "-lc hostname"
}
`,
			wantW075: 0,
			valueKey: "nodes",
		},
		{
			name: "single-row direct identifier does not warn",
			src: `
param p {
  nodes = (1)
  nodes
}
submit run with p {
  account = "a"
  queue = "q"
  nodes = nodes
  args_exec = "-lc hostname"
}
`,
			wantW075: 0,
			valueKey: "nodes",
		},
		{
			name: "non-identifier expression does not warn",
			src: `
param p {
  nodes = (1,2)
  nodes
}
submit run with p {
  account = "a"
  queue = "q"
  nodes = nodes if true else nodes
  args_exec = "-lc hostname"
}
`,
			wantW075:  0,
			wantList:  true,
			wantListN: 2,
			valueKey:  "nodes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			prog := parser.Parse("in.jbs", tc.src, diags)
			res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got := diagCount(diags, "W075"); got != tc.wantW075 {
				t.Fatalf("unexpected W075 count: got=%d want=%d diags=%s", got, tc.wantW075, diags.String())
			}
			spec := res.SubmitByName["run"]
			if spec == nil {
				t.Fatalf("missing submit spec for run")
			}
			value, ok := submitValueByName(spec, tc.valueKey)
			if !ok {
				t.Fatalf("missing submit value %q: %#v", tc.valueKey, spec.Values)
			}
			if tc.wantList {
				if value.Value.Kind != eval.KindList {
					t.Fatalf("expected list value for %q, got %#v", tc.valueKey, value.Value)
				}
				if gotN := len(value.Value.L); gotN != tc.wantListN {
					t.Fatalf("unexpected list length for %q: got=%d want=%d", tc.valueKey, gotN, tc.wantListN)
				}
			}
		})
	}
}

func TestSubmitAutoAddsTasksFromNodesWhenMissing(t *testing.T) {
	src := `
submit run {
  nodes = 4
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	spec := res.SubmitByName["run"]
	if spec == nil {
		t.Fatalf("missing submit spec for run")
	}
	tasks, ok := submitValueByName(spec, "tasks")
	if !ok {
		t.Fatalf("expected auto tasks in submit values: %#v", spec.Values)
	}
	if tasks.Value.Kind != eval.KindInt || tasks.Value.I != 4 {
		t.Fatalf("expected tasks to inherit nodes value 4, got %#v", tasks.Value)
	}
}

func TestSubmitAutoAddsTasksAsNodesReferenceWhenNodesMissing(t *testing.T) {
	src := `
submit run {
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	spec := res.SubmitByName["run"]
	if spec == nil {
		t.Fatalf("missing submit spec for run")
	}
	tasks, ok := submitValueByName(spec, "tasks")
	if !ok {
		t.Fatalf("expected auto tasks in submit values: %#v", spec.Values)
	}
	if tasks.Value.Kind != eval.KindString || tasks.Value.S != "$nodes" {
		t.Fatalf("expected tasks to default to \"$nodes\", got %#v", tasks.Value)
	}
}

func TestSubmitHeaderUseMultipleClauses(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantW072  int
		wantQueue string
	}{
		{
			name: "last wins and warns on collision",
			src: `
let defaults {
  queue = "batch"
  tasks = 2
}
let gpu_defaults {
  queue = "devel"
  gres = "gpu:4"
}
submit run
  use defaults
  use gpu_defaults
{
  args_exec = "-lc hostname"
}
`,
			wantW072:  1,
			wantQueue: "devel",
		},
		{
			name: "no collision no warning",
			src: `
let defaults {
  queue = "batch"
}
let gpu_defaults {
  gres = "gpu:4"
}
submit run
  use defaults
  use gpu_defaults
{
  args_exec = "-lc hostname"
}
`,
			wantW072:  0,
			wantQueue: "batch",
		},
		{
			name: "same namespace repeated has no warning",
			src: `
let defaults {
  queue = "batch"
}
submit run
  use defaults
  use defaults
{
  args_exec = "-lc hostname"
}
`,
			wantW072:  0,
			wantQueue: "batch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			prog := parser.Parse("in.jbs", tc.src, diags)
			res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			spec := res.SubmitByName["run"]
			if spec == nil {
				t.Fatalf("missing submit spec for run")
			}
			if got := diagCount(diags, "W072"); got != tc.wantW072 {
				t.Fatalf("unexpected W072 count: got=%d want=%d diags=%s", got, tc.wantW072, diags.String())
			}
			queue, ok := submitValueByName(spec, "queue")
			if !ok {
				t.Fatalf("missing queue submit value: %#v", spec.Values)
			}
			if queue.Value.Kind != eval.KindString || queue.Value.S != tc.wantQueue {
				t.Fatalf("unexpected queue value: got=%#v want=%q", queue.Value, tc.wantQueue)
			}
		})
	}
}

func TestSubmitHeaderUseKeepsNonSubmitHelperKeys(t *testing.T) {
	src := `
let defaults {
  queue = "batch"
  ignored_key = 1
  queue_expr = python("'${ignored_key:-x}'")
  preprocess = "module load CUDA"
}
submit run
  use defaults
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	spec := res.SubmitByName["run"]
	if spec == nil {
		t.Fatalf("missing submit spec for run")
	}
	if got := diagCount(diags, "W070"); got != 0 {
		t.Fatalf("did not expect W070 for non-submit helper from submit use, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W071"); got != 1 {
		t.Fatalf("expected one W071 for ignored raw submit key, got %d: %s", got, diags.String())
	}
	if helper, ok := submitHelperByOriginal(spec, "ignored_key"); !ok {
		t.Fatalf("expected ignored_key helper in submit spec: %#v", spec.Helpers)
	} else if helper.Aliased == "" {
		t.Fatalf("expected non-empty alias for ignored_key helper: %#v", helper)
	}
	if helper, ok := submitHelperByOriginal(spec, "queue_expr"); !ok {
		t.Fatalf("expected queue_expr helper in submit spec: %#v", spec.Helpers)
	} else if helper.Mode != "python" {
		t.Fatalf("expected python mode for queue_expr helper, got %#v", helper)
	}
	if _, ok := submitHelperByOriginal(spec, "preprocess"); ok {
		t.Fatalf("did not expect preprocess helper imported from let: %#v", spec.Helpers)
	}
	if _, ok := submitValueByName(spec, "preprocess"); ok {
		t.Fatalf("did not expect preprocess default imported from let: %#v", spec.Values)
	}
}

func TestSubmitHeaderUseRejectsParamSource(t *testing.T) {
	src := `
param defaults {
  a = 1
  a
}
submit run
  use defaults
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E071") {
		t.Fatalf("expected E071 for submit use param source, got: %s", diags.String())
	}
}

func TestSubmitHeaderUseUnknownNamespace(t *testing.T) {
	src := `
submit run
  use missing
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E078") {
		t.Fatalf("expected E078 for unknown submit use namespace, got: %s", diags.String())
	}
}

func TestSubmitHeaderUseCountsAsUsageForW310(t *testing.T) {
	src := `
let defaults {
  queue = "batch"
  gres = "gpu:4"
}
submit run
  use defaults
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if hasW310ForLet(diags, "defaults", "queue") {
		t.Fatalf("did not expect W310 for defaults.queue when used by submit header use, got: %s", diags.String())
	}
	if hasW310ForLet(diags, "defaults", "gres") {
		t.Fatalf("did not expect W310 for defaults.gres when used by submit header use, got: %s", diags.String())
	}
}

func TestSubmitHeaderUseHelperReferenceCountsAsUsageForW310(t *testing.T) {
	src := `
let defaults {
  systemname = shell("hostname | tr -d '\n'")
  queue = python("'${systemname:-batch}'")
}
submit run
  use defaults
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if hasW310ForLet(diags, "defaults", "systemname") {
		t.Fatalf("did not expect W310 for defaults.systemname when used by submit default expression, got: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311 for helper reference in submit defaults, got %d: %s", got, diags.String())
	}
}

func TestSubmitHeaderUseUnusedHelperStillWarnsW310(t *testing.T) {
	src := `
let defaults {
  systemname = shell("hostname | tr -d '\n'")
  queue = "batch"
}
submit run
  use defaults
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !hasW310ForLet(diags, "defaults", "systemname") {
		t.Fatalf("expected W310 for truly unused helper defaults.systemname, got: %s", diags.String())
	}
}

func TestSubmitHeaderUseHelperCollisionWarnsAndLastWins(t *testing.T) {
	src := `
let d0 {
  systemname = "first"
}
let d1 {
  systemname = "second"
}
submit run
  use d0
  use d1
{
  queue = "${systemname}"
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W072"); got != 1 {
		t.Fatalf("expected one W072 for helper collision, got %d: %s", got, diags.String())
	}
	spec := res.SubmitByName["run"]
	if spec == nil {
		t.Fatalf("missing submit spec for run")
	}
	helper, ok := submitHelperByOriginal(spec, "systemname")
	if !ok {
		t.Fatalf("expected helper systemname in submit spec: %#v", spec.Helpers)
	}
	if helper.Value.Kind != eval.KindString || helper.Value.S != "second" {
		t.Fatalf("expected helper last-win value from d1, got %#v", helper.Value)
	}
	if helper.UseName != "d1" {
		t.Fatalf("expected helper source namespace d1, got %#v", helper)
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311 when helper is available in submit scope, got %d: %s", got, diags.String())
	}
}

func TestSubmitLaunchWarnings(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantW073 int
		wantW074 int
	}{
		{
			name: "warns for empty account and queue",
			src: `
submit run {
  account = ""
  queue = ""
  args_exec = "-lc hostname"
}
`,
			wantW073: 2,
			wantW074: 0,
		},
		{
			name: "warns for missing account and queue",
			src: `
submit run {
  args_exec = "-lc hostname"
}
`,
			wantW073: 2,
			wantW074: 0,
		},
		{
			name: "warns when executable and args_exec are both empty",
			src: `
submit run {
  starter = "srun"
  executable = ""
  args_exec = ""
}
`,
			wantW073: 2,
			wantW074: 1,
		},
		{
			name: "warns when executable and args_exec are both missing with starter set",
			src: `
submit run {
  account = "acc"
  queue = "batch"
  starter = "srun"
}
`,
			wantW073: 0,
			wantW074: 1,
		},
		{
			name: "does not warn when executable and args_exec are both missing and starter missing",
			src: `
submit run {
  account = "acc"
  queue = "batch"
}
`,
			wantW073: 0,
			wantW074: 0,
		},
		{
			name: "does not warn when executable and args_exec are both empty and starter empty",
			src: `
submit run {
  starter = ""
  executable = ""
  args_exec = ""
}
`,
			wantW073: 2,
			wantW074: 0,
		},
		{
			name: "does not warn when only one launch field is empty",
			src: `
submit run {
  executable = ""
  args_exec = "-lc hostname"
}
`,
			wantW073: 2,
			wantW074: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			prog := parser.Parse("in.jbs", tc.src, diags)
			_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got := diagCount(diags, "W073"); got != tc.wantW073 {
				t.Fatalf("unexpected W073 count: got=%d want=%d diags=%s", got, tc.wantW073, diags.String())
			}
			if got := diagCount(diags, "W074"); got != tc.wantW074 {
				t.Fatalf("unexpected W074 count: got=%d want=%d diags=%s", got, tc.wantW074, diags.String())
			}
		})
	}
}

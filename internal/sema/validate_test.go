package sema_test

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func diagCount(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, d := range diags.Items {
		if d.Code == code {
			count++
		}
	}
	return count
}

func hasW310For(diags *diag.Diagnostics, param, variable string) bool {
	target := "exposed variable '" + variable + "' from param '" + param + "'"
	for _, d := range diags.Items {
		if d.Code != "W310" {
			continue
		}
		if strings.Contains(d.Message, target) {
			return true
		}
	}
	return false
}

func w310HintFor(diags *diag.Diagnostics, param, variable string) string {
	target := "exposed variable '" + variable + "' from param '" + param + "'"
	for _, d := range diags.Items {
		if d.Code != "W310" {
			continue
		}
		if strings.Contains(d.Message, target) {
			return d.Hint
		}
	}
	return ""
}

func TestImportRebindingIsLocal(t *testing.T) {
	src := `
param base {
  x = (1,2)
  x
}

param derived with x from base {
  x = x + 10
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	base := res.ParamByName["base"]
	derived := res.ParamByName["derived"]
	if got := base.Vars["x"][0].I; got != 1 {
		t.Fatalf("base.x mutated, first value=%d", got)
	}
	if got := derived.Vars["x"][0].I; got != 11 {
		t.Fatalf("unexpected derived.x first value=%d", got)
	}
}

func TestUnknownImportVariableError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do work with missing from p {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)

	found := false
	for _, d := range diags.Items {
		if d.Code == "E021" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E021 for missing imported variable, got: %s", diags.String())
	}
}

func TestDependencyCycleError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do a after b {
  echo a
}
do b after a {
  echo b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)

	found := false
	for _, d := range diags.Items {
		if d.Code == "E213" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E213 cycle error, got: %s", diags.String())
	}
}

func TestMixedWithVariableAndParamsetImport(t *testing.T) {
	src := `
param p1 {
  a = (1,2)
  a
}
param p2 {
  b = ("x","y")
  b
}
do work with a from p1, p2 {
  echo ${a} ${b}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected mixed with import to be valid, got: %s", diags.String())
	}
}

func TestMixedWithTupleVariableAndParamsetImport(t *testing.T) {
	src := `
param p1 {
  a = (1,2)
  b = ("m","n")
  a * b
}
param p2 {
  c = ("x","y")
  c
}
do work with (a,b) from p1, p2 {
  echo ${a} ${b} ${c}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected mixed tuple with import to be valid, got: %s", diags.String())
	}
}

func TestRepeatedWithClausesAreConcatenated(t *testing.T) {
	src := `
param params {
  a = ("x","y")
  a
}
param params2 {
  x = (1,2)
  x
}
do work
  with params
  with x from params2
{
  echo ${a} ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected concatenated with clauses to be valid, got: %s", diags.String())
	}
}

func TestWithVariableConflictAcrossCombinedClauses(t *testing.T) {
	src := `
param params {
  x = ("ddp","fsdp")
  x
}
param params2 {
  x = (1,2)
  x
}
do work
  with params
  with x from params2
{
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E214" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E214 for conflicting combined with imports, got: %s", diags.String())
	}
}

func TestAfterInheritancePrunesExplicitDelta(t *testing.T) {
	src := `
param pm0 {
  a = (1,2)
  b = ("x","y")
  c = (true,false)
  a * b * c
}
do step0 with (a,b) from pm0 {
  echo ${a} ${b}
}
do step1 after step0 with (b,c) from pm0 {
  echo ${a} ${b} ${c}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected valid inheritance pruning, got: %s", diags.String())
	}
	plan := res.StepImportByName["step1"]
	if plan == nil {
		t.Fatalf("missing step import plan for step1")
	}
	if len(plan.ExplicitDelta) != 1 {
		t.Fatalf("expected one explicit delta item (c), got %#v", plan.ExplicitDelta)
	}
	if plan.ExplicitDelta[0].Visible != "c" || plan.ExplicitDelta[0].Source != "pm0" || plan.ExplicitDelta[0].SourceVar != "c" {
		t.Fatalf("unexpected explicit delta for step1: %#v", plan.ExplicitDelta[0])
	}
	for _, name := range []string{"a", "b", "c"} {
		origin, ok := plan.Effective[name]
		if !ok {
			t.Fatalf("missing effective inherited/imported variable %q in step1 plan", name)
		}
		if origin.Paramset != "pm0" {
			t.Fatalf("expected %q from pm0, got %#v", name, origin)
		}
	}
}

func TestAfterInheritanceConflictAcrossDependencies(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p2 {
  x = ("a","b")
  x
}
do a with x from p1 {
  echo ${x}
}
do b with x from p2 {
  echo ${x}
}
do c after a,b {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E214" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E214 for conflicting inherited dependencies, got: %s", diags.String())
	}
}

func TestAfterInheritanceConflictWithExplicitImport(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p2 {
  x = ("a","b")
  x
}
do a with x from p1 {
  echo ${x}
}
do c after a with x from p2 {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E214" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E214 for explicit import conflicting with inherited variable source, got: %s", diags.String())
	}
}

func TestSubmitAfterInheritsVarsForExpressions(t *testing.T) {
	src := `
param p {
  n = (1,2)
  n
}
do prep with n from p {
  echo ${n}
}
submit run after prep {
  nodes = n
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected submit expression to resolve inherited variable n, got: %s", diags.String())
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

func TestWarnUnusedExposedVariableW310(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = (3,4)
  a + b
}
do run with p {
  echo ${a}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W310"); got != 1 {
		t.Fatalf("expected exactly one W310, got %d: %s", got, diags.String())
	}
	if hint := w310HintFor(diags, "p", "b"); hint != "remove it from the final expression or reference it with $b/${b} in a step" {
		t.Fatalf("unexpected W310 hint: %q", hint)
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311, got %d: %s", got, diags.String())
	}
}

func TestWarnMissingImportW311WithoutW310(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
do run {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 1 {
		t.Fatalf("expected one W311, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 because variable is referenced, got %d: %s", got, diags.String())
	}
}

func TestUnknownEnvVarDoesNotTriggerW311(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
do run {
  echo ${SLURM_JOB_ID}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311 for external env variable, got %d: %s", got, diags.String())
	}
}

func TestSubmitAnyExpressionFieldCountsAsUsage(t *testing.T) {
	src := `
param p {
  queue_name = ("devel")
  queue_name
}
submit run with p {
  queue = "${queue_name}"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 for queue_name used in submit key, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311 for imported queue_name, got %d: %s", got, diags.String())
	}
}

func TestSubmitRawBlocksCountAsUsage(t *testing.T) {
	src := `
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
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 for x used in raw submit blocks, got %d: %s", got, diags.String())
	}
}

func TestW311IsDedupedPerStepAndVariable(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
do run {
  echo ${x}
  echo $x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 1 {
		t.Fatalf("expected one deduped W311, got %d: %s", got, diags.String())
	}
}

func TestWithVariableOnlyLeavesOtherExposedVarUnused(t *testing.T) {
	src := `
param p {
  x = (1,2)
  y = (3,4)
  x + y
}
do run with x from p {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 1 {
		t.Fatalf("expected one W310 for y, got %d: %s", got, diags.String())
	}
}

func TestW310OverlapUsesOnlyImportedOrigin(t *testing.T) {
	src := `
param p0 {
  a = (1,2)
  b = ("x","y")
  a + b
}
param p1 {
  b = (3,4)
  b
}
do s with b from p1 {
  echo ${b}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 2 {
		t.Fatalf("expected two W310 warnings, got %d: %s", got, diags.String())
	}
	if !hasW310For(diags, "p0", "a") {
		t.Fatalf("expected W310 for p0.a, got: %s", diags.String())
	}
	if !hasW310For(diags, "p0", "b") {
		t.Fatalf("expected W310 for p0.b, got: %s", diags.String())
	}
	if hasW310For(diags, "p1", "b") {
		t.Fatalf("did not expect W310 for p1.b, got: %s", diags.String())
	}
}

func TestW310ImportedButUnusedStillWarns(t *testing.T) {
	src := `
param p0 {
  a = (1,2)
  b = ("x","y")
  a * b
}
do s0 with p0 {
  echo ${a}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 1 {
		t.Fatalf("expected one W310 warning, got %d: %s", got, diags.String())
	}
	if !hasW310For(diags, "p0", "b") {
		t.Fatalf("expected W310 for p0.b, got: %s", diags.String())
	}
}

func TestW310ComplexOverlapGraph(t *testing.T) {
	src := `
param p0
{
        a = (0, 1, 2, 3, 4, 5)
        b = ("a", "b", "c")
        c = ("x", "z")
        d = (true, false)

        d * (a + b) + c
}

param p1
{
        a = ("a", "b")
        b = (0, 1, 2, 3)
        a + b
}

do step0
        with a from p0
{
        echo "a=${a}" > step0.out
}

do step1
        after step0
        with c from p0
{
        echo "a=${a}" > step1.out
        echo "c=${c}" >> step1.out
}

do step2
        after step0
        with b from p1, d from p0
{
        echo "a=${a}" > step2.out
        echo "b=${b}" >> step2.out
        echo "d=${d}" >> step2.out
}

do step3
        after step2
        with c from p0
{
        echo "a=${a}" > step3.out
        echo "b=${b}" >> step3.out
        echo "c=${c}" >> step3.out
        echo "d=${d}" >> step3.out
}

do step4
        after step1
        with d from p0
{
        echo "a=${a}" > step4.out
        echo "c=${c}" >> step4.out
        echo "d=${d}" >> step4.out
}

do step5
        after step3
{
        echo "a=${a}" > step5.out
        echo "b=${b}" >> step5.out
        echo "c=${c}" >> step5.out
        echo "d=${d}" >> step5.out
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 2 {
		t.Fatalf("expected two W310 warnings, got %d: %s", got, diags.String())
	}
	if !hasW310For(diags, "p0", "b") {
		t.Fatalf("expected W310 for p0.b, got: %s", diags.String())
	}
	if !hasW310For(diags, "p1", "a") {
		t.Fatalf("expected W310 for p1.a, got: %s", diags.String())
	}
	if hasW310For(diags, "p0", "a") {
		t.Fatalf("did not expect W310 for p0.a, got: %s", diags.String())
	}
	if hasW310For(diags, "p1", "b") {
		t.Fatalf("did not expect W310 for p1.b, got: %s", diags.String())
	}
}

func TestW311OverlapRemainsCompatible(t *testing.T) {
	src := `
param p0 {
  b = ("x","y")
  b
}
param p1 {
  b = (1,2)
  b
}
do s {
  echo ${b}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 1 {
		t.Fatalf("expected one W311 warning, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 for unresolved overlap reference, got %d: %s", got, diags.String())
	}
}

func TestMultipleStepsUsingSameVarNoW310(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
do a with p {
  echo ${x}
}
do b with p {
  echo $x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W311"); got != 0 {
		t.Fatalf("did not expect W311, got %d: %s", got, diags.String())
	}
}

func TestW311IncludesRelatedParamSourceSpan(t *testing.T) {
	src := `
param p1 {
  x = ("a","b")
  x
}
param p2 {
  x = ("c","d")
  x
}
do run {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	for _, d := range diags.Items {
		if d.Code != "W311" {
			continue
		}
		if len(d.Related) == 0 {
			t.Fatalf("expected W311 to include related parameter source span, got: %s", diags.String())
		}
		return
	}
	t.Fatalf("expected W311, got: %s", diags.String())
}

func TestUnknownTopLevelGlobalRejected(t *testing.T) {
	src := `
not_a_global = "x"
param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E300" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E300, got: %s", diags.String())
	}
}

func TestSpecialRootGlobalsValidation(t *testing.T) {
	src := `
jbs_name = python("abc")
jbs_outpath = 12
param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	has301 := false
	has302 := false
	has303 := false
	for _, d := range diags.Items {
		if d.Code == "E301" {
			has301 = true
		}
		if d.Code == "E302" {
			has302 = true
		}
		if d.Code == "E303" {
			has303 = true
		}
	}
	if !has303 || !has302 {
		t.Fatalf("expected E303 and E302, got: %s", diags.String())
	}
	if has301 {
		t.Fatalf("unexpected E301; jbs_name mode error should be E303")
	}
}

func TestGlobalScalarOnlyRule(t *testing.T) {
	src := `
jbs_outpath = (1,2)
param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E302" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E302, got: %s", diags.String())
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

func TestAnalyseUnknownStepError(t *testing.T) {
	src := `
let p {
  a = "A: %d"
}
analyse missing_step {
  p0 = p.a in "stdout"
  (p0)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E410" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E410, got: %s", diags.String())
	}
}

func TestAnalyseUnknownNamespaceAndMember(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
let g {
  one = "A: %d"
}
analyse run {
  p0 = missing.one in "stdout"
  p1 = g.missing in "stdout"
  (a, p0, p1)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	has100 := false
	for _, d := range diags.Items {
		if d.Code == "E100" {
			has100 = true
		}
	}
	if !has100 {
		t.Fatalf("expected E100, got: %s", diags.String())
	}
}

func TestAnalyseAssignmentCollisionAndDuplicate(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
let g {
  one = "A: %d"
}
analyse run with g {
  a = one in "stdout"
  p0 = one in "stdout"
  p0 = one in "stdout"
  (a, p0)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	has413 := false
	has414 := false
	for _, d := range diags.Items {
		if d.Code == "E413" {
			has413 = true
		}
		if d.Code == "E414" {
			has414 = true
		}
	}
	if !has413 || !has414 {
		t.Fatalf("expected E413 and E414, got: %s", diags.String())
	}
}

func TestAnalyseUnknownTupleSymbol(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
let g {
  one = "A: %d"
}
analyse run with g {
  p0 = one in "stdout"
  (a, p0, unknown)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E415" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E415, got: %s", diags.String())
	}
}

func TestPatternPlaceholderNormalizationAndTypeInference(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run {
  i = "Count: %d" in "stdout"
  f = "Time: %f" in "stdout"
  w = "Word: %w" in "stdout"
  (a, i, f, w)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(res.Analyse) != 1 {
		t.Fatalf("expected one analyse spec, got %d", len(res.Analyse))
	}
	var i, f, w *sema.AnalyseAssignmentSpec
	for idx := range res.Analyse[0].Assignments {
		asn := &res.Analyse[0].Assignments[idx]
		switch asn.Name {
		case "i":
			i = asn
		case "f":
			f = asn
		case "w":
			w = asn
		}
	}
	if i == nil || f == nil || w == nil {
		t.Fatalf("expected analyse extraction assignments i/f/w, got %#v", res.Analyse[0].Assignments)
	}
	if i.Template.Regex != "Count: $jube_pat_int" || i.Template.Type != "int" {
		t.Fatalf("unexpected int pattern normalization: %#v", i.Template)
	}
	if f.Template.Regex != "Time: $jube_pat_fp" || f.Template.Type != "float" {
		t.Fatalf("unexpected float pattern normalization: %#v", f.Template)
	}
	if w.Template.Regex != "Word: $jube_pat_wrd" || w.Template.Type != "string" {
		t.Fatalf("unexpected word pattern normalization: %#v", w.Template)
	}
}

func TestPatternRejectsPercentS(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run {
  bad = "Letter: %s" in "stdout"
  (a, bad)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E402" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E402 for %%s placeholder, got: %s", diags.String())
	}
}

func TestAnalyseWithParamRejected(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run with p {
  x = "A: %d" in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E420" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E420 for analyse with param import, got: %s", diags.String())
	}
}

func TestAnalyseWithLetImportRequiresString(t *testing.T) {
	src := `
let l {
  p = 3
  s = "A: %d"
}
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run with l {
  x = s in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E422" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E422 for non-string let import in analyse, got: %s", diags.String())
	}
}

func TestAnalyseWithSelectedNonStringLetImportRejected(t *testing.T) {
	src := `
let l {
  p = 3
  s = "A: %d"
}
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run with p from l {
  x = "A: %d" in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E422" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E422 for selected non-string let import in analyse, got: %s", diags.String())
	}
}

func TestParamWithLetImportQualifiedCombinationRejected(t *testing.T) {
	src := `
let l {
  a = 1
  b = "tag"
}
param p with l {
  x = (a, b)
  x + l.a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E111" || d.Code == "E100" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected qualified let usage rejection, got: %s", diags.String())
	}
}

func TestNestedTupleTransitiveAcrossBlocksRejected(t *testing.T) {
	src := `
let l {
  t = (1,2)
}
param p {
  x = (l.t, 3)
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E403" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E403 for let tuple rejection, got: %s", diags.String())
	}
}

func TestAnalyseLocalHelperCollisionWarning(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run {
  a = "Number: %d"
  x = a in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	hasW320 := false
	for _, d := range diags.Items {
		if d.Code == "W320" {
			hasW320 = true
		}
	}
	if !hasW320 {
		t.Fatalf("expected W320 for analyse helper collision, got: %s", diags.String())
	}
}

func TestParamWithAmbiguousSourceNameBetweenParamAndLet(t *testing.T) {
	src := `
let same {
  x = 1
}
param same {
  a = 1
  a
}
param q with same {
  b = 1
  b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E022" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E022 for ambiguous with source, got: %s", diags.String())
	}
}

func TestStepWithLetNamespaceImport(t *testing.T) {
	src := `
let l {
  systemname = shell("hostname")
  queue = "batch"
}
do s0 with l {
  echo ${systemname} ${queue}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected valid step let import, got: %s", diags.String())
	}
	plan := res.StepImportByName["s0"]
	if plan == nil {
		t.Fatalf("missing step import plan for s0")
	}
	if _, ok := plan.Effective["systemname"]; !ok {
		t.Fatalf("expected systemname in effective imports: %#v", plan.Effective)
	}
	if _, ok := plan.Effective["queue"]; !ok {
		t.Fatalf("expected queue in effective imports: %#v", plan.Effective)
	}
}

func TestStepWithTupleImportFromLet(t *testing.T) {
	src := `
let l {
  x = 1
  y = 2
  z = 3
}
do s0 with (x,y) from l {
  echo ${x} ${y}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected valid tuple import from let, got: %s", diags.String())
	}
	plan := res.StepImportByName["s0"]
	if plan == nil {
		t.Fatalf("missing step import plan for s0")
	}
	if _, ok := plan.Effective["x"]; !ok {
		t.Fatalf("expected x in effective imports: %#v", plan.Effective)
	}
	if _, ok := plan.Effective["y"]; !ok {
		t.Fatalf("expected y in effective imports: %#v", plan.Effective)
	}
	if _, ok := plan.Effective["z"]; ok {
		t.Fatalf("did not expect z in effective imports: %#v", plan.Effective)
	}
}

func TestQualifiedLetReferenceInStepErrors(t *testing.T) {
	src := `
let l {
  systemname = shell("hostname")
}
do s0 {
  echo ${l.systemname}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	has100 := false
	for _, d := range diags.Items {
		if d.Code == "E100" {
			has100 = true
			break
		}
	}
	if !has100 {
		t.Fatalf("expected E100 for qualified let reference, got: %s", diags.String())
	}
}

func TestLetWarningsW310AndW311(t *testing.T) {
	src := `
let l {
  x = 1
  y = 2
}
do s0 {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W311"); got != 1 {
		t.Fatalf("expected one W311 for x missing import, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 1 {
		t.Fatalf("expected one W310 for let.y unused, got %d: %s", got, diags.String())
	}
}

func TestLetVariablesUsedInAnalyseDoNotTriggerW310(t *testing.T) {
	src := `
param p0 {
  a = ("a")
  x = (1)
  a + x
}
do write with p0 {
  echo "Number: ${x}" > en
  echo "Zahl: ${x}" > de
  echo "Letter: ${a}" >> en
}
let p {
  number = "Number: %d"
  zahl = "Zahl: %d"
  letter = "Letter: %w"
}
analyse write
  with p
{
  p0 = number in "en"
  p1 = zahl in "de"
  p2 = letter in "en"
  (a, x, p0, p1, p2)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 when let vars are used in analyse with-clause, got %d: %s", got, diags.String())
	}
}

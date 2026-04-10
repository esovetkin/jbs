package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

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

func TestStepHeaderOptionRangeValidation(t *testing.T) {
	src := `
do prep max_async=-1 iterations=0 {
  echo prep
}

submit run max_async=-2 iterations=0 {
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E216") {
		t.Fatalf("expected E216 for invalid max_async, got: %s", diags.String())
	}
	if !hasDiagCode(diags, "E217") {
		t.Fatalf("expected E217 for invalid iterations, got: %s", diags.String())
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

func TestParamWithImportConflictAcrossSources(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p2 {
  x = ("a","b")
  x
}
param p3 with p1, p2 {
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E214") {
		t.Fatalf("expected E214 for conflicting param imports, got: %s", diags.String())
	}
}

func TestParamWithImportConflictMixedClauses(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p2 {
  x = ("a","b")
  x
}
param p3 with p1, x from p2 {
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E214") {
		t.Fatalf("expected E214 for mixed conflicting param imports, got: %s", diags.String())
	}
}

func TestParamWithDuplicateSameSourceNoConflict(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p3 with p1, x from p1 {
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if hasDiagCode(diags, "E214") {
		t.Fatalf("did not expect E214 for same-source duplicate import, got: %s", diags.String())
	}
}

func TestParamImportConflictKeepsFirstSourceForEvaluation(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p2 {
  x = ("a","b")
  x
}
param p3 with p1, p2 {
  y = x * 2
  y
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E214") {
		t.Fatalf("expected E214 for conflicting param imports, got: %s", diags.String())
	}
	if hasDiagCode(diags, "E105") {
		t.Fatalf("unexpected E105 indicates order-dependent overwrite in param imports: %s", diags.String())
	}
}

func TestParamTransitiveContributionSuppressesW312(t *testing.T) {
	src := `
param p {
  a = (1,2,3)
  x = "hello "
  y = x + "world"
  b = ("a","b","c",y)
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 0 {
		t.Fatalf("did not expect W312, got %d: %s", got, diags.String())
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
		if d.Code == "E218" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E218 for ambiguous with source, got: %s", diags.String())
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

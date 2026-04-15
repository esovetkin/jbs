package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestWarnParamVariableNotContributingW312(t *testing.T) {
	src := `
param p {
  a = (1,2,3)
  x = "hello "
  b = ("a","b","c")
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 1 {
		t.Fatalf("expected one W312, got %d: %s", got, diags.String())
	}
	if !hasW312For(diags, "x") {
		t.Fatalf("expected W312 for x, got: %s", diags.String())
	}
}

func TestWarnMultipleParamVariablesNotContributingW312(t *testing.T) {
	src := `
param p {
  a = (1,2)
  x = "x"
  y = "y"
  b = ("a","b")
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 2 {
		t.Fatalf("expected two W312 warnings, got %d: %s", got, diags.String())
	}
	if !hasW312For(diags, "x") || !hasW312For(diags, "y") {
		t.Fatalf("expected W312 for both x and y, got: %s", diags.String())
	}
}

func TestImportedVariablesAreNotW312Targets(t *testing.T) {
	src := `
param base {
  x = (1,2)
  x
}
param derived with base {
  y = x + 1
  y
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 0 {
		t.Fatalf("did not expect W312 for imported variables, got %d: %s", got, diags.String())
	}
}

func TestW312ImportedShadowedByLocalStillWarns(t *testing.T) {
	src := `
param base {
  x = (1,2)
  x
}
param derived with base {
  x = 1
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 1 {
		t.Fatalf("expected one W312 warning, got %d: %s", got, diags.String())
	}
	if !hasW312ImportedFor(diags, "base", "x") {
		t.Fatalf("expected W312 for imported base.x, got: %s", diags.String())
	}
	if hasW312For(diags, "x") {
		t.Fatalf("did not expect local W312 for x, got: %s", diags.String())
	}
}

func TestW312ImportedSelfRebindCountsAsUsed(t *testing.T) {
	src := `
param base {
  x = (1,2)
  x
}
param derived with base {
  x = x + 1
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 0 {
		t.Fatalf("did not expect W312 for self-rebind from imported x, got %d: %s", got, diags.String())
	}
}

func TestW312ImportedShadowedTransitiveLocalStillUnused(t *testing.T) {
	src := `
param base {
  x = (1,2)
  x
}
param derived with base {
  y = x
  x = 1
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !hasW312ImportedFor(diags, "base", "x") {
		t.Fatalf("expected W312 for imported base.x, got: %s", diags.String())
	}
	if !hasW312For(diags, "y") {
		t.Fatalf("expected local W312 for y, got: %s", diags.String())
	}
}

func TestW312ImportedUsedViaIntermediateLocalNoWarn(t *testing.T) {
	src := `
param base {
  x = (1,2)
  x
}
param derived with base {
  y = x
  y
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 0 {
		t.Fatalf("did not expect W312 when imported x contributes via y, got %d: %s", got, diags.String())
	}
}

func TestDuplicateParamAssignmentEmitsSingleW312ForName(t *testing.T) {
	src := `
param p {
  a = (1,2)
  x = "old"
  x = "new"
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := diagCount(diags, "W312"); got != 1 {
		t.Fatalf("expected one W312 for duplicate-assigned x, got %d: %s", got, diags.String())
	}
	if !hasW312For(diags, "x") {
		t.Fatalf("expected W312 for x, got: %s", diags.String())
	}
}

func TestNoW312WhenParamFinalExpressionMissing(t *testing.T) {
	src := `
param p {
  x = (1,2)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if got := diagCount(diags, "W312"); got != 0 {
		t.Fatalf("did not expect W312 when final expression is missing, got %d: %s", got, diags.String())
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

func TestW310ParamLetSameNameBothReported(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
let p {
  y = "z"
}
do run {
  echo hi
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
	if !hasW310For(diags, "p", "x") {
		t.Fatalf("expected W310 for param p.x, got: %s", diags.String())
	}
	if !hasW310ForLet(diags, "p", "y") {
		t.Fatalf("expected W310 for let p.y, got: %s", diags.String())
	}
}

func TestW310ParamLetSameNameOneSideUsed(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
let p {
  y = "z"
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
		t.Fatalf("expected one W311 for missing import of x, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 1 {
		t.Fatalf("expected one W310 warning, got %d: %s", got, diags.String())
	}
	if hasW310For(diags, "p", "x") {
		t.Fatalf("did not expect W310 for param p.x because it is referenced, got: %s", diags.String())
	}
	if !hasW310ForLet(diags, "p", "y") {
		t.Fatalf("expected W310 for let p.y, got: %s", diags.String())
	}
}

func TestW311WithParamLetSameNameAndSameVariable(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
let p {
  x = 7
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
		t.Fatalf("expected one W311 warning, got %d: %s", got, diags.String())
	}
	if got := diagCount(diags, "W310"); got != 0 {
		t.Fatalf("did not expect W310 because both sources for x are referenced, got %d: %s", got, diags.String())
	}
}

func TestW310ParamLetSameNameMultiplePairs(t *testing.T) {
	src := `
param p {
  x = (1,2)
  x
}
let p {
  y = "z"
}
param q {
  a = (3,4)
  a
}
let q {
  b = "k"
}
do run {
  echo hi
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
	if got := diagCount(diags, "W310"); got != 4 {
		t.Fatalf("expected four W310 warnings, got %d: %s", got, diags.String())
	}
	if !hasW310For(diags, "p", "x") || !hasW310ForLet(diags, "p", "y") {
		t.Fatalf("expected both W310 warnings for source name p, got: %s", diags.String())
	}
	if !hasW310For(diags, "q", "a") || !hasW310ForLet(diags, "q", "b") {
		t.Fatalf("expected both W310 warnings for source name q, got: %s", diags.String())
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

package parser

import (
	"math"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestParseAfterAndWith(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

do work after prep,seed with p, x from p {
  echo hi
}

submit run after work with p {
  preprocess = {
    export X=1
  }
  args_exec = "python main.py"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("test.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}

	if len(prog.Stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Stmts))
	}
	doBlock, ok := prog.Stmts[1].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block at stmt[1]")
	}
	if got := len(doBlock.After); got != 2 {
		t.Fatalf("expected 2 dependencies, got %d", got)
	}
	if doBlock.After[0] != "prep" || doBlock.After[1] != "seed" {
		t.Fatalf("unexpected dependencies: %#v", doBlock.After)
	}
	if got := len(doBlock.WithItems); got != 2 {
		t.Fatalf("expected 2 with items, got %d", got)
	}
	if doBlock.WithItems[0].Name != "p" || doBlock.WithItems[0].From != "" {
		t.Fatalf("unexpected first with item: %#v", doBlock.WithItems[0])
	}
	if doBlock.WithItems[1].Name != "x" || doBlock.WithItems[1].From != "p" {
		t.Fatalf("unexpected second with item: %#v", doBlock.WithItems[1])
	}
	if doBlock.BodyStart.Line != 7 {
		t.Fatalf("unexpected do body start line: %d", doBlock.BodyStart.Line)
	}
}

func TestParseWithTupleImports(t *testing.T) {
	src := `
do task with (a,b) from p, q {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("tuple.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if got := len(db.WithItems); got != 3 {
		t.Fatalf("expected 3 with items, got %d", got)
	}
	if db.WithItems[0].Name != "a" || db.WithItems[0].From != "p" {
		t.Fatalf("unexpected first tuple item: %#v", db.WithItems[0])
	}
	if db.WithItems[1].Name != "b" || db.WithItems[1].From != "p" {
		t.Fatalf("unexpected second tuple item: %#v", db.WithItems[1])
	}
	if db.WithItems[2].Name != "q" || db.WithItems[2].From != "p" {
		t.Fatalf("unexpected carry-forward item: %#v", db.WithItems[2])
	}
}

func TestParseRepeatedWithClausesConcatenate(t *testing.T) {
	src := `
do task
  with params
  with x from params2
{
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("concat_with.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if got := len(db.WithItems); got != 2 {
		t.Fatalf("expected concatenated with items, got %d", got)
	}
	if db.WithItems[0].Name != "params" || db.WithItems[0].From != "" {
		t.Fatalf("unexpected first with item: %#v", db.WithItems[0])
	}
	if db.WithItems[1].Name != "x" || db.WithItems[1].From != "params2" {
		t.Fatalf("unexpected second with item: %#v", db.WithItems[1])
	}
}

func TestParseWithQualifiedSourceName(t *testing.T) {
	src := `
do task with test_lib.p {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("with_qualified_name.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if got := len(db.WithItems); got != 1 {
		t.Fatalf("expected one with item, got %d", got)
	}
	if db.WithItems[0].Name != "test_lib.p" || db.WithItems[0].From != "" {
		t.Fatalf("unexpected with item: %#v", db.WithItems[0])
	}
}

func TestParseWithQualifiedFromSource(t *testing.T) {
	src := `
do task with x from test_lib.p {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("with_qualified_from.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if got := len(db.WithItems); got != 1 {
		t.Fatalf("expected one with item, got %d", got)
	}
	if db.WithItems[0].Name != "x" || db.WithItems[0].From != "test_lib.p" {
		t.Fatalf("unexpected with item: %#v", db.WithItems[0])
	}
}

func TestParseWithTupleQualifiedFromSource(t *testing.T) {
	src := `
do task with (x,y) from test_lib.p {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("with_tuple_qualified_from.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if got := len(db.WithItems); got != 2 {
		t.Fatalf("expected two with items, got %d", got)
	}
	if db.WithItems[0].Name != "x" || db.WithItems[0].From != "test_lib.p" {
		t.Fatalf("unexpected first with item: %#v", db.WithItems[0])
	}
	if db.WithItems[1].Name != "y" || db.WithItems[1].From != "test_lib.p" {
		t.Fatalf("unexpected second with item: %#v", db.WithItems[1])
	}
}

func TestParseWithQualifiedMalformed(t *testing.T) {
	src := `
do task with test_lib. {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("with_qualified_malformed.jbs", src, diags)
	found := false
	for _, item := range diags.Items {
		if item.Code == "E023" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E023 for malformed qualified with item, got: %s", diags.String())
	}
}

func TestParseWithTupleMalformed(t *testing.T) {
	src := `
do task with (a,b from p {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("tuple_bad.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error")
	}
	found := false
	for _, item := range diags.Items {
		if item.Code == "E023" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E023 for malformed tuple, got: %s", diags.String())
	}
}

func TestSubmitMalformedStatementError(t *testing.T) {
	src := `
submit run {
  export X=1
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error")
	}
	found := false
	for _, item := range diags.Items {
		if item.Code == "E077" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E077, got diagnostics: %s", diags.String())
	}
}

func TestParseSubmitRawAndExprFields(t *testing.T) {
	src := `
submit run {
  preprocess = {
    module load CUDA
  }
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if len(sb.Fields) != 2 {
		t.Fatalf("expected 2 submit fields, got %d", len(sb.Fields))
	}
	if sb.Fields[0].Name != "preprocess" || !sb.Fields[0].IsRaw {
		t.Fatalf("expected first field to be raw preprocess, got %#v", sb.Fields[0])
	}
	if sb.Fields[0].RawStart.Line != 3 {
		t.Fatalf("unexpected preprocess raw start line: %d", sb.Fields[0].RawStart.Line)
	}
	if sb.Fields[1].Name != "args_exec" || sb.Fields[1].IsRaw || sb.Fields[1].Expr == nil {
		t.Fatalf("expected second field to be expression args_exec, got %#v", sb.Fields[1])
	}
}

func TestAssignmentTrailingTokensError(t *testing.T) {
	src := `
param p {
  a = 1 trailing
  a
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad.jbs", src, diags)
	found := false
	for _, item := range diags.Items {
		if item.Code == "E061" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E061 trailing token error, got: %s", diags.String())
	}
}

func TestDictLiteralNotSupported(t *testing.T) {
	src := `
param p {
  d = {"lr": 0.001}
  d
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("dict_bad.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error for dict literal")
	}
	found := false
	for _, item := range diags.Items {
		if item.Code == "E058" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E058 for dict literal, got: %s", diags.String())
	}
}

func TestParseModeExprAssignment(t *testing.T) {
	src := `
param p {
  queue = python("__import__(\"os\").environ.get(\"JUBE_QUEUE\", \"devel\")")
  system_name = shell("cat /etc/FZJ/systemname | tr -d '\n'")
  queue * system_name
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("mode.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok || len(pb.Assignments) < 2 {
		t.Fatalf("expected param block assignments")
	}
	if _, ok := pb.Assignments[0].Expr.(ast.ModeExpr); !ok {
		t.Fatalf("expected first assignment to be ModeExpr")
	}
	if _, ok := pb.Assignments[1].Expr.(ast.ModeExpr); !ok {
		t.Fatalf("expected second assignment to be ModeExpr")
	}
}

func TestParseTopLevelGlobalAssignments(t *testing.T) {
	src := `
jbs_name = "demo"
jbs_outpath = "results"
jbs_queue = python("__import__('os').environ.get('JUBE_QUEUE', 'devel')")

param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("globals.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(prog.Stmts) != 4 {
		t.Fatalf("expected 4 statements, got %d", len(prog.Stmts))
	}
	if _, ok := prog.Stmts[0].(ast.GlobalAssign); !ok {
		t.Fatalf("expected stmt 0 to be global assignment")
	}
	if _, ok := prog.Stmts[1].(ast.GlobalAssign); !ok {
		t.Fatalf("expected stmt 1 to be global assignment")
	}
	if _, ok := prog.Stmts[2].(ast.GlobalAssign); !ok {
		t.Fatalf("expected stmt 2 to be global assignment")
	}
	if _, ok := prog.Stmts[3].(ast.ParamBlock); !ok {
		t.Fatalf("expected stmt 3 to be param block")
	}
}

func TestParseMalformedTopLevelGlobalAssignment(t *testing.T) {
	src := `
jbs_name =
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad_globals.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error for malformed global assignment")
	}
}

func TestParseParamBlockCapturesBodyRaw(t *testing.T) {
	src := `
param p {
  # comment
  a = (1,2)
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_body.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if pb.BodyRaw == "" {
		t.Fatalf("expected BodyRaw to be populated")
	}
	if pb.BodyRaw[0] == '{' || pb.BodyRaw[len(pb.BodyRaw)-1] == '}' {
		t.Fatalf("BodyRaw should contain only inner block text, got %q", pb.BodyRaw)
	}
}

func TestParseSubmitBlockCapturesBodyRaw(t *testing.T) {
	src := `
submit run {
  preprocess = {
    export X=1
  }
  args_exec = "python main.py"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_body.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if sb.BodyRaw == "" {
		t.Fatalf("expected BodyRaw to be populated")
	}
	if sb.BodyRaw[0] == '{' || sb.BodyRaw[len(sb.BodyRaw)-1] == '}' {
		t.Fatalf("BodyRaw should contain only inner block text, got %q", sb.BodyRaw)
	}
}

func TestParseLetBlock(t *testing.T) {
	src := `
let p {
  number = "Number: %d"
  letter = "Letter: %w"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("let.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if pb.Name != "p" {
		t.Fatalf("unexpected let block name: %s", pb.Name)
	}
	if len(pb.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(pb.Assignments))
	}
	if pb.Assignments[0].Name != "number" {
		t.Fatalf("unexpected first assignment: %#v", pb.Assignments[0])
	}
}

func TestParseAnalyseBlock(t *testing.T) {
	src := `
analyse write {
  helper = "Number: %d"
  p0 = helper in "en"
  p1 = "Zahl: %d" in "de"
  (
    a,
    helper,
    p0,
    p1 as "Zahl",
  )
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("analyse.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	ab, ok := prog.Stmts[0].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block")
	}
	if ab.StepName != "write" {
		t.Fatalf("unexpected analyse target: %s", ab.StepName)
	}
	if len(ab.Assignments) != 3 {
		t.Fatalf("expected 3 analyse assignments, got %d", len(ab.Assignments))
	}
	if ab.Assignments[0].File != "" {
		t.Fatalf("expected first analyse assignment to be helper assignment: %#v", ab.Assignments[0])
	}
	if ab.Assignments[1].File != "en" || ab.Assignments[2].File != "de" {
		t.Fatalf("expected extraction assignments with files, got %#v", ab.Assignments)
	}
	if len(ab.Columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(ab.Columns))
	}
	if ab.Columns[3].Name != "p1" || ab.Columns[3].Title != "Zahl" {
		t.Fatalf("unexpected aliased column: %#v", ab.Columns[3])
	}
}

func TestParseAnalyseMalformedAssignment(t *testing.T) {
	src := `
analyse write {
  p0 p.number in "en"
  (p0)
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("analyse_bad.jbs", src, diags)
	found := false
	for _, item := range diags.Items {
		if item.Code == "E416" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E416, got: %s", diags.String())
	}
}

func TestParseAnalyseMissingFinalTuple(t *testing.T) {
	src := `
analyse write {
  p0 = p.number in "en"
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("analyse_missing_tuple.jbs", src, diags)
	found := false
	for _, item := range diags.Items {
		if item.Code == "E417" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E417, got: %s", diags.String())
	}
}

func TestParseAnalyseWithClause(t *testing.T) {
	src := `
analyse write with p, (x, y) from q {
  n = "N: %d" in "out"
  (n)
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("analyse_with.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	ab, ok := prog.Stmts[0].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block")
	}
	if len(ab.WithItems) != 3 {
		t.Fatalf("expected 3 with-items, got %#v", ab.WithItems)
	}
	if ab.WithItems[0].Name != "p" || ab.WithItems[0].From != "" {
		t.Fatalf("unexpected first with-item: %#v", ab.WithItems[0])
	}
	if ab.WithItems[1].Name != "x" || ab.WithItems[1].From != "q" {
		t.Fatalf("unexpected second with-item: %#v", ab.WithItems[1])
	}
	if ab.WithItems[2].Name != "y" || ab.WithItems[2].From != "q" {
		t.Fatalf("unexpected third with-item: %#v", ab.WithItems[2])
	}
}

func TestParseAnalyseRejectsAfterClause(t *testing.T) {
	src := `
analyse write after prep {
  n = "N: %d" in "out"
  (n)
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("analyse_after.jbs", src, diags)
	found := false
	for _, item := range diags.Items {
		if item.Code == "E416" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E416, got: %s", diags.String())
	}
}

func TestParseParamMultilineListTupleRegression(t *testing.T) {
	src := `
param p {
  a = (
    1,
    2,
    3,
  )
  b = [
    "x",
    "y",
  ]
  a + b
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("param_multiline.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("expected multiline tuple/list in param to remain valid, got: %s", diags.String())
	}
}

func TestParseParamFunctionCallExpressions(t *testing.T) {
	src := `
param p {
  a = tuple(1)
  b = list((1,2))
  c = tuple(list((3,4)))
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_convert.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block, got %T", prog.Stmts[0])
	}
	if len(pb.Assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(pb.Assignments))
	}
	c0, ok := pb.Assignments[0].Expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression for first assignment, got %#v", pb.Assignments[0].Expr)
	}
	c0callee, ok := c0.Callee.(ast.IdentExpr)
	if !ok || c0callee.Name != "tuple" {
		t.Fatalf("expected tuple callee, got %#v", c0.Callee)
	}
	c1, ok := pb.Assignments[1].Expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression for second assignment, got %#v", pb.Assignments[1].Expr)
	}
	c1callee, ok := c1.Callee.(ast.IdentExpr)
	if !ok || c1callee.Name != "list" {
		t.Fatalf("expected list callee, got %#v", c1.Callee)
	}
	c2, ok := pb.Assignments[2].Expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression for third assignment, got %#v", pb.Assignments[2].Expr)
	}
	c2callee, ok := c2.Callee.(ast.IdentExpr)
	if !ok || c2callee.Name != "tuple" {
		t.Fatalf("expected tuple callee for third assignment, got %#v", c2.Callee)
	}
	if len(c2.Args) != 1 {
		t.Fatalf("expected 1 arg for third assignment, got %d", len(c2.Args))
	}
	inner, ok := c2.Args[0].(ast.CallExpr)
	if !ok {
		t.Fatalf("expected nested call expression, got %#v", c2.Args[0])
	}
	innerCallee, ok := inner.Callee.(ast.IdentExpr)
	if !ok || innerCallee.Name != "list" {
		t.Fatalf("expected nested list call, got %#v", inner.Callee)
	}
}

func TestParseTupleListAsPlainIdentifiersWhenNotCallSyntax(t *testing.T) {
	src := `
param p {
  tuple = 1
  list = 2
  tuple + list
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_tuple_ident.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block, got %T", prog.Stmts[0])
	}
	if pb.Assignments[0].Name != "tuple" || pb.Assignments[1].Name != "list" {
		t.Fatalf("unexpected assignment names: %#v", pb.Assignments)
	}
}

func TestParseConversionMalformedExpressionReportsError(t *testing.T) {
	src := `
param p {
  a = tuple(
  a
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("param_convert_bad.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse errors for malformed conversion expression")
	}
	found := false
	for _, item := range diags.Items {
		if item.Code == "E063" || item.Code == "E053" || item.Code == "E054" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected conversion-closing parse error, got: %s", diags.String())
	}
}

func TestParseAnalyseTupleOneLineEqualsMultiline(t *testing.T) {
	oneLine := `
analyse write {
  p0 = p.number in "en"
  (a, x, p0, p0 as "X")
}
`
	multiLine := `
analyse write {
  p0 = p.number in "en"
  (
    a,
    x,
    p0,
    p0 as "X",
  )
}
`
	parseCols := func(src string) []ast.AnalyseColumn {
		diags := &diag.Diagnostics{}
		prog := Parse("tuple_eq.jbs", src, diags)
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		if len(prog.Stmts) != 1 {
			t.Fatalf("expected one statement")
		}
		ab, ok := prog.Stmts[0].(ast.AnalyseBlock)
		if !ok {
			t.Fatalf("expected analyse block")
		}
		return ab.Columns
	}
	left := parseCols(oneLine)
	right := parseCols(multiLine)
	if len(left) != len(right) {
		t.Fatalf("tuple column count mismatch: %d vs %d", len(left), len(right))
	}
	for i := range left {
		if left[i].Name != right[i].Name || left[i].Title != right[i].Title {
			t.Fatalf("tuple column mismatch at %d: %#v vs %#v", i, left[i], right[i])
		}
	}
}

func TestParseParamCommentApostropheDoesNotBreakBlock(t *testing.T) {
	src := `
param p {
  a = (1, 2)
  # ` + "`a + b` is like python's zip" + `
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("comment_apostrophe.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if !strings.Contains(pb.BodyRaw, "`a + b` is like python's zip") {
		t.Fatalf("expected apostrophe/backtick comment in BodyRaw, got %q", pb.BodyRaw)
	}
}

func TestParseUseBareModule(t *testing.T) {
	src := "use jsc\n"
	diags := &diag.Diagnostics{}
	prog := Parse("use_bare.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	useStmt, ok := prog.Stmts[0].(ast.UseStmt)
	if !ok {
		t.Fatalf("expected use statement, got %T", prog.Stmts[0])
	}
	if useStmt.Source.Kind != ast.UseSourceBare || useStmt.Source.Value != "jsc" {
		t.Fatalf("unexpected use source: %#v", useStmt.Source)
	}
	if useStmt.Alias != "jsc" {
		t.Fatalf("expected alias 'jsc', got %q", useStmt.Alias)
	}
	if len(useStmt.Names) != 0 {
		t.Fatalf("expected no selective names, got %#v", useStmt.Names)
	}
}

func TestParseUsePathAlias(t *testing.T) {
	src := `use "./mods/base.jbs" as base` + "\n"
	diags := &diag.Diagnostics{}
	prog := Parse("use_path_alias.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	useStmt, ok := prog.Stmts[0].(ast.UseStmt)
	if !ok {
		t.Fatalf("expected use statement, got %T", prog.Stmts[0])
	}
	if useStmt.Source.Kind != ast.UseSourcePath {
		t.Fatalf("expected path source, got %#v", useStmt.Source)
	}
	if useStmt.Source.Value != "./mods/base.jbs" {
		t.Fatalf("unexpected path value: %q", useStmt.Source.Value)
	}
	if useStmt.Alias != "base" {
		t.Fatalf("unexpected alias: %q", useStmt.Alias)
	}
}

func TestParseUseSelectiveImports(t *testing.T) {
	src := `
use submit_defaults, common_setup_step from jsc
use helper from "./local.jbs"
`
	diags := &diag.Diagnostics{}
	prog := Parse("use_selective.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Stmts))
	}
	first, ok := prog.Stmts[0].(ast.UseStmt)
	if !ok {
		t.Fatalf("expected first stmt use, got %T", prog.Stmts[0])
	}
	if len(first.Names) != 2 || first.Names[0] != "submit_defaults" || first.Names[1] != "common_setup_step" {
		t.Fatalf("unexpected selective names: %#v", first.Names)
	}
	if first.Source.Kind != ast.UseSourceBare || first.Source.Value != "jsc" {
		t.Fatalf("unexpected first source: %#v", first.Source)
	}
	second, ok := prog.Stmts[1].(ast.UseStmt)
	if !ok {
		t.Fatalf("expected second stmt use, got %T", prog.Stmts[1])
	}
	if len(second.Names) != 1 || second.Names[0] != "helper" {
		t.Fatalf("unexpected second selective names: %#v", second.Names)
	}
	if second.Source.Kind != ast.UseSourcePath || second.Source.Value != "./local.jbs" {
		t.Fatalf("unexpected second source: %#v", second.Source)
	}
}

func TestParseUseMalformedForms(t *testing.T) {
	sources := []string{
		`use "./x.jbs"`,
		`use a, b`,
		`use x from`,
	}
	for _, src := range sources {
		diags := &diag.Diagnostics{}
		_ = Parse("use_bad.jbs", src+"\n", diags)
		found := false
		for _, item := range diags.Items {
			if item.Code == "E430" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected E430 for malformed use statement %q, got: %s", src, diags.String())
		}
	}
}

func TestParseSubmitHeaderSingleUseClause(t *testing.T) {
	src := `
submit run
  after prep
  use defaults, gpu_defaults
  with p
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_use_ok.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block, got %T", prog.Stmts[0])
	}
	if len(sb.UseNames) != 2 || sb.UseNames[0] != "defaults" || sb.UseNames[1] != "gpu_defaults" {
		t.Fatalf("unexpected submit use names: %#v", sb.UseNames)
	}
	if len(sb.After) != 1 || sb.After[0] != "prep" {
		t.Fatalf("unexpected after clause: %#v", sb.After)
	}
	if len(sb.WithItems) != 1 || sb.WithItems[0].Name != "p" {
		t.Fatalf("unexpected with clause: %#v", sb.WithItems)
	}
}

func TestParseSubmitHeaderRepeatedUseClausesAreMerged(t *testing.T) {
	src := `
submit run
  use defaults
  use gpu_defaults
  use fast_defaults
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_use_merged.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block, got %T", prog.Stmts[0])
	}
	want := []string{"defaults", "gpu_defaults", "fast_defaults"}
	if len(sb.UseNames) != len(want) {
		t.Fatalf("unexpected submit use names length: got=%d want=%d values=%#v", len(sb.UseNames), len(want), sb.UseNames)
	}
	for i := range want {
		if sb.UseNames[i] != want[i] {
			t.Fatalf("unexpected submit use names: got=%#v want=%#v", sb.UseNames, want)
		}
	}
}

func TestParseDoHeaderStepOptions(t *testing.T) {
	src := `
do run
  with p
  max_async=5 procs=4 iterations=2
{
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("do_header_options.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block, got %T", prog.Stmts[0])
	}
	if db.MaxAsync == nil || *db.MaxAsync != 5 {
		t.Fatalf("expected max_async=5, got %#v", db.MaxAsync)
	}
	if db.Procs == nil || *db.Procs != 4 {
		t.Fatalf("expected procs=4, got %#v", db.Procs)
	}
	if db.Iterations == nil || *db.Iterations != 2 {
		t.Fatalf("expected iterations=2, got %#v", db.Iterations)
	}
}

func TestParseSubmitHeaderStepOptionsInterleaved(t *testing.T) {
	src := `
submit run
  iterations=3
  use defaults
  with p
  procs=2
  max_async=0
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_header_options.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block, got %T", prog.Stmts[0])
	}
	if sb.MaxAsync == nil || *sb.MaxAsync != 0 {
		t.Fatalf("expected max_async=0, got %#v", sb.MaxAsync)
	}
	if sb.Procs == nil || *sb.Procs != 2 {
		t.Fatalf("expected procs=2, got %#v", sb.Procs)
	}
	if sb.Iterations == nil || *sb.Iterations != 3 {
		t.Fatalf("expected iterations=3, got %#v", sb.Iterations)
	}
	if len(sb.UseNames) != 1 || sb.UseNames[0] != "defaults" {
		t.Fatalf("unexpected use names: %#v", sb.UseNames)
	}
	if len(sb.WithItems) != 1 || sb.WithItems[0].Name != "p" {
		t.Fatalf("unexpected with items: %#v", sb.WithItems)
	}
}

func TestParseSubmitHeaderStepOptionsInterleavedWithAfter(t *testing.T) {
	src := `
submit run
  with p
  max_async=1
  after prep
  iterations=2
  use defaults
  procs=0
{
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_header_options_interleaved_after.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block, got %T", prog.Stmts[0])
	}
	if len(sb.After) != 1 || sb.After[0] != "prep" {
		t.Fatalf("unexpected after dependencies: %#v", sb.After)
	}
	if len(sb.UseNames) != 1 || sb.UseNames[0] != "defaults" {
		t.Fatalf("unexpected use names: %#v", sb.UseNames)
	}
	if len(sb.WithItems) != 1 || sb.WithItems[0].Name != "p" {
		t.Fatalf("unexpected with items: %#v", sb.WithItems)
	}
	if sb.MaxAsync == nil || *sb.MaxAsync != 1 {
		t.Fatalf("expected max_async=1, got %#v", sb.MaxAsync)
	}
	if sb.Procs == nil || *sb.Procs != 0 {
		t.Fatalf("expected procs=0, got %#v", sb.Procs)
	}
	if sb.Iterations == nil || *sb.Iterations != 2 {
		t.Fatalf("expected iterations=2, got %#v", sb.Iterations)
	}
}

func TestParseStepHeaderUnknownOptionReportsE032(t *testing.T) {
	src := `
do run iterattions=1 {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad_header_unknown.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E032") {
		t.Fatalf("expected E032 for unknown header option, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "max_async, procs and iterations") {
		t.Fatalf("expected unknown option hint to list procs, got: %s", diags.String())
	}
}

func TestParseStepHeaderDuplicateOptionReportsE033(t *testing.T) {
	src := `
submit run procs=1 procs=2 {
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad_header_duplicate.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E033") {
		t.Fatalf("expected E033 for duplicate header option, got: %s", diags.String())
	}
}

func TestParseStepHeaderNonIntegerProcsOptionReportsE034(t *testing.T) {
	src := `
do run procs=abc {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad_header_nonint_procs.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E034") {
		t.Fatalf("expected E034 for non-integer procs header option, got: %s", diags.String())
	}
}

func TestParseStepHeaderNonIntegerOptionReportsE034(t *testing.T) {
	src := `
do run iterations=abc {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad_header_nonint.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E034") {
		t.Fatalf("expected E034 for non-integer header option, got: %s", diags.String())
	}
}

func TestParseStepHeaderMissingEqualsReportsE035(t *testing.T) {
	src := `
do run max_async 1 {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad_header_missing_eq.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E035") {
		t.Fatalf("expected E035 for missing '=' in header option, got: %s", diags.String())
	}
}

func TestParseParamCommentQuoteDoesNotBreakBlock(t *testing.T) {
	src := `
param p {
  # it's a comment in param block
  a = (1, 2)
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_comment_quote.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if pb.Final == nil {
		t.Fatalf("expected final combination expression")
	}
}

func TestParseDoCommentApostropheDoesNotBreakBlock(t *testing.T) {
	src := `
do work {
  # it's a comment in do block
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("do_comment_quote.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if db.Body == "" {
		t.Fatalf("expected do body")
	}
}

func TestParseSubmitRawCommentApostropheDoesNotBreakBlock(t *testing.T) {
	src := `
submit run {
  preprocess = {
    # it's a comment in preprocess
    export X=1
  }
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_comment_quote.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if len(sb.Fields) != 2 {
		t.Fatalf("expected 2 submit fields, got %d", len(sb.Fields))
	}
	if !sb.Fields[0].IsRaw {
		t.Fatalf("expected preprocess to be raw field")
	}
}

func TestParseCommentBracesDoNotAffectBlockDepth(t *testing.T) {
	src := `
param p {
  a = (1, 2)
  # comment with fake braces: { } {nested}
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("comment_braces.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
}

func TestParseSemicolonSeparatedLetParamAnalyse(t *testing.T) {
	src := `
let p {
  number = "Number: %d"; letter = "Letter: %w"; retries = 3;
}

param cases with p {
  x = (1, 2); y = (number, letter); x + y;
}

analyse write {
  n = p.number in "out.log"; w = "Word: %w" in "out.log"; (n, w);
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("semicolon_blocks.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Stmts))
	}
	lb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if len(lb.Assignments) != 3 {
		t.Fatalf("expected 3 let assignments, got %d", len(lb.Assignments))
	}
	pb, ok := prog.Stmts[1].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if len(pb.Assignments) != 2 {
		t.Fatalf("expected 2 param assignments, got %d", len(pb.Assignments))
	}
	ab, ok := prog.Stmts[2].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block")
	}
	if len(ab.Assignments) != 2 {
		t.Fatalf("expected 2 analyse assignments, got %d", len(ab.Assignments))
	}
	if len(ab.Columns) != 2 {
		t.Fatalf("expected 2 analyse columns, got %d", len(ab.Columns))
	}
}

func TestParseSemicolonSeparatedSubmitFields(t *testing.T) {
	src := `
submit run {
  queue = "batch"; account = "myacct"; args_exec = "-lc hostname";
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("semicolon_submit.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if len(sb.Fields) != 3 {
		t.Fatalf("expected 3 submit fields, got %d", len(sb.Fields))
	}
	if sb.Fields[0].Name != "queue" || sb.Fields[1].Name != "account" || sb.Fields[2].Name != "args_exec" {
		t.Fatalf("unexpected submit field order: %#v", sb.Fields)
	}
}

func TestParseSubmitRawThenSemicolonThenExpr(t *testing.T) {
	src := `
submit run {
  preprocess = {
    export X=1
  }; args_exec = "-lc hostname";
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("semicolon_submit_raw.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if len(sb.Fields) != 2 {
		t.Fatalf("expected 2 submit fields, got %d", len(sb.Fields))
	}
	if !sb.Fields[0].IsRaw || sb.Fields[0].Name != "preprocess" {
		t.Fatalf("expected first field to be raw preprocess, got %#v", sb.Fields[0])
	}
	if sb.Fields[1].IsRaw || sb.Fields[1].Name != "args_exec" {
		t.Fatalf("expected second field to be expression args_exec, got %#v", sb.Fields[1])
	}
}

func TestParseSemicolonSeparatedTopLevelGlobals(t *testing.T) {
	src := `jbs_name = "demo"; jbs_outpath = "out";
param p { a = 1; a; }
`
	diags := &diag.Diagnostics{}
	prog := Parse("semicolon_globals.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Stmts))
	}
	if _, ok := prog.Stmts[0].(ast.GlobalAssign); !ok {
		t.Fatalf("expected first statement to be global assignment")
	}
	if _, ok := prog.Stmts[1].(ast.GlobalAssign); !ok {
		t.Fatalf("expected second statement to be global assignment")
	}
	if _, ok := prog.Stmts[2].(ast.ParamBlock); !ok {
		t.Fatalf("expected third statement to be param block")
	}
}

func TestParseRepeatedSemicolonSeparators(t *testing.T) {
	src := `
let p {
  a = 1;;; b = 2;;
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("semicolon_repeated.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	lb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if len(lb.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(lb.Assignments))
	}
}

func TestParseRepeatedTopLevelSeparators(t *testing.T) {
	src := `jbs_name = "demo";;; jbs_outpath = "out";
`
	diags := &diag.Diagnostics{}
	prog := Parse("semicolon_top_repeated.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected 2 top-level assignments, got %d", len(prog.Stmts))
	}
}

func TestParseUnterminatedBlockStillReportsE025(t *testing.T) {
	src := `
param p {
  a = (1, 2)
  a
`
	diags := &diag.Diagnostics{}
	_ = Parse("unterminated.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse errors")
	}
	if !hasDiagCode(diags.Items, "E025") {
		t.Fatalf("expected E025 for unterminated block, got: %s", diags.String())
	}
}

func TestParseParamBackslashContinuationInAssignment(t *testing.T) {
	src := `
param p {
  v = 1 + \
      2 + 3
  v
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_backslash_assign.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if len(pb.Assignments) != 1 || pb.Assignments[0].Name != "v" {
		t.Fatalf("expected assignment v, got %#v", pb.Assignments)
	}
}

func TestParseParamBackslashContinuationInFinalComb(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = (3,4)
  a + \
  b
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_backslash_comb.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if pb.Final == nil {
		t.Fatalf("expected final combination expression")
	}
}

func TestParseTopLevelGlobalBackslashContinuation(t *testing.T) {
	src := `jbs_name = "demo_" + \
           "x"
jbs_outpath = "out"
`
	diags := &diag.Diagnostics{}
	prog := Parse("global_backslash.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Stmts))
	}
	if _, ok := prog.Stmts[0].(ast.GlobalAssign); !ok {
		t.Fatalf("expected first statement to be global assignment")
	}
	if _, ok := prog.Stmts[1].(ast.GlobalAssign); !ok {
		t.Fatalf("expected second statement to be global assignment")
	}
}

func TestParseSubmitBackslashContinuationInExpr(t *testing.T) {
	src := `
submit run {
  args_exec = "-lc " + \
              "hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_backslash_expr.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if len(sb.Fields) != 1 || sb.Fields[0].Name != "args_exec" || sb.Fields[0].Expr == nil {
		t.Fatalf("expected args_exec expression field, got %#v", sb.Fields)
	}
}

func TestParseAssignmentNewlineWithoutBackslashStillFails(t *testing.T) {
	src := `
param p {
  v = 1 +
      2
  v
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("param_newline_no_backslash.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error without backslash continuation")
	}
	if !hasDiagCode(diags.Items, "E058") {
		t.Fatalf("expected E058, got: %s", diags.String())
	}
}

func TestParseDanglingBackslashStillFails(t *testing.T) {
	src := `
param p {
  v = 1 + \ 
  v
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("param_dangling_backslash.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error for dangling backslash")
	}
	if !hasDiagCode(diags.Items, "E003") {
		t.Fatalf("expected E003 for dangling backslash, got: %s", diags.String())
	}
}

func TestParseCommentTrailingBackslashDoesNotContinue(t *testing.T) {
	src := `
let p {
  a = 1 # trailing \
  b = 2
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("comment_backslash_no_continue.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	lb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if len(lb.Assignments) != 2 {
		t.Fatalf("expected two assignments, got %d", len(lb.Assignments))
	}
}

func TestParseDoHeaderElementsPreserveComments(t *testing.T) {
	src := `do run
        with p  # comment 3
        # comment 1
        procs=4
        # comment 2
{
        echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("header_comments_do.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Stmts))
	}
	block, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if len(block.Header) != 4 {
		t.Fatalf("expected 4 header elements, got %d", len(block.Header))
	}
	if block.Header[0].Kind != ast.HeaderElemWith {
		t.Fatalf("expected first header element to be with, got %s", block.Header[0].Kind)
	}
	if block.Header[0].Inline == nil || block.Header[0].Inline.Text != "comment 3" {
		t.Fatalf("expected inline comment on with clause, got %#v", block.Header[0].Inline)
	}
	if block.Header[1].Kind != ast.HeaderElemComment || block.Header[1].Comment == nil || block.Header[1].Comment.Text != "comment 1" {
		t.Fatalf("expected standalone comment element for comment 1, got %#v", block.Header[1])
	}
	if block.Header[2].Kind != ast.HeaderElemOption {
		t.Fatalf("expected option element, got %s", block.Header[2].Kind)
	}
	if block.Header[3].Kind != ast.HeaderElemComment || block.Header[3].Comment == nil || block.Header[3].Comment.Text != "comment 2" {
		t.Fatalf("expected standalone comment element for comment 2, got %#v", block.Header[3])
	}
}

func TestParseParamHeaderElementsPreserveCommentBeforeBrace(t *testing.T) {
	src := `param p
	      # comment 0
{
	        a = (1,2,3)
	        a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("header_comments_param.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Stmts))
	}
	block, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if len(block.Header) != 1 {
		t.Fatalf("expected 1 header element, got %d", len(block.Header))
	}
	if block.Header[0].Kind != ast.HeaderElemComment || block.Header[0].Comment == nil || block.Header[0].Comment.Text != "comment 0" {
		t.Fatalf("expected comment element for comment 0, got %#v", block.Header[0])
	}
}

func TestParseSubmitHeaderElementsPreserveComments(t *testing.T) {
	src := `submit run
	        use defaults  # c0
	        # c1
        with p
        iterations=2 # c2
{
        args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("header_comments_submit.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	block, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if len(block.Header) != 4 {
		t.Fatalf("expected 4 header elements, got %d", len(block.Header))
	}
	if block.Header[0].Kind != ast.HeaderElemUse || block.Header[0].Inline == nil || block.Header[0].Inline.Text != "c0" {
		t.Fatalf("unexpected first header element: %#v", block.Header[0])
	}
	if block.Header[1].Kind != ast.HeaderElemComment || block.Header[1].Comment == nil || block.Header[1].Comment.Text != "c1" {
		t.Fatalf("unexpected second header element: %#v", block.Header[1])
	}
	if block.Header[2].Kind != ast.HeaderElemWith {
		t.Fatalf("unexpected third header element: %#v", block.Header[2])
	}
	if block.Header[3].Kind != ast.HeaderElemOption || block.Header[3].Inline == nil || block.Header[3].Inline.Text != "c2" {
		t.Fatalf("unexpected fourth header element: %#v", block.Header[3])
	}
}

func TestParseLetHeaderElementsPreserveCommentBeforeBrace(t *testing.T) {
	src := `let l
        # c0
{
        x = "a"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("header_comments_let.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	block, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if len(block.Header) != 1 || block.Header[0].Kind != ast.HeaderElemComment || block.Header[0].Comment == nil || block.Header[0].Comment.Text != "c0" {
		t.Fatalf("unexpected let header elements: %#v", block.Header)
	}
}

func TestParseAnalyseHeaderElementsPreserveCommentBeforeBrace(t *testing.T) {
	src := `analyse write
        with p
        # c0
{
        p0 = number in "out"
        (p0)
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("header_comments_analyse.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	block, ok := prog.Stmts[0].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block")
	}
	if len(block.Header) != 2 {
		t.Fatalf("expected 2 header elements, got %d", len(block.Header))
	}
	if block.Header[0].Kind != ast.HeaderElemWith {
		t.Fatalf("unexpected first analyse header element: %#v", block.Header[0])
	}
	if block.Header[1].Kind != ast.HeaderElemComment || block.Header[1].Comment == nil || block.Header[1].Comment.Text != "c0" {
		t.Fatalf("unexpected second analyse header element: %#v", block.Header[1])
	}
}

func TestParseIntegerLiteralBoundariesExact(t *testing.T) {
	cases := []struct {
		name      string
		literal   string
		want      int64
		wantError bool
	}{
		{name: "2^53-1", literal: "9007199254740991", want: 9007199254740991},
		{name: "2^53", literal: "9007199254740992", want: 9007199254740992},
		{name: "2^53+1", literal: "9007199254740993", want: 9007199254740993},
		{name: "int64_overflow", literal: "9223372036854775808", want: 0, wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `
param p {
  x = ` + tc.literal + `
  x
}
`
			diags := &diag.Diagnostics{}
			prog := Parse("int_boundary.jbs", src, diags)

			if tc.wantError {
				if !diags.HasErrors() {
					t.Fatalf("expected parse error for %s", tc.literal)
				}
				if !hasDiagCode(diags.Items, "E065") {
					t.Fatalf("expected E065 for %s, got: %s", tc.literal, diags.String())
				}
			} else if diags.HasErrors() {
				t.Fatalf("unexpected parse errors for %s: %s", tc.literal, diags.String())
			}

			if len(prog.Stmts) != 1 {
				t.Fatalf("expected one statement, got %d", len(prog.Stmts))
			}
			pb, ok := prog.Stmts[0].(ast.ParamBlock)
			if !ok {
				t.Fatalf("expected param block")
			}
			if len(pb.Assignments) != 1 {
				t.Fatalf("expected one assignment, got %d", len(pb.Assignments))
			}
			num, ok := pb.Assignments[0].Expr.(ast.NumberExpr)
			if !ok {
				t.Fatalf("expected number expression, got %T", pb.Assignments[0].Expr)
			}
			if !num.Int {
				t.Fatalf("expected integer literal flag for %s", tc.literal)
			}
			if num.IntValue != tc.want {
				t.Fatalf("unexpected int literal value for %s: got=%d want=%d", tc.literal, num.IntValue, tc.want)
			}
		})
	}
}

func TestParseFloatLiteralUsesFloatPayload(t *testing.T) {
	src := `
param p {
  x = 1.25
  x
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("float_literal.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if len(pb.Assignments) != 1 {
		t.Fatalf("expected one assignment")
	}
	num, ok := pb.Assignments[0].Expr.(ast.NumberExpr)
	if !ok {
		t.Fatalf("expected number expression, got %T", pb.Assignments[0].Expr)
	}
	if num.Int {
		t.Fatalf("expected float literal flag")
	}
	if num.FloatValue != 1.25 {
		t.Fatalf("unexpected float literal value: got=%v want=1.25", num.FloatValue)
	}
}

func TestParseFloatLiteralScientificAndLeadingDotVariants(t *testing.T) {
	src := `
param p {
  a = 1e3
  b = 1E5
  c = .121e-1
  d = .1E-12
  e = -.2
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("float_literal_variants.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if len(pb.Assignments) != 5 {
		t.Fatalf("expected five assignments, got %d", len(pb.Assignments))
	}
	assertFloat := func(expr ast.Expr, want float64) {
		t.Helper()
		num, ok := expr.(ast.NumberExpr)
		if !ok {
			t.Fatalf("expected number expression, got %T", expr)
		}
		if num.Int {
			t.Fatalf("expected float number expression, got int")
		}
		if math.Abs(num.FloatValue-want) > 1e-15*math.Max(1, math.Abs(want)) {
			t.Fatalf("unexpected float value: got=%v want=%v", num.FloatValue, want)
		}
	}
	assertFloat(pb.Assignments[0].Expr, 1e3)
	assertFloat(pb.Assignments[1].Expr, 1e5)
	assertFloat(pb.Assignments[2].Expr, .121e-1)
	assertFloat(pb.Assignments[3].Expr, .1e-12)
	unary, ok := pb.Assignments[4].Expr.(ast.UnaryExpr)
	if !ok || unary.Op != "-" {
		t.Fatalf("expected unary minus expression for e, got %#v", pb.Assignments[4].Expr)
	}
	assertFloat(unary.Expr, .2)
}

func TestParseParamBlockWithClauseVariants(t *testing.T) {
	src := `
param derived with base, x from lib, (y,z) from lib2 {
  a = (1, 2)
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_with_variants.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if got := len(pb.WithItems); got != 4 {
		t.Fatalf("expected 4 with items, got %d", got)
	}
	if pb.WithItems[0].Name != "base" || pb.WithItems[0].From != "" {
		t.Fatalf("unexpected with item 0: %#v", pb.WithItems[0])
	}
	if pb.WithItems[1].Name != "x" || pb.WithItems[1].From != "lib" {
		t.Fatalf("unexpected with item 1: %#v", pb.WithItems[1])
	}
	if pb.WithItems[2].Name != "y" || pb.WithItems[2].From != "lib2" {
		t.Fatalf("unexpected with item 2: %#v", pb.WithItems[2])
	}
	if pb.WithItems[3].Name != "z" || pb.WithItems[3].From != "lib2" {
		t.Fatalf("unexpected with item 3: %#v", pb.WithItems[3])
	}
}

func TestParseParamBlockWithRepeatedWithClauses(t *testing.T) {
	src := `
param derived
  with base
  with x from lib
  with (y,z) from lib2
{
  a = (1, 2)
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_with_repeated_with.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if got := len(pb.WithItems); got != 4 {
		t.Fatalf("expected 4 with items, got %d", got)
	}
	if pb.WithItems[0].Name != "base" || pb.WithItems[0].From != "" {
		t.Fatalf("unexpected with item 0: %#v", pb.WithItems[0])
	}
	if pb.WithItems[1].Name != "x" || pb.WithItems[1].From != "lib" {
		t.Fatalf("unexpected with item 1: %#v", pb.WithItems[1])
	}
	if pb.WithItems[2].Name != "y" || pb.WithItems[2].From != "lib2" {
		t.Fatalf("unexpected with item 2: %#v", pb.WithItems[2])
	}
	if pb.WithItems[3].Name != "z" || pb.WithItems[3].From != "lib2" {
		t.Fatalf("unexpected with item 3: %#v", pb.WithItems[3])
	}
}

func TestParseParamWithClauseAliasVariants(t *testing.T) {
	src := `
param derived
  with a from p0 as a_0, p1 as p1_0
{
  a_0 + p1_0
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_with_alias_variants.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if got := len(pb.WithItems); got != 2 {
		t.Fatalf("expected 2 with items, got %d", got)
	}
	if pb.WithItems[0].Name != "a" || pb.WithItems[0].From != "p0" || pb.WithItems[0].Alias != "a_0" {
		t.Fatalf("unexpected aliased variable import: %#v", pb.WithItems[0])
	}
	if pb.WithItems[1].Name != "p1" || pb.WithItems[1].From != "" || pb.WithItems[1].Alias != "p1_0" {
		t.Fatalf("unexpected aliased full import: %#v", pb.WithItems[1])
	}
}

func TestParseWithClauseTupleAliasReportsE023(t *testing.T) {
	src := `
param derived with (a,b) from p0 as pair {
  a + b
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("param_tuple_alias_error.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E023") {
		t.Fatalf("expected E023 for tuple alias in with clause, got: %s", diags.String())
	}
}

func TestParseParamBlockHeaderWithInlineComment(t *testing.T) {
	src := `param p with base # header comment
{
  a = (1)
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_header_with_comment.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if len(pb.Header) != 1 {
		t.Fatalf("expected one header element, got %d", len(pb.Header))
	}
	if pb.Header[0].Kind != ast.HeaderElemWith {
		t.Fatalf("expected with header element, got %#v", pb.Header[0])
	}
	if pb.Header[0].Inline == nil || pb.Header[0].Inline.Text != "header comment" {
		t.Fatalf("expected inline comment in with header, got %#v", pb.Header[0].Inline)
	}
}

func TestParseParamBlockMissingNameReportsE082(t *testing.T) {
	src := `
param {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_missing_name.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E082") {
		t.Fatalf("expected E082, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if pb.Name != "" {
		t.Fatalf("expected empty param block name after E082, got %q", pb.Name)
	}
}

func TestParseParamBlockMissingBodyStartReportsE083(t *testing.T) {
	src := `param p`
	diags := &diag.Diagnostics{}
	prog := Parse("param_missing_body_start.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E083") {
		t.Fatalf("expected E083, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if pb.Final != nil || len(pb.Assignments) != 0 {
		t.Fatalf("expected no parsed param body when missing '{', got assignments=%d final=%#v", len(pb.Assignments), pb.Final)
	}
}

func TestParseParamBlockUnterminatedReportsE025(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
`
	diags := &diag.Diagnostics{}
	prog := Parse("param_unterminated.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E025") {
		t.Fatalf("expected E025, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block")
	}
	if pb.Final != nil || len(pb.Assignments) != 0 {
		t.Fatalf("expected no parsed param body for unterminated block, got assignments=%d final=%#v", len(pb.Assignments), pb.Final)
	}
}

func TestParseDoBlockMissingNameReportsE030(t *testing.T) {
	src := `
do {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("do_missing_name.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E030") {
		t.Fatalf("expected E030, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if db.Name != "" {
		t.Fatalf("expected empty do block name after E030, got %q", db.Name)
	}
	if strings.TrimSpace(db.Body) != "echo hi" {
		t.Fatalf("expected do body to still be parsed, got %q", db.Body)
	}
}

func TestParseDoBlockMissingBodyStartReportsE031(t *testing.T) {
	src := `do run`
	diags := &diag.Diagnostics{}
	prog := Parse("do_missing_body_start.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E031") {
		t.Fatalf("expected E031, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if db.Body != "" {
		t.Fatalf("expected empty do body when '{' is missing, got %q", db.Body)
	}
}

func TestParseDoBlockUnterminatedReportsE025(t *testing.T) {
	src := `
do run with p {
  echo hi
`
	diags := &diag.Diagnostics{}
	prog := Parse("do_unterminated.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E025") {
		t.Fatalf("expected E025, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	db, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block")
	}
	if db.Body != "" {
		t.Fatalf("expected empty do body for unterminated block, got %q", db.Body)
	}
	if got := len(db.WithItems); got != 1 || db.WithItems[0].Name != "p" {
		t.Fatalf("expected parsed with clause to be preserved, got %#v", db.WithItems)
	}
}

func TestParseSubmitBlockMissingNameReportsE040(t *testing.T) {
	src := `
submit {
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_missing_name.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E040") {
		t.Fatalf("expected E040, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if sb.Name != "" {
		t.Fatalf("expected empty submit block name after E040, got %q", sb.Name)
	}
	if len(sb.Fields) != 1 || sb.Fields[0].Name != "args_exec" {
		t.Fatalf("expected submit body to still parse fields, got %#v", sb.Fields)
	}
}

func TestParseSubmitBlockMissingBodyStartReportsE041(t *testing.T) {
	src := `submit run after prep use defaults with p iterations=2`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_missing_body_start.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E041") {
		t.Fatalf("expected E041, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if sb.Fields != nil {
		t.Fatalf("expected no parsed submit fields when '{' is missing, got %#v", sb.Fields)
	}
	if got := len(sb.After); got != 1 || sb.After[0] != "prep" {
		t.Fatalf("expected after clause to be preserved, got %#v", sb.After)
	}
	if got := len(sb.UseNames); got != 1 || sb.UseNames[0] != "defaults" {
		t.Fatalf("expected use clause to be preserved, got %#v", sb.UseNames)
	}
	if got := len(sb.WithItems); got != 1 || sb.WithItems[0].Name != "p" {
		t.Fatalf("expected with clause to be preserved, got %#v", sb.WithItems)
	}
	if sb.Iterations == nil || *sb.Iterations != 2 {
		t.Fatalf("expected iterations option to be preserved, got %#v", sb.Iterations)
	}
}

func TestParseSubmitBlockUnterminatedReportsE025(t *testing.T) {
	src := `
submit run after prep use defaults with p {
  args_exec = "-lc hostname"
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit_unterminated.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E025") {
		t.Fatalf("expected E025, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if sb.Fields != nil {
		t.Fatalf("expected no parsed submit fields for unterminated block, got %#v", sb.Fields)
	}
	if got := len(sb.After); got != 1 || sb.After[0] != "prep" {
		t.Fatalf("expected after clause to be preserved, got %#v", sb.After)
	}
	if got := len(sb.UseNames); got != 1 || sb.UseNames[0] != "defaults" {
		t.Fatalf("expected use clause to be preserved, got %#v", sb.UseNames)
	}
	if got := len(sb.WithItems); got != 1 || sb.WithItems[0].Name != "p" {
		t.Fatalf("expected with clause to be preserved, got %#v", sb.WithItems)
	}
}

func TestParseLetBlockMissingNameReportsE080(t *testing.T) {
	src := `
let {
  x = "a"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("let_missing_name.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E080") {
		t.Fatalf("expected E080, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	lb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if lb.Name != "" {
		t.Fatalf("expected empty let block name after E080, got %q", lb.Name)
	}
	if len(lb.Assignments) != 1 || lb.Assignments[0].Name != "x" {
		t.Fatalf("expected let body to still parse, got %#v", lb.Assignments)
	}
}

func TestParseLetBlockMissingBodyStartReportsE081(t *testing.T) {
	src := `let l`
	diags := &diag.Diagnostics{}
	prog := Parse("let_missing_body_start.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E081") {
		t.Fatalf("expected E081, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	lb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if lb.Assignments != nil {
		t.Fatalf("expected no parsed let assignments when '{' is missing, got %#v", lb.Assignments)
	}
}

func TestParseLetBlockUnterminatedReportsE025(t *testing.T) {
	src := `
let l {
  x = "a"
`
	diags := &diag.Diagnostics{}
	prog := Parse("let_unterminated.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E025") {
		t.Fatalf("expected E025, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	lb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block")
	}
	if lb.Assignments != nil {
		t.Fatalf("expected no parsed let assignments for unterminated block, got %#v", lb.Assignments)
	}
}

func TestParseAnalyseBlockMissingTargetReportsE416(t *testing.T) {
	src := `
analyse {
  x = "Number: %d" in "out"
  (x)
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("analyse_missing_target.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E416") {
		t.Fatalf("expected E416, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	ab, ok := prog.Stmts[0].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block")
	}
	if ab.StepName != "" {
		t.Fatalf("expected empty analyse step name after missing target, got %q", ab.StepName)
	}
	if len(ab.Assignments) != 1 || ab.Assignments[0].Name != "x" {
		t.Fatalf("expected analyse body to still parse assignments, got %#v", ab.Assignments)
	}
	if len(ab.Columns) != 1 || ab.Columns[0].Name != "x" {
		t.Fatalf("expected analyse result tuple to parse, got %#v", ab.Columns)
	}
}

func TestParseAnalyseBlockMissingBodyStartReportsE416(t *testing.T) {
	src := `analyse step with p`
	diags := &diag.Diagnostics{}
	prog := Parse("analyse_missing_body_start.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E416") {
		t.Fatalf("expected E416, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	ab, ok := prog.Stmts[0].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block")
	}
	if got := len(ab.WithItems); got != 1 || ab.WithItems[0].Name != "p" {
		t.Fatalf("expected with clause to be preserved, got %#v", ab.WithItems)
	}
	if ab.Assignments != nil || ab.Columns != nil {
		t.Fatalf("expected no parsed analyse body when '{' is missing, got assignments=%#v columns=%#v", ab.Assignments, ab.Columns)
	}
}

func TestParseAnalyseBlockUnterminatedReportsE025(t *testing.T) {
	src := `
analyse step with p {
  x = "Number: %d" in "out"
  (x)
`
	diags := &diag.Diagnostics{}
	prog := Parse("analyse_unterminated.jbs", src, diags)
	if !hasDiagCode(diags.Items, "E025") {
		t.Fatalf("expected E025, got: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	ab, ok := prog.Stmts[0].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block")
	}
	if got := len(ab.WithItems); got != 1 || ab.WithItems[0].Name != "p" {
		t.Fatalf("expected with clause to be preserved, got %#v", ab.WithItems)
	}
	if ab.Assignments != nil || ab.Columns != nil {
		t.Fatalf("expected no parsed analyse body for unterminated block, got assignments=%#v columns=%#v", ab.Assignments, ab.Columns)
	}
}

func hasDiagCode(items []diag.Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

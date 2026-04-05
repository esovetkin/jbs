package lower_test

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func compileDoc(t *testing.T, src string) (lower.Document, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	doc := lower.ToJUBEYAML(res, lower.Options{InputPath: "in.jbs"}, diags)
	return doc, diags
}

func TestPureOuterProductLowering(t *testing.T) {
	src := `
param p {
  a = ("a","b","c")
  b = ("1","2")
  c = (1,2)
  a * b * c
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.ParameterSet) != 1 {
		t.Fatalf("expected one parameterset")
	}
	ps := doc.ParameterSet[0]
	if ps.Name != "p" {
		t.Fatalf("unexpected parameterset name: %s", ps.Name)
	}
	if len(ps.Parameter) != 4 {
		t.Fatalf("expected i + 3 params, got %d", len(ps.Parameter))
	}
	if ps.Parameter[0].Name != "_ji_p" || ps.Parameter[0].Type != "int" || ps.Parameter[0].Mode != "text" {
		t.Fatalf("expected indexed lowering via context index parameter, got %#v", ps.Parameter[0])
	}
	if ps.Parameter[0].Value != "0,1,2,3,4,5,6,7,8,9,10,11" {
		t.Fatalf("unexpected i range: %#v", ps.Parameter[0].Value)
	}
	for _, p := range ps.Parameter[1:] {
		if p.Mode != "python" {
			t.Fatalf("expected python mode payload parameter, got %#v", p)
		}
	}
}

func TestGroupedDirectSumLowering(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y","z")
  a + b
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	ps := doc.ParameterSet[0]
	if ps.Parameter[0].Name != "_ji_p" {
		t.Fatalf("expected grouped index parameter _ji_p, got %s", ps.Parameter[0].Name)
	}
	if ps.Parameter[0].Value != "0,1,2" {
		t.Fatalf("expected grouped indices 0,1,2 got %#v", ps.Parameter[0].Value)
	}
	if ps.Parameter[1].Mode != "python" || ps.Parameter[2].Mode != "python" {
		t.Fatalf("expected python-indexed grouped payload parameters")
	}
}

func TestSubsetSingleVarUsesIndexMask(t *testing.T) {
	src := `
param param {
  a = ("a","b","c")
  b = ("1","2")
  c = (1,2)
  a * b + c
}

do task with a from param {
  echo ${a}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		if strings.HasPrefix(doc.ParameterSet[i].Name, "_js__task__param__a") {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected subset parameterset for a import")
	}
	if len(subset.Parameter) != 3 {
		t.Fatalf("expected i + rows + a in subset, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "_ji__task__param__a" || subset.Parameter[0].Value != "0,2,4" {
		t.Fatalf("expected masked context index for subset, got %#v", subset.Parameter[0])
	}
	if subset.Parameter[1].Name != "_jr__task__param__a" || subset.Parameter[1].Mode != "python" {
		t.Fatalf("expected python rows context param, got %#v", subset.Parameter[1])
	}
	if subset.Parameter[1].Separator != "####" {
		t.Fatalf("expected rows context separator ####, got %#v", subset.Parameter[1].Separator)
	}
	rowsExpr, ok := subset.Parameter[1].Value.(lower.SingleQuoted)
	if !ok {
		t.Fatalf("expected single-quoted rows lookup expression, got %T", subset.Parameter[1].Value)
	}
	if string(rowsExpr) != "{\"0\":\"0,1\",\"2\":\"2,3\",\"4\":\"4,5\"}[\"${_ji__task__param__a}\"]" {
		t.Fatalf("unexpected rows context payload: %q", string(rowsExpr))
	}
	if subset.Parameter[2].Name != "a" || subset.Parameter[2].Mode != "python" {
		t.Fatalf("expected python indexed a param, got %#v", subset.Parameter[2])
	}
	gotExpr, ok := subset.Parameter[2].Value.(lower.SingleQuoted)
	if !ok {
		t.Fatalf("expected single-quoted python expression, got %T", subset.Parameter[2].Value)
	}
	if string(gotExpr) != "[\"a\",\"a\",\"b\",\"b\",\"c\",\"c\"][$_ji__task__param__a]" {
		t.Fatalf("unexpected subset a expression: %q", string(gotExpr))
	}
}

func TestSubsetTupleFromSameParam(t *testing.T) {
	src := `
param param {
  a = ("a","b","c")
  b = ("1","2")
  c = (1,2)
  a * b + c
}

do task with (a,b) from param {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		if strings.HasPrefix(doc.ParameterSet[i].Name, "_js__task__param__a_b") {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected subset parameterset for (a,b) import")
	}
	if len(subset.Parameter) != 4 {
		t.Fatalf("expected i + rows + a + b in subset, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "_ji__task__param__a_b" || subset.Parameter[0].Value != "0,1,2,3,4,5" {
		t.Fatalf("unexpected tuple subset i mask: %#v", subset.Parameter[0])
	}
	if subset.Parameter[1].Name != "_jr__task__param__a_b" || subset.Parameter[1].Mode != "python" {
		t.Fatalf("expected rows helper for tuple subset, got %#v", subset.Parameter[1])
	}
	if subset.Parameter[1].Separator != "####" {
		t.Fatalf("expected tuple rows helper separator ####, got %#v", subset.Parameter[1].Separator)
	}
	tupleRowsExpr, okTupleRows := subset.Parameter[1].Value.(lower.SingleQuoted)
	if !okTupleRows {
		t.Fatalf("expected single-quoted tuple rows lookup expression, got %T", subset.Parameter[1].Value)
	}
	if string(tupleRowsExpr) != "{\"0\":\"0\",\"1\":\"1\",\"2\":\"2\",\"3\":\"3\",\"4\":\"4\",\"5\":\"5\"}[\"${_ji__task__param__a_b}\"]" {
		t.Fatalf("unexpected tuple rows helper payload: %q", string(tupleRowsExpr))
	}
	if subset.Parameter[2].Mode != "python" || subset.Parameter[3].Mode != "python" {
		t.Fatalf("expected python indexed tuple subset params, got %#v", subset.Parameter)
	}
	aExpr, okA := subset.Parameter[2].Value.(lower.SingleQuoted)
	bExpr, okB := subset.Parameter[3].Value.(lower.SingleQuoted)
	if !okA || !okB {
		t.Fatalf("expected single-quoted tuple expressions, got %T %T", subset.Parameter[2].Value, subset.Parameter[3].Value)
	}
	if string(aExpr) != "[\"a\",\"a\",\"b\",\"b\",\"c\",\"c\"][$_ji__task__param__a_b]" {
		t.Fatalf("unexpected a expression: %q", string(aExpr))
	}
	if string(bExpr) != "[\"1\",\"2\",\"1\",\"2\",\"1\",\"2\"][$_ji__task__param__a_b]" {
		t.Fatalf("unexpected b expression: %q", string(bExpr))
	}
}

func TestSubmitAndSubsetLowering(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}

do prep with a from p {
  echo prep
}

submit run after prep with p {
  preprocess = {
    export X=1
  }
  args_exec = "python main.py"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(doc.Step))
	}
	if len(doc.ParameterSet) < 3 {
		t.Fatalf("expected main + subset + submit parameter sets")
	}

	var hasSubset bool
	var hasSubmitSet bool
	for _, ps := range doc.ParameterSet {
		if strings.HasPrefix(ps.Name, "_js__") {
			hasSubset = true
		}
		if strings.HasSuffix(ps.Name, "__submit_params") {
			hasSubmitSet = true
		}
	}
	if !hasSubset {
		t.Fatalf("expected synthetic subset parameterset")
	}
	if !hasSubmitSet {
		t.Fatalf("expected synthetic submit parameterset")
	}
}

func TestDoRawBlockIndentationNormalized(t *testing.T) {
	src := `
param p {
  a = 1
  a
}

do run with p {
    echo one
      echo two
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one step")
	}
	body, ok := doc.Step[0].Do[0].(lower.Literal)
	if !ok {
		t.Fatalf("expected literal do body, got %T", doc.Step[0].Do[0])
	}
	got := string(body)
	if strings.Contains(got, "set -euo pipefail") || strings.Contains(got, "${jube_benchmark_home}") {
		t.Fatalf("unexpected preamble in do body: %q", got)
	}
	if strings.HasPrefix(got, " ") {
		t.Fatalf("unexpected leading indentation in do body: %q", got)
	}
	if got != "echo one\n  echo two\n" {
		t.Fatalf("expected normalized do body, got: %q", got)
	}
}

func TestMixedWithVariableAndWholeParamsetLowering(t *testing.T) {
	src := `
param p1 {
  a = (1,2)
  a
}
param p2 {
  b = ("x","y")
  b
}
do setup with a from p1, p2 {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one step")
	}
	use := doc.Step[0].Use
	hasP2 := false
	hasSubsetP1 := false
	for _, u := range use {
		if s, ok := u.(string); ok {
			if s == "p2" {
				hasP2 = true
			}
			if strings.HasPrefix(s, "_js__setup__p1__") {
				hasSubsetP1 = true
			}
		}
	}
	if !hasP2 {
		t.Fatalf("expected direct import of full parameterset p2 in step use list: %#v", use)
	}
	if !hasSubsetP1 {
		t.Fatalf("expected subset import for a from p1 in step use list: %#v", use)
	}
}

func TestMixedWithTupleVariableAndWholeParamsetLowering(t *testing.T) {
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
do setup with (a,b) from p1, p2 {
  echo ${a} ${b} ${c}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one step")
	}
	use := doc.Step[0].Use
	hasP2 := false
	hasSubsetP1 := false
	for _, u := range use {
		if s, ok := u.(string); ok {
			if s == "p2" {
				hasP2 = true
			}
			if strings.HasPrefix(s, "_js__setup__p1__") {
				hasSubsetP1 = true
			}
		}
	}
	if !hasP2 {
		t.Fatalf("expected direct import of full parameterset p2 in step use list: %#v", use)
	}
	if !hasSubsetP1 {
		t.Fatalf("expected subset import for (a,b) from p1 in step use list: %#v", use)
	}
}

func TestAfterInheritanceWithParamsetUsesOnlyDeltaSubset(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with p {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var hasSubsetB bool
	for _, ps := range doc.ParameterSet {
		if strings.Contains(ps.Name, "_js__s1__p__b") {
			hasSubsetB = true
			break
		}
	}
	if !hasSubsetB {
		t.Fatalf("expected synthetic subset for non-inherited delta variable b, got %#v", doc.ParameterSet)
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected two steps, got %d", len(doc.Step))
	}
	use := doc.Step[1].Use
	hasSubset := false
	hasWhole := false
	for _, u := range use {
		s, ok := u.(string)
		if !ok {
			continue
		}
		if s == "p" {
			hasWhole = true
		}
		if strings.Contains(s, "_js__s1__p__b") {
			hasSubset = true
		}
	}
	if hasWhole {
		t.Fatalf("did not expect full paramset import p for inherited+delta case: %#v", use)
	}
	if !hasSubset {
		t.Fatalf("expected subset import for only variable b, got %#v", use)
	}
}

func TestAfterInheritanceWithOnlyInheritedVarsHasNoUseEntries(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with a from p {
  echo ${a}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected two steps, got %d", len(doc.Step))
	}
	if got := len(doc.Step[1].Use); got != 0 {
		t.Fatalf("expected no explicit use entries for fully inherited imports, got %#v", doc.Step[1].Use)
	}
}

func TestAfterInheritanceSubsetUsesInheritedRowsContext(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with b from p {
  echo ${a} ${b}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		if doc.ParameterSet[i].Name == "_js__s1__p__b" {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected contextual subset for step s1")
	}
	if len(subset.Parameter) != 3 {
		t.Fatalf("expected contextual subset params i + rows + b, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "_ji__s1__p__b" || subset.Parameter[0].Separator != "," {
		t.Fatalf("expected contextual index with separator ',', got %#v", subset.Parameter[0])
	}
	if subset.Parameter[0].Value != "$_jr__s0__p__a" {
		t.Fatalf("expected inherited rows context reference, got %#v", subset.Parameter[0].Value)
	}
	if subset.Parameter[1].Name != "_jr__s1__p__b" || subset.Parameter[1].Mode != "text" {
		t.Fatalf("expected contextual rows helper, got %#v", subset.Parameter[1])
	}
	if subset.Parameter[1].Value != "${_ji__s1__p__b}" {
		t.Fatalf("expected contextual rows helper to mirror idx, got %#v", subset.Parameter[1].Value)
	}
}

func TestAfterInheritanceTransitiveContextPropagation(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y","z")
  c = ("u","v","w")
  a * (b + c)
}
do s0 with a from p {
  echo ${a}
}
do s1 after s0 with b from p {
  echo ${a} ${b}
}
do s2 after s1 with c from p {
  echo ${a} ${b} ${c}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var s1Subset, s2Subset *lower.ParameterSet
	for i := range doc.ParameterSet {
		switch doc.ParameterSet[i].Name {
		case "_js__s1__p__b":
			s1Subset = &doc.ParameterSet[i]
		case "_js__s2__p__c":
			s2Subset = &doc.ParameterSet[i]
		}
	}
	if s1Subset == nil || s2Subset == nil {
		t.Fatalf("expected contextual subsets for s1 and s2, got %#v", doc.ParameterSet)
	}
	if s1Subset.Parameter[0].Value != "$_jr__s0__p__a" {
		t.Fatalf("expected s1 contextual source from s0 rows helper, got %#v", s1Subset.Parameter[0].Value)
	}
	if s2Subset.Parameter[0].Value != "$_jr__s1__p__b" {
		t.Fatalf("expected s2 contextual source from s1 rows helper, got %#v", s2Subset.Parameter[0].Value)
	}
}

func TestAfterInheritanceConflictingRowContextsRaiseError(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  c = ("u","v")
  a + b + c
}
do s0 with a from p {
  echo ${a}
}
do s1 with b from p {
  echo ${b}
}
do s2 after s0,s1 with c from p {
  echo ${a} ${b} ${c}
}
`
	_, diags := compileDoc(t, src)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E232" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E232 for conflicting inherited row contexts, got: %s", diags.String())
	}
}

func TestModeDeclarationsLowering(t *testing.T) {
	src := `
param p {
  queue = python("__import__(\"os\").environ.get(\"JUBE_QUEUE\", \"devel\")")
  system_name = shell("cat /etc/FZJ/systemname | tr -d '\n'")
  queue * system_name
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	ps := doc.ParameterSet[0]
	if len(ps.Parameter) != 3 {
		t.Fatalf("expected i + two parameters, got %d", len(ps.Parameter))
	}
	if ps.Parameter[0].Name != "_ji_p" {
		t.Fatalf("expected context index variable _ji_p, got %#v", ps.Parameter[0])
	}
	var queue, system lower.Parameter
	for _, p := range ps.Parameter {
		if p.Name == "queue" {
			queue = p
		}
		if p.Name == "system_name" {
			system = p
		}
	}
	if queue.Mode != "python" {
		t.Fatalf("expected queue mode python, got %q", queue.Mode)
	}
	if _, ok := queue.Value.(lower.SingleQuoted); !ok {
		t.Fatalf("expected queue value to be single-quoted scalar wrapper, got %T", queue.Value)
	}
	if system.Mode != "shell" {
		t.Fatalf("expected system_name mode shell, got %q", system.Mode)
	}
	if _, ok := system.Value.(string); !ok {
		t.Fatalf("expected system_name shell payload string, got %T", system.Value)
	}
}

func TestRootGlobalsAndSubmitFieldsDriveOutput(t *testing.T) {
	src := `
jbs_name = "demo_bench"
jbs_outpath = "results"
jbs_comment = "demo comment"

param p {
  a = 1
  a
}

submit run with p {
  queue = python("__import__('os').environ.get('JUBE_QUEUE', 'devel')")
  nodes = 2
  tasks = 2
  preprocess = {
    export X=1
  }
  postprocess = {
    export Y=2
  }
  args_exec = "echo ok"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if doc.Name != "demo_bench" {
		t.Fatalf("expected root name from jbs_name, got %q", doc.Name)
	}
	if doc.Outpath != "results" {
		t.Fatalf("expected root outpath from jbs_outpath, got %q", doc.Outpath)
	}
	if doc.Comment != "demo comment" {
		t.Fatalf("expected root comment from jbs_comment, got %q", doc.Comment)
	}
	var submitSet *lower.ParameterSet
	for i := range doc.ParameterSet {
		if strings.HasSuffix(doc.ParameterSet[i].Name, "__submit_params") {
			submitSet = &doc.ParameterSet[i]
			break
		}
	}
	if submitSet == nil {
		t.Fatalf("submit parameterset missing")
	}
	foundQueue := false
	foundNodes := false
	foundTasks := false
	foundPreprocess := false
	foundPostprocess := false
	for _, p := range submitSet.Parameter {
		if p.Name == "queue" {
			foundQueue = true
			if p.Mode != "python" {
				t.Fatalf("expected queue mode python, got %q", p.Mode)
			}
			if _, ok := p.Value.(lower.SingleQuoted); !ok {
				t.Fatalf("expected queue python payload single-quoted, got %T", p.Value)
			}
		}
		if p.Name == "nodes" {
			foundNodes = true
			if p.Value != "2" {
				t.Fatalf("expected nodes 2, got %#v", p.Value)
			}
		}
		if p.Name == "tasks" {
			foundTasks = true
			if p.Value != "2" {
				t.Fatalf("expected tasks 2, got %#v", p.Value)
			}
		}
		if p.Name == "preprocess" {
			foundPreprocess = true
			if p.Mode != "text" {
				t.Fatalf("expected preprocess mode text, got %q", p.Mode)
			}
			if p.Separator != "" {
				t.Fatalf("expected preprocess separator to be omitted, got %q", p.Separator)
			}
			body, ok := p.Value.(lower.Literal)
			if !ok {
				t.Fatalf("expected preprocess literal payload, got %T", p.Value)
			}
			text := string(body)
			if strings.Contains(text, "set -euo pipefail") || strings.Contains(text, "${jube_benchmark_home}") {
				t.Fatalf("unexpected preprocess preamble in payload: %q", text)
			}
			if strings.Contains(text, "\n  \n") {
				t.Fatalf("unexpected trailing whitespace-only lines in preprocess payload: %q", text)
			}
			if text != "export X=1\n" {
				t.Fatalf("expected normalized preprocess body, got: %q", text)
			}
		}
		if p.Name == "postprocess" {
			foundPostprocess = true
			if p.Mode != "text" {
				t.Fatalf("expected postprocess mode text, got %q", p.Mode)
			}
			if p.Separator != "" {
				t.Fatalf("expected postprocess separator to be omitted, got %q", p.Separator)
			}
			body, ok := p.Value.(lower.Literal)
			if !ok {
				t.Fatalf("expected postprocess literal payload, got %T", p.Value)
			}
			if string(body) != "export Y=2\n" {
				t.Fatalf("expected normalized postprocess body, got: %q", string(body))
			}
		}
	}
	if !foundQueue || !foundNodes || !foundTasks || !foundPreprocess || !foundPostprocess {
		t.Fatalf("missing required params in submit set: %#v", submitSet.Parameter)
	}
}

func TestSubmitAutoAddsTasksFromNodesReferenceWhenMissing(t *testing.T) {
	src := `
param p {
  a = 1
  a
}

submit run with p {
  executable = "/bin/bash"
  args_exec = "-lc hostname"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	var submitSet *lower.ParameterSet
	for i := range doc.ParameterSet {
		if strings.HasSuffix(doc.ParameterSet[i].Name, "__submit_params") {
			submitSet = &doc.ParameterSet[i]
			break
		}
	}
	if submitSet == nil {
		t.Fatalf("submit parameterset missing")
	}
	if len(submitSet.Parameter) != 3 {
		t.Fatalf("expected executable,args_exec plus auto tasks, got %#v", submitSet.Parameter)
	}
	foundTasks := false
	for _, p := range submitSet.Parameter {
		if p.Name == "tasks" {
			foundTasks = true
			if p.Value != "$nodes" {
				t.Fatalf("expected auto tasks to reference nodes, got %#v", p.Value)
			}
			continue
		}
		if p.Name != "executable" && p.Name != "args_exec" {
			t.Fatalf("unexpected implicit submit parameter %q in %#v", p.Name, submitSet.Parameter)
		}
	}
	if !foundTasks {
		t.Fatalf("expected auto tasks parameter in submit set: %#v", submitSet.Parameter)
	}
}

func TestLoweringPopulatesRoleMetadata(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

do prep with a from p {
  echo prep
}

submit run after prep with p {
  preprocess = {
    export X=1
  }
  args_exec = "echo ok"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	var paramSet *lower.ParameterSet
	var subsetSet *lower.ParameterSet
	var submitSet *lower.ParameterSet
	for i := range doc.ParameterSet {
		ps := &doc.ParameterSet[i]
		switch {
		case ps.Name == "p":
			paramSet = ps
		case strings.HasPrefix(ps.Name, "_js__"):
			subsetSet = ps
		case strings.HasSuffix(ps.Name, "__submit_params"):
			submitSet = ps
		}
	}
	if paramSet == nil || subsetSet == nil || submitSet == nil {
		t.Fatalf("expected param/subset/submit parametersets, got %#v", doc.ParameterSet)
	}
	if paramSet.Meta.Kind != lower.ParameterSetKindParam || paramSet.Meta.Source != "p" {
		t.Fatalf("unexpected paramset meta: %#v", paramSet.Meta)
	}
	if subsetSet.Meta.Kind != lower.ParameterSetKindSubset || subsetSet.Meta.Source != "p" {
		t.Fatalf("unexpected subset meta: %#v", subsetSet.Meta)
	}
	if submitSet.Meta.Kind != lower.ParameterSetKindSubmitInit || submitSet.Meta.Source != "run" {
		t.Fatalf("unexpected submit meta: %#v", submitSet.Meta)
	}

	if len(doc.Step) != 2 {
		t.Fatalf("expected two steps, got %d", len(doc.Step))
	}
	var prep, run *lower.Step
	for i := range doc.Step {
		s := &doc.Step[i]
		if s.Name == "prep" {
			prep = s
		}
		if s.Name == "run" {
			run = s
		}
	}
	if prep == nil || run == nil {
		t.Fatalf("missing prep/run steps: %#v", doc.Step)
	}
	if prep.Meta.Kind != lower.StepKindDo || prep.Meta.Source != "prep" {
		t.Fatalf("unexpected prep step meta: %#v", prep.Meta)
	}
	if run.Meta.Kind != lower.StepKindSubmit || run.Meta.Source != "run" {
		t.Fatalf("unexpected run step meta: %#v", run.Meta)
	}
}

func TestLetAndAnalyseLowering(t *testing.T) {
	src := `
param params {
  x = (1,2,3)
  a = ("a","b","c")
  a + x
}

do write with params {
  echo "Number: ${x}" > en
  echo "Letter: ${a}" >> en
  echo "Zahl: ${x}" > de
}

let p {
  number = "Number: %d"
  zahl = "Zahl: %d"
  letter = "Letter: %w"
}

analyse write with p {
  p0 = number in "en"
  p1 = zahl in "de"
  p2 = letter in "en"
  (
    a,
    x,
    p0,
    p1 as "de zahl",
    p2,
  )
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.PatternSet) != 1 {
		t.Fatalf("expected one grouped patternset, got %#v", doc.PatternSet)
	}
	ps := doc.PatternSet[0]
	if ps.Name != "p" {
		t.Fatalf("expected grouped patternset named p, got %#v", ps.Name)
	}
	if len(ps.Pattern) != 3 {
		t.Fatalf("expected 3 alias patterns, got %#v", ps.Pattern)
	}
	for _, p := range ps.Pattern {
		if !strings.Contains(p.Name, "__write__") {
			t.Fatalf("expected analyse alias pattern naming only, got %#v", p.Name)
		}
		if !p.Meta.IsAnalyseAlias || p.Meta.AnalyseStep != "write" {
			t.Fatalf("expected analyse alias pattern metadata, got %#v", p.Meta)
		}
	}
	if len(doc.Analyser) != 1 {
		t.Fatalf("expected one analyser, got %#v", doc.Analyser)
	}
	an := doc.Analyser[0]
	if an.Use != "p" {
		t.Fatalf("expected compact analyser use 'p', got %#v", an.Use)
	}
	if an.Analyse[0].Step != "write" {
		t.Fatalf("unexpected analyse step: %#v", an.Analyse[0])
	}
	if len(an.Analyse[0].File) != 2 {
		t.Fatalf("expected deduplicated analyse files, got %#v", an.Analyse[0].File)
	}
	if an.Analyse[0].File[0].Use != "p" || an.Analyse[0].File[0].Value != "en" {
		t.Fatalf("unexpected first analyse file: %#v", an.Analyse[0].File[0])
	}
	if an.Analyse[0].File[1].Use != "p" || an.Analyse[0].File[1].Value != "de" {
		t.Fatalf("unexpected second analyse file: %#v", an.Analyse[0].File[1])
	}
	if doc.Result == nil {
		t.Fatalf("expected result object")
	}
	if len(doc.Result.Use) != 1 || doc.Result.Use[0] != an.Name {
		t.Fatalf("unexpected result use list: %#v", doc.Result.Use)
	}
	if len(doc.Result.Table) != 1 {
		t.Fatalf("expected one result table, got %#v", doc.Result.Table)
	}
	table := doc.Result.Table[0]
	if table.Style != "csv" {
		t.Fatalf("expected csv style, got %#v", table.Style)
	}
	if len(table.Column) != 5 {
		t.Fatalf("unexpected columns: %#v", table.Column)
	}
	if table.Column[2].Title != "p0" || table.Column[2].Expr != "_jp__p_number__write__p0" {
		t.Fatalf("unexpected first analyse result column: %#v", table.Column[2])
	}
	if table.Column[3].Title != "de zahl" || table.Column[3].Expr != "_jp__p_zahl__write__p1" {
		t.Fatalf("unexpected aliased result column: %#v", table.Column[3])
	}
	if table.Column[4].Title != "p2" || table.Column[4].Expr != "_jp__p_letter__write__p2" {
		t.Fatalf("unexpected second analyse result column: %#v", table.Column[4])
	}
}

func TestAnalyseAliasPatternsetMaterialization(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "Number: ${a}" > en
  echo "Number: ${a}" > de
}
let g {
  number = "Number: %d"
}
analyse write with g {
  p0 = number in "en"
  p1 = number in "de"
  (a, p0, p1)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Analyser) != 1 {
		t.Fatalf("expected one analyser")
	}
	if doc.Analyser[0].Use != "g" {
		t.Fatalf("expected compact analyser use 'g', got %#v", doc.Analyser[0].Use)
	}
	files := doc.Analyser[0].Analyse[0].File
	if len(files) != 2 {
		t.Fatalf("expected two analyse file entries, got %#v", files)
	}
	if files[0].Use != "g" || files[1].Use != "g" {
		t.Fatalf("expected grouped patternset use for both files, got %#v", files)
	}
	if len(doc.PatternSet) != 1 || doc.PatternSet[0].Name != "g" {
		t.Fatalf("expected one grouped patternset g, got %#v", doc.PatternSet)
	}
	if len(doc.PatternSet[0].Pattern) != 2 {
		t.Fatalf("expected only alias patterns in grouped set, got %#v", doc.PatternSet[0].Pattern)
	}
	names := make(map[string]struct{}, len(doc.PatternSet[0].Pattern))
	for _, pat := range doc.PatternSet[0].Pattern {
		names[pat.Name] = struct{}{}
	}
	if _, ok := names["_jp__g_number__write__p0"]; !ok {
		t.Fatalf("expected alias pattern p0 in grouped set: %#v", doc.PatternSet[0].Pattern)
	}
	if _, ok := names["_jp__g_number__write__p1"]; !ok {
		t.Fatalf("expected alias pattern p1 in grouped set: %#v", doc.PatternSet[0].Pattern)
	}
}

func TestAnalyseResultColumnUsesAliasPatternName(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "Number: ${a}" > en
}
let g {
  number = "Number: %d"
}
analyse write with g {
  number = number in "en"
  (a, number)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if doc.Result == nil || len(doc.Result.Table) != 1 || len(doc.Result.Table[0].Column) != 2 {
		t.Fatalf("unexpected result shape: %#v", doc.Result)
	}
	col := doc.Result.Table[0].Column[1]
	if col.Title != "number" || col.Expr != "_jp__g_number__write__number" {
		t.Fatalf("unexpected analyse column mapping: %#v", col)
	}
}

func TestAnalyseCompactUseAcrossMultiplePatternGroups(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "A 1" > a.out
  echo "B 1" > b.out
}
let g1 {
  x = "A %d"
}
let g2 {
  y = "B %d"
}
analyse write with g1, g2 {
  ax = x in "a.out"
  by = y in "b.out"
  (a, ax, by)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Analyser) != 1 {
		t.Fatalf("expected one analyser")
	}
	if doc.Analyser[0].Use != "g1, g2" {
		t.Fatalf("expected compact analyser use 'g1, g2', got %#v", doc.Analyser[0].Use)
	}
}

func TestAnalyseInlineExpressionsUseDistinctSyntheticIds(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "A 1" > a.out
  echo "B 1" > b.out
}
analyse write {
  ax = "A %d" in "a.out"
  by = "B %d" in "b.out"
  (a, ax, by)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.PatternSet) != 2 {
		t.Fatalf("expected two synthetic pattern sets, got %#v", doc.PatternSet)
	}
	names := map[string]struct{}{}
	for _, ps := range doc.PatternSet {
		names[ps.Name] = struct{}{}
	}
	if _, ok := names["_ja_write_ax"]; !ok {
		t.Fatalf("missing synthetic inline pattern set for ax: %#v", doc.PatternSet)
	}
	if _, ok := names["_ja_write_by"]; !ok {
		t.Fatalf("missing synthetic inline pattern set for by: %#v", doc.PatternSet)
	}
	if doc.Result == nil || len(doc.Result.Table) != 1 {
		t.Fatalf("missing result table")
	}
	cols := doc.Result.Table[0].Column
	if len(cols) != 3 {
		t.Fatalf("unexpected result columns: %#v", cols)
	}
	if cols[1].Expr == cols[2].Expr {
		t.Fatalf("expected distinct synthetic ids for inline expressions, got %#v", cols)
	}
}

func TestStepWithLetNamespaceLowersToSyntheticSubset(t *testing.T) {
	src := `
let l {
  systemname = shell("hostname")
  queue = "batch"
}
do run with l {
  echo ${systemname} ${queue}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.ParameterSet) != 1 {
		t.Fatalf("expected one synthetic parameterset for let import, got %#v", doc.ParameterSet)
	}
	ps := doc.ParameterSet[0]
	if !strings.HasPrefix(ps.Name, "_js__run__l__") {
		t.Fatalf("expected let synthetic subset name, got %#v", ps.Name)
	}
	if len(doc.Step) != 1 || len(doc.Step[0].Use) != 1 {
		t.Fatalf("expected one step use entry, got %#v", doc.Step)
	}
	if useName, ok := doc.Step[0].Use[0].(string); !ok || useName != ps.Name {
		t.Fatalf("expected step to use synthetic let subset %q, got %#v", ps.Name, doc.Step[0].Use)
	}
	foundSystem := false
	foundQueue := false
	for _, p := range ps.Parameter {
		if p.Name == "systemname" && p.Mode == "shell" {
			foundSystem = true
		}
		if p.Name == "queue" && p.Mode == "text" {
			foundQueue = true
		}
	}
	if !foundSystem || !foundQueue {
		t.Fatalf("expected systemname(shell) and queue(text) in synthetic subset, got %#v", ps.Parameter)
	}
}

func TestQualifiedLetReferenceIsRejected(t *testing.T) {
	src := `
let l {
  systemname = shell("hostname")
}
do run {
  echo ${l.systemname}
}
`
	_, diags := compileDoc(t, src)
	if !diags.HasErrors() {
		t.Fatalf("expected errors for qualified let reference")
	}
	found := false
	for _, d := range diags.Items {
		if d.Code == "E100" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E100 for qualified let reference, got: %s", diags.String())
	}
}

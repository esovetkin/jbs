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
	if ps.Parameter[0].Name != "i" || ps.Parameter[0].Type != "int" || ps.Parameter[0].Mode != "text" {
		t.Fatalf("expected indexed lowering via i parameter, got %#v", ps.Parameter[0])
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
	if ps.Parameter[0].Name != "i" {
		t.Fatalf("expected grouped index parameter i, got %s", ps.Parameter[0].Name)
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
		if strings.HasPrefix(doc.ParameterSet[i].Name, "__subset_param__a") {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected subset parameterset for a import")
	}
	if len(subset.Parameter) != 2 {
		t.Fatalf("expected i + a in subset, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "i" || subset.Parameter[0].Value != "0,2,4" {
		t.Fatalf("expected i mask 0,2,4, got %#v", subset.Parameter[0])
	}
	if subset.Parameter[1].Name != "a" || subset.Parameter[1].Mode != "python" {
		t.Fatalf("expected python indexed a param, got %#v", subset.Parameter[1])
	}
	gotExpr, ok := subset.Parameter[1].Value.(lower.SingleQuoted)
	if !ok {
		t.Fatalf("expected single-quoted python expression, got %T", subset.Parameter[1].Value)
	}
	if string(gotExpr) != "[\"a\",\"a\",\"b\",\"b\",\"c\",\"c\"][$i]" {
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
		if strings.HasPrefix(doc.ParameterSet[i].Name, "__subset_param__a_b") {
			subset = &doc.ParameterSet[i]
			break
		}
	}
	if subset == nil {
		t.Fatalf("expected subset parameterset for (a,b) import")
	}
	if len(subset.Parameter) != 3 {
		t.Fatalf("expected i + a + b in subset, got %#v", subset.Parameter)
	}
	if subset.Parameter[0].Name != "i" || subset.Parameter[0].Value != "0,1,2,3,4,5" {
		t.Fatalf("unexpected tuple subset i mask: %#v", subset.Parameter[0])
	}
	if subset.Parameter[1].Mode != "python" || subset.Parameter[2].Mode != "python" {
		t.Fatalf("expected python indexed tuple subset params, got %#v", subset.Parameter)
	}
	aExpr, okA := subset.Parameter[1].Value.(lower.SingleQuoted)
	bExpr, okB := subset.Parameter[2].Value.(lower.SingleQuoted)
	if !okA || !okB {
		t.Fatalf("expected single-quoted tuple expressions, got %T %T", subset.Parameter[1].Value, subset.Parameter[2].Value)
	}
	if string(aExpr) != "[\"a\",\"a\",\"b\",\"b\",\"c\",\"c\"][$i]" {
		t.Fatalf("unexpected a expression: %q", string(aExpr))
	}
	if string(bExpr) != "[\"1\",\"2\",\"1\",\"2\",\"1\",\"2\"][$i]" {
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
		if strings.HasPrefix(ps.Name, "__subset_") {
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

func TestDoRawBlockIndentationAlignedWithPrelude(t *testing.T) {
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
	if !strings.Contains(got, "set -euo pipefail\ncd \"${jube_benchmark_home}\"\n") {
		t.Fatalf("missing preamble in do body: %q", got)
	}
	if strings.Contains(got, "\n    echo one") {
		t.Fatalf("unexpected extra indentation for first line after preamble: %q", got)
	}
	if !strings.Contains(got, "\necho one\n  echo two\n") {
		t.Fatalf("expected normalized relative indentation after preamble, got: %q", got)
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
			if strings.HasPrefix(s, "__subset_p1__") {
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
			if strings.HasPrefix(s, "__subset_p1__") {
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
			body, ok := p.Value.(lower.Literal)
			if !ok {
				t.Fatalf("expected preprocess literal payload, got %T", p.Value)
			}
			text := string(body)
			if !strings.Contains(text, "set -euo pipefail\n") || !strings.Contains(text, "cd \"${jube_benchmark_home}\"\n") {
				t.Fatalf("expected preprocess preamble in payload, got: %q", text)
			}
			if strings.Contains(text, "\n  \n") {
				t.Fatalf("unexpected trailing whitespace-only lines in preprocess payload: %q", text)
			}
			if !strings.Contains(text, "\nexport X=1\n") {
				t.Fatalf("expected normalized preprocess body without source indentation, got: %q", text)
			}
		}
	}
	if !foundQueue || !foundNodes || !foundTasks || !foundPreprocess {
		t.Fatalf("missing required params in submit set: %#v", submitSet.Parameter)
	}
}

func TestSubmitEmitsOnlyExplicitKeys(t *testing.T) {
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
	if len(submitSet.Parameter) != 2 {
		t.Fatalf("expected exactly 2 submit params, got %#v", submitSet.Parameter)
	}
	for _, p := range submitSet.Parameter {
		if p.Name != "executable" && p.Name != "args_exec" {
			t.Fatalf("unexpected implicit submit parameter %q in %#v", p.Name, submitSet.Parameter)
		}
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
		case strings.HasPrefix(ps.Name, "__subset_"):
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

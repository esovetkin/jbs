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
  a = (1,2)
  b = ("x","y")
  a * b
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
	if len(ps.Parameter) != 2 {
		t.Fatalf("expected 2 params, got %d", len(ps.Parameter))
	}
	if ps.Parameter[0].Separator != lower.ReservedSeparator || ps.Parameter[1].Separator != lower.ReservedSeparator {
		t.Fatalf("expected reserved separator in both template params")
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

func TestReservedSeparatorError(t *testing.T) {
	src := `
param p {
  a = ("ok","bad####value")
  a
}
`
	_, diags := compileDoc(t, src)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E053" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected E053, got: %s", diags.String())
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
	if len(ps.Parameter) != 2 {
		t.Fatalf("expected two parameters, got %d", len(ps.Parameter))
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

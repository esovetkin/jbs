package lower_test

import (
	"strings"
	"testing"

	"jbs/internal/lower"
)

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

func TestSubmitDirectSeriesIdentifierStillLowersAsListLiteral(t *testing.T) {
	src := `
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
	foundNodes := false
	foundTasks := false
	for _, p := range submitSet.Parameter {
		switch p.Name {
		case "nodes":
			foundNodes = true
			if p.Value != "[1,2]" {
				t.Fatalf("expected direct series submit assignment to lower as list literal, got %#v", p.Value)
			}
		case "tasks":
			foundTasks = true
			if p.Value != "[1,2]" {
				t.Fatalf("expected auto tasks from nodes list literal, got %#v", p.Value)
			}
		}
	}
	if !foundNodes || !foundTasks {
		t.Fatalf("expected nodes and tasks submit parameters, got %#v", submitSet.Parameter)
	}
}

func TestSubmitCollisionEscapesImportedVariablesAndRewritesRefs(t *testing.T) {
	src := `
param p {
  nodes = (1,2)
  a = ("x","y")
  a + nodes
}

submit run with p {
  nodes = "${nodes:-$nodes}"
  preprocess = {
    echo ${nodes} ${a}
    echo $nodes $a
    echo ${nodes:-x} ${nodes:=x} ${nodes:+x}
    echo ${nodes:-$nodes} ${nodes:+${nodes}}
    echo ${nodes%.*} ${nodes%%.*} ${nodes#n} ${nodes##n}
    echo ${nodes:1} ${nodes:1:2}
    echo ${#nodes} ${!nodes}
    echo \$nodes $$ $? $1 ${1} ${unknown:-x}
  }
  postprocess = {
    echo ${nodes:+done}
  }
  args_exec = "-lc 'echo ${nodes:-$nodes} ${nodes:+${nodes}} ${nodes:1:2} ${nodes#n} ${#nodes} ${a}'"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one submit step, got %d", len(doc.Step))
	}
	use := doc.Step[0].Use
	hasDirectP := false
	hasSubset := false
	for _, u := range use {
		s, ok := u.(string)
		if !ok {
			continue
		}
		if s == "p" {
			hasDirectP = true
		}
		if strings.HasPrefix(s, "_js__run__p__") {
			hasSubset = true
		}
	}
	if hasDirectP {
		t.Fatalf("did not expect direct use of p under submit-key collision: %#v", use)
	}
	if !hasSubset {
		t.Fatalf("expected subset use under submit-key collision: %#v", use)
	}

	var subset *lower.ParameterSet
	var submitSet *lower.ParameterSet
	for i := range doc.ParameterSet {
		ps := &doc.ParameterSet[i]
		if strings.HasPrefix(ps.Name, "_js__run__p__") {
			subset = ps
		}
		if strings.HasSuffix(ps.Name, "__submit_params") {
			submitSet = ps
		}
	}
	if subset == nil || submitSet == nil {
		t.Fatalf("expected both subset and submit parametersets, got %#v", doc.ParameterSet)
	}
	foundAliasedNodes := false
	foundPlainNodes := false
	for _, p := range subset.Parameter {
		if p.Name == "_ja__nodes" {
			foundAliasedNodes = true
		}
		if p.Name == "nodes" {
			foundPlainNodes = true
		}
	}
	if !foundAliasedNodes {
		t.Fatalf("expected aliased subset variable _ja__nodes, got %#v", subset.Parameter)
	}
	if foundPlainNodes {
		t.Fatalf("did not expect plain nodes in collision subset: %#v", subset.Parameter)
	}

	foundSubmitNodes := false
	foundPreprocess := false
	foundPostprocess := false
	foundArgsExec := false
	for _, p := range submitSet.Parameter {
		if p.Name == "nodes" {
			foundSubmitNodes = true
			if got, ok := p.Value.(string); !ok || got != "${_ja__nodes:-$_ja__nodes}" {
				t.Fatalf("expected submit nodes to reference aliased import, got %#v", p.Value)
			}
		}
		if p.Name == "preprocess" {
			foundPreprocess = true
			body, ok := p.Value.(lower.Literal)
			if !ok {
				t.Fatalf("expected preprocess literal payload, got %T", p.Value)
			}
			text := string(body)
			if !strings.Contains(text, "echo ${_ja__nodes} ${a}") {
				t.Fatalf("expected braced rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, "echo $_ja__nodes $a") {
				t.Fatalf("expected simple rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, `echo ${_ja__nodes:-x} ${_ja__nodes:=x} ${_ja__nodes:+x}`) {
				t.Fatalf("expected default/assign/alternate braced rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, `echo ${_ja__nodes:-$_ja__nodes} ${_ja__nodes:+${_ja__nodes}}`) {
				t.Fatalf("expected nested tail rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, `echo ${_ja__nodes%.*} ${_ja__nodes%%.*} ${_ja__nodes#n} ${_ja__nodes##n}`) {
				t.Fatalf("expected prefix/suffix braced rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, `echo ${_ja__nodes:1} ${_ja__nodes:1:2}`) {
				t.Fatalf("expected slice braced rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, `echo ${#_ja__nodes} ${!_ja__nodes}`) {
				t.Fatalf("expected length/indirect braced rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, `echo \$nodes $$ $? $1 ${1} ${unknown:-x}`) {
				t.Fatalf("expected special shell vars preserved, got %q", text)
			}
			if strings.Contains(text, "${nodes") {
				t.Fatalf("did not expect direct braced ${nodes...} after rewrite, got %q", text)
			}
		}
		if p.Name == "args_exec" {
			foundArgsExec = true
			if got, ok := p.Value.(string); !ok || got != "-lc 'echo ${_ja__nodes:-$_ja__nodes} ${_ja__nodes:+${_ja__nodes}} ${_ja__nodes:1:2} ${_ja__nodes#n} ${#_ja__nodes} ${a}'" {
				t.Fatalf("expected args_exec rewrite, got %#v", p.Value)
			}
		}
		if p.Name == "postprocess" {
			foundPostprocess = true
			body, ok := p.Value.(lower.Literal)
			if !ok {
				t.Fatalf("expected postprocess literal payload, got %T", p.Value)
			}
			text := string(body)
			if !strings.Contains(text, "echo ${_ja__nodes:+done}") {
				t.Fatalf("expected postprocess operator rewrite, got %q", text)
			}
			if strings.Contains(text, "${nodes") {
				t.Fatalf("did not expect direct braced ${nodes...} in postprocess after rewrite, got %q", text)
			}
		}
	}
	if !foundSubmitNodes || !foundPreprocess || !foundPostprocess || !foundArgsExec {
		t.Fatalf("missing expected submit parameters: %#v", submitSet.Parameter)
	}
}

func TestSubmitNoCollisionKeepsDirectFullUse(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

submit run with p {
  args_exec = "$a"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one step")
	}
	hasDirect := false
	hasSubset := false
	for _, u := range doc.Step[0].Use {
		s, ok := u.(string)
		if !ok {
			continue
		}
		if s == "p" {
			hasDirect = true
		}
		if strings.HasPrefix(s, "_js__run__p__") {
			hasSubset = true
		}
	}
	if !hasDirect {
		t.Fatalf("expected direct full parameterset use without collision: %#v", doc.Step[0].Use)
	}
	if hasSubset {
		t.Fatalf("did not expect synthetic subset without collision: %#v", doc.Step[0].Use)
	}
}

func TestSubmitUseHelpersAreEmittedAndRewritten(t *testing.T) {
	src := `
let submit_defaults {
  systemname = shell("hostname | tr -d '\n'")
  queue = python("'${systemname:-batch}'")
}

submit run
  use submit_defaults
{
  account = "project"
  executable = "/bin/bash"
  preprocess = {
    echo $systemname ${systemname%foo}
  }
  args_exec = "-lc 'echo ${systemname:-na} ${#systemname} ${queue}'"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var submitSet *lower.ParameterSet
	for i := range doc.ParameterSet {
		ps := &doc.ParameterSet[i]
		if strings.HasSuffix(ps.Name, "__submit_params") {
			submitSet = ps
			break
		}
	}
	if submitSet == nil {
		t.Fatalf("expected submit parameter set, got %#v", doc.ParameterSet)
	}
	foundHelper := false
	foundQueue := false
	foundArgsExec := false
	foundPreprocess := false
	for _, p := range submitSet.Parameter {
		if p.Name == "_jk__run_systemname" {
			foundHelper = true
			if p.Mode != "shell" {
				t.Fatalf("expected shell mode for helper, got %#v", p)
			}
		}
		if p.Name == "queue" {
			foundQueue = true
			got, ok := p.Value.(lower.SingleQuoted)
			if !ok || string(got) != "'${_jk__run_systemname:-batch}'" {
				t.Fatalf("expected queue helper rewrite, got %#v", p.Value)
			}
		}
		if p.Name == "preprocess" {
			foundPreprocess = true
			body, ok := p.Value.(lower.Literal)
			if !ok {
				t.Fatalf("expected preprocess literal payload, got %T", p.Value)
			}
			text := string(body)
			if !strings.Contains(text, "echo $_jk__run_systemname ${_jk__run_systemname%foo}") {
				t.Fatalf("expected preprocess helper rewrite, got %q", text)
			}
		}
		if p.Name == "args_exec" {
			foundArgsExec = true
			if got, ok := p.Value.(string); !ok || got != "-lc 'echo ${_jk__run_systemname:-na} ${#_jk__run_systemname} ${queue}'" {
				t.Fatalf("expected args_exec helper rewrite, got %#v", p.Value)
			}
		}
		if p.Name == "systemname" {
			t.Fatalf("did not expect unaliased helper name in submit parameters: %#v", submitSet.Parameter)
		}
	}
	if !foundHelper || !foundQueue || !foundArgsExec || !foundPreprocess {
		t.Fatalf("missing expected helper rewritten parameters: %#v", submitSet.Parameter)
	}
}

func TestSubmitUseHelperIdentifierExpressionIsEvaluated(t *testing.T) {
	src := `
let defaults {
  mynodes = 4
}

submit run
  use defaults
{
  nodes = mynodes
  args_exec = "-lc hostname"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var submitSet *lower.ParameterSet
	for i := range doc.ParameterSet {
		ps := &doc.ParameterSet[i]
		if strings.HasSuffix(ps.Name, "__submit_params") {
			submitSet = ps
			break
		}
	}
	if submitSet == nil {
		t.Fatalf("expected submit parameterset, got %#v", doc.ParameterSet)
	}
	foundHelper := false
	foundNodes := false
	for _, p := range submitSet.Parameter {
		if p.Name == "_jk__run_mynodes" {
			foundHelper = true
			if got, ok := p.Value.(string); !ok || got != "4" {
				t.Fatalf("expected helper value 4, got %#v", p.Value)
			}
		}
		if p.Name == "nodes" {
			foundNodes = true
			if got, ok := p.Value.(string); !ok || got != "4" {
				t.Fatalf("expected nodes value 4, got %#v", p.Value)
			}
		}
	}
	if !foundHelper || !foundNodes {
		t.Fatalf("missing helper/nodes params in submit set: %#v", submitSet.Parameter)
	}
}

func TestSubmitLetCollisionEscapesImportedVariables(t *testing.T) {
	src := `
let l {
  queue = "batch"
  host = shell("hostname | tr -d '\n'")
}

submit run with l {
  queue = "${queue:-batch}"
  preprocess = {
    echo ${queue} ${host}
    echo ${queue%dev} ${queue#b}
  }
  args_exec = "-lc 'echo ${queue:1:2} ${queue#b} ${#queue} ${host}'"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	var subset *lower.ParameterSet
	var submitSet *lower.ParameterSet
	for i := range doc.ParameterSet {
		ps := &doc.ParameterSet[i]
		if strings.HasPrefix(ps.Name, "_js__run__l__") {
			subset = ps
		}
		if strings.HasSuffix(ps.Name, "__submit_params") {
			submitSet = ps
		}
	}
	if subset == nil || submitSet == nil {
		t.Fatalf("expected subset and submit paramsets, got %#v", doc.ParameterSet)
	}
	hasAliasedQueue := false
	hasPlainQueue := false
	for _, p := range subset.Parameter {
		if p.Name == "_ja__queue" {
			hasAliasedQueue = true
		}
		if p.Name == "queue" {
			hasPlainQueue = true
		}
	}
	if !hasAliasedQueue || hasPlainQueue {
		t.Fatalf("unexpected queue naming in let subset: %#v", subset.Parameter)
	}
	for _, p := range submitSet.Parameter {
		if p.Name == "queue" {
			if got, ok := p.Value.(string); !ok || got != "${_ja__queue:-batch}" {
				t.Fatalf("expected queue rewrite to aliased variable, got %#v", p.Value)
			}
		}
		if p.Name == "preprocess" {
			body, ok := p.Value.(lower.Literal)
			if !ok {
				t.Fatalf("expected preprocess literal payload, got %T", p.Value)
			}
			text := string(body)
			if !strings.Contains(text, "echo ${_ja__queue} ${host}") {
				t.Fatalf("expected braced queue rewrite in preprocess, got %q", text)
			}
			if !strings.Contains(text, "echo ${_ja__queue%dev} ${_ja__queue#b}") {
				t.Fatalf("expected queue operator rewrite in preprocess, got %q", text)
			}
			if strings.Contains(text, "${queue") {
				t.Fatalf("did not expect direct braced ${queue...} after rewrite, got %q", text)
			}
		}
		if p.Name == "args_exec" {
			if got, ok := p.Value.(string); !ok || got != "-lc 'echo ${_ja__queue:1:2} ${_ja__queue#b} ${#_ja__queue} ${host}'" {
				t.Fatalf("expected args_exec queue operator rewrite, got %#v", p.Value)
			}
		}
	}
}

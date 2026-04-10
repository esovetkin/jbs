package lower_test

import (
	"strings"
	"testing"

	"jbs/internal/lower"
)

func TestLowerStepHeaderOptions(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

do prep
  with p
  max_async=0 iterations=2
{
  echo prep
}

submit run
  after prep
  with p
  max_async=3
{
  args_exec = "-lc hostname"
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected two steps, got %d", len(doc.Step))
	}
	if doc.Step[0].MaxAsync == nil || *doc.Step[0].MaxAsync != 0 {
		t.Fatalf("expected prep max_async=0, got %#v", doc.Step[0].MaxAsync)
	}
	if doc.Step[0].Iterations == nil || *doc.Step[0].Iterations != 2 {
		t.Fatalf("expected prep iterations=2, got %#v", doc.Step[0].Iterations)
	}
	if doc.Step[1].MaxAsync == nil || *doc.Step[1].MaxAsync != 3 {
		t.Fatalf("expected run max_async=3, got %#v", doc.Step[1].MaxAsync)
	}
	if doc.Step[1].Iterations != nil {
		t.Fatalf("expected run iterations to be omitted, got %#v", doc.Step[1].Iterations)
	}
}

func TestLowerStepHeaderOptionsOmittedByDefault(t *testing.T) {
	src := `
do run {
  echo hi
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one step")
	}
	if doc.Step[0].MaxAsync != nil || doc.Step[0].Iterations != nil {
		t.Fatalf("expected max_async/iterations omitted by default, got %#v", doc.Step[0])
	}
}

func TestLowerPreservesLargeIntegerLiteral(t *testing.T) {
	src := `
param p {
  x = 9007199254740993
  x
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.ParameterSet) != 1 {
		t.Fatalf("expected one parameterset, got %d", len(doc.ParameterSet))
	}
	ps := doc.ParameterSet[0]
	var valueText string
	found := false
	for _, p := range ps.Parameter {
		if p.Name != "x" {
			continue
		}
		found = true
		switch v := p.Value.(type) {
		case lower.SingleQuoted:
			valueText = string(v)
		case string:
			valueText = v
		default:
			t.Fatalf("unexpected parameter value type for x: %T", p.Value)
		}
		break
	}
	if !found {
		t.Fatalf("expected parameter 'x' in lowered parameterset")
	}
	if !strings.Contains(valueText, "9007199254740993") {
		t.Fatalf("expected exact integer literal in lowered value, got %q", valueText)
	}
	if strings.Contains(valueText, "9007199254740992") {
		t.Fatalf("unexpected rounded integer literal in lowered value: %q", valueText)
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

func TestQualifiedLikeShellReferenceDoesNotRaiseE100(t *testing.T) {
	src := `
let l {
  systemname = shell("hostname")
}
do run {
  echo ${l.systemname}
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("did not expect hard errors for shell-like qualified token, got: %s", diags.String())
	}
	if len(doc.Step) != 1 {
		t.Fatalf("expected one generated step, got %#v", doc.Step)
	}
	for _, d := range diags.Items {
		if d.Code == "E100" {
			t.Fatalf("did not expect E100 for shell text scanning, got: %s", diags.String())
		}
	}
}

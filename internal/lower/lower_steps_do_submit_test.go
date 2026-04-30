package lower

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func TestAddSubmitParameterSetWithoutSpec(t *testing.T) {
	ctx := &lowerContext{
		res:   &sema.Result{SubmitByName: map[string]*sema.SubmitSpec{}},
		names: map[string]struct{}{"run__submit_params": {}},
	}

	gotName := ctx.addSubmitParameterSet(ast.SubmitBlock{Name: "run"}, nil)
	if gotName != "run__submit_params_1" {
		t.Fatalf("expected unique submit params name with suffix, got %q", gotName)
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one emitted parameter set, got %d", len(ctx.doc.ParameterSet))
	}
	ps := ctx.doc.ParameterSet[0]
	if ps.Name != "run__submit_params_1" || ps.InitWith != "platform.xml:systemParameter" {
		t.Fatalf("unexpected submit parameterset header: %#v", ps)
	}
	if len(ps.Parameter) != 0 {
		t.Fatalf("expected empty parameter list without submit spec, got %#v", ps.Parameter)
	}
	if ps.Meta.Kind != ParameterSetKindSubmitInit || ps.Meta.Source != "run" {
		t.Fatalf("unexpected submit metadata: %#v", ps.Meta)
	}
}

func TestAddSubmitParameterSetModesHelpersAndAliasRewrite(t *testing.T) {
	ctx := &lowerContext{
		res: &sema.Result{SubmitByName: map[string]*sema.SubmitSpec{
			"run": {
				Name: "run",
				Values: []sema.SubmitValue{
					{Name: "preprocess", IsRaw: true, Raw: "echo $x"},
					{Name: "queue", Mode: "python", Value: eval.String("${x:-batch}")},
					{Name: "mail", Mode: "shell", Value: eval.String("echo ${x}")},
					{Name: "threadspertask", Value: eval.Int(8)},
					{Name: "measurement", Value: eval.List([]eval.Value{eval.String("$x"), eval.Int(1)})},
					{Name: "notification", Value: eval.String("$x")},
					{Name: "starter", Value: eval.Null()},
				},
				Helpers: []sema.SubmitHelper{
					{Original: "skip", Aliased: "", Value: eval.String("ignored")},
					{Original: "hpy", Aliased: "_jk__run_hpy", Mode: "python", Value: eval.String("${x}")},
					{Original: "hsh", Aliased: "_jk__run_hsh", Mode: "shell", Value: eval.String("$x")},
					{Original: "hnodes", Aliased: "nodes", Value: eval.Int(9)},
					{Original: "htuple", Aliased: "_jk__run_htuple", Value: eval.Tuple([]eval.Value{eval.String("$x")})},
					{Original: "hlist", Aliased: "_jk__run_hlist", Value: eval.List([]eval.Value{eval.String("$x"), eval.Int(2)})},
					{Original: "hnull", Aliased: "_jk__run_hnull", Value: eval.Null()},
					{Original: "hstr", Aliased: "_jk__run_hstr", Value: eval.String("$x")},
				},
			},
		}},
		names: map[string]struct{}{},
	}

	gotName := ctx.addSubmitParameterSet(ast.SubmitBlock{Name: "run"}, map[string]string{"x": "_ja__x"})
	if gotName != "run__submit_params" {
		t.Fatalf("unexpected submit params name: %q", gotName)
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one emitted submit parameterset, got %d", len(ctx.doc.ParameterSet))
	}

	params := map[string]Parameter{}
	for _, param := range ctx.doc.ParameterSet[0].Parameter {
		params[param.Name] = param
	}
	if got, ok := params["preprocess"].Value.(Literal); !ok || string(got) != "echo $_ja__x\n" {
		t.Fatalf("expected raw preprocess rewrite, got %#v", params["preprocess"].Value)
	}
	if params["preprocess"].Separator != "" {
		t.Fatalf("raw preprocess must not get separator, got %#v", params["preprocess"])
	}
	if got, ok := params["queue"].Value.(SingleQuoted); !ok || string(got) != "${_ja__x:-batch}" {
		t.Fatalf("expected python queue rewrite, got %#v", params["queue"].Value)
	}
	if params["queue"].Separator != ReservedSeparator {
		t.Fatalf("expected python queue separator, got %#v", params["queue"])
	}
	if params["mail"].Mode != "shell" || params["mail"].Value != "echo ${_ja__x}" || params["mail"].Separator != ReservedSeparator {
		t.Fatalf("expected shell mail rewrite, got %#v", params["mail"])
	}
	if params["threadspertask"].Type != "int" || params["threadspertask"].Value != "8" || params["threadspertask"].Separator != "" {
		t.Fatalf("expected typed int submit value, got %#v", params["threadspertask"])
	}
	if params["measurement"].Value != "[\"$_ja__x\",1]" || params["measurement"].Separator != "" {
		t.Fatalf("expected list python literal rewrite, got %#v", params["measurement"])
	}
	if params["notification"].Value != "$_ja__x" || params["notification"].Separator != ReservedSeparator {
		t.Fatalf("expected scalar fallback rewrite, got %#v", params["notification"])
	}
	if params["starter"].Value != "None" || params["starter"].Separator != "" {
		t.Fatalf("expected null python literal, got %#v", params["starter"])
	}
	if _, exists := params["skip"]; exists {
		t.Fatalf("did not expect helper with empty alias to be emitted")
	}
	if got, ok := params["_jk__run_hpy"].Value.(SingleQuoted); !ok || string(got) != "${_ja__x}" {
		t.Fatalf("expected python helper rewrite, got %#v", params["_jk__run_hpy"].Value)
	}
	if params["_jk__run_hpy"].Separator != ReservedSeparator {
		t.Fatalf("expected python helper separator, got %#v", params["_jk__run_hpy"])
	}
	if params["_jk__run_hsh"].Value != "$_ja__x" || params["_jk__run_hsh"].Separator != ReservedSeparator {
		t.Fatalf("expected shell helper rewrite, got %#v", params["_jk__run_hsh"])
	}
	if params["nodes"].Type != "" || params["nodes"].Value != "9" || params["nodes"].Separator != "" {
		t.Fatalf("expected helper alias nodes without type inference, got %#v", params["nodes"])
	}
	if params["_jk__run_htuple"].Value != "(\"$_ja__x\",)" || params["_jk__run_htuple"].Separator != "" {
		t.Fatalf("expected tuple helper rewrite, got %#v", params["_jk__run_htuple"])
	}
	if params["_jk__run_hlist"].Value != "[\"$_ja__x\",2]" || params["_jk__run_hlist"].Separator != "" {
		t.Fatalf("expected list helper rewrite, got %#v", params["_jk__run_hlist"])
	}
	if params["_jk__run_hnull"].Value != "None" || params["_jk__run_hnull"].Separator != "" {
		t.Fatalf("expected null helper rewrite, got %#v", params["_jk__run_hnull"])
	}
	if params["_jk__run_hstr"].Value != "$_ja__x" || params["_jk__run_hstr"].Separator != ReservedSeparator {
		t.Fatalf("expected scalar helper rewrite, got %#v", params["_jk__run_hstr"])
	}
}

func TestBuildSubmitParameterStringSeparatorCollision(t *testing.T) {
	param := buildSubmitParameter("label", "", eval.String("a####b,c"), nil, false)
	if param.Value != "a####b,c" || param.Separator != "#####" {
		t.Fatalf("expected collision-free submit separator, got %#v", param)
	}
}

func TestLowerDoBuildsStepAndTracksSourceRows(t *testing.T) {
	maxAsync := 2
	procs := 3
	iterations := 4
	ctx := &lowerContext{
		res: &sema.Result{StepScopeByName: map[string]*sema.StepScopePlan{
			"run": {
				InheritedSteps: []string{"prep"},
				Inherited: map[string]sema.VisibleBinding{
					"b": {},
					"a": {},
				},
			},
		}},
		stepSourceRows: map[string]map[sourceRowKey]sourceRowContext{},
	}
	block := ast.DoBlock{
		Name:       "run",
		After:      []string{"prep"},
		MaxAsync:   &maxAsync,
		Procs:      &procs,
		Iterations: &iterations,
		Body:       "echo hi\n",
	}

	got := ctx.lowerDo(block)
	if got.Name != "run" || got.Depend != "prep" {
		t.Fatalf("unexpected lowered do identity: %#v", got)
	}
	if got.MaxAsync == nil || *got.MaxAsync != 2 || got.Procs == nil || *got.Procs != 3 || got.Iterations == nil || *got.Iterations != 4 {
		t.Fatalf("unexpected lowered do options: %#v", got)
	}
	if got.Meta.Kind != StepKindDo || got.Meta.Source != "run" {
		t.Fatalf("unexpected do metadata: %#v", got.Meta)
	}
	if !reflect.DeepEqual(got.Meta.InheritsFrom, []string{"prep"}) {
		t.Fatalf("unexpected inherited steps: %#v", got.Meta.InheritsFrom)
	}
	if !reflect.DeepEqual(got.Meta.InheritedVars, []string{"a", "b"}) {
		t.Fatalf("expected sorted inherited vars, got %#v", got.Meta.InheritedVars)
	}
	if len(got.Use) != 0 {
		t.Fatalf("expected no explicit use entries for empty import delta, got %#v", got.Use)
	}
	if len(got.Do) != 1 {
		t.Fatalf("expected one do operation, got %#v", got.Do)
	}
	if lit, ok := got.Do[0].(Literal); !ok || string(lit) != "echo hi\n" {
		t.Fatalf("unexpected lowered do literal: %#v", got.Do[0])
	}
	if rows, ok := ctx.stepSourceRows["run"]; !ok || rows == nil {
		t.Fatalf("expected do step source-row tracking, got %#v", ctx.stepSourceRows)
	}
}

func TestLowerSubmitBuildsStepUseAndOperations(t *testing.T) {
	maxAsync := 1
	procs := 2
	iterations := 3
	ctx := &lowerContext{
		res: &sema.Result{StepScopeByName: map[string]*sema.StepScopePlan{
			"run": {
				InheritedSteps: []string{"prep"},
				Inherited: map[string]sema.VisibleBinding{
					"queue": {},
				},
			},
		}},
		stepSourceRows: map[string]map[sourceRowKey]sourceRowContext{},
	}
	block := ast.SubmitBlock{
		Name:       "run",
		After:      []string{"prep"},
		MaxAsync:   &maxAsync,
		Procs:      &procs,
		Iterations: &iterations,
	}

	got := ctx.lowerSubmit(block, "run__submit_params", map[string]string{})
	if got.Name != "run" || got.Depend != "prep" {
		t.Fatalf("unexpected lowered submit identity: %#v", got)
	}
	if got.MaxAsync == nil || *got.MaxAsync != 1 || got.Procs == nil || *got.Procs != 2 || got.Iterations == nil || *got.Iterations != 3 {
		t.Fatalf("unexpected lowered submit options: %#v", got)
	}
	if got.Meta.Kind != StepKindSubmit || got.Meta.Source != "run" {
		t.Fatalf("unexpected submit metadata: %#v", got.Meta)
	}
	if !reflect.DeepEqual(got.Meta.InheritsFrom, []string{"prep"}) || !reflect.DeepEqual(got.Meta.InheritedVars, []string{"queue"}) {
		t.Fatalf("unexpected inherited submit metadata: %#v", got.Meta)
	}
	if len(got.Use) != 4 {
		t.Fatalf("expected submit set plus platform entries, got %#v", got.Use)
	}
	if setName, ok := got.Use[0].(string); !ok || setName != "run__submit_params" {
		t.Fatalf("unexpected submit use[0]: %#v", got.Use[0])
	}
	if ue, ok := got.Use[1].(UseEntry); !ok || ue.From != "platform.xml" || ue.Value != "jobfiles" {
		t.Fatalf("unexpected submit use[1]: %#v", got.Use[1])
	}
	if ue, ok := got.Use[2].(UseEntry); !ok || ue.Value != "executesub" {
		t.Fatalf("unexpected submit use[2]: %#v", got.Use[2])
	}
	if ue, ok := got.Use[3].(UseEntry); !ok || ue.Value != "executeset" {
		t.Fatalf("unexpected submit use[3]: %#v", got.Use[3])
	}
	if len(got.Do) != 2 {
		t.Fatalf("expected two submit operations, got %#v", got.Do)
	}
	op, ok := got.Do[0].(SubmitOperation)
	if !ok {
		t.Fatalf("expected first submit operation to be SubmitOperation, got %#v", got.Do[0])
	}
	if op.DoneFile != "$done_file" || op.ErrorFile != "$error_file" || op.Command != `${submit} --parsable ${submit_script} > run.jobid` {
		t.Fatalf("unexpected submit operation payload: %#v", op)
	}
	if cmd, ok := got.Do[1].(string); !ok || cmd != `echo "true" > success` {
		t.Fatalf("unexpected second submit operation: %#v", got.Do[1])
	}
	if rows, ok := ctx.stepSourceRows["run"]; !ok || rows == nil {
		t.Fatalf("expected submit source-row tracking, got %#v", ctx.stepSourceRows)
	}
}

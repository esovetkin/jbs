package lower

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
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
	if ps.Name != "run__submit_params_1" {
		t.Fatalf("unexpected parameterset name: %q", ps.Name)
	}
	if ps.InitWith != "platform.xml:systemParameter" {
		t.Fatalf("unexpected init_with: %q", ps.InitWith)
	}
	if len(ps.Parameter) != 0 {
		t.Fatalf("expected empty parameter list without submit spec, got %#v", ps.Parameter)
	}
	if ps.Meta.Kind != ParameterSetKindSubmitInit || ps.Meta.Source != "run" {
		t.Fatalf("unexpected submit meta: %#v", ps.Meta)
	}
}

func TestAddSubmitParameterSetModesHelpersAndAliasRewrite(t *testing.T) {
	ctx := &lowerContext{
		res: &sema.Result{
			SubmitByName: map[string]*sema.SubmitSpec{
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
			},
		},
		names: map[string]struct{}{},
	}

	gotName := ctx.addSubmitParameterSet(ast.SubmitBlock{Name: "run"}, map[string]string{"x": "_ja__x"})
	if gotName != "run__submit_params" {
		t.Fatalf("unexpected submit params name: %q", gotName)
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one emitted submit parameterset, got %d", len(ctx.doc.ParameterSet))
	}

	ps := ctx.doc.ParameterSet[0]
	params := map[string]Parameter{}
	for _, p := range ps.Parameter {
		params[p.Name] = p
	}

	preprocess, ok := params["preprocess"]
	if !ok {
		t.Fatalf("missing preprocess parameter in %#v", ps.Parameter)
	}
	if preprocess.Mode != "text" {
		t.Fatalf("expected preprocess mode text, got %q", preprocess.Mode)
	}
	if got, ok := preprocess.Value.(Literal); !ok || string(got) != "echo $_ja__x\n" {
		t.Fatalf("expected raw preprocess rewrite to literal, got %#v (%T)", preprocess.Value, preprocess.Value)
	}

	queue, ok := params["queue"]
	if !ok {
		t.Fatalf("missing queue parameter")
	}
	if queue.Mode != "python" {
		t.Fatalf("expected queue mode python, got %q", queue.Mode)
	}
	if got, ok := queue.Value.(SingleQuoted); !ok || string(got) != "${_ja__x:-batch}" {
		t.Fatalf("expected python single-quoted queue rewrite, got %#v (%T)", queue.Value, queue.Value)
	}

	mail, ok := params["mail"]
	if !ok {
		t.Fatalf("missing mail parameter")
	}
	if mail.Mode != "shell" {
		t.Fatalf("expected mail mode shell, got %q", mail.Mode)
	}
	if got, ok := mail.Value.(string); !ok || got != "echo ${_ja__x}" {
		t.Fatalf("expected shell string rewrite for mail, got %#v (%T)", mail.Value, mail.Value)
	}

	threads, ok := params["threadspertask"]
	if !ok {
		t.Fatalf("missing threadspertask parameter")
	}
	if threads.Type != "int" {
		t.Fatalf("expected int type for threadspertask, got %q", threads.Type)
	}
	if got, ok := threads.Value.(string); !ok || got != "8" {
		t.Fatalf("expected threadspertask value 8, got %#v (%T)", threads.Value, threads.Value)
	}

	measurement, ok := params["measurement"]
	if !ok {
		t.Fatalf("missing measurement parameter")
	}
	if got, ok := measurement.Value.(string); !ok || got != "[\"$_ja__x\",1]" {
		t.Fatalf("expected list python literal rewrite, got %#v (%T)", measurement.Value, measurement.Value)
	}

	notification, ok := params["notification"]
	if !ok {
		t.Fatalf("missing notification parameter")
	}
	if notification.Mode != "" {
		t.Fatalf("expected no explicit mode for notification, got %q", notification.Mode)
	}
	if got, ok := notification.Value.(string); !ok || got != "$_ja__x" {
		t.Fatalf("expected scalar fallback rewrite for notification, got %#v (%T)", notification.Value, notification.Value)
	}

	starter, ok := params["starter"]
	if !ok {
		t.Fatalf("missing starter parameter")
	}
	if got, ok := starter.Value.(string); !ok || got != "None" {
		t.Fatalf("expected null python literal for starter, got %#v (%T)", starter.Value, starter.Value)
	}

	if _, exists := params["skip"]; exists {
		t.Fatalf("did not expect helper with empty alias to be emitted")
	}

	hpy, ok := params["_jk__run_hpy"]
	if !ok {
		t.Fatalf("missing python helper parameter")
	}
	if hpy.Mode != "python" {
		t.Fatalf("expected python mode for helper, got %q", hpy.Mode)
	}
	if got, ok := hpy.Value.(SingleQuoted); !ok || string(got) != "${_ja__x}" {
		t.Fatalf("expected python helper rewrite, got %#v (%T)", hpy.Value, hpy.Value)
	}

	hsh, ok := params["_jk__run_hsh"]
	if !ok {
		t.Fatalf("missing shell helper parameter")
	}
	if hsh.Mode != "shell" {
		t.Fatalf("expected shell mode for helper, got %q", hsh.Mode)
	}
	if got, ok := hsh.Value.(string); !ok || got != "$_ja__x" {
		t.Fatalf("expected shell helper rewrite, got %#v (%T)", hsh.Value, hsh.Value)
	}

	hnodes, ok := params["nodes"]
	if !ok {
		t.Fatalf("missing helper nodes parameter")
	}
	if hnodes.Type != "" {
		t.Fatalf("did not expect submit-key type inference for helper alias nodes, got %q", hnodes.Type)
	}
	if got, ok := hnodes.Value.(string); !ok || got != "9" {
		t.Fatalf("expected helper nodes scalar template value, got %#v (%T)", hnodes.Value, hnodes.Value)
	}

	htuple, ok := params["_jk__run_htuple"]
	if !ok {
		t.Fatalf("missing tuple helper parameter")
	}
	if got, ok := htuple.Value.(string); !ok || got != "(\"$_ja__x\",)" {
		t.Fatalf("expected tuple helper python literal rewrite, got %#v (%T)", htuple.Value, htuple.Value)
	}

	hlist, ok := params["_jk__run_hlist"]
	if !ok {
		t.Fatalf("missing list helper parameter")
	}
	if got, ok := hlist.Value.(string); !ok || got != "[\"$_ja__x\",2]" {
		t.Fatalf("expected list helper python literal rewrite, got %#v (%T)", hlist.Value, hlist.Value)
	}

	hnull, ok := params["_jk__run_hnull"]
	if !ok {
		t.Fatalf("missing null helper parameter")
	}
	if got, ok := hnull.Value.(string); !ok || got != "None" {
		t.Fatalf("expected null helper python literal rewrite, got %#v (%T)", hnull.Value, hnull.Value)
	}

	hstr, ok := params["_jk__run_hstr"]
	if !ok {
		t.Fatalf("missing scalar helper parameter")
	}
	if got, ok := hstr.Value.(string); !ok || got != "$_ja__x" {
		t.Fatalf("expected scalar helper template rewrite, got %#v (%T)", hstr.Value, hstr.Value)
	}
}

func TestLowerDoBuildsStepAndTracksSourceRows(t *testing.T) {
	maxAsync := 2
	procs := 3
	iterations := 4
	ctx := &lowerContext{
		res: &sema.Result{
			StepImportByName: map[string]*sema.StepImportPlan{
				"run": {
					InheritedSteps: []string{"prep"},
					Inherited: map[string]sema.VarOrigin{
						"a": {},
						"b": {},
					},
					ExplicitDelta: nil,
					Effective:     map[string]sema.VarOrigin{},
				},
			},
			ImportSourceByName: map[string]*sema.ImportSource{},
		},
		diags:          &diag.Diagnostics{},
		stepSourceRows: map[string]map[string]string{},
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
		t.Fatalf("unexpected lowered do step identity: %#v", got)
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
		t.Fatalf("expected sorted inherited vars [a b], got %#v", got.Meta.InheritedVars)
	}
	if len(got.Use) != 0 {
		t.Fatalf("expected no explicit use entries for empty import plan delta, got %#v", got.Use)
	}
	if len(got.Do) != 1 {
		t.Fatalf("expected one do operation, got %#v", got.Do)
	}
	if lit, ok := got.Do[0].(Literal); !ok || string(lit) != "echo hi\n" {
		t.Fatalf("unexpected lowered do literal: %#v", got.Do[0])
	}
	if rows, ok := ctx.stepSourceRows["run"]; !ok || rows == nil {
		t.Fatalf("expected step source-row tracking for do step, got %#v", ctx.stepSourceRows)
	}
}

func TestLowerSubmitBuildsStepUseAndOperations(t *testing.T) {
	maxAsync := 1
	procs := 2
	iterations := 3
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	ctx := &lowerContext{
		res: &sema.Result{
			StepImportByName: map[string]*sema.StepImportPlan{
				"run": {
					InheritedSteps: []string{"prep"},
					Inherited: map[string]sema.VarOrigin{
						"queue": {},
					},
					ExplicitDelta: nil,
					Effective:     map[string]sema.VarOrigin{},
				},
			},
			ImportSourceByName: map[string]*sema.ImportSource{},
			DoBlocks:           []ast.DoBlock{{Name: "prep", Span: span}},
			Submits:            []ast.SubmitBlock{{Name: "run", Span: span}},
		},
		diags:          &diag.Diagnostics{},
		stepSourceRows: map[string]map[string]string{},
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
	if !reflect.DeepEqual(got.Meta.InheritsFrom, []string{"prep"}) {
		t.Fatalf("unexpected submit inherited steps: %#v", got.Meta.InheritsFrom)
	}
	if !reflect.DeepEqual(got.Meta.InheritedVars, []string{"queue"}) {
		t.Fatalf("unexpected submit inherited vars: %#v", got.Meta.InheritedVars)
	}

	if len(got.Use) != 4 {
		t.Fatalf("expected submit use entries (submit set + platform entries), got %#v", got.Use)
	}
	if setName, ok := got.Use[0].(string); !ok || setName != "run__submit_params" {
		t.Fatalf("unexpected submit use[0]: %#v", got.Use[0])
	}
	if ue, ok := got.Use[1].(UseEntry); !ok || ue.From != "platform.xml" || ue.Value != "jobfiles" {
		t.Fatalf("unexpected submit use[1]: %#v", got.Use[1])
	}
	if ue, ok := got.Use[2].(UseEntry); !ok || ue.From != "platform.xml" || ue.Value != "executesub" {
		t.Fatalf("unexpected submit use[2]: %#v", got.Use[2])
	}
	if ue, ok := got.Use[3].(UseEntry); !ok || ue.From != "platform.xml" || ue.Value != "executeset" {
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

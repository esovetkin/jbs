package lower

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func countLowerDiag(diags *diag.Diagnostics, code diag.Code) int {
	count := 0
	for _, item := range diags.Items {
		if item.Code == string(code) {
			count++
		}
	}
	return count
}

func TestEnsureSourceParameterSet(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	ctx := &lowerContext{
		res: &sema.Result{BindingsByName: map[string]*sema.GlobalBinding{
			"table": {
				Name:    "table",
				Shape:   sema.BindingTable,
				Order:   []string{"a"},
				Rows:    []eval.Row{{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}}}},
				Vars:    map[string][]eval.Value{"a": {eval.Int(1)}},
				Origins: map[string]diag.Span{"a": span},
				Span:    span,
			},
			"scalar": {
				Name:  "scalar",
				Shape: sema.BindingScalar,
				Order: []string{"x"},
				Vars:  map[string][]eval.Value{"x": {eval.Int(1)}},
				Span:  span,
			},
		}},
		diags:                     &diag.Diagnostics{},
		names:                     map[string]struct{}{},
		sourceParameterSetEmitted: map[string]struct{}{},
	}

	if ctx.ensureSourceParameterSet("") {
		t.Fatalf("expected empty source to be rejected")
	}
	if ctx.ensureSourceParameterSet("missing") {
		t.Fatalf("expected missing source to be rejected")
	}
	if ctx.ensureSourceParameterSet("scalar") {
		t.Fatalf("expected scalar binding to be rejected")
	}
	if !ctx.ensureSourceParameterSet("table") {
		t.Fatalf("expected table binding to be emitted")
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one emitted parameter set, got %#v", ctx.doc.ParameterSet)
	}
	if _, ok := ctx.names["table"]; !ok {
		t.Fatalf("expected emitted source name to be reserved")
	}
	if _, ok := ctx.sourceParameterSetEmitted["table"]; !ok {
		t.Fatalf("expected emitted source marker to be set")
	}
	if !ctx.ensureSourceParameterSet("table") {
		t.Fatalf("expected already-emitted source to return true")
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected source parameter set to be emitted once, got %#v", ctx.doc.ParameterSet)
	}
}

func TestLowerGlobalBindingZeroRowsEmitsE230(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	binding := &sema.GlobalBinding{
		Name:  "p",
		Shape: sema.BindingTable,
		Order: []string{"a"},
		Vars: map[string][]eval.Value{
			"a": {},
		},
		Origins: map[string]diag.Span{"a": span},
		Span:    span,
	}
	diags := &diag.Diagnostics{}
	got := lowerGlobalBinding(binding, diags)

	if countLowerDiag(diags, diag.CodeE230) != 1 {
		t.Fatalf("expected one E230, got %d: %s", countLowerDiag(diags, diag.CodeE230), diags.String())
	}
	if got.Name != "p" || got.Meta.Kind != ParameterSetKindGlobalTable || got.Meta.Source != "p" {
		t.Fatalf("unexpected lowered binding metadata: %#v", got)
	}
	if len(got.Parameter) != 2 {
		t.Fatalf("expected index and payload parameters, got %#v", got.Parameter)
	}
	if got.Parameter[0].Name != "_ji_p" || got.Parameter[0].Type != "int" || got.Parameter[0].Mode != "text" || got.Parameter[0].Value != "0" {
		t.Fatalf("unexpected index parameter: %#v", got.Parameter[0])
	}
	payload, ok := got.Parameter[1].Value.(SingleQuoted)
	if !ok || !strings.Contains(string(payload), "None") {
		t.Fatalf("expected null fallback python payload, got %#v", got.Parameter[1].Value)
	}
}

func TestLowerIndexedParametersAndPayloadModes(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	origin := func(string) diag.Span { return span }
	values := map[string][]eval.Value{
		"py":    {eval.String("same"), eval.String("same")},
		"shell": {eval.String("a"), eval.String("b")},
		"x":     {eval.Int(1), eval.Int(2)},
		"vary":  {eval.String("a,b"), eval.String("c,d")},
	}
	modes := map[string]string{"py": "python", "shell": "shell"}
	diags := &diag.Diagnostics{}

	params := lowerIndexedParameters([]string{"py", "shell", "x", "vary"}, values, modes, []int{0, 1}, "", origin, diags)
	if len(params) != 5 {
		t.Fatalf("expected index and three payload params, got %#v", params)
	}
	if params[0].Name != "_ji_set" || params[0].Value != "0,1" {
		t.Fatalf("expected default index parameter, got %#v", params[0])
	}
	if value, ok := params[1].Value.(SingleQuoted); !ok || string(value) != "same" {
		t.Fatalf("expected constant python-mode value, got %#v", params[1].Value)
	}
	if params[1].Separator != ReservedSeparator {
		t.Fatalf("expected constant python-mode string separator, got %#v", params[1])
	}
	if params[2].Mode != "shell" || params[2].Value != "a" || params[2].Separator != ReservedSeparator {
		t.Fatalf("expected shell mode to use first selected value, got %#v", params[2])
	}
	if value, ok := params[3].Value.(SingleQuoted); !ok || string(value) != "[1,2][$_ji_set]" {
		t.Fatalf("expected default python index expression, got %#v", params[3].Value)
	}
	if params[3].Separator != "" {
		t.Fatalf("did not expect separator for integer index expression, got %#v", params[3])
	}
	if value, ok := params[4].Value.(SingleQuoted); !ok || string(value) != `["a,b","c,d"][$_ji_set]` {
		t.Fatalf("expected varying string index expression, got %#v", params[4].Value)
	}
	if params[4].Separator != "" {
		t.Fatalf("did not expect separator for varying string index expression, got %#v", params[4])
	}
	if countLowerDiag(diags, diag.CodeE231) != 1 {
		t.Fatalf("expected one E231 for varying shell values, got %d: %s", countLowerDiag(diags, diag.CodeE231), diags.String())
	}

	fallback := lowerIndexedPayloadParameters(
		[]string{"empty", "py_fallback"},
		map[string][]eval.Value{
			"empty":       nil,
			"py_fallback": {eval.String("alpha"), eval.String("beta")},
		},
		map[string]string{"py_fallback": "python"},
		nil,
		"$idx",
		origin,
		&diag.Diagnostics{},
	)
	if len(fallback) != 2 {
		t.Fatalf("expected fallback payload params, got %#v", fallback)
	}
	if value, ok := fallback[0].Value.(SingleQuoted); !ok || string(value) != "[None][$idx]" {
		t.Fatalf("expected empty value set to lower as null index expr, got %#v", fallback[0].Value)
	}
	if value, ok := fallback[1].Value.(SingleQuoted); !ok || string(value) != "alpha" {
		t.Fatalf("expected out-of-range selected values to fall back to first full value, got %#v", fallback[1].Value)
	}
	if fallback[1].Separator != ReservedSeparator {
		t.Fatalf("expected fallback direct string separator, got %#v", fallback[1])
	}
}

func TestLowerContextualPayloadParameters(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	origin := func(string) diag.Span { return span }
	values := map[string][]eval.Value{
		"py":    {eval.String("same"), eval.String("same")},
		"shell": {eval.String("x"), eval.String("y")},
		"n":     {eval.Int(1), eval.Int(2)},
		"empty": nil,
		"vary":  {eval.String("a,b"), eval.String("c,d")},
	}
	modes := map[string]string{"py": "python", "shell": "shell"}
	diags := &diag.Diagnostics{}

	params := lowerContextualPayloadParameters([]string{"py", "shell", "n", "empty", "vary"}, values, modes, "$idx", origin, diags)
	if len(params) != 5 {
		t.Fatalf("expected four contextual payload params, got %#v", params)
	}
	if value, ok := params[0].Value.(SingleQuoted); !ok || string(value) != "same" {
		t.Fatalf("expected constant contextual python value, got %#v", params[0].Value)
	}
	if params[0].Separator != ReservedSeparator {
		t.Fatalf("expected constant contextual python separator, got %#v", params[0])
	}
	if params[1].Mode != "shell" || params[1].Value != "x" || params[1].Separator != ReservedSeparator {
		t.Fatalf("expected shell mode to use first contextual value, got %#v", params[1])
	}
	if value, ok := params[2].Value.(SingleQuoted); !ok || string(value) != "[1,2][$idx]" {
		t.Fatalf("expected contextual python index expr, got %#v", params[2].Value)
	}
	if value, ok := params[3].Value.(SingleQuoted); !ok || string(value) != "[None][$idx]" {
		t.Fatalf("expected empty contextual values to lower as null index expr, got %#v", params[3].Value)
	}
	if value, ok := params[4].Value.(SingleQuoted); !ok || string(value) != `["a,b","c,d"][$idx]` {
		t.Fatalf("expected varying contextual string index expr, got %#v", params[4].Value)
	}
	for _, param := range []Parameter{params[2], params[3], params[4]} {
		if param.Separator != "" {
			t.Fatalf("did not expect separator for index expression payload, got %#v", param)
		}
	}
	if countLowerDiag(diags, diag.CodeE231) != 1 {
		t.Fatalf("expected one E231 for varying shell contextual values, got %d: %s", countLowerDiag(diags, diag.CodeE231), diags.String())
	}
}

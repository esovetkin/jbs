package lower

import (
	"reflect"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func countCode(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, d := range diags.Items {
		if d.Code == code {
			count++
		}
	}
	return count
}

func TestLowerParamsetZeroRowsEmitsE230(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	ps := &sema.Paramset{
		Name: "p",
		Block: ast.ParamBlock{
			Span: span,
		},
		Order: []string{"a"},
		Vars: map[string][]eval.Value{
			"a": {},
		},
		Origins: map[string]diag.Span{
			"a": span,
		},
		Modes: map[string]string{},
		Rows:  nil,
	}
	diags := &diag.Diagnostics{}
	got := lowerParamset(ps, diags)

	if countCode(diags, "E230") != 1 {
		t.Fatalf("expected one E230, got %d: %s", countCode(diags, "E230"), diags.String())
	}
	if got.Name != "p" || got.Meta.Kind != ParameterSetKindParam || got.Meta.Source != "p" {
		t.Fatalf("unexpected lowered paramset metadata: %#v", got)
	}
	if len(got.Parameter) != 2 {
		t.Fatalf("expected index + payload parameter, got %#v", got.Parameter)
	}
	if got.Parameter[0].Name != "_ji_p" || got.Parameter[0].Type != "int" || got.Parameter[0].Mode != "text" || got.Parameter[0].Value != "0" {
		t.Fatalf("unexpected index parameter: %#v", got.Parameter[0])
	}
	val, ok := got.Parameter[1].Value.(SingleQuoted)
	if !ok {
		t.Fatalf("expected python payload as SingleQuoted, got %T", got.Parameter[1].Value)
	}
	if !strings.Contains(string(val), "None") {
		t.Fatalf("expected null fallback value in python index expression, got %q", string(val))
	}
}

func TestLowerIndexedParametersAndPayloadModes(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	origin := func(_ string) diag.Span { return span }

	values := map[string][]eval.Value{
		"p": {eval.String("same"), eval.String("same")},
		"s": {eval.String("a"), eval.String("b")},
		"x": {eval.Int(1), eval.Int(2)},
	}
	modes := map[string]string{
		"p": "python",
		"s": "shell",
	}
	diags := &diag.Diagnostics{}
	params := lowerIndexedParameters([]string{"p", "s", "x"}, values, modes, []int{0, 1}, "", origin, diags)
	if len(params) != 4 {
		t.Fatalf("expected index + 3 payload params, got %#v", params)
	}
	if params[0].Name != "_ji_set" || params[0].Value != "0,1" {
		t.Fatalf("expected default index name and explicit index rows, got %#v", params[0])
	}
	if params[1].Name != "p" || params[1].Mode != "python" {
		t.Fatalf("unexpected python mode parameter: %#v", params[1])
	}
	if sq, ok := params[1].Value.(SingleQuoted); !ok || string(sq) != "same" {
		t.Fatalf("expected constant python mode value, got %#v", params[1].Value)
	}
	if params[2].Name != "s" || params[2].Mode != "shell" || params[2].Value != "a" {
		t.Fatalf("unexpected shell mode parameter: %#v", params[2])
	}
	if params[3].Name != "x" || params[3].Mode != "python" {
		t.Fatalf("unexpected default mode parameter: %#v", params[3])
	}
	if countCode(diags, "E231") != 1 {
		t.Fatalf("expected E231 for varying shell values, got %d: %s", countCode(diags, "E231"), diags.String())
	}
}

func TestLowerContextualPayloadParameters(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	origin := func(_ string) diag.Span { return span }

	values := map[string][]eval.Value{
		"p": {eval.String("same"), eval.String("same")},
		"s": {eval.String("x"), eval.String("y")},
		"n": {eval.Int(1), eval.Int(2)},
	}
	modes := map[string]string{
		"p": "python",
		"s": "shell",
	}
	diags := &diag.Diagnostics{}
	params := lowerContextualPayloadParameters([]string{"p", "s", "n"}, values, modes, "$idx", origin, diags)
	if len(params) != 3 {
		t.Fatalf("expected 3 contextual payload params, got %#v", params)
	}
	if sq, ok := params[0].Value.(SingleQuoted); !ok || string(sq) != "same" {
		t.Fatalf("expected constant python mode value, got %#v", params[0].Value)
	}
	if params[1].Mode != "shell" || params[1].Value != "x" {
		t.Fatalf("expected shell mode first value selection, got %#v", params[1])
	}
	if params[2].Mode != "python" {
		t.Fatalf("expected default python mode, got %#v", params[2])
	}
	if sq, ok := params[2].Value.(SingleQuoted); !ok || string(sq) != "[1,2][$idx]" {
		t.Fatalf("unexpected contextual python index expression: %#v", params[2].Value)
	}
	if countCode(diags, "E231") != 1 {
		t.Fatalf("expected one E231 for varying shell contextual values, got %d: %s", countCode(diags, "E231"), diags.String())
	}
}

func TestEnsureSourceParamset(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	ps := &sema.Paramset{
		Name: "p",
		Block: ast.ParamBlock{
			Name: "p",
			Span: span,
		},
		Order: []string{"a"},
		Rows: []eval.Row{
			{
				Values: map[string]eval.Cell{
					"a": {Value: eval.Int(1)},
				},
			},
		},
		Vars:    map[string][]eval.Value{"a": {eval.Int(1)}},
		Origins: map[string]diag.Span{"a": span},
		Modes:   map[string]string{},
	}
	ctx := &lowerContext{
		res: &sema.Result{
			ParamByName: map[string]*sema.Paramset{
				"p": ps,
			},
		},
		diags:                 &diag.Diagnostics{},
		doc:                   Document{},
		names:                 map[string]struct{}{},
		sourceParamsetEmitted: map[string]struct{}{},
	}

	if got := ctx.ensureSourceParamset(""); got {
		t.Fatalf("expected empty source to be rejected")
	}
	if got := ctx.ensureSourceParamset("missing"); got {
		t.Fatalf("expected missing source to be rejected")
	}
	if got := ctx.ensureSourceParamset("p"); !got {
		t.Fatalf("expected source paramset p to be emitted")
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one emitted source paramset, got %#v", ctx.doc.ParameterSet)
	}
	if _, ok := ctx.names["p"]; !ok {
		t.Fatalf("expected emitted source name to be reserved")
	}
	if _, ok := ctx.sourceParamsetEmitted["p"]; !ok {
		t.Fatalf("expected emitted source marker to be set")
	}
	// Re-emission should be skipped while still reporting success.
	if got := ctx.ensureSourceParamset("p"); !got {
		t.Fatalf("expected already-emitted source to return true")
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected source paramset to be emitted only once, got %#v", ctx.doc.ParameterSet)
	}
}

func TestValuesForAndOriginFor(t *testing.T) {
	blockSpan := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(2, 1, 3))
	nameSpan := diag.NewSpan("in.jbs", diag.NewPos(5, 2, 1), diag.NewPos(6, 2, 2))
	ps := &sema.Paramset{
		Block: ast.ParamBlock{Span: blockSpan},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(10)}}},
			{Values: map[string]eval.Cell{}}, // missing a => forces fallback
		},
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1), eval.Int(2), eval.Int(3)},
			"b": {},
		},
		Origins: map[string]diag.Span{
			"a": nameSpan,
		},
	}

	gotA := valuesFor(ps, "a", 4)
	wantA := []eval.Value{eval.Int(1), eval.Int(2), eval.Int(3), eval.Int(1)}
	if !reflect.DeepEqual(gotA, wantA) {
		t.Fatalf("expected fallback cyclic values for partial row coverage, got=%#v want=%#v", gotA, wantA)
	}

	gotB := valuesFor(ps, "b", 3)
	wantB := []eval.Value{eval.Null(), eval.Null(), eval.Null()}
	if !reflect.DeepEqual(gotB, wantB) {
		t.Fatalf("expected null fill for empty base values, got=%#v want=%#v", gotB, wantB)
	}

	if got := originFor(ps, "a"); got != nameSpan {
		t.Fatalf("expected explicit origin span, got=%+v want=%+v", got, nameSpan)
	}
	if got := originFor(ps, "missing"); got != blockSpan {
		t.Fatalf("expected fallback block span, got=%+v want=%+v", got, blockSpan)
	}
}

func TestHelpersInLowerParamset(t *testing.T) {
	if got := joinIntIndices(nil); got != "" {
		t.Fatalf("expected empty join for nil indices, got %q", got)
	}
	if got := joinIntIndices([]int{1, 2, 3}); got != "1,2,3" {
		t.Fatalf("unexpected joined indices: %q", got)
	}

	values := []eval.Value{eval.String("a"), eval.String("b")}
	picked := pickValuesAtIndices(values, []int{1, 3, -1, 0})
	wantPick := []eval.Value{eval.String("b"), eval.Null(), eval.Null(), eval.String("a")}
	if !reflect.DeepEqual(picked, wantPick) {
		t.Fatalf("unexpected pickValuesAtIndices result: got=%#v want=%#v", picked, wantPick)
	}

	if !allEqualValues(nil) || !allEqualValues([]eval.Value{eval.Int(1)}) {
		t.Fatalf("allEqualValues should be true for len<=1")
	}
	if !allEqualValues([]eval.Value{eval.Int(1), eval.Float(1.0)}) {
		t.Fatalf("allEqualValues should use numeric equality")
	}
	if allEqualValues([]eval.Value{eval.Int(1), eval.Int(2)}) {
		t.Fatalf("allEqualValues should detect mismatch")
	}

	if got := asString(eval.String("x")); got != "x" {
		t.Fatalf("asString should preserve raw string, got %q", got)
	}
	if got := asString(eval.Int(7)); got != "7" {
		t.Fatalf("asString should stringify non-string values, got %q", got)
	}
}

func TestTemplateValueAndPythonLiteral(t *testing.T) {
	casesTemplate := []struct {
		in   eval.Value
		want string
	}{
		{in: eval.Int(7), want: "7"},
		{in: eval.Float(1.5), want: "1.5"},
		{in: eval.String("x"), want: "x"},
		{in: eval.Bool(true), want: "true"},
		{in: eval.List([]eval.Value{eval.Int(1)}), want: "[1]"},
	}
	for _, tc := range casesTemplate {
		if got := templateValue(tc.in); got != tc.want {
			t.Fatalf("templateValue(%#v)=%q, want %q", tc.in, got, tc.want)
		}
	}

	casesLiteral := []struct {
		in   eval.Value
		want string
	}{
		{in: eval.Null(), want: "None"},
		{in: eval.Int(1), want: "1"},
		{in: eval.Float(2.5), want: "2.5"},
		{in: eval.String("x"), want: `"x"`},
		{in: eval.Bool(true), want: "True"},
		{in: eval.Bool(false), want: "False"},
		{in: eval.List([]eval.Value{eval.Int(1), eval.String("a")}), want: `[1,"a"]`},
		{in: eval.Tuple([]eval.Value{eval.Int(1)}), want: "(1,)"},
		{in: eval.Tuple([]eval.Value{eval.Int(1), eval.Int(2)}), want: "(1,2)"},
		{in: eval.Value{Kind: "other", S: "q"}, want: `""`},
	}
	for _, tc := range casesLiteral {
		if got := pythonLiteral(tc.in); got != tc.want {
			t.Fatalf("pythonLiteral(%#v)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLowerParamsetRowCountDerivationBranches(t *testing.T) {
	t.Run("row count derived from rows", func(t *testing.T) {
		span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
		ps := &sema.Paramset{
			Name:  "p",
			Block: ast.ParamBlock{Span: span},
			Order: []string{"a"},
			Rows: []eval.Row{
				{Values: map[string]eval.Cell{"a": {Value: eval.Int(10)}}},
				{Values: map[string]eval.Cell{"a": {Value: eval.Int(20)}}},
			},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1)},
			},
			Origins: map[string]diag.Span{"a": span},
			Modes:   map[string]string{},
		}
		diags := &diag.Diagnostics{}
		got := lowerParamset(ps, diags)
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
		if len(got.Parameter) != 2 {
			t.Fatalf("expected index + payload parameter, got %#v", got.Parameter)
		}
		if got.Parameter[0].Value != "0,1" {
			t.Fatalf("expected row-based index values, got %#v", got.Parameter[0].Value)
		}
		sq, ok := got.Parameter[1].Value.(SingleQuoted)
		if !ok {
			t.Fatalf("expected payload as SingleQuoted, got %T", got.Parameter[1].Value)
		}
		if string(sq) != "[10,20][$_ji_p]" {
			t.Fatalf("expected row-driven payload expression, got %q", string(sq))
		}
	})

	t.Run("row count derived from longest variable when rows missing", func(t *testing.T) {
		span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
		ps := &sema.Paramset{
			Name:  "p2",
			Block: ast.ParamBlock{Span: span},
			Order: []string{"a", "b"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1), eval.Int(2), eval.Int(3)},
				"b": {eval.String("x")},
			},
			Origins: map[string]diag.Span{"a": span, "b": span},
			Modes:   map[string]string{},
		}
		diags := &diag.Diagnostics{}
		got := lowerParamset(ps, diags)
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
		if got.Parameter[0].Value != "0,1,2" {
			t.Fatalf("expected index size from longest variable, got %#v", got.Parameter[0].Value)
		}
	})
}

func TestLowerIndexedParametersAndPayloadAdditionalBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	origin := func(_ string) diag.Span { return span }

	t.Run("empty indices fallback and explicit idx name", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		params := lowerIndexedParameters(
			[]string{"x"},
			map[string][]eval.Value{"x": {eval.Int(9)}},
			map[string]string{},
			nil,
			"idx",
			origin,
			diags,
		)
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
		if len(params) != 2 {
			t.Fatalf("expected index + payload params, got %#v", params)
		}
		if params[0].Name != "idx" || params[0].Value != "0" {
			t.Fatalf("expected explicit idx name with fallback single index, got %#v", params[0])
		}
	})

	t.Run("mode branches for indexed payload", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		params := lowerIndexedPayloadParameters(
			[]string{"pyv", "shv", "txt", "emp"},
			map[string][]eval.Value{
				"pyv": {eval.Int(1), eval.Int(2)},
				"shv": {eval.Int(5), eval.Int(5)},
				"txt": {eval.String("a"), eval.String("b")},
				"emp": {},
			},
			map[string]string{
				"pyv": "python",
				"shv": "shell",
				"txt": "text",
				"emp": "python",
			},
			[]int{0, 1},
			"$idx",
			origin,
			diags,
		)
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
		if len(params) != 4 {
			t.Fatalf("expected four payload params, got %#v", params)
		}
		if sq, ok := params[0].Value.(SingleQuoted); !ok || string(sq) != "[1,2][$idx]" {
			t.Fatalf("expected varying python mode to lower as index expression, got %#v", params[0].Value)
		}
		if params[1].Mode != "shell" || params[1].Value != "5" {
			t.Fatalf("expected shell mode with equal values to keep scalar, got %#v", params[1])
		}
		if params[2].Mode != "text" || params[2].Value != "a" {
			t.Fatalf("expected non-special mode to use first selected value, got %#v", params[2])
		}
		if sq, ok := params[3].Value.(SingleQuoted); !ok || string(sq) != "" {
			t.Fatalf("expected empty python mode to fallback to empty scalar rendering, got %#v", params[3].Value)
		}
	})

	t.Run("selected values empty fallback", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		params := lowerIndexedPayloadParameters(
			[]string{"s"},
			map[string][]eval.Value{
				"s": {eval.String("only")},
			},
			map[string]string{"s": "shell"},
			nil,
			"$idx",
			origin,
			diags,
		)
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
		if len(params) != 1 || params[0].Value != "only" {
			t.Fatalf("expected selected-values fallback to first full value, got %#v", params)
		}
	})
}

func TestLowerContextualPayloadParametersAdditionalBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	origin := func(_ string) diag.Span { return span }
	diags := &diag.Diagnostics{}
	params := lowerContextualPayloadParameters(
		[]string{"py", "txt", "shEmpty"},
		map[string][]eval.Value{
			"py":      {eval.Int(1), eval.Int(2)},
			"txt":     {eval.String("m"), eval.String("n")},
			"shEmpty": {},
		},
		map[string]string{
			"py":      "python",
			"txt":     "text",
			"shEmpty": "shell",
		},
		"$idx",
		origin,
		diags,
	)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(params) != 3 {
		t.Fatalf("expected three contextual payload params, got %#v", params)
	}
	if sq, ok := params[0].Value.(SingleQuoted); !ok || string(sq) != "[1,2][$idx]" {
		t.Fatalf("expected varying python mode to use index expression, got %#v", params[0].Value)
	}
	if params[1].Mode != "text" || params[1].Value != "m" {
		t.Fatalf("expected text mode first value selection, got %#v", params[1])
	}
	if params[2].Mode != "shell" || params[2].Value != "" {
		t.Fatalf("expected empty shell mode values to fallback to null string, got %#v", params[2])
	}
}

func TestPickValuesAtIndicesAndTemplateValueAdditionalBranches(t *testing.T) {
	if got := pickValuesAtIndices([]eval.Value{eval.Int(1)}, nil); got != nil {
		t.Fatalf("expected nil for empty index selection, got %#v", got)
	}
	if got := templateValue(eval.Bool(false)); got != "false" {
		t.Fatalf("expected false bool template rendering, got %q", got)
	}
	if got := templateValue(eval.Value{Kind: "unknown", S: "z"}); got != `""` {
		t.Fatalf("expected unknown template value to fall back to pythonLiteral quoting, got %q", got)
	}
}

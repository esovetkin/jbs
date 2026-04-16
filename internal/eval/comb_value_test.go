package eval

import "testing"

func testCombValue() Value {
	return CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
		},
	})
}

func TestCombRowCount(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want int
	}{
		{name: "non-comb", v: Int(1), want: 0},
		{name: "comb nil payload", v: CombValue(nil), want: 0},
		{
			name: "comb rows",
			v: CombValue(&Comb{
				Order: []string{"a"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}}},
					{Values: map[string]Cell{"a": {Value: Int(3)}}},
				},
			}),
			want: 3,
		},
	}
	for _, tc := range tests {
		if got := CombRowCount(tc.v); got != tc.want {
			t.Fatalf("%s: expected %d, got %d", tc.name, tc.want, got)
		}
	}
}

func TestCombColumn(t *testing.T) {
	base := testCombValue()

	if got, ok := CombColumn(Int(1), "a"); ok || got != nil {
		t.Fatalf("expected non-comb lookup to fail, got ok=%v value=%#v", ok, got)
	}
	if got, ok := CombColumn(base, ""); ok || got != nil {
		t.Fatalf("expected empty-name lookup to fail, got ok=%v value=%#v", ok, got)
	}
	if got, ok := CombColumn(base, "missing"); ok || got != nil {
		t.Fatalf("expected unknown-column lookup to fail, got ok=%v value=%#v", ok, got)
	}

	broken := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(2)}}},
		},
	})
	if got, ok := CombColumn(broken, "b"); ok || got != nil {
		t.Fatalf("expected missing row-cell lookup to fail, got ok=%v value=%#v", ok, got)
	}

	got, ok := CombColumn(base, "a")
	if !ok {
		t.Fatalf("expected column lookup to succeed")
	}
	if len(got) != 2 || got[0].I != 1 || got[1].I != 2 {
		t.Fatalf("unexpected extracted column: %#v", got)
	}
}

func TestCombProject(t *testing.T) {
	base := testCombValue()

	if got, ok := CombProject(Int(1), []string{"a"}); ok || got.Kind != KindNull {
		t.Fatalf("expected non-comb projection to fail with null, got ok=%v value=%#v", ok, got)
	}
	if got, ok := CombProject(base, nil); ok || got.Kind != KindNull {
		t.Fatalf("expected empty projection to fail with null, got ok=%v value=%#v", ok, got)
	}
	if got, ok := CombProject(base, []string{""}); ok || got.Kind != KindNull {
		t.Fatalf("expected empty-column projection to fail with null, got ok=%v value=%#v", ok, got)
	}
	if got, ok := CombProject(base, []string{"z"}); ok || got.Kind != KindNull {
		t.Fatalf("expected unknown-column projection to fail with null, got ok=%v value=%#v", ok, got)
	}

	broken := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(2)}}},
		},
	})
	if got, ok := CombProject(broken, []string{"b"}); ok || got.Kind != KindNull {
		t.Fatalf("expected projection with missing row cell to fail, got ok=%v value=%#v", ok, got)
	}

	v, ok := CombProject(base, []string{"b", "a", "b"})
	if !ok {
		t.Fatalf("expected projection with duplicate requested columns to succeed")
	}
	if !IsComb(v) {
		t.Fatalf("expected comb projection, got %#v", v)
	}
	if len(v.C.Order) != 2 || v.C.Order[0] != "b" || v.C.Order[1] != "a" {
		t.Fatalf("unexpected projected order: %#v", v.C.Order)
	}
	if len(v.C.Rows) != 2 {
		t.Fatalf("expected 2 projected rows, got %d", len(v.C.Rows))
	}

	withDupRows := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
		},
	})
	dedup, ok := CombProject(withDupRows, []string{"a", "b"})
	if !ok {
		t.Fatalf("expected dedup projection to succeed")
	}
	if len(dedup.C.Rows) != 2 {
		t.Fatalf("expected duplicate projected rows to be removed, got %d rows", len(dedup.C.Rows))
	}
}

func TestValueKey(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string
	}{
		{name: "null", in: Null(), want: "n:"},
		{name: "int", in: Int(7), want: "i:7"},
		{name: "float", in: Float(1.5), want: "f:1.5"},
		{name: "string", in: String("abc"), want: "s:abc"},
		{name: "bool true", in: Bool(true), want: "b:1"},
		{name: "bool false", in: Bool(false), want: "b:0"},
		{name: "list", in: List([]Value{Int(1), String("x")}), want: "l:i:1,s:x"},
		{name: "tuple", in: Tuple([]Value{Int(2), Bool(false)}), want: "t:i:2,b:0"},
		{name: "comb nil", in: CombValue(nil), want: "c:nil"},
		{
			name: "comb value",
			in: CombValue(&Comb{
				Order: []string{"a", "b"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
				},
			}),
			want: "c:2:2",
		},
		{name: "unknown kind", in: Value{Kind: Kind("unknown-kind")}, want: "u:"},
	}

	for _, tc := range tests {
		if got := valueKey(tc.in); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

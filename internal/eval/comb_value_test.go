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
	if len(dedup.C.Rows) != 3 {
		t.Fatalf("expected fallback row identity to preserve duplicate rows, got %d rows", len(dedup.C.Rows))
	}
}

func TestCombProjectKeepsDelimiterCollisionRows(t *testing.T) {
	in := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{
				"a": {Value: String("a\x1fs:b")},
				"b": {Value: String("c")},
			}},
			{Values: map[string]Cell{
				"a": {Value: String("a")},
				"b": {Value: String("b\x1fs:c")},
			}},
		},
	})

	got, ok := CombProject(in, []string{"a", "b"})
	if !ok {
		t.Fatalf("projection failed")
	}
	if len(got.C.Rows) != 2 {
		t.Fatalf("expected distinct rows to survive, got %d", len(got.C.Rows))
	}
}

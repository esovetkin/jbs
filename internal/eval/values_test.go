package eval

import "testing"

func TestListAndTupleConstructorsKinds(t *testing.T) {
	listValue := List([]Value{Int(1)})
	if listValue.Kind != KindList {
		t.Fatalf("expected list kind, got %#v", listValue)
	}
	tupleValue := Tuple([]Value{Int(1)})
	if tupleValue.Kind != KindTuple {
		t.Fatalf("expected tuple kind, got %#v", tupleValue)
	}
}

func TestToSeriesSupportsListAndTuple(t *testing.T) {
	listSeries := ToSeries(List([]Value{Int(1), Int(2)}))
	if len(listSeries) != 2 || listSeries[0].I != 1 || listSeries[1].I != 2 {
		t.Fatalf("unexpected list series: %#v", listSeries)
	}
	tupleSeries := ToSeries(Tuple([]Value{Int(3), Int(4)}))
	if len(tupleSeries) != 2 || tupleSeries[0].I != 3 || tupleSeries[1].I != 4 {
		t.Fatalf("unexpected tuple series: %#v", tupleSeries)
	}
}

func TestEqualListAndTupleAreDifferentKinds(t *testing.T) {
	listValue := List([]Value{Int(1), Int(2)})
	tupleValue := Tuple([]Value{Int(1), Int(2)})
	if Equal(listValue, tupleValue) {
		t.Fatalf("expected list and tuple values to differ by kind")
	}
}

func TestEqual(t *testing.T) {
	dictAB := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
		{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: List([]Value{String("x")})},
	})
	dictBA := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: List([]Value{String("x")})},
		{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
	})
	tests := []struct {
		name string
		a    Value
		b    Value
		want bool
	}{
		{name: "null equals null", a: Null(), b: Null(), want: true},
		{name: "int equals same", a: Int(7), b: Int(7), want: true},
		{name: "int differs", a: Int(7), b: Int(8), want: false},
		{name: "float equals same", a: Float(1.25), b: Float(1.25), want: true},
		{name: "float differs", a: Float(1.25), b: Float(1.5), want: false},
		{name: "string equals same", a: String("abc"), b: String("abc"), want: true},
		{name: "string differs", a: String("abc"), b: String("def"), want: false},
		{name: "bool equals same", a: Bool(true), b: Bool(true), want: true},
		{name: "bool differs", a: Bool(true), b: Bool(false), want: false},
		{name: "numeric cross kind equal", a: Int(2), b: Float(2.0), want: true},
		{name: "numeric cross kind differs", a: Int(2), b: Float(2.1), want: false},
		{name: "non numeric mixed kind", a: Int(1), b: Bool(true), want: false},
		{
			name: "list equals",
			a:    List([]Value{Int(1), String("x"), Bool(false)}),
			b:    List([]Value{Int(1), String("x"), Bool(false)}),
			want: true,
		},
		{
			name: "list length mismatch",
			a:    List([]Value{Int(1), Int(2)}),
			b:    List([]Value{Int(1)}),
			want: false,
		},
		{
			name: "list element mismatch",
			a:    List([]Value{Int(1), Int(2)}),
			b:    List([]Value{Int(1), Int(3)}),
			want: false,
		},
		{
			name: "tuple equals",
			a:    Tuple([]Value{String("a"), Int(1)}),
			b:    Tuple([]Value{String("a"), Int(1)}),
			want: true,
		},
		{
			name: "tuple element mismatch",
			a:    Tuple([]Value{String("a"), Int(1)}),
			b:    Tuple([]Value{String("a"), Int(2)}),
			want: false,
		},
		{
			name: "nested containers equal",
			a: List([]Value{
				Tuple([]Value{Int(1), Int(2)}),
				List([]Value{String("a"), Bool(true)}),
			}),
			b: List([]Value{
				Tuple([]Value{Int(1), Int(2)}),
				List([]Value{String("a"), Bool(true)}),
			}),
			want: true,
		},
		{
			name: "nested containers differ",
			a: List([]Value{
				Tuple([]Value{Int(1), Int(2)}),
				List([]Value{String("a"), Bool(true)}),
			}),
			b: List([]Value{
				Tuple([]Value{Int(1), Int(3)}),
				List([]Value{String("a"), Bool(true)}),
			}),
			want: false,
		},
		{
			name: "list and tuple differ by kind",
			a:    List([]Value{Int(1), Int(2)}),
			b:    Tuple([]Value{Int(1), Int(2)}),
			want: false,
		},
		{
			name: "dict equal independent of insertion order",
			a:    dictAB,
			b:    dictBA,
			want: true,
		},
		{
			name: "dict missing key differs",
			a:    dictAB,
			b: DictValue([]DictEntry{
				{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
			}),
			want: false,
		},
		{
			name: "dict different key with same length differs",
			a:    dictAB,
			b: DictValue([]DictEntry{
				{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
				{Key: DictKey{Kind: DictKeyString, S: "c"}, Value: List([]Value{String("x")})},
			}),
			want: false,
		},
		{
			name: "dict nested value differs",
			a:    dictAB,
			b: DictValue([]DictEntry{
				{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
				{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: List([]Value{String("y")})},
			}),
			want: false,
		},
		{
			name: "nil dict payload equals empty dict",
			a:    Value{Kind: KindDict},
			b:    DictValue(nil),
			want: true,
		},
		{
			name: "unknown kind same kind defaults equal",
			a:    Value{Kind: Kind("custom")},
			b:    Value{Kind: Kind("custom")},
			want: true,
		},
		{
			name: "unknown kind different kind",
			a:    Value{Kind: Kind("custom_a")},
			b:    Value{Kind: Kind("custom_b")},
			want: false,
		},
	}
	for _, tt := range tests {
		if got := Equal(tt.a, tt.b); got != tt.want {
			t.Fatalf("%s: expected %v, got %v", tt.name, tt.want, got)
		}
		if got := Equal(tt.b, tt.a); got != tt.want {
			t.Fatalf("%s (symmetric): expected %v, got %v", tt.name, tt.want, got)
		}
	}
}

func TestDictKeyStableString(t *testing.T) {
	tests := []struct {
		name string
		key  DictKey
		want string
	}{
		{name: "string", key: DictKey{Kind: DictKeyString, S: "a b"}, want: `s:"a b"`},
		{name: "int", key: DictKey{Kind: DictKeyInt, I: -7}, want: "i:-7"},
		{name: "bool true", key: DictKey{Kind: DictKeyBool, B: true}, want: "b:true"},
		{name: "bool false", key: DictKey{Kind: DictKeyBool, B: false}, want: "b:false"},
		{name: "unsupported", key: DictKey{Kind: DictKeyKind("float")}, want: "u:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.key.StableString(); got != tc.want {
				t.Fatalf("StableString()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestValueString(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string
	}{
		{name: "null", in: Null(), want: ""},
		{name: "int", in: Int(42), want: "42"},
		{name: "float integer like", in: Float(2.0), want: "2.0"},
		{name: "float fractional", in: Float(2.5), want: "2.5"},
		{name: "float scientific", in: Float(1.2e-07), want: "1.2e-07"},
		{name: "string", in: String("alpha beta"), want: "alpha beta"},
		{name: "bool true", in: Bool(true), want: "true"},
		{name: "bool false", in: Bool(false), want: "false"},
		{name: "empty list", in: List(nil), want: "[]"},
		{name: "empty tuple", in: Tuple(nil), want: "()"},
		{name: "list", in: List([]Value{Int(1), String("x"), Bool(true)}), want: "[1,x,true]"},
		{name: "tuple", in: Tuple([]Value{Int(1), String("x"), Bool(false)}), want: "(1,x,false)"},
		{name: "function", in: Function(&FunctionValue{}), want: "<function>"},
		{name: "empty dict", in: DictValue(nil), want: "{}"},
		{
			name: "dict with scalar keys and nested value",
			in: DictValue([]DictEntry{
				{Key: DictKey{Kind: DictKeyString, S: "name"}, Value: String("alice")},
				{Key: DictKey{Kind: DictKeyString, S: "not simple"}, Value: Float(1.25)},
				{Key: DictKey{Kind: DictKeyInt, I: 2}, Value: Bool(true)},
				{Key: DictKey{Kind: DictKeyBool, B: false}, Value: List([]Value{Int(1), Int(2)})},
			}),
			want: `{name:alice,"not simple":1.25,2:true,false:[1,2]}`,
		},
		{
			name: "nested list and tuple",
			in: List([]Value{
				Tuple([]Value{Int(1), Int(2)}),
				List([]Value{String("a"), Bool(false)}),
			}),
			want: "[(1,2),[a,false]]",
		},
		{name: "unknown kind", in: Value{Kind: Kind("custom")}, want: ""},
	}
	for _, tt := range tests {
		got := tt.in.String()
		if got != tt.want {
			t.Fatalf("%s: expected %q, got %q", tt.name, tt.want, got)
		}
	}
}

func TestValueStringComb(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string
	}{
		{
			name: "table nil payload",
			in:   CombValue(nil),
			want: "table()",
		},
		{
			name: "table with rows and cols",
			in: CombValue(&Comb{
				Order: []string{"a", "b"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
					{Values: map[string]Cell{"a": {Value: Int(3)}, "b": {Value: String("z")}}},
				},
			}),
			want: "table(rows=3,cols=2)",
		},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Fatalf("%s: expected %q, got %q", tt.name, tt.want, got)
		}
	}
}

func TestIsScalar(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want bool
	}{
		{name: "int", in: Int(1), want: true},
		{name: "float", in: Float(1.5), want: true},
		{name: "string", in: String("x"), want: true},
		{name: "bool", in: Bool(true), want: true},
		{name: "null", in: Null(), want: false},
		{name: "list", in: List([]Value{Int(1)}), want: false},
		{name: "tuple", in: Tuple([]Value{Int(1)}), want: false},
		{name: "comb", in: CombValue(&Comb{}), want: false},
	}
	for _, tt := range tests {
		if got := tt.in.IsScalar(); got != tt.want {
			t.Fatalf("%s: expected %v, got %v", tt.name, tt.want, got)
		}
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want float64
	}{
		{name: "float", in: Float(2.5), want: 2.5},
		{name: "int", in: Int(7), want: 7.0},
		{name: "default non numeric", in: String("x"), want: 0.0},
	}
	for _, tt := range tests {
		if got := toFloat(tt.in); got != tt.want {
			t.Fatalf("%s: expected %v, got %v", tt.name, tt.want, got)
		}
	}
}

func TestEqualComb(t *testing.T) {
	baseA := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
		},
	})
	baseB := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
		},
	})

	tests := []struct {
		name string
		a    Value
		b    Value
		want bool
	}{
		{
			name: "both nil comb payloads equal",
			a:    CombValue(nil),
			b:    CombValue(nil),
			want: true,
		},
		{
			name: "one nil comb payload differs",
			a:    CombValue(nil),
			b:    baseA,
			want: false,
		},
		{
			name: "equal comb values",
			a:    baseA,
			b:    baseB,
			want: true,
		},
		{
			name: "order length mismatch",
			a:    baseA,
			b: CombValue(&Comb{
				Order: []string{"a"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}}},
				},
			}),
			want: false,
		},
		{
			name: "order value mismatch",
			a:    baseA,
			b: CombValue(&Comb{
				Order: []string{"a", "c"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}, "c": {Value: String("x")}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}, "c": {Value: String("y")}}},
				},
			}),
			want: false,
		},
		{
			name: "row count mismatch",
			a:    baseA,
			b: CombValue(&Comb{
				Order: []string{"a", "b"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
				},
			}),
			want: false,
		},
		{
			name: "row value count mismatch",
			a:    baseA,
			b: CombValue(&Comb{
				Order: []string{"a", "b"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
				},
			}),
			want: false,
		},
		{
			name: "row missing key mismatch",
			a:    baseA,
			b: CombValue(&Comb{
				Order: []string{"a", "b"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}, "b0": {Value: String("x")}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
				},
			}),
			want: false,
		},
		{
			name: "row cell value mismatch",
			a:    baseA,
			b: CombValue(&Comb{
				Order: []string{"a", "b"},
				Rows: []Row{
					{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
					{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("z")}}},
				},
			}),
			want: false,
		},
	}

	for _, tt := range tests {
		if got := Equal(tt.a, tt.b); got != tt.want {
			t.Fatalf("%s: expected %v, got %v", tt.name, tt.want, got)
		}
		if got := Equal(tt.b, tt.a); got != tt.want {
			t.Fatalf("%s (symmetric): expected %v, got %v", tt.name, tt.want, got)
		}
	}
}

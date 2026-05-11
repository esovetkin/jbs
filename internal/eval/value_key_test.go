package eval

import "testing"

func TestStableValueKey(t *testing.T) {
	values := []Value{
		Null(),
		Int(7),
		Float(1.5),
		String("abc"),
		Bool(true),
		Bool(false),
		List([]Value{Int(1), String("x")}),
		Tuple([]Value{Int(2), Bool(false)}),
		DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: Int(2)},
			{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
		}),
		CombValue(nil),
		CombValue(&Comb{
			Order: []string{"a", "b"},
			Rows: []Row{
				{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
				{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
			},
		}),
		Value{Kind: Kind("unknown-kind")},
	}

	for _, value := range values {
		if got := StableValueKey(value); got == "" {
			t.Fatalf("StableValueKey must not be empty for %#v", value)
		}
		if got, again := StableValueKey(value), StableValueKey(value); got != again {
			t.Fatalf("StableValueKey must be deterministic for %#v: %q != %q", value, got, again)
		}
	}
}

func TestStableValueTupleKeyKeepsDelimiterCollisionRowsDistinct(t *testing.T) {
	left := StableValueTupleKey([]Value{String("a\x1fs:b"), String("c")})
	right := StableValueTupleKey([]Value{String("a"), String("b\x1fs:c")})
	if left == right {
		t.Fatalf("stable tuple keys collided")
	}
}

func TestStableNamedValueTupleKeyIncludesNamesSafely(t *testing.T) {
	left := StableNamedValueTupleKey([]StableNamedValuePart{
		{Name: "a=b", Value: String("c")},
	})
	right := StableNamedValueTupleKey([]StableNamedValuePart{
		{Name: "a", Value: String("b=c")},
	})
	if left == right {
		t.Fatalf("stable named tuple keys collided")
	}
}

func TestStableValueKeyDistinguishesAmbiguousValues(t *testing.T) {
	pairs := [][2]Value{
		{String("a,b"), List([]Value{String("a"), String("b")})},
		{List([]Value{String("a,b")}), List([]Value{String("a"), String("b")})},
		{Tuple([]Value{String("x,y")}), Tuple([]Value{String("x"), String("y")})},
		{
			DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "a,b"}, Value: String("x")}}),
			DictValue([]DictEntry{
				{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: String("b")},
				{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: String("")},
			}),
		},
	}
	for _, pair := range pairs {
		if StableValueKey(pair[0]) == StableValueKey(pair[1]) {
			t.Fatalf("stable keys collided for %#v and %#v", pair[0], pair[1])
		}
	}
}

func TestStableValueKeySortsDictionaryEntries(t *testing.T) {
	left := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: Int(2)},
		{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
	})
	right := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
		{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: Int(2)},
	})
	if StableValueKey(left) != StableValueKey(right) {
		t.Fatalf("expected dictionary key to be independent of insertion order")
	}
}

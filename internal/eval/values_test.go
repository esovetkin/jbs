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

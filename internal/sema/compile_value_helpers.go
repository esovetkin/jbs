package sema

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"

// seriesAsValue converts a row series into scalar/list value representation.
func seriesAsValue(v []eval.Value) eval.Value {
	if len(v) == 0 {
		return eval.Null()
	}
	if len(v) == 1 {
		return eval.CloneValue(v[0])
	}
	out := make([]eval.Value, len(v))
	for i, value := range v {
		out[i] = eval.CloneValue(value)
	}
	return eval.List(out)
}

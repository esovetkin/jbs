package sema

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"

// seriesAsValue converts a row series into scalar/list value representation.
func seriesAsValue(v []eval.Value) eval.Value {
	if len(v) == 0 {
		return eval.Null()
	}
	if len(v) == 1 {
		return v[0]
	}
	out := make([]eval.Value, len(v))
	copy(out, v)
	return eval.List(out)
}

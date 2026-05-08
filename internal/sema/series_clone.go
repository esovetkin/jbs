package sema

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"

func cloneSeriesMap(src map[string][]eval.Value) map[string][]eval.Value {
	if len(src) == 0 {
		return map[string][]eval.Value{}
	}
	out := make(map[string][]eval.Value, len(src))
	for name, values := range src {
		next := make([]eval.Value, len(values))
		for i, value := range values {
			next[i] = eval.CloneValue(value)
		}
		out[name] = next
	}
	return out
}

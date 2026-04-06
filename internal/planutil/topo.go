package planutil

import (
	"maps"
	"slices"
)

func TopoStepOrder(deps map[string][]string, preferred []string) []string {
	state := make(map[string]int, len(deps))
	order := make([]string, 0, len(deps))

	var visit func(string)
	visit = func(name string) {
		if state[name] == 2 {
			return
		}
		if state[name] == 1 {
			return
		}
		children, ok := deps[name]
		if !ok {
			return
		}
		state[name] = 1
		for _, dep := range children {
			if _, exists := deps[dep]; exists {
				visit(dep)
			}
		}
		state[name] = 2
		order = append(order, name)
	}

	for _, name := range preferred {
		visit(name)
	}
	for _, name := range slices.Sorted(maps.Keys(deps)) {
		visit(name)
	}
	return order
}

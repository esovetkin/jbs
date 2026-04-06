package planutil

import "sort"

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
	extra := make([]string, 0, len(deps))
	for name := range deps {
		extra = append(extra, name)
	}
	sort.Strings(extra)
	for _, name := range extra {
		visit(name)
	}
	return order
}

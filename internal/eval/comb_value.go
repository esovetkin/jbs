package eval

func IsComb(v Value) bool {
	return v.Kind == KindComb && v.C != nil
}

func CombRowCount(v Value) int {
	if !IsComb(v) {
		return 0
	}
	return len(v.C.Rows)
}

func CombColumn(v Value, name string) ([]Value, bool) {
	if !IsComb(v) || name == "" {
		return nil, false
	}
	if !containsCombColumn(v.C.Order, name) {
		return nil, false
	}
	out := make([]Value, 0, len(v.C.Rows))
	for _, row := range v.C.Rows {
		cell, ok := row.Values[name]
		if !ok {
			return nil, false
		}
		out = append(out, cell.Value)
	}
	return out, true
}

func CombProject(v Value, cols []string) (Value, bool) {
	if !IsComb(v) || len(cols) == 0 {
		return Null(), false
	}
	seenCols := make(map[string]struct{}, len(cols))
	order := make([]string, 0, len(cols))
	for _, col := range cols {
		if col == "" {
			return Null(), false
		}
		if _, exists := seenCols[col]; exists {
			continue
		}
		if !containsCombColumn(v.C.Order, col) {
			return Null(), false
		}
		seenCols[col] = struct{}{}
		order = append(order, col)
	}
	if len(order) == 0 {
		return Null(), false
	}
	outRows := make([]Row, 0, len(v.C.Rows))
	seenKeys := make(map[string]struct{}, len(v.C.Rows))
	for _, row := range v.C.Rows {
		projected := Row{Values: make(map[string]Cell, len(order))}
		keyValues := make([]Value, 0, len(order))
		for _, col := range order {
			cell, ok := row.Values[col]
			if !ok {
				return Null(), false
			}
			projected.Values[col] = cell
			keyValues = append(keyValues, cell.Value)
		}
		// The stable tuple key is length-prefixed, so user strings cannot
		// collide with tuple boundaries.
		key := StableValueTupleKey(keyValues)
		if _, exists := seenKeys[key]; exists {
			continue
		}
		seenKeys[key] = struct{}{}
		outRows = append(outRows, projected)
	}
	return CombValue(&Comb{
		Order: append([]string(nil), order...),
		Rows:  outRows,
	}), true
}

func CombNames(v Value) []string {
	if !IsComb(v) {
		return nil
	}
	if len(v.C.Order) > 0 {
		return uniqueStringsPreserveOrder(v.C.Order)
	}
	return RowVariableNames(v.C.Rows)
}

func containsCombColumn(order []string, name string) bool {
	for _, col := range order {
		if col == name {
			return true
		}
	}
	return false
}

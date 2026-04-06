package sema

import (
	"maps"
	"slices"
)

var allowedSubmitKeys = map[string]struct{}{
	"account":        {},
	"args_exec":      {},
	"args_starter":   {},
	"executable":     {},
	"gres":           {},
	"mail":           {},
	"measurement":    {},
	"nodes":          {},
	"notification":   {},
	"outlogfile":     {},
	"outerrfile":     {},
	"queue":          {},
	"starter":        {},
	"tasks":          {},
	"threadspertask": {},
	"timelimit":      {},
	"preprocess":     {},
	"postprocess":    {},
}

func IsSubmitKey(name string) bool {
	_, ok := allowedSubmitKeys[name]
	return ok
}

func SubmitKeys() []string {
	return slices.Sorted(maps.Keys(allowedSubmitKeys))
}

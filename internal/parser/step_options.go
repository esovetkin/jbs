package parser

import "strings"

var stepOptionKeys = []string{
	"max_async",
	"procs",
	"iterations",
}

var stepOptionKeySet = map[string]struct{}{
	"max_async":  {},
	"procs":      {},
	"iterations": {},
}

func isAllowedStepOptionKey(key string) bool {
	_, ok := stepOptionKeySet[key]
	return ok
}

func allowedStepOptionKeysHint() string {
	if len(stepOptionKeys) == 0 {
		return ""
	}
	if len(stepOptionKeys) == 1 {
		return stepOptionKeys[0]
	}
	if len(stepOptionKeys) == 2 {
		return stepOptionKeys[0] + " and " + stepOptionKeys[1]
	}
	return strings.Join(stepOptionKeys[:len(stepOptionKeys)-1], ", ") + " and " + stepOptionKeys[len(stepOptionKeys)-1]
}

package runtimevar

var reservedNames = map[string]string{
	"JBS_RUN_DIR":  "absolute run directory",
	"JBS_SRC_DIR":  "absolute source directory",
	"JBS_STEP":     "current step name",
	"JBS_ROW":      "current row id",
	"JBS_WORK_DIR": "absolute work-package directory",
}

func ReservedName(name string) (string, bool) {
	reason, ok := reservedNames[name]
	return reason, ok
}

func ReservedNames() map[string]string {
	out := make(map[string]string, len(reservedNames))
	for name, reason := range reservedNames {
		out[name] = reason
	}
	return out
}

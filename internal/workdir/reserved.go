package workdir

var reservedEntries = map[string]string{
	"run.sh":   "runtime script",
	"stdout":   "runtime stdout file",
	"stderr":   "runtime stderr file",
	"status":   "runtime status file",
	"exitcode": "runtime exit-code file",
}

func ReservedEntry(name string) (string, bool) {
	reason, ok := reservedEntries[name]
	return reason, ok
}

func ReservedEntries() map[string]string {
	out := make(map[string]string, len(reservedEntries))
	for name, reason := range reservedEntries {
		out[name] = reason
	}
	return out
}

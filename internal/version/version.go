package version

import "strings"

var (
	Version   = "v0.1.0"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func String() string {
	return clean(Version)
}

func Full() string {
	return "version " + clean(Version) + ", commit " + clean(Commit) + ", built " + clean(BuildDate)
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	return s
}

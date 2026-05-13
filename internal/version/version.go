package version

import (
	"os"
	"runtime/debug"
	"strings"
	"time"
)

var Version = "v0.1.1"

var (
	readBuildInfo = debug.ReadBuildInfo
	executable    = os.Executable
)

type Info struct {
	Version string
	Commit  string
	Built   string
}

func String() string {
	return clean(Version)
}

func Current() Info {
	return Info{
		Version: String(),
		Commit:  commitHash(),
		Built:   builtTime(),
	}
}

func Full() string {
	info := Current()
	return "version " + clean(info.Version) + ", commit " + clean(info.Commit) + ", built " + clean(info.Built)
}

func commitHash() string {
	if rev := buildSetting("vcs.revision"); rev != "" {
		return shortHash(rev)
	}
	if hash := pseudoVersionHash(mainModuleVersion()); hash != "" {
		return hash
	}
	if sum := mainModuleSum(); sum != "" {
		return sum
	}
	return "unknown"
}

func builtTime() string {
	path, err := executable()
	if err == nil && path != "" {
		if stat, statErr := os.Stat(path); statErr == nil {
			return stat.ModTime().UTC().Format(time.RFC3339)
		}
	}
	if vcsTime := buildSetting("vcs.time"); vcsTime != "" {
		return normalizeTime(vcsTime)
	}
	return "unknown"
}

func buildSetting(key string) string {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return strings.TrimSpace(setting.Value)
		}
	}
	return ""
}

func mainModuleVersion() string {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return ""
	}
	return strings.TrimSpace(info.Main.Version)
}

func mainModuleSum() string {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return ""
	}
	return strings.TrimSpace(info.Main.Sum)
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func pseudoVersionHash(version string) string {
	parts := strings.Split(strings.TrimSpace(version), "-")
	if len(parts) < 3 {
		return ""
	}
	return shortHash(parts[len(parts)-1])
}

func normalizeTime(value string) string {
	value = strings.TrimSpace(value)
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.UTC().Format(time.RFC3339)
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	return s
}

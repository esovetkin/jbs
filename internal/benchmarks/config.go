package benchmarks

import (
	"fmt"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

type Spec struct {
	Name     string
	DirName  string
	Analyses []string
}

type Config struct {
	Configured bool
	Specs      []Spec
	ByName     map[string]Spec
}

type Problem struct {
	Message string
}

func SafeComponent(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return ""
	}
	return out
}

func FromValue(value eval.Value, sanitize func(string) string) (Config, []Problem) {
	if sanitize == nil {
		sanitize = SafeComponent
	}
	if value.Kind == "" {
		value = eval.DictValue(nil)
	}
	if value.Kind != eval.KindDict {
		return Config{}, []Problem{{Message: "jbs_benchmarks must be a dictionary"}}
	}
	if value.D == nil || len(value.D.Order) == 0 {
		return Config{Configured: false, ByName: map[string]Spec{}}, nil
	}

	cfg := Config{Configured: true, ByName: map[string]Spec{}}
	usedDirs := map[string]string{}
	problems := make([]Problem, 0)
	for _, key := range value.D.Order {
		if key.Kind != eval.DictKeyString {
			problems = append(problems, Problem{Message: "jbs_benchmarks key must be a string benchmark name"})
			continue
		}
		rawName := key.S
		dirName := sanitize(rawName)
		if dirName == "" {
			problems = append(problems, Problem{Message: fmt.Sprintf("jbs_benchmarks benchmark name %q must produce a valid directory name", rawName)})
			continue
		}
		if prev, ok := usedDirs[dirName]; ok && prev != rawName {
			problems = append(problems, Problem{Message: fmt.Sprintf("jbs_benchmarks benchmark names %q and %q both map to directory %q", prev, rawName, dirName)})
			continue
		}
		value := value.D.Entries[key]
		analyses, itemProblems := analyseNamesFromValue(rawName, value)
		if len(itemProblems) > 0 {
			problems = append(problems, itemProblems...)
			continue
		}
		spec := Spec{Name: rawName, DirName: dirName, Analyses: analyses}
		cfg.Specs = append(cfg.Specs, spec)
		cfg.ByName[rawName] = spec
		usedDirs[dirName] = rawName
	}
	if len(problems) > 0 {
		return cfg, problems
	}
	return cfg, nil
}

func analyseNamesFromValue(benchmarkName string, value eval.Value) ([]string, []Problem) {
	switch value.Kind {
	case eval.KindString:
		name := strings.TrimSpace(value.S)
		if name == "" {
			return nil, []Problem{{Message: fmt.Sprintf("jbs_benchmarks[%q] contains an empty analyse name", benchmarkName)}}
		}
		return []string{name}, nil
	case eval.KindList, eval.KindTuple:
		if len(value.L) == 0 {
			return nil, []Problem{{Message: fmt.Sprintf("jbs_benchmarks[%q] must list at least one analyse block", benchmarkName)}}
		}
		names := make([]string, 0, len(value.L))
		seen := make(map[string]struct{}, len(value.L))
		problems := make([]Problem, 0)
		for _, item := range value.L {
			if item.Kind != eval.KindString {
				problems = append(problems, Problem{Message: fmt.Sprintf("jbs_benchmarks[%q] analyse names must be strings", benchmarkName)})
				continue
			}
			name := strings.TrimSpace(item.S)
			if name == "" {
				problems = append(problems, Problem{Message: fmt.Sprintf("jbs_benchmarks[%q] contains an empty analyse name", benchmarkName)})
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
		if len(problems) > 0 {
			return nil, problems
		}
		if len(names) == 0 {
			return nil, []Problem{{Message: fmt.Sprintf("jbs_benchmarks[%q] must list at least one analyse block", benchmarkName)}}
		}
		return names, nil
	default:
		return nil, []Problem{{Message: fmt.Sprintf("jbs_benchmarks[%q] must be a string or a list of strings", benchmarkName)}}
	}
}

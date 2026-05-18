package cli

import (
	"fmt"
	"strings"

	helpdocs "gitlab.jsc.fz-juelich.de/sdlaml/jbs/docs"
)

type Flags struct {
	Input             string
	Run               bool
	Continue          bool
	Status            bool
	Tree              bool
	LsAnalyse         bool
	Archive           bool
	FWait             bool
	FWaitExitExisting bool
	FWaitPaths        []string
	DryRun            bool
	Weak              bool
	NoStrict          bool
	Benchmark         string
	Output            string
	Repl              bool
	Check             bool
	Param             bool
	PrintType         string
	Help              bool
	HelpTopic         string
	CPUProf           string
	MemProf           string
}

const (
	defaultCPUProfilePath = "cpu.pprof"
	defaultMemProfilePath = "mem.pprof"
)

type UsageError struct {
	Message string
}

func (e UsageError) Error() string {
	return e.Message
}

func ParseFlags(args []string) (Flags, error) {
	profiles, rest, err := extractProfileOptions(args)
	if err != nil {
		return Flags{}, err
	}
	cfg, err := parseFlagsWithoutProfileOptions(rest)
	if err != nil {
		return Flags{}, err
	}
	cfg.CPUProf = profiles.CPUProf
	cfg.MemProf = profiles.MemProf
	return cfg, nil
}

type profileFlagValues struct {
	CPUProf string
	MemProf string
}

func extractProfileOptions(args []string) (profileFlagValues, []string, error) {
	var profiles profileFlagValues
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg == "--cpuprof":
			if profiles.CPUProf != "" {
				return profileFlagValues{}, nil, UsageError{Message: profileUsageMessage()}
			}
			profiles.CPUProf = defaultCPUProfilePath
		case strings.HasPrefix(arg, "--cpuprof="):
			value := strings.TrimPrefix(arg, "--cpuprof=")
			if value == "" || profiles.CPUProf != "" {
				return profileFlagValues{}, nil, UsageError{Message: profileUsageMessage()}
			}
			profiles.CPUProf = value
		case arg == "--memprof":
			if profiles.MemProf != "" {
				return profileFlagValues{}, nil, UsageError{Message: profileUsageMessage()}
			}
			profiles.MemProf = defaultMemProfilePath
		case strings.HasPrefix(arg, "--memprof="):
			value := strings.TrimPrefix(arg, "--memprof=")
			if value == "" || profiles.MemProf != "" {
				return profileFlagValues{}, nil, UsageError{Message: profileUsageMessage()}
			}
			profiles.MemProf = value
		default:
			rest = append(rest, arg)
		}
	}
	if profiles.CPUProf != "" && profiles.CPUProf == profiles.MemProf {
		return profileFlagValues{}, nil, UsageError{Message: "--cpuprof and --memprof must use different output files"}
	}
	return profiles, rest, nil
}

func parseFlagsWithoutProfileOptions(args []string) (Flags, error) {
	cfg := Flags{Output: "-"}
	if len(args) == 0 {
		cfg.Repl = true
		return cfg, nil
	}
	if args[0] == "repl" {
		cfg.Repl = true
		if len(args) == 1 {
			return cfg, nil
		}
		return Flags{}, UsageError{Message: "usage: jbs repl"}
	}
	if args[0] == "run" {
		return parseRunArgs(args[1:])
	}
	if args[0] == "continue" {
		return parseContinueArgs(args[1:])
	}
	if args[0] == "status" {
		return parseBenchmarkInputArgs(args[1:], "status")
	}
	if args[0] == "tree" {
		return parseBenchmarkInputArgs(args[1:], "tree")
	}
	if args[0] == "ls-analyse" {
		return parseBenchmarkInputArgs(args[1:], "ls-analyse")
	}
	if args[0] == "archive" {
		if len(args) == 2 && !strings.HasPrefix(args[1], "-") {
			cfg.Archive = true
			cfg.Input = args[1]
			return cfg, nil
		}
		return Flags{}, UsageError{Message: "usage: jbs archive <file.jbs|benchmark-dir>"}
	}
	if args[0] == "fwait" {
		return parseFWaitArgs(args[1:])
	}
	if args[0] == "help" {
		cfg.Help = true
		switch len(args) {
		case 1:
			return cfg, nil
		case 2:
			if isKnownHelpTopic(args[1]) {
				cfg.HelpTopic = args[1]
				return cfg, nil
			}
		}
		return Flags{}, UsageError{Message: helpUsageMessage()}
	}
	if args[0] == "param" {
		return parseParamArgs(args[1:])
	}
	for i := 0; i < len(args); i++ {
		next, consumed, err := consumeBenchmarkOption(&cfg, args, i, defaultRunUsageMessage())
		if err != nil {
			return Flags{}, err
		}
		if consumed {
			i = next
			continue
		}

		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			cfg.Help = true
		case arg == "-c" || arg == "--check":
			if cfg.Check {
				return Flags{}, UsageError{Message: checkUsageMessage()}
			}
			cfg.Check = true
		case arg == "-n" || arg == "--dry-run":
			if cfg.DryRun {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			cfg.DryRun = true
		case arg == "-w" || arg == "--weak":
			if cfg.Weak {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			cfg.Weak = true
		case arg == "--no-strict":
			if cfg.NoStrict {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			cfg.NoStrict = true
		case strings.HasPrefix(arg, "-"):
			return Flags{}, UsageError{Message: fmt.Sprintf("unknown option: %s", arg)}
		default:
			if cfg.Input == "" {
				cfg.Input = arg
			} else {
				return Flags{}, UsageError{Message: fmt.Sprintf("unexpected extra arguments: [%s]", arg)}
			}
		}
	}
	if cfg.Check {
		if cfg.Input == "" || cfg.Help || cfg.DryRun || cfg.Weak || cfg.NoStrict || cfg.Benchmark != "" {
			return Flags{}, UsageError{Message: checkUsageMessage()}
		}
		return cfg, nil
	}
	if (cfg.NoStrict || cfg.DryRun || cfg.Weak || cfg.Benchmark != "") && (cfg.Help || cfg.Input == "") {
		return Flags{}, UsageError{Message: defaultRunUsageMessage()}
	}
	if cfg.Input != "" && !cfg.Help {
		cfg.Run = true
	}
	return cfg, nil
}

func UsageText() string {
	return fmt.Sprintf(`Usage:

Read examples/help:
  jbs help [%s]

Run:
  jbs <file.jbs> [-n|--dry-run] [-w|--weak] [--no-strict] [-b|--benchmark <name>]
  jbs run <file.jbs> [-n|--dry-run] [-w|--weak] [--no-strict] [-b|--benchmark <name>]
  jbs continue <file.jbs> [-b|--benchmark <name>]

Check syntax:
  jbs -c|--check <file.jbs>

Print status of the latest run:
  jbs status <file.jbs|benchmark-dir> [-b|--benchmark <name>]

List generated analyse tables:
  jbs ls-analyse <file.jbs|benchmark-dir> [-b|--benchmark <name>]

Options:
  -n, --dry-run  Create the run directory without starting workpackages
  -w, --weak     Generate analyse outputs even when some workpackages fail
  -b, --benchmark <name>
                 Run, continue, or inspect one configured benchmark component
  --no-strict   Do not add set -euo pipefail to generated run.sh
  -c, --check   Parse syntax only; do not evaluate expressions or imports

Profiling:
  --cpuprof[=<file>]
                 Write a CPU pprof profile; default: cpu.pprof
  --memprof[=<file>]
                 Write a heap pprof profile at command exit; default: mem.pprof

Archive benchmark directory:
  jbs archive <file.jbs|benchmark-dir>

Wait for files:
  jbs fwait [-e] <path> [path...]

Inspect job dependencies:
  jbs tree <file.jbs> [-b|--benchmark <name>]

Inspect step parameter expansion:
  jbs param [-t pretty|csv] [-o <outputfile>] <file.jbs>
  defaults: -t pretty, -o - (stdout)

Interactive mode:
  jbs
  jbs repl`, helpUsageTopics())
}

func parseParamArgs(args []string) (Flags, error) {
	cfg := Flags{Param: true, Output: "-", PrintType: "pretty"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-t" || arg == "--type":
			if i+1 >= len(args) {
				return Flags{}, UsageError{Message: paramUsageMessage()}
			}
			i++
			cfg.PrintType = args[i]
		case strings.HasPrefix(arg, "--type="):
			cfg.PrintType = strings.TrimPrefix(arg, "--type=")
		case strings.HasPrefix(arg, "-t="):
			cfg.PrintType = strings.TrimPrefix(arg, "-t=")
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) {
				return Flags{}, UsageError{Message: paramUsageMessage()}
			}
			i++
			cfg.Output = args[i]
		case strings.HasPrefix(arg, "--output="):
			cfg.Output = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-o="):
			cfg.Output = strings.TrimPrefix(arg, "-o=")
		case strings.HasPrefix(arg, "-"):
			return Flags{}, UsageError{Message: paramUsageMessage()}
		default:
			if cfg.Input == "" {
				cfg.Input = arg
				continue
			}
			return Flags{}, UsageError{Message: paramUsageMessage()}
		}
	}
	if cfg.Input == "" {
		return Flags{}, UsageError{Message: paramUsageMessage()}
	}
	if cfg.PrintType != "pretty" && cfg.PrintType != "csv" {
		return Flags{}, UsageError{Message: fmt.Sprintf("unknown param type: %s", cfg.PrintType)}
	}
	return cfg, nil
}

func paramUsageMessage() string {
	return "usage: jbs param [-t pretty|csv] [-o <outputfile>] <file.jbs>"
}

func parseFWaitArgs(args []string) (Flags, error) {
	cfg := Flags{FWait: true, Output: "-"}
	for _, arg := range args {
		switch {
		case arg == "-e":
			cfg.FWaitExitExisting = true
		case strings.HasPrefix(arg, "-"):
			return Flags{}, UsageError{Message: "usage: jbs fwait [-e] <path> [path...]"}
		default:
			cfg.FWaitPaths = append(cfg.FWaitPaths, arg)
		}
	}
	if len(cfg.FWaitPaths) == 0 {
		return Flags{}, UsageError{Message: "usage: jbs fwait [-e] <path> [path...]"}
	}
	return cfg, nil
}

func parseRunArgs(args []string) (Flags, error) {
	cfg := Flags{Run: true, Output: "-"}
	for i := 0; i < len(args); i++ {
		next, consumed, err := consumeBenchmarkOption(&cfg, args, i, runUsageMessage())
		if err != nil {
			return Flags{}, err
		}
		if consumed {
			i = next
			continue
		}

		arg := args[i]
		switch {
		case arg == "-n" || arg == "--dry-run":
			if cfg.DryRun {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.DryRun = true
		case arg == "-w" || arg == "--weak":
			if cfg.Weak {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.Weak = true
		case arg == "--no-strict":
			if cfg.NoStrict {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.NoStrict = true
		case strings.HasPrefix(arg, "-"):
			return Flags{}, UsageError{Message: runUsageMessage()}
		default:
			if cfg.Input != "" {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.Input = arg
		}
	}
	if cfg.Input == "" {
		return Flags{}, UsageError{Message: runUsageMessage()}
	}
	return cfg, nil
}

func runUsageMessage() string {
	return "usage: jbs run [-n|--dry-run] [-w|--weak] [--no-strict] [-b|--benchmark <name>] <file.jbs>"
}

func defaultRunUsageMessage() string {
	return "usage: jbs [-n|--dry-run] [-w|--weak] [--no-strict] [-b|--benchmark <name>] <file.jbs>"
}

func checkUsageMessage() string {
	return "usage: jbs -c|--check <file.jbs>"
}

func profileUsageMessage() string {
	return "usage: jbs [--cpuprof[=<file>]] [--memprof[=<file>]] <command>"
}

func parseContinueArgs(args []string) (Flags, error) {
	cfg := Flags{Continue: true, Output: "-"}
	for i := 0; i < len(args); i++ {
		next, consumed, err := consumeBenchmarkOption(&cfg, args, i, continueUsageMessage())
		if err != nil {
			return Flags{}, err
		}
		if consumed {
			i = next
			continue
		}

		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "-"):
			return Flags{}, UsageError{Message: continueUsageMessage()}
		default:
			if cfg.Input != "" {
				return Flags{}, UsageError{Message: continueUsageMessage()}
			}
			cfg.Input = arg
		}
	}
	if cfg.Input == "" {
		return Flags{}, UsageError{Message: continueUsageMessage()}
	}
	return cfg, nil
}

func continueUsageMessage() string {
	return "usage: jbs continue [-b|--benchmark <name>] <file.jbs>"
}

func parseBenchmarkInputArgs(args []string, command string) (Flags, error) {
	cfg := Flags{Output: "-"}
	switch command {
	case "status":
		cfg.Status = true
	case "tree":
		cfg.Tree = true
	case "ls-analyse":
		cfg.LsAnalyse = true
	default:
		return Flags{}, UsageError{Message: fmt.Sprintf("unknown command: %s", command)}
	}
	usage := benchmarkInputUsageMessage(command)
	for i := 0; i < len(args); i++ {
		next, consumed, err := consumeBenchmarkOption(&cfg, args, i, usage)
		if err != nil {
			return Flags{}, err
		}
		if consumed {
			i = next
			continue
		}

		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "-"):
			return Flags{}, UsageError{Message: usage}
		default:
			if cfg.Input != "" {
				return Flags{}, UsageError{Message: usage}
			}
			cfg.Input = arg
		}
	}
	if cfg.Input == "" {
		return Flags{}, UsageError{Message: usage}
	}
	return cfg, nil
}

func benchmarkInputUsageMessage(command string) string {
	return fmt.Sprintf("usage: jbs %s [-b|--benchmark <name>] <file.jbs|benchmark-dir>", command)
}

func consumeBenchmarkOption(cfg *Flags, args []string, i int, usage string) (int, bool, error) {
	arg := args[i]
	switch {
	case arg == "-b" || arg == "--benchmark":
		if cfg.Benchmark != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
			return i, true, UsageError{Message: usage}
		}
		cfg.Benchmark = args[i+1]
		return i + 1, true, nil
	case strings.HasPrefix(arg, "--benchmark="):
		value := strings.TrimPrefix(arg, "--benchmark=")
		if cfg.Benchmark != "" || value == "" {
			return i, true, UsageError{Message: usage}
		}
		cfg.Benchmark = value
		return i, true, nil
	case strings.HasPrefix(arg, "-b="):
		value := strings.TrimPrefix(arg, "-b=")
		if cfg.Benchmark != "" || value == "" {
			return i, true, UsageError{Message: usage}
		}
		cfg.Benchmark = value
		return i, true, nil
	default:
		return i, false, nil
	}
}

func helpTopics() []string {
	return helpdocs.Topics()
}

func isKnownHelpTopic(topic string) bool {
	for _, known := range helpTopics() {
		if topic == known {
			return true
		}
	}
	return false
}

func helpUsageTopics() string {
	return strings.Join(helpTopics(), "|")
}

func helpUsageMessage() string {
	return fmt.Sprintf("usage: jbs help [%s]", helpUsageTopics())
}

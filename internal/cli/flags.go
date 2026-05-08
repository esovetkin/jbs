package cli

import (
	"fmt"
	"strings"
)

var knownHelpTopics = []string{
	"analyse",
	"archive",
	"continue",
	"do",
	"functions",
	"fwait",
	"globals",
	"repl",
	"use",
}

var knownHelpTopicSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(knownHelpTopics))
	for _, topic := range knownHelpTopics {
		set[topic] = struct{}{}
	}
	return set
}()

type Flags struct {
	Input             string
	Run               bool
	Continue          bool
	Archive           bool
	FWait             bool
	FWaitExitExisting bool
	FWaitPaths        []string
	DryRun            bool
	NoStrict          bool
	Benchmark         string
	Output            string
	Repl              bool
	Check             bool
	Fmt               bool
	FmtStrict         bool
	PrintParam        bool
	PrintType         string
	Help              bool
	HelpTopic         string
}

type UsageError struct {
	Message string
}

func (e UsageError) Error() string {
	return e.Message
}

func ParseFlags(args []string) (Flags, error) {
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
	if args[0] == "archive" {
		if len(args) == 2 && !strings.HasPrefix(args[1], "-") {
			cfg.Archive = true
			cfg.Input = args[1]
			return cfg, nil
		}
		return Flags{}, UsageError{Message: "usage: jbs archive <file.jbs>"}
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
	if args[0] == "fmt" {
		return parseFmtArgs(args[1:])
	}
	if args[0] == "printparam" {
		cfg.PrintParam = true
		cfg.PrintType = "pretty"
		for i := 1; i < len(args); i++ {
			arg := args[i]
			switch {
			case arg == "-t" || arg == "--type":
				if i+1 >= len(args) {
					return Flags{}, UsageError{Message: "usage: jbs printparam [-t pretty|csv] [-o <outputfile>] <file.jbs>"}
				}
				i++
				cfg.PrintType = args[i]
			case strings.HasPrefix(arg, "--type="):
				cfg.PrintType = strings.TrimPrefix(arg, "--type=")
			case strings.HasPrefix(arg, "-t="):
				cfg.PrintType = strings.TrimPrefix(arg, "-t=")
			case arg == "-o" || arg == "--output":
				if i+1 >= len(args) {
					return Flags{}, UsageError{Message: "usage: jbs printparam [-t pretty|csv] [-o <outputfile>] <file.jbs>"}
				}
				i++
				cfg.Output = args[i]
			case strings.HasPrefix(arg, "--output="):
				cfg.Output = strings.TrimPrefix(arg, "--output=")
			case strings.HasPrefix(arg, "-o="):
				cfg.Output = strings.TrimPrefix(arg, "-o=")
			case arg == "-c" || arg == "--check":
				return Flags{}, UsageError{Message: "usage: jbs printparam [-t pretty|csv] [-o <outputfile>] <file.jbs>"}
			case strings.HasPrefix(arg, "-"):
				return Flags{}, UsageError{Message: "usage: jbs printparam [-t pretty|csv] [-o <outputfile>] <file.jbs>"}
			default:
				if cfg.Input == "" {
					cfg.Input = arg
					continue
				}
				return Flags{}, UsageError{Message: "usage: jbs printparam [-t pretty|csv] [-o <outputfile>] <file.jbs>"}
			}
		}
		if cfg.Input == "" {
			return Flags{}, UsageError{Message: "usage: jbs printparam [-t pretty|csv] [-o <outputfile>] <file.jbs>"}
		}
		if cfg.PrintType != "pretty" && cfg.PrintType != "csv" {
			return Flags{}, UsageError{Message: fmt.Sprintf("unknown printparam type: %s", cfg.PrintType)}
		}
		return cfg, nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			cfg.Help = true
		case arg == "-c" || arg == "--check":
			cfg.Check = true
		case arg == "-n" || arg == "--dry-run":
			if cfg.DryRun {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			cfg.DryRun = true
		case arg == "--no-strict":
			if cfg.NoStrict {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			cfg.NoStrict = true
		case arg == "-b" || arg == "--benchmark":
			if cfg.Benchmark != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			i++
			cfg.Benchmark = args[i]
		case strings.HasPrefix(arg, "--benchmark="):
			value := strings.TrimPrefix(arg, "--benchmark=")
			if cfg.Benchmark != "" || value == "" {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			cfg.Benchmark = value
		case strings.HasPrefix(arg, "-b="):
			value := strings.TrimPrefix(arg, "-b=")
			if cfg.Benchmark != "" || value == "" {
				return Flags{}, UsageError{Message: defaultRunUsageMessage()}
			}
			cfg.Benchmark = value
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
	if (cfg.NoStrict || cfg.DryRun || cfg.Benchmark != "") && (cfg.Check || cfg.Help || cfg.Input == "") {
		return Flags{}, UsageError{Message: defaultRunUsageMessage()}
	}
	if cfg.Input != "" && !cfg.Check && !cfg.Help {
		cfg.Run = true
	}
	return cfg, nil
}

func UsageText() string {
	return `Usage:

Run:
  jbs input.jbs [-n|--dry-run] [--no-strict] [-b|--benchmark <name>]
  jbs run input.jbs [-n|--dry-run] [--no-strict] [-b|--benchmark <name>]
  jbs continue input.jbs [-b|--benchmark <name>]

Archive:
  jbs archive input.jbs

Wait for files:
  jbs fwait [-e] <path> [path...]

Options:
  -n, --dry-run  Create the run directory without starting workpackages
  -b, --benchmark <name>
                 Run or continue one configured benchmark component
  --no-strict   Do not add set -euo pipefail to generated run.sh
  -c, --check   Parse+validate only

Read examples/help:
  jbs help [analyse|archive|continue|do|functions|fwait|globals|repl|use]

Inspect step parameter expansion:
  jbs printparam [-t pretty|csv] [-o <outputfile>] script.jbs
  defaults: -t pretty, -o -

Format jbs in place:
  jbs fmt [-s|--strict] script.jbs

Interactive mode:
  jbs
  jbs repl`
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
		arg := args[i]
		switch {
		case arg == "-n" || arg == "--dry-run":
			if cfg.DryRun {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.DryRun = true
		case arg == "--no-strict":
			if cfg.NoStrict {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.NoStrict = true
		case arg == "-b" || arg == "--benchmark":
			if cfg.Benchmark != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			i++
			cfg.Benchmark = args[i]
		case strings.HasPrefix(arg, "--benchmark="):
			value := strings.TrimPrefix(arg, "--benchmark=")
			if cfg.Benchmark != "" || value == "" {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.Benchmark = value
		case strings.HasPrefix(arg, "-b="):
			value := strings.TrimPrefix(arg, "-b=")
			if cfg.Benchmark != "" || value == "" {
				return Flags{}, UsageError{Message: runUsageMessage()}
			}
			cfg.Benchmark = value
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
	return "usage: jbs run [-n|--dry-run] [--no-strict] [-b|--benchmark <name>] <file.jbs>"
}

func defaultRunUsageMessage() string {
	return "usage: jbs [-n|--dry-run] [--no-strict] [-b|--benchmark <name>] <file.jbs>"
}

func parseContinueArgs(args []string) (Flags, error) {
	cfg := Flags{Continue: true, Output: "-"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-b" || arg == "--benchmark":
			if cfg.Benchmark != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return Flags{}, UsageError{Message: continueUsageMessage()}
			}
			i++
			cfg.Benchmark = args[i]
		case strings.HasPrefix(arg, "--benchmark="):
			value := strings.TrimPrefix(arg, "--benchmark=")
			if cfg.Benchmark != "" || value == "" {
				return Flags{}, UsageError{Message: continueUsageMessage()}
			}
			cfg.Benchmark = value
		case strings.HasPrefix(arg, "-b="):
			value := strings.TrimPrefix(arg, "-b=")
			if cfg.Benchmark != "" || value == "" {
				return Flags{}, UsageError{Message: continueUsageMessage()}
			}
			cfg.Benchmark = value
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

func isKnownHelpTopic(topic string) bool {
	_, ok := knownHelpTopicSet[topic]
	return ok
}

func helpUsageTopics() string {
	return strings.Join(knownHelpTopics, "|")
}

func helpUsageMessage() string {
	return fmt.Sprintf("usage: jbs help [%s]", helpUsageTopics())
}

func parseFmtArgs(args []string) (Flags, error) {
	cfg := Flags{
		Fmt:    true,
		Output: "",
	}
	for _, arg := range args {
		switch {
		case arg == "-s" || arg == "--strict":
			if cfg.FmtStrict {
				return Flags{}, UsageError{Message: "usage: jbs fmt [-s|--strict] <file.jbs>"}
			}
			cfg.FmtStrict = true
		case strings.HasPrefix(arg, "-"):
			return Flags{}, UsageError{Message: "usage: jbs fmt [-s|--strict] <file.jbs>"}
		default:
			if cfg.Input != "" {
				return Flags{}, UsageError{Message: "usage: jbs fmt [-s|--strict] <file.jbs>"}
			}
			cfg.Input = arg
		}
	}
	if cfg.Input == "" {
		return Flags{}, UsageError{Message: "usage: jbs fmt [-s|--strict] <file.jbs>"}
	}
	return cfg, nil
}

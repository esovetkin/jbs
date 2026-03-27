package cli

import (
	"fmt"
	"strings"
)

type Flags struct {
	Input       string
	Output      string
	Check       bool
	Help        bool
	HelpGlobals bool
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
		cfg.Help = true
		return cfg, nil
	}
	if args[0] == "help" {
		if len(args) == 1 {
			cfg.Help = true
			return cfg, nil
		}
		if len(args) == 2 && args[1] == "globals" {
			cfg.Help = true
			cfg.HelpGlobals = true
			return cfg, nil
		}
		return Flags{}, UsageError{Message: "usage: jbs help [globals]"}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			cfg.Help = true
		case arg == "--check":
			cfg.Check = true
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) {
				return Flags{}, UsageError{Message: fmt.Sprintf("missing value for %s", arg)}
			}
			i++
			cfg.Output = args[i]
		case strings.HasPrefix(arg, "--output="):
			cfg.Output = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-o="):
			cfg.Output = strings.TrimPrefix(arg, "-o=")
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
	return cfg, nil
}

func UsageText() string {
	return `Usage:
  jbs input.jbs
  jbs input.jbs -o JUBE.yaml
  jbs input.jbs --check
  jbs help
  jbs help globals

Options:
  -o, --output   Output path (default: - for stdout)
  --check        Parse+validate only
  -h, --help     Show this help`
}

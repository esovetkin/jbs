package cli

import (
	"fmt"
	"strings"
)

type Flags struct {
	Input        string
	Output       string
	Check        bool
	Fmt          bool
	Help         bool
	HelpAnalyse  bool
	HelpDo       bool
	HelpLet      bool
	HelpParam    bool
	HelpSubmit   bool
	HelpGlobals  bool
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
		if len(args) == 2 && args[1] == "do" {
			cfg.Help = true
			cfg.HelpDo = true
			return cfg, nil
		}
		if len(args) == 2 && args[1] == "analyse" {
			cfg.Help = true
			cfg.HelpAnalyse = true
			return cfg, nil
		}
		if len(args) == 2 && args[1] == "let" {
			cfg.Help = true
			cfg.HelpLet = true
			return cfg, nil
		}
		if len(args) == 2 && args[1] == "param" {
			cfg.Help = true
			cfg.HelpParam = true
			return cfg, nil
		}
		if len(args) == 2 && args[1] == "submit" {
			cfg.Help = true
			cfg.HelpSubmit = true
			return cfg, nil
		}
		return Flags{}, UsageError{Message: "usage: jbs help [analyse|do|globals|let|param|submit]"}
	}
	if args[0] == "fmt" {
		if len(args) != 2 {
			return Flags{}, UsageError{Message: "usage: jbs fmt <file.jbs>"}
		}
		if strings.HasPrefix(args[1], "-") {
			return Flags{}, UsageError{Message: "usage: jbs fmt <file.jbs>"}
		}
		cfg.Fmt = true
		cfg.Input = args[1]
		cfg.Output = ""
		return cfg, nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			cfg.Help = true
		case arg == "-c" || arg == "--check":
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

Compile with:
  jbs input.jbs -o output.yaml

Options:
  -o, --output   Output path (default: - for stdout)
  -c, --check    Parse+validate only

Read examples/help:
  jbs help [globals|param|do|submit|let|analyse]

Format jbs in place:
  jbs fmt script.jbs`
}

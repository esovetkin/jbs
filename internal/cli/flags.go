package cli

import (
	"fmt"
	"strings"
)

var knownHelpTopics = []string{
	"analyse",
	"do",
	"functions",
	"globals",
	"repl",
	"submit",
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
	Input      string
	Output     string
	Repl       bool
	Check      bool
	Fmt        bool
	FmtStrict  bool
	Embed      bool
	EmbedName  string
	PrintParam bool
	PrintType  string
	Help       bool
	HelpTopic  string
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
	if args[0] == "embed" {
		cfg.Embed = true
		if len(args) == 1 {
			return cfg, nil
		}
		if len(args) == 2 && !strings.HasPrefix(args[1], "-") {
			cfg.EmbedName = args[1]
			return cfg, nil
		}
		return Flags{}, UsageError{Message: "usage: jbs embed [filename]"}
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
  jbs help [analyse|do|functions|globals|repl|submit|use]

Inspect embedded shared scripts:
  jbs embed [filename]

Inspect step parameter expansion:
  jbs printparam [-t pretty|csv] [-o <outputfile>] script.jbs
  defaults: -t pretty, -o -

Format jbs in place:
  jbs fmt [-s|--strict] script.jbs

Interactive mode:
  jbs
  jbs repl`
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

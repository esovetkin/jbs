package eval

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/shellref"
)

type ShellCommand struct {
	Command string
	Dir     string
	Env     []string
}

type ShellRunner func(ShellCommand) ([]byte, error)

type ShellUseEvent struct {
	Name   string
	Value  Value
	Scalar bool
	Span   diag.Span
}

type ShellError struct {
	Err      error
	ExitCode *int
	Stderr   string
}

func (e ShellError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e ShellError) Unwrap() error {
	return e.Err
}

type shellArgs struct {
	Command string
	Strip   bool
	Span    diag.Span
}

func evalShellValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	parsed, ok := parseShellValueArgs(args, at, diags)
	if !ok {
		return Null()
	}
	return evalParsedShellCall(parsed, env, at, diags, opts, ctx)
}

func evalParsedShellCall(args shellArgs, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	shellEnv := buildShellEnv(args.Command, args.Span, env, diags, opts, ctx)
	if diags.HasErrors() {
		return Null()
	}

	runner := opts.ShellRunner
	if runner == nil {
		runner = defaultShellRunner
	}
	out, err := runner(ShellCommand{
		Command: args.Command,
		Dir:     shellWorkingDir(opts),
		Env:     shellEnv,
	})
	if err != nil {
		addShellError(err, at, diags)
		return Null()
	}

	result := string(out)
	if args.Strip {
		result = stripOneTrailingNewline(result)
	}
	return String(result)
}

func parseShellValueArgs(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) (shellArgs, bool) {
	parsed := shellArgs{Strip: true, Span: at}
	positional := 0
	commandSet := false
	stripSet := false

	for _, arg := range args {
		switch arg.Name {
		case "":
			positional++
			if positional > 1 || commandSet {
				diags.AddError(diag.CodeE106, "shell() expects exactly one command argument", arg.Span, `use shell("command")`)
				return parsed, false
			}
			if arg.Value.Kind != KindString {
				diags.AddError(diag.CodeE106, "shell() command must be a string", arg.Span, `use shell("command")`)
				return parsed, false
			}
			parsed.Command = arg.Value.S
			parsed.Span = arg.Span
			commandSet = true
		case "command":
			if commandSet {
				diags.AddError(diag.CodeE106, "shell() received command more than once", arg.Span, "pass command at most once")
				return parsed, false
			}
			if arg.Value.Kind != KindString {
				diags.AddError(diag.CodeE106, "shell() command must be a string", arg.Span, `use shell(command = "command")`)
				return parsed, false
			}
			parsed.Command = arg.Value.S
			parsed.Span = arg.Span
			commandSet = true
		case "strip":
			if stripSet {
				diags.AddError(diag.CodeE106, "shell() received strip more than once", arg.Span, "pass strip at most once")
				return parsed, false
			}
			stripSet = true
			if arg.Value.Kind != KindBool {
				diags.AddError(diag.CodeE106, "shell() strip argument must be boolean", arg.Span, "use strip=true or strip=false")
				return parsed, false
			}
			parsed.Strip = arg.Value.B
		default:
			diags.AddError(diag.CodeE106, "unknown named argument '"+arg.Name+"' for shell()", arg.Span, "supported named arguments: command, strip")
			return parsed, false
		}
	}

	if !commandSet {
		diags.AddError(diag.CodeE106, "shell() expects exactly one command argument", at, `use shell("command")`)
		return parsed, false
	}
	return parsed, true
}

func buildShellEnv(command string, span diag.Span, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) []string {
	overrides := make(map[string]string)
	warned := make(map[string]struct{})

	for _, name := range shellref.Names(command) {
		value, found, assigned := lookupLocalOrCapturedValue(name, env, span, diags, ctx)
		if !found || !assigned {
			continue
		}

		scalar := value.IsScalar()
		if opts.ShellUse != nil {
			opts.ShellUse(ShellUseEvent{Name: name, Value: value, Scalar: scalar, Span: span})
		}
		if !scalar {
			overrides[name] = ""
			if _, ok := warned[name]; !ok {
				warned[name] = struct{}{}
				diags.AddWarning(diag.CodeW103, fmt.Sprintf("shell() referenced non-scalar JBS variable '%s'", name), span, "only int/float/string/bool variables are exported to shell(); convert explicitly or avoid $"+name)
			}
			continue
		}
		overrides[name] = value.String()
	}

	return mergeEnv(currentEnviron(opts), overrides)
}

func mergeEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		name, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, replaced := overrides[name]; replaced {
			continue
		}
		out = append(out, item)
	}
	for _, name := range slices.Sorted(maps.Keys(overrides)) {
		out = append(out, name+"="+overrides[name])
	}
	return out
}

func defaultShellRunner(spec ShellCommand) ([]byte, error) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return nil, ShellError{Err: fmt.Errorf("find bash: %w", err)}
	}
	cmd := exec.Command(bash, "-c", spec.Command)
	if strings.TrimSpace(spec.Dir) != "" {
		cmd.Dir = spec.Dir
	}
	cmd.Env = spec.Env

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, newShellError(err, stderr.String())
	}
	return out, nil
}

func newShellError(err error, stderr string) ShellError {
	shellErr := ShellError{Err: err, Stderr: stderr}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		shellErr.ExitCode = &code
	}
	return shellErr
}

func addShellError(err error, at diag.Span, diags *diag.Diagnostics) {
	var shellErr ShellError
	if errors.As(err, &shellErr) {
		if shellErr.ExitCode != nil {
			hint := strings.TrimSpace(shellErr.Stderr)
			if hint != "" {
				hint = "stderr: " + hint
			} else {
				hint = shellErr.Error()
			}
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("shell() command failed with exit code %d", *shellErr.ExitCode),
				at,
				hint,
			)
			return
		}
		diags.AddError(diag.CodeE106, "shell() command failed to start", at, shellErr.Error())
		return
	}
	diags.AddError(diag.CodeE106, "shell() command failed", at, err.Error())
}

func shellWorkingDir(opts ExprOptions) string {
	if opts.Files == nil {
		return ""
	}
	return opts.Files.BaseDir
}

func stripOneTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return strings.TrimSuffix(s, "\r\n")
	}
	return strings.TrimSuffix(s, "\n")
}

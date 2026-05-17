package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

const defaultPrintNRow = 10

type PrintOptions struct {
	NRow int
}

type PrintEvent struct {
	Index   int
	Seq     int
	Span    diag.Span
	Values  []Value
	Options PrintOptions
}

type PrintSink func(PrintEvent)

type printArgs struct {
	Values  []Value
	Options PrintOptions
}

func bindPrintArgs(args []CallValueArg, diags *diag.Diagnostics) (printArgs, bool) {
	out := printArgs{Options: PrintOptions{NRow: defaultPrintNRow}}
	namedSeen := false
	seen := make(map[string]diag.Span)
	for _, arg := range args {
		if arg.Name == "" {
			if namedSeen {
				diags.AddError(diag.CodeE106, "positional arguments cannot follow named arguments", arg.Span, "pass positional arguments before any named arguments")
				return out, false
			}
			out.Values = append(out.Values, CloneValue(arg.Value))
			continue
		}
		namedSeen = true
		if prev, exists := seen[arg.Name]; exists {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("argument '%s' received multiple values", arg.Name),
				arg.Span,
				"pass each argument at most once",
				diag.RelatedSpan{Message: "previous value", Span: prev},
			)
			return out, false
		}
		seen[arg.Name] = arg.Span
		switch arg.Name {
		case "values":
			items, ok := callSpreadItems(arg.Value, arg.Span, diags)
			if !ok {
				return out, false
			}
			out.Values = append(out.Values, CloneValues(items)...)
		case "nrow":
			if arg.Value.Kind != KindInt {
				diags.AddError(diag.CodeE106, "print() nrow argument must be an integer", arg.Span, "pass nrow as an integer value >= 0")
				return out, false
			}
			if arg.Value.I < 0 {
				diags.AddError(diag.CodeE106, "print() nrow argument must be non-negative", arg.Span, "pass nrow as an integer value >= 0")
				return out, false
			}
			out.Options.NRow = int(arg.Value.I)
		default:
			diags.AddError(diag.CodeE106, fmt.Sprintf("unknown named argument '%s' for print()", arg.Name), arg.Span, "use one of: values, nrow")
			return out, false
		}
	}
	return out, true
}

func evalPrintCall(args []Value, printOpts PrintOptions, at diag.Span, opts ExprOptions) Value {
	if opts.Print != nil {
		seq := 0
		if opts.NextPrintSeq != nil {
			seq = opts.NextPrintSeq()
		}
		opts.Print(PrintEvent{
			Index:  opts.PrintIndex,
			Seq:    seq,
			Span:   at,
			Values: CloneValues(args),
			Options: PrintOptions{
				NRow: printOpts.NRow,
			},
		})
	}
	return Null()
}

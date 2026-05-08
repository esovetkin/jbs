package eval

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"

type PrintEvent struct {
	Index  int
	Seq    int
	Span   diag.Span
	Values []Value
}

type PrintSink func(PrintEvent)

func evalPrintCall(args []Value, at diag.Span, opts ExprOptions) Value {
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
		})
	}
	return Null()
}

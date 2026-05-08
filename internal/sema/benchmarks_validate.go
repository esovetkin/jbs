package sema

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/benchmarks"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func validateBenchmarksGlobal(res *Result, diags *diag.Diagnostics) {
	if res == nil || diags == nil {
		return
	}
	span := res.Globals.Spans["jbs_benchmarks"]
	cfg, problems := benchmarks.FromValue(res.Globals.Values["jbs_benchmarks"], benchmarks.SafeComponent)
	for _, problem := range problems {
		diags.AddError(diag.CodeE430, problem.Message, span, "configure jbs_benchmarks as {name: analyse_name}")
	}
	if len(problems) > 0 || !cfg.Configured {
		return
	}

	analyses := make(map[string]diag.Span, len(res.Analyse))
	for _, spec := range res.Analyse {
		if spec == nil {
			continue
		}
		analyses[spec.Name] = spec.Span
	}
	for _, bench := range cfg.Specs {
		for _, name := range bench.Analyses {
			if _, ok := analyses[name]; ok {
				continue
			}
			diags.AddError(
				diag.CodeE430,
				fmt.Sprintf("jbs_benchmarks[%q] references unknown analyse block %q", bench.Name, name),
				span,
				"use the name from an existing analyse block",
			)
		}
	}
}

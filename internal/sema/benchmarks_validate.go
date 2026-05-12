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
		diags.AddError(diag.CodeE430, problem.Message, span, "configure jbs_benchmarks as {name: target_name}")
	}
	if len(problems) > 0 || !cfg.Configured {
		return
	}

	doSteps := make(map[string]diag.Span, len(res.DoBlocks))
	for _, block := range res.DoBlocks {
		doSteps[block.Name] = block.Span
	}

	analyses := make(map[string]diag.Span, len(res.Analyse))
	for _, spec := range res.Analyse {
		if spec == nil {
			continue
		}
		analyses[spec.Name] = spec.Span
	}
	for _, bench := range cfg.Specs {
		for _, name := range bench.Targets {
			if _, ok := doSteps[name]; ok {
				continue
			}
			if _, ok := analyses[name]; ok {
				continue
			}
			diags.AddError(
				diag.CodeE430,
				fmt.Sprintf("jbs_benchmarks[%q] references unknown benchmark target %q", bench.Name, name),
				span,
				"use the name of an existing do step or analyse block",
			)
		}
	}
}

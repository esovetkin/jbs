package sema

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"

func BuiltinGlobalValues() map[string]eval.Value {
	return map[string]eval.Value{
		"jbs_name":  eval.String("jbs_benchmark"),
		"jbs_nproc": eval.Int(0),
	}
}

package sema

import "jbs/internal/eval"

func BuiltinGlobalValues() map[string]eval.Value {
	return map[string]eval.Value{
		"jbs_name":  eval.String("jbs_benchmark"),
		"jbs_nproc": eval.Int(0),
	}
}

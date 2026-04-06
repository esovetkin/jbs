package lower

import "jbs/internal/eval"

func BuiltinGlobalValues() map[string]eval.Value {
	return map[string]eval.Value{
		"jbs_name":    eval.String("jbs_benchmark"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}
}

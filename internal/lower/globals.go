package lower

import "jbs/internal/eval"

type GlobalSpec struct {
	Name        string
	DefaultExpr string
	Mode        string
	Type        string
	Target      string
	Description string
}

func BuiltinGlobals() []GlobalSpec {
	return []GlobalSpec{
		{
			Name:        "jbs_name",
			DefaultExpr: "jbs_benchmark",
			Description: "Benchmark name (root name field).",
		},
		{
			Name:        "jbs_outpath",
			DefaultExpr: "out",
			Description: "Benchmark output path (root outpath field).",
		},
	}
}

func BuiltinGlobalValues() map[string]eval.Value {
	return map[string]eval.Value{
		"jbs_name":    eval.String("jbs_benchmark"),
		"jbs_outpath": eval.String("out"),
	}
}

func GlobalDefault(name string) (GlobalSpec, bool) {
	for _, spec := range BuiltinGlobals() {
		if spec.Name == name {
			return spec, true
		}
	}
	return GlobalSpec{}, false
}

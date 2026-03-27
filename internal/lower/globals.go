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
			Name:        "jbs_systemname",
			DefaultExpr: `__import__("os").environ.get("SYSTEMNAME", "")`,
			Mode:        "python",
			Description: "System name used by queue defaults.",
		},
		{
			Name:        "jbs_queue",
			DefaultExpr: `__import__("os").environ.get("JUBE_QUEUE", {"juwelsbooster":"develbooster","jurecadc":"dc-gpu-devel","jusuf":"batch"}["${jbs_systemname}"])`,
			Mode:        "python",
			Target:      "queue",
			Description: "Slurm queue/partition name.",
		},
		{
			Name:        "jbs_account",
			DefaultExpr: `__import__("os").environ.get("JUBE_ACCOUNT", "atmlaml")`,
			Mode:        "python",
			Target:      "account",
			Description: "Slurm account.",
		},
		{
			Name:        "jbs_timelimit",
			DefaultExpr: `__import__("os").environ.get("JUBE_TIMELIMIT", "00:15:00")`,
			Mode:        "python",
			Target:      "timelimit",
			Description: "Job time limit.",
		},
		{
			Name:        "jbs_outlogfile",
			DefaultExpr: "job.out",
			Target:      "outlogfile",
			Description: "Stdout log filename.",
		},
		{
			Name:        "jbs_outerrfile",
			DefaultExpr: "job.err",
			Target:      "outerrfile",
			Description: "Stderr log filename.",
		},
		{
			Name:        "jbs_gres",
			DefaultExpr: "gpu:4",
			Target:      "gres",
			Description: "Slurm generic resources.",
		},
		{
			Name:        "jbs_threadspertask",
			DefaultExpr: "48",
			Type:        "int",
			Target:      "threadspertask",
			Description: "CPU threads per task.",
		},
		{
			Name:        "jbs_nnodes",
			DefaultExpr: "1",
			Type:        "int",
			Target:      "nodes",
			Description: "Number of nodes.",
		},
		{
			Name:        "jbs_tasks",
			DefaultExpr: "$jbs_nnodes",
			Type:        "int",
			Target:      "tasks",
			Description: "Number of tasks.",
		},
		{
			Name:        "jbs_executable",
			DefaultExpr: "/bin/bash",
			Target:      "executable",
			Description: "Submit executable.",
		},
	}
}

func BuiltinGlobalValues() map[string]eval.Value {
	return map[string]eval.Value{
		"jbs_systemname":     eval.String(`__import__("os").environ.get("SYSTEMNAME", "")`),
		"jbs_queue":          eval.String(`__import__("os").environ.get("JUBE_QUEUE", {"juwelsbooster":"develbooster","jurecadc":"dc-gpu-devel","jusuf":"batch"}["${jbs_systemname}"])`),
		"jbs_account":        eval.String(`__import__("os").environ.get("JUBE_ACCOUNT", "atmlaml")`),
		"jbs_timelimit":      eval.String(`__import__("os").environ.get("JUBE_TIMELIMIT", "00:15:00")`),
		"jbs_outlogfile":     eval.String("job.out"),
		"jbs_outerrfile":     eval.String("job.err"),
		"jbs_gres":           eval.String("gpu:4"),
		"jbs_threadspertask": eval.Int(48),
		"jbs_nnodes":         eval.Int(1),
		"jbs_tasks":          eval.Int(1),
		"jbs_executable":     eval.String("/bin/bash"),
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

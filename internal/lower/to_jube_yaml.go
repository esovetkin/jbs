package lower

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/sema"
)

func ToJUBEYAML(res *sema.Result, diags *diag.Diagnostics) Document {
	ctx := &lowerContext{
		res:                    res,
		diags:                  diags,
		names:                  make(map[string]struct{}),
		subsetNames:            make(map[subsetKey]subsetInfo),
		stepSourceRows:         make(map[string]map[string]string),
		patternSetIndexByGroup: make(map[string]int),
		analyserNames:          make(map[string]string),
	}
	ctx.doc = Document{
		Name:    globalString(res.Globals, "jbs_name", "jbs_benchmark"),
		Outpath: globalString(res.Globals, "jbs_outpath", "out"),
		Comment: globalString(res.Globals, "jbs_comment", ""),
	}

	for _, param := range res.Paramsets {
		ctx.names[param.Name] = struct{}{}
		ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, lowerParamset(param, diags))
	}

	for _, stmt := range res.Program.Stmts {
		switch node := stmt.(type) {
		case ast.DoBlock:
			ctx.doc.Step = append(ctx.doc.Step, ctx.lowerDo(node))
		case ast.SubmitBlock:
			aliases := ctx.stepAliasMap(node.Name, true)
			submitSetName := ctx.addSubmitParameterSet(node, aliases)
			ctx.doc.Step = append(ctx.doc.Step, ctx.lowerSubmit(node, submitSetName, aliases))
		}
	}

	ctx.lowerAnalyseAndResult()

	return ctx.doc
}

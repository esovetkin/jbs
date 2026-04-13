// compile `param` blocks into `Paramset`
//
// resolve `with` imports, build evaluation environment/origins/modes,
// evaluate local assignments, evaluate final combination rows, derive
// exposed variable series/order/origins
package sema

import (
	"fmt"
	"slices"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
)

func compileParamBlock(block ast.ParamBlock, known map[string]*Paramset, globals map[string]eval.Value, lets map[string]*LetNamespace, diags *diag.Diagnostics) *Paramset {
	env := make(map[string]eval.Value, len(globals)+16)
	origins := make(map[string]diag.Span, len(globals)+16)
	modes := make(map[string]string, 16)
	localAssigns := make(map[string]localAssignMeta, len(block.Assignments))
	localAssignOrder := make([]string, 0, len(block.Assignments))
	localAssignSeen := make(map[string]bool, len(block.Assignments))
	for k, v := range globals {
		env[k] = v
	}

	bindings := make(map[string]paramBindingInfo, len(block.Assignments)+16)
	type importedOwner struct {
		Source string
	}
	importedOwners := make(map[string]importedOwner)
	importedVars := make(map[string]importedContribution, len(block.WithItems))
	importedVarOrder := make([]string, 0, len(block.WithItems))
	sourceSymbols := make(map[string]sourceSymbolInfo, len(block.WithItems))
	sourceRows := make(map[string][]eval.Row, len(block.WithItems))
	ambiguousSourceSymbols := make(map[string]bool)

	canImport := func(visible, source string) bool {
		if prev, exists := importedOwners[visible]; exists {
			return prev.Source == source
		}
		importedOwners[visible] = importedOwner{Source: source}
		return true
	}
	recordImportedContribution := func(visible, source, sourceVar string, span diag.Span) {
		if _, exists := importedVars[visible]; exists {
			return
		}
		importedVars[visible] = importedContribution{
			Source:    source,
			SourceVar: sourceVar,
			Span:      span,
		}
		importedVarOrder = append(importedVarOrder, visible)
	}
	importParamVar := func(visible, sourceVar string, src *Paramset, at diag.Span) {
		if src == nil {
			return
		}
		vals, ok := src.BaseVars[sourceVar]
		if !ok {
			vals, ok = src.Vars[sourceVar]
		}
		if !ok {
			return
		}
		if !canImport(visible, src.Name) {
			return
		}
		env[visible] = seriesAsValue(vals)
		if origin, ok := src.Origins[sourceVar]; ok {
			origins[visible] = origin
		}
		if mode, ok := src.Modes[sourceVar]; ok {
			modes[visible] = mode
		}
		bindings[visible] = paramBindingInfo{
			Kind:      paramBindingImported,
			Source:    src.Name,
			SourceVar: sourceVar,
			Span:      at,
		}
		recordImportedContribution(visible, src.Name, sourceVar, at)
	}
	importLetVar := func(visible, sourceVar string, src *LetNamespace, at diag.Span) {
		if src == nil {
			return
		}
		v, ok := src.Vars[sourceVar]
		if !ok {
			return
		}
		if !canImport(visible, src.Name) {
			return
		}
		env[visible] = v
		if origin, ok := src.Origins[sourceVar]; ok {
			origins[visible] = origin
		}
		if mode, ok := src.Modes[sourceVar]; ok {
			modes[visible] = mode
		}
		bindings[visible] = paramBindingInfo{
			Kind:      paramBindingImported,
			Source:    src.Name,
			SourceVar: sourceVar,
			Span:      at,
		}
		recordImportedContribution(visible, src.Name, sourceVar, at)
	}

	importSources := make(map[string]*ImportSource, len(known)+len(lets))
	for name, ps := range known {
		importSources[name] = importSourceFromParam(ps)
	}
	for name, ls := range lets {
		importSources[name] = importSourceFromLet(ls)
	}
	resolver := WithResolver{
		Params:  known,
		Lets:    lets,
		Sources: importSources,
	}
	expanded, issues := resolver.ExpandWithItems(block.WithItems, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  true,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     true,
	})
	emitWithIssues(diags, paramWithDiagPolicy(), issues)
	for _, item := range expanded {
		switch item.Kind {
		case SourceKindParam:
			src := known[item.Source]
			if item.Full {
				sourceSymbol := item.SourceExpr
				if sourceSymbol == "" {
					sourceSymbol = item.Source
				}
				if sourceSymbol != "" {
					if prev, exists := sourceSymbols[sourceSymbol]; exists {
						if prev.Source != item.Source {
							diags.AddError(
								diag.CodeE221,
								fmt.Sprintf("ambiguous source symbol '%s' resolves to both '%s' and '%s'", sourceSymbol, prev.Source, item.Source),
								item.Span,
								"use `with <source> as <alias>` to disambiguate source symbols",
								diag.RelatedSpan{Message: "first source symbol binding", Span: prev.Span},
							)
							ambiguousSourceSymbols[sourceSymbol] = true
						}
					} else {
						sourceSymbols[sourceSymbol] = sourceSymbolInfo{
							Source:   item.Source,
							Span:     item.Span,
							VarOrder: planutil.SourceVarNames(src.Order, src.Vars),
						}
						sourceRows[sourceSymbol] = cloneEvalRows(src.Rows)
					}
				}
			}
			for _, v := range item.Vars {
				importParamVar(v.Visible, v.SourceVar, src, item.Span)
			}
		case SourceKindLet:
			src := lets[item.Source]
			for _, v := range item.Vars {
				importLetVar(v.Visible, v.SourceVar, src, item.Span)
			}
		}
	}

	for _, asn := range block.Assignments {
		effectiveExpr := assignmentExpr(asn.Name, asn.Op, asn.Expr, asn.Span)
		warnModeExprInCollections(effectiveExpr, diags)
		localAssigns[asn.Name] = localAssignMeta{
			Expr: asn.Expr,
			Span: asn.Span,
		}
		if !localAssignSeen[asn.Name] {
			localAssignSeen[asn.Name] = true
			localAssignOrder = append(localAssignOrder, asn.Name)
		}
		mode, inner, isModeExpr := unwrapModeExpr(effectiveExpr)
		expr := effectiveExpr
		if isModeExpr {
			expr = inner
		}
		value := eval.EvalExprWithOptions(expr, env, diags, eval.ExprOptions{
			ParamAssignmentTupleArithmetic: true,
		})
		if isModeExpr {
			value = coerceModeValue(mode, value, asn.Span, diags)
			modes[asn.Name] = mode
		} else {
			delete(modes, asn.Name)
		}
		if hasNestedList(value) {
			diags.AddError(
				diag.CodeE305,
				fmt.Sprintf("nested tuple/list value is not allowed for param variable '%s'", asn.Name),
				asn.Span,
				"use flat tuple/list values only",
			)
		}
		env[asn.Name] = value
		origins[asn.Name] = asn.Span
		bindings[asn.Name] = paramBindingInfo{
			Kind: paramBindingLocal,
			Span: asn.Span,
		}
	}

	series := make(map[string][]eval.Value, len(env))
	for name, value := range env {
		series[name] = eval.ToSeries(value)
	}

	if block.Final == nil {
		return &Paramset{
			Name:     block.Name,
			Block:    block,
			Rows:     nil,
			Vars:     map[string][]eval.Value{},
			BaseVars: map[string][]eval.Value{},
			Origins:  map[string]diag.Span{},
			Modes:    map[string]string{},
			Order:    nil,
			HasPlus:  false,
		}
	}

	refs := combIdentRefs(block.Final)
	ambiguousIdentifierSpans := make(map[string][]diag.Span)
	sourceReferenced := make(map[string]bool)
	for _, ref := range refs {
		_, hasSourceSymbol := sourceSymbols[ref.Name]
		if ambiguousSourceSymbols[ref.Name] {
			hasSourceSymbol = false
		}
		_, hasBinding := bindings[ref.Name]
		if hasSourceSymbol && hasBinding {
			ambiguousIdentifierSpans[ref.Name] = append(ambiguousIdentifierSpans[ref.Name], ref.Span)
			continue
		}
		if hasSourceSymbol {
			sourceReferenced[ref.Name] = true
		}
	}
	for name, spans := range ambiguousIdentifierSpans {
		sourceInfo := sourceSymbols[name]
		for _, span := range spans {
			related := []diag.RelatedSpan{
				{Message: "source symbol binding", Span: sourceInfo.Span},
			}
			if binding := bindings[name]; !binding.Span.IsZero() {
				related = append(related, diag.RelatedSpan{Message: "variable binding", Span: binding.Span})
			}
			diags.AddError(
				diag.CodeE221,
				fmt.Sprintf("ambiguous identifier '%s' in final expression: matches both a variable and a source symbol", name),
				span,
				"use `with ... as ...` aliases to disambiguate variable/source references",
				related...,
			)
		}
	}

	reportedMixed := make(map[string]struct{})
	for sourceName := range sourceReferenced {
		sourceInfo := sourceSymbols[sourceName]
		for _, ref := range refs {
			binding, ok := bindings[ref.Name]
			if !ok || binding.Kind != paramBindingImported {
				continue
			}
			if binding.Source != sourceInfo.Source {
				continue
			}
			key := sourceName + "::" + ref.Name
			if _, exists := reportedMixed[key]; exists {
				continue
			}
			reportedMixed[key] = struct{}{}
			diags.AddError(
				diag.CodeE220,
				fmt.Sprintf("cannot mix full source '%s' with variable '%s' from the same source '%s' in one final expression", sourceName, ref.Name, sourceInfo.Source),
				ref.Span,
				"use either source-level expressions or variable-level expressions from that source, not both",
				diag.RelatedSpan{Message: "source symbol usage", Span: sourceInfo.Span},
			)
		}
	}

	seed := make([]string, 0, len(refs))
	seedSeen := make(map[string]struct{}, len(refs))
	addSeed := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seedSeen[name]; exists {
			return
		}
		seedSeen[name] = struct{}{}
		seed = append(seed, name)
	}
	for _, ref := range refs {
		name := ref.Name
		if name == "" {
			continue
		}
		if sourceReferenced[name] {
			if sourceInfo, ok := sourceSymbols[name]; ok {
				for _, sourceVar := range sourceInfo.VarOrder {
					addSeed(sourceVar)
				}
			}
			continue
		}
		addSeed(name)
	}
	warnUnusedParamContributors(localAssigns, localAssignOrder, importedVars, importedVarOrder, seed, diags)

	rows := eval.EvalCombinationWithOptions(block.Final, series, origins, eval.CombEvalOptions{
		SourceRows: sourceRows,
	}, diags)
	if rows == nil {
		rows = make([]eval.Row, 0)
	}

	ambiguousRefNames := make(map[string]bool, len(ambiguousIdentifierSpans))
	for name := range ambiguousIdentifierSpans {
		ambiguousRefNames[name] = true
	}
	order := resolveFinalExposeOrder(refs, sourceSymbols, ambiguousRefNames)

	vars := make(map[string][]eval.Value, len(order))
	varOrigins := make(map[string]diag.Span, len(order))
	baseVars := make(map[string][]eval.Value, len(order))

	for _, name := range order {
		values := make([]eval.Value, 0, len(rows))
		for _, row := range rows {
			cell, ok := row.Values[name]
			if !ok {
				continue
			}
			values = append(values, cell.Value)
			if _, exists := varOrigins[name]; !exists && !cell.Origin.IsZero() {
				varOrigins[name] = cell.Origin
			}
		}
		if len(values) == 0 {
			if s, ok := series[name]; ok {
				values = append(values, s...)
			}
		}
		vars[name] = values
		if s, ok := series[name]; ok {
			baseVars[name] = slices.Clone(s)
		}
		if _, exists := varOrigins[name]; !exists {
			if o, ok := origins[name]; ok {
				varOrigins[name] = o
			}
		}
	}

	return &Paramset{
		Name:     block.Name,
		Block:    block,
		Rows:     rows,
		Vars:     vars,
		BaseVars: baseVars,
		Origins:  varOrigins,
		Modes:    modes,
		Order:    order,
		HasPlus:  combHasOp(block.Final, "+"),
	}
}

type paramBindingKind uint8

const (
	paramBindingLocal paramBindingKind = iota
	paramBindingImported
)

type paramBindingInfo struct {
	Kind      paramBindingKind
	Source    string
	SourceVar string
	Span      diag.Span
}

type importedContribution struct {
	Source    string
	SourceVar string
	Span      diag.Span
}

type sourceSymbolInfo struct {
	Source   string
	Span     diag.Span
	VarOrder []string
}

type combIdentRef struct {
	Name string
	Span diag.Span
}

func combIdentRefs(expr ast.CombExpr) []combIdentRef {
	out := make([]combIdentRef, 0)
	var walk func(ast.CombExpr)
	walk = func(node ast.CombExpr) {
		switch n := node.(type) {
		case ast.CombIdent:
			if n.Name != "" {
				out = append(out, combIdentRef{Name: n.Name, Span: n.Span})
			}
		case ast.CombBinary:
			walk(n.Left)
			walk(n.Right)
		}
	}
	walk(expr)
	return out
}

func resolveFinalExposeOrder(refs []combIdentRef, sourceSymbols map[string]sourceSymbolInfo, ambiguous map[string]bool) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(refs))
	add := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		if !ambiguous[ref.Name] {
			if source, ok := sourceSymbols[ref.Name]; ok {
				for _, name := range source.VarOrder {
					add(name)
				}
				continue
			}
		}
		add(ref.Name)
	}
	return out
}

func cloneEvalRows(in []eval.Row) []eval.Row {
	if len(in) == 0 {
		return nil
	}
	out := make([]eval.Row, len(in))
	for i, row := range in {
		values := make(map[string]eval.Cell, len(row.Values))
		for name, cell := range row.Values {
			values[name] = cell
		}
		out[i] = eval.Row{Values: values}
	}
	return out
}

func warnModeExprInCollections(expr ast.Expr, diags *diag.Diagnostics) {
	var walk func(ast.Expr, bool)
	walk = func(node ast.Expr, inCollection bool) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.ModeExpr:
			if inCollection {
				diags.AddWarning(
					diag.CodeW301,
					fmt.Sprintf("%s(...) used inside tuple/list expression", n.Mode),
					n.Span,
					"use shell()/python() as a standalone assignment value, then reference the variable",
				)
			}
			walk(n.Expr, inCollection)
		case ast.ListExpr:
			for _, item := range n.Items {
				walk(item, true)
			}
		case ast.TupleExpr:
			for _, item := range n.Items {
				walk(item, true)
			}
		case ast.ConvertExpr:
			walk(n.Expr, inCollection)
		case ast.UnaryExpr:
			walk(n.Expr, inCollection)
		case ast.BinaryExpr:
			walk(n.Left, inCollection)
			walk(n.Right, inCollection)
		case ast.CompareExpr:
			walk(n.Left, inCollection)
			walk(n.Right, inCollection)
		case ast.ConditionalExpr:
			walk(n.Then, inCollection)
			walk(n.Cond, inCollection)
			walk(n.Else, inCollection)
		}
	}
	walk(expr, false)
}

func seriesAsValue(v []eval.Value) eval.Value {
	if len(v) == 0 {
		return eval.Null()
	}
	if len(v) == 1 {
		return v[0]
	}
	out := make([]eval.Value, len(v))
	copy(out, v)
	return eval.List(out)
}

func unwrapModeExpr(expr ast.Expr) (string, ast.Expr, bool) {
	modeExpr, ok := expr.(ast.ModeExpr)
	if !ok {
		return "", nil, false
	}
	return modeExpr.Mode, modeExpr.Expr, true
}

func coerceModeValue(mode string, value eval.Value, at diag.Span, diags *diag.Diagnostics) eval.Value {
	switch value.Kind {
	case eval.KindString:
		return value
	case eval.KindList, eval.KindTuple:
		items := make([]eval.Value, len(value.L))
		for i, it := range value.L {
			if it.Kind != eval.KindString {
				diags.AddError(
					diag.CodeE215,
					fmt.Sprintf("%s(...) requires string values", mode),
					at,
					"pass a string expression to mode declarations",
				)
			}
			items[i] = eval.String(it.String())
		}
		return eval.List(items)
	default:
		diags.AddError(
			diag.CodeE215,
			fmt.Sprintf("%s(...) requires string values", mode),
			at,
			"pass a string expression to mode declarations",
		)
		return eval.String(value.String())
	}
}

func combHasOp(expr ast.CombExpr, op string) bool {
	switch e := expr.(type) {
	case ast.CombBinary:
		if e.Op == op {
			return true
		}
		return combHasOp(e.Left, op) || combHasOp(e.Right, op)
	default:
		return false
	}
}

func combIdentOrder(expr ast.CombExpr) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	var walk func(ast.CombExpr)
	walk = func(node ast.CombExpr) {
		switch n := node.(type) {
		case ast.CombIdent:
			if n.Name == "" {
				return
			}
			if _, ok := seen[n.Name]; ok {
				return
			}
			seen[n.Name] = struct{}{}
			out = append(out, n.Name)
		case ast.CombBinary:
			walk(n.Left)
			walk(n.Right)
		}
	}
	walk(expr)
	return out
}

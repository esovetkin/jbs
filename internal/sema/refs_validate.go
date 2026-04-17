// validate variable-reference usage across steps and emits warnings
//
// scan do/submit raw text and relevant string/expression payloads for
// `$var`/ `${var}` and identifier refs, compares references with
// effective imports, accounts usage per source variable, emits W311
// for referenced-but-not-imported vars and W310 for
// exposed-but-never-used vars, and includes shell-like
// scanners/parsers (escape, quote/comment awareness, ...) plus small
// naming helpers.
package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
)

type varRef struct {
	Name string
	Span diag.Span
}

type stepUnusedImport struct {
	StepName   string
	Visible    string
	Source     string
	SourceVar  string
	ImportSpan diag.Span
}

type sourceCandidate struct {
	Source    string
	SourceVar string
}

type warningSource struct {
	Name       string
	Span       diag.Span
	Order      []string
	VarOrigins map[string]diag.Span
}

func buildWarningSources(res *Result) []warningSource {
	out := make([]warningSource, 0, len(res.Bindings))
	for _, binding := range res.Bindings {
		if binding == nil {
			continue
		}
		order := planutil.SourceVarNames(binding.Order, binding.Vars)
		if len(order) == 0 {
			continue
		}
		origins := make(map[string]diag.Span, len(order))
		for _, name := range order {
			origin := binding.Origins[name]
			if origin.IsZero() {
				origin = binding.Span
			}
			origins[name] = origin
		}
		out = append(out, warningSource{
			Name:       binding.Name,
			Span:       binding.Span,
			Order:      order,
			VarOrigins: origins,
		})
	}
	return out
}

func buildGlobalSourceDeps(res *Result, exposedBySource map[string]map[string]diag.Span) map[string][]string {
	out := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	for _, name := range res.GlobalVarOrder {
		gv := res.GlobalVarByName[name]
		if gv == nil || len(gv.DependsOn) == 0 {
			continue
		}
		from := gv.Name
		if from == "" {
			continue
		}
		if _, ok := exposedBySource[from]; !ok {
			continue
		}
		for _, depName := range gv.DependsOn {
			if depName == "" {
				continue
			}
			to := depName
			if to == "" || to == from {
				continue
			}
			if _, ok := exposedBySource[to]; !ok {
				continue
			}
			if _, ok := seen[from]; !ok {
				seen[from] = make(map[string]struct{})
			}
			if _, ok := seen[from][to]; ok {
				continue
			}
			seen[from][to] = struct{}{}
			out[from] = append(out[from], to)
		}
	}
	for key := range out {
		slices.Sort(out[key])
	}
	return out
}

func cloneUsedBySource(used map[string]map[string]bool) map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(used))
	for src, vars := range used {
		if len(vars) == 0 {
			out[src] = map[string]bool{}
			continue
		}
		cp := make(map[string]bool, len(vars))
		for name, mark := range vars {
			cp[name] = mark
		}
		out[src] = cp
	}
	return out
}

func propagateUsedByGlobalDeps(used map[string]map[string]bool, exposedBySource map[string]map[string]diag.Span, deps map[string][]string) {
	if len(used) == 0 || len(deps) == 0 {
		return
	}
	queue := make([]string, 0, len(used))
	seen := make(map[string]bool, len(used))
	for src, vars := range used {
		if len(vars) == 0 {
			continue
		}
		if seen[src] {
			continue
		}
		seen[src] = true
		queue = append(queue, src)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dep := range deps[current] {
			exposed := exposedBySource[dep]
			if len(exposed) == 0 {
				continue
			}
			if _, ok := used[dep]; !ok {
				used[dep] = make(map[string]bool, len(exposed))
			}
			for varName := range exposed {
				used[dep][varName] = true
			}
			if !seen[dep] {
				seen[dep] = true
				queue = append(queue, dep)
			}
		}
	}
}

func validateStepVarReferences(res *Result, diags *diag.Diagnostics) {
	warningSources := buildWarningSources(res)
	exposedBySource := make(map[string]map[string]diag.Span, len(warningSources))
	candidatesByVar := make(map[string][]sourceCandidate)
	used := make(map[string]map[string]bool)
	stepUnused := make(map[string]stepUnusedImport)

	for _, src := range warningSources {
		if len(src.Order) == 0 {
			continue
		}
		exposedBySource[src.Name] = maps.Clone(src.VarOrigins)
		for _, name := range src.Order {
			candidatesByVar[name] = append(candidatesByVar[name], sourceCandidate{
				Source:    src.Name,
				SourceVar: name,
			})
		}
	}

	markUsedExact := func(source string, sourceVar string) {
		if source == "" || sourceVar == "" {
			return
		}
		if _, ok := used[source]; !ok {
			used[source] = make(map[string]bool)
		}
		used[source][sourceVar] = true
	}

	markUsedByImports := func(imports []importedVar) {
		for _, imp := range imports {
			sourceVar := imp.SourceVar
			if sourceVar == "" {
				sourceVar = imp.Name
			}
			markUsedExact(imp.Source, sourceVar)
		}
	}

	markUsedCandidates := func(candidates []sourceCandidate) {
		for _, cand := range candidates {
			markUsedExact(cand.Source, cand.SourceVar)
		}
	}

	warnMissing := func(stepName string, ref varRef, candidates []sourceCandidate) {
		if len(candidates) == 0 {
			return
		}
		originSpan := diag.Span{}
		source := candidates[0]
		if byVar, ok := exposedBySource[source.Source]; ok {
			sourceVar := source.SourceVar
			originSpan = byVar[sourceVar]
		}
		related := []diag.RelatedSpan{}
		if !originSpan.IsZero() {
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", source.Source),
				Span:    originSpan,
			})
		}
		diags.AddWarning(
			diag.CodeW311,
			fmt.Sprintf("variable '%s' is referenced in step '%s' but not imported via with-clause", ref.Name, stepName),
			ref.Span,
			"add `with <source>` or `with <variable> from <source>`",
			related...,
		)
	}

	processStepWithImports := func(stepName string, imports map[string][]importedVar, refs []varRef) {
		if imports == nil {
			imports = map[string][]importedVar{}
		}
		warned := make(map[string]struct{})
		for _, ref := range refs {
			candidates := candidatesByVar[ref.Name]
			if len(candidates) == 0 {
				continue
			}
			origins := imports[ref.Name]
			if len(origins) > 0 {
				markUsedByImports(origins)
				continue
			}
			markUsedCandidates(candidates)
			key := stepName + "::" + ref.Name
			if _, exists := warned[key]; exists {
				continue
			}
			warned[key] = struct{}{}
			warnMissing(stepName, ref, candidates)
		}
	}
	resolveEffectiveImports := func(stepName string) map[string][]importedVar {
		return importsFromStepPlan(res.StepScopeByName[stepName])
	}
	resolveExplicitImports := func(stepName string) map[string][]importedVar {
		return explicitImportsFromStepPlan(res.StepScopeByName[stepName], res.BindingsByName)
	}
	collectStepUnusedImports := func(stepName string, imports map[string][]importedVar, refs []varRef) {
		if len(imports) == 0 {
			return
		}
		referenced := make(map[string]struct{}, len(refs))
		for _, ref := range refs {
			referenced[ref.Name] = struct{}{}
		}
		for visible, origins := range imports {
			if _, ok := referenced[visible]; ok {
				continue
			}
			for _, origin := range origins {
				sourceVar := origin.SourceVar
				if sourceVar == "" {
					sourceVar = origin.Name
				}
				key := stepName + "::" + visible + "::" + origin.Source + "::" + sourceVar
				if _, ok := stepUnused[key]; ok {
					continue
				}
				stepUnused[key] = stepUnusedImport{
					StepName:   stepName,
					Visible:    visible,
					Source:     origin.Source,
					SourceVar:  sourceVar,
					ImportSpan: origin.Span,
				}
			}
		}
	}

	for _, block := range res.DoBlocks {
		base := block.BodyStart
		if base.Line == 0 {
			base = block.Span.Start
		}
		refs := collectShellLikeRefs(block.Body, base, block.Span.File)
		effectiveImports := resolveEffectiveImports(block.Name)
		processStepWithImports(block.Name, effectiveImports, refs)
		explicitImports := resolveExplicitImports(block.Name)
		collectStepUnusedImports(block.Name, explicitImports, refs)
	}
	for _, block := range res.Submits {
		for _, useName := range block.UseNames {
			markSubmitUseBindingRefs(res, useName, markUsedExact)
		}

		imports := resolveEffectiveImports(block.Name)
		explicitImports := resolveExplicitImports(block.Name)
		if spec := res.SubmitByName[block.Name]; spec != nil {
			for _, helper := range spec.Helpers {
				imports[helper.Original] = append(imports[helper.Original], importedVar{
					Name:      helper.Original,
					SourceVar: helper.Original,
					Source:    helper.UseName,
					Span:      helper.Span,
				})
			}
		}

		refs := make([]varRef, 0)
		for _, field := range block.Fields {
			if field.IsRaw {
				base := field.RawStart
				if base.Line == 0 {
					base = field.Span.Start
				}
				refs = append(refs, collectShellLikeRefs(field.Raw, base, field.Span.File)...)
				continue
			}
			refs = append(refs, collectExprIdentRefs(field.Expr)...)
			refs = append(refs, collectExprStringRefsWith(field.Expr, collectSubmitStringRefs)...)
		}
		if spec := res.SubmitByName[block.Name]; spec != nil {
			for _, value := range spec.Values {
				if value.IsRaw {
					base := value.Span.Start
					if base.Line == 0 {
						base = block.Span.Start
					}
					refs = append(refs, collectShellLikeRefs(value.Raw, base, value.Span.File)...)
					continue
				}
				refs = append(refs, collectEvalStringRefsWith(value.Value, value.Span, collectSubmitStringRefs)...)
			}
		}
		processStepWithImports(block.Name, imports, refs)
		collectStepUnusedImports(block.Name, explicitImports, refs)
	}
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.AnalyseBlock)
		if !ok {
			continue
		}
		// Analyse usage accounting must follow canonical analyse import
		// resolution and only mark semantically valid imports.
		imports := resolveAnalyseImportsCanonical(block.WithItems, res, nil, analyseImportOptions{
			EmitDiagnostics: false,
		})
		for _, origin := range imports {
			markUsedExact(origin.Source, origin.SourceVar)
		}
	}
	usedForW310 := cloneUsedBySource(used)
	propagateUsedByGlobalDeps(usedForW310, exposedBySource, buildGlobalSourceDeps(res, exposedBySource))

	for _, src := range warningSources {
		byVar := exposedBySource[src.Name]
		for _, varName := range src.Order {
			origin := byVar[varName]
			if usedForW310[src.Name][varName] {
				continue
			}
			message := fmt.Sprintf("exposed variable '%s' from global '%s' is never used in any do/submit/analyse block", varName, src.Name)
			hint := fmt.Sprintf("remove it from the global binding or reference it with %s via imports", varName)
			diags.AddWarning(
				diag.CodeW310,
				message,
				origin,
				hint,
			)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(stepUnused)) {
		item := stepUnused[key]
		if item.Source == "" || item.SourceVar == "" {
			continue
		}
		if !used[item.Source][item.SourceVar] {
			continue
		}
		sourceSpan := diag.Span{}
		if byVar, ok := exposedBySource[item.Source]; ok {
			sourceSpan = byVar[item.SourceVar]
		}
		span := item.ImportSpan
		if span.IsZero() {
			span = sourceSpan
		}
		related := []diag.RelatedSpan{}
		if !sourceSpan.IsZero() {
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", item.Source),
				Span:    sourceSpan,
			})
		}
		diags.AddWarning(
			diag.CodeW313,
			fmt.Sprintf("variable '%s' is imported in step '%s' but not referenced in this step", item.Visible, item.StepName),
			span,
			"remove it from the with-clause or reference it in this step",
			related...,
		)
	}
}

func markSubmitUseBindingRefs(res *Result, useName string, mark func(string, string)) {
	if binding := res.BindingsByName[useName]; binding != nil {
		for _, name := range planutil.SourceVarNames(binding.Order, binding.Vars) {
			mark(binding.Name, name)
		}
		return
	}
	ns := res.Namespaces[useName]
	if ns == nil {
		return
	}
	for _, bindingName := range ns.Bindings {
		rest := strings.TrimPrefix(bindingName, useName+".")
		if rest == bindingName || strings.Contains(rest, ".") {
			continue
		}
		binding := res.BindingsByName[bindingName]
		if binding == nil || !binding.Supports(ImportIntoSubmitUse) {
			continue
		}
		for _, name := range planutil.SourceVarNames(binding.Order, binding.Vars) {
			mark(binding.Name, name)
		}
	}
}

type shellScanState uint8

const (
	shellScanCode shellScanState = iota
	shellScanSingleQuote
	shellScanDoubleQuote
	shellScanComment
)

// collectShellLikeRefs scans shell-like text to detect unqualified variable
// references for W310/W311 usage accounting. This scanner is intentionally
// lightweight and context-aware (comments/quotes), not a full shell parser.
func collectShellLikeRefs(text string, base diag.Position, file string) []varRef {
	runes := []rune(text)
	refs := make([]varRef, 0)
	line := base.Line
	col := base.Column
	off := base.Offset
	i := 0
	state := shellScanCode

	advance := func() {
		if i >= len(runes) {
			return
		}
		r := runes[i]
		i++
		off++
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	advanceN := func(target int) {
		for i < target {
			advance()
		}
	}
	appendRef := func(name string, start diag.Position) {
		end := diag.NewPos(off, line, col)
		refs = append(refs, varRef{
			Name: name,
			Span: diag.NewSpan(file, start, end),
		})
	}
	parseExpansion := func(start diag.Position) {
		if i+1 < len(runes) && runes[i+1] == '{' {
			name, end, ok := parseBracedVarRef(runes, i+2)
			if ok {
				advanceN(end + 1)
				appendRef(name, start)
				return
			}
			advance()
			return
		}
		if end, ok := parseBareVarName(runes, i+1); ok {
			name := string(runes[i+1 : end])
			advanceN(end)
			appendRef(name, start)
			return
		}
		advance()
	}

	for i < len(runes) {
		switch state {
		case shellScanCode:
			curr := runes[i]
			if curr == '\'' {
				advance()
				state = shellScanSingleQuote
				continue
			}
			if curr == '"' {
				advance()
				state = shellScanDoubleQuote
				continue
			}
			if curr == '#' && isCommentStart(runes, i) {
				advance()
				state = shellScanComment
				continue
			}
			if curr == '$' && !isEscapedDollar(runes, i) {
				start := diag.NewPos(off, line, col)
				parseExpansion(start)
				continue
			}
			advance()
		case shellScanSingleQuote:
			if runes[i] == '\'' {
				advance()
				state = shellScanCode
				continue
			}
			advance()
		case shellScanDoubleQuote:
			curr := runes[i]
			if curr == '\\' {
				advance()
				if i < len(runes) {
					advance()
				}
				continue
			}
			if curr == '"' {
				advance()
				state = shellScanCode
				continue
			}
			if curr == '$' && !isEscapedDollar(runes, i) {
				start := diag.NewPos(off, line, col)
				parseExpansion(start)
				continue
			}
			advance()
		case shellScanComment:
			if runes[i] == '\n' {
				advance()
				state = shellScanCode
				continue
			}
			advance()
		default:
			advance()
			continue
		}
	}
	return refs
}

// collectSubmitStringRefs scans submit expression string payloads to detect
// unqualified variable references for W310/W311 usage accounting.
//
// Unlike collectShellLikeRefs, this intentionally does not apply shell quote
// or comment suppression because submit expression values often embed nested
// shell snippets inside JBS strings (e.g. args_exec = "-lc '...${x}...'").
func collectSubmitStringRefs(text string, base diag.Position, file string) []varRef {
	runes := []rune(text)
	refs := make([]varRef, 0)
	line := base.Line
	col := base.Column
	off := base.Offset
	i := 0

	advance := func() {
		if i >= len(runes) {
			return
		}
		r := runes[i]
		i++
		off++
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	advanceN := func(target int) {
		for i < target {
			advance()
		}
	}
	appendRef := func(name string, start diag.Position) {
		end := diag.NewPos(off, line, col)
		refs = append(refs, varRef{
			Name: name,
			Span: diag.NewSpan(file, start, end),
		})
	}

	for i < len(runes) {
		if runes[i] == '$' && !isEscapedDollar(runes, i) {
			start := diag.NewPos(off, line, col)
			if i+1 < len(runes) && runes[i+1] == '{' {
				name, end, ok := parseBracedVarRef(runes, i+2)
				if ok {
					advanceN(end + 1)
					appendRef(name, start)
					continue
				}
				advance()
				continue
			}
			if end, ok := parseBareVarName(runes, i+1); ok {
				name := string(runes[i+1 : end])
				advanceN(end)
				appendRef(name, start)
				continue
			}
		}
		advance()
	}
	return refs
}

func collectExprStringRefs(expr ast.Expr) []varRef {
	return collectExprStringRefsWith(expr, collectShellLikeRefs)
}

func collectExprIdentRefs(expr ast.Expr) []varRef {
	if expr == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.IdentExpr:
			out = append(out, varRef{
				Name: n.Name,
				Span: n.Span,
			})
		case ast.QualifiedIdentExpr:
			if n.Namespace != "" {
				out = append(out, varRef{
					Name: n.Namespace,
					Span: n.Span,
				})
			}
		case ast.ListExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.TupleExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.ConvertExpr:
			walk(n.Expr)
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg)
			}
		case ast.AliasExpr:
			walk(n.Expr)
		case ast.IndexExpr:
			walk(n.Base)
			for _, item := range n.Items {
				walk(item)
			}
		case ast.UnaryExpr:
			walk(n.Expr)
		case ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.CompareExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.ConditionalExpr:
			walk(n.Then)
			walk(n.Cond)
			walk(n.Else)
		case ast.ModeExpr:
			walk(n.Expr)
		}
	}
	walk(expr)
	return out
}

type stringRefCollector func(text string, base diag.Position, file string) []varRef

func collectExprStringRefsWith(expr ast.Expr, collect stringRefCollector) []varRef {
	if expr == nil {
		return nil
	}
	if collect == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.StringExpr:
			base := n.Span.Start
			base.Offset++
			base.Column++
			out = append(out, collect(n.Value, base, n.Span.File)...)
		case ast.ListExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.TupleExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.ConvertExpr:
			walk(n.Expr)
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg)
			}
		case ast.AliasExpr:
			walk(n.Expr)
		case ast.IndexExpr:
			walk(n.Base)
			for _, item := range n.Items {
				walk(item)
			}
		case ast.UnaryExpr:
			walk(n.Expr)
		case ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.CompareExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.ConditionalExpr:
			walk(n.Then)
			walk(n.Cond)
			walk(n.Else)
		case ast.ModeExpr:
			walk(n.Expr)
		}
	}
	walk(expr)
	return out
}

func collectEvalStringRefsWith(value eval.Value, span diag.Span, collect stringRefCollector) []varRef {
	if collect == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walk func(eval.Value)
	walk = func(v eval.Value) {
		switch v.Kind {
		case eval.KindString:
			base := span.Start
			if base.Line == 0 {
				base = diag.NewPos(0, 1, 1)
			}
			out = append(out, collect(v.S, base, span.File)...)
		case eval.KindList, eval.KindTuple:
			for _, item := range v.L {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

func isEscapedDollar(runes []rune, idx int) bool {
	count := 0
	for i := idx - 1; i >= 0; i-- {
		if runes[i] != '\\' {
			break
		}
		count++
	}
	return count%2 == 1
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsDigit(r) || isIdentStart(r)
}

func parseBareVarName(runes []rune, start int) (int, bool) {
	j := start
	if j >= len(runes) || !isIdentStart(runes[j]) {
		return 0, false
	}
	j++
	for j < len(runes) && isIdentPart(runes[j]) {
		j++
	}
	return j, true
}

func parseBracedVarRef(runes []rune, start int) (string, int, bool) {
	j := start
	if j >= len(runes) {
		return "", 0, false
	}
	if runes[j] == '#' || runes[j] == '!' {
		j++
	}
	nameStart := j
	nameEnd, ok := parseBareVarName(runes, j)
	if !ok {
		return "", 0, false
	}
	name := string(runes[nameStart:nameEnd])
	j = nameEnd
	depth := 1
	for j < len(runes) {
		switch runes[j] {
		case '\\':
			j += 2
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return name, j, true
			}
		}
		j++
	}
	return "", 0, false
}

func isCommentStart(runes []rune, idx int) bool {
	if idx < 0 || idx >= len(runes) || runes[idx] != '#' {
		return false
	}
	if idx == 0 {
		return true
	}
	return isShellCommentBoundary(runes[idx-1])
}

func isShellCommentBoundary(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	switch r {
	case ';', '|', '&', '(', ')', '{', '}':
		return true
	default:
		return false
	}
}

func sanitizeStepName(input string) string {
	if input == "" {
		return "x"
	}
	var b strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	out := b.String()
	if out == "" {
		return "x"
	}
	return out
}

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

func validateStepVarReferences(res *Result, diags *diag.Diagnostics) {
	type sourceCandidate struct {
		Source    string
		SourceVar string
	}
	exposedBySource := make(map[string]map[string]diag.Span)
	candidatesByVar := make(map[string][]sourceCandidate)
	used := make(map[string]map[string]bool)

	for _, sourceName := range slices.Sorted(maps.Keys(res.ImportSourceByName)) {
		src := res.ImportSourceByName[sourceName]
		if src == nil {
			continue
		}
		varNames := planutil.SourceVarNames(src.Order, src.Vars)
		if len(varNames) == 0 {
			continue
		}
		if _, ok := exposedBySource[sourceName]; !ok {
			exposedBySource[sourceName] = make(map[string]diag.Span)
		}
		for _, name := range varNames {
			origin := src.Origins[name]
			if origin.IsZero() {
				origin = src.Span
			}
			exposedBySource[sourceName][name] = origin
			candidatesByVar[name] = append(candidatesByVar[name], sourceCandidate{
				Source:    sourceName,
				SourceVar: name,
			})
		}
	}

	markUsedExact := func(sourceName, sourceVar string) {
		if _, ok := used[sourceName]; !ok {
			used[sourceName] = make(map[string]bool)
		}
		used[sourceName][sourceVar] = true
	}

	markUsedByImports := func(imports []importedVar) {
		for _, imp := range imports {
			sourceVar := imp.SourceVar
			if sourceVar == "" {
				sourceVar = imp.Name
			}
			markUsedExact(imp.Paramset, sourceVar)
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
		source := candidates[0].Source
		sourceVar := candidates[0].SourceVar
		if byVar, ok := exposedBySource[source]; ok {
			originSpan = byVar[sourceVar]
		}
		related := []diag.RelatedSpan{}
		if !originSpan.IsZero() {
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", source),
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
	processStep := func(stepName string, withItems []ast.WithItem, refs []varRef) {
		imports := resolveImportedVars(withItems, res.ImportSourceByName)
		if plan := res.StepImportByName[stepName]; plan != nil {
			imports = resolveImportedVarsFromPlan(plan)
		}
		processStepWithImports(stepName, imports, refs)
	}

	for _, block := range res.DoBlocks {
		base := block.BodyStart
		if base.Line == 0 {
			base = block.Span.Start
		}
		refs := collectShellLikeRefs(block.Body, base, block.Span.File)
		processStep(block.Name, block.WithItems, refs)
	}
	for _, block := range res.Submits {
		for _, useName := range block.UseNames {
			src := res.ImportSourceByName[useName]
			if src == nil || src.Kind != SourceKindLet {
				continue
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				if _, ok := allowedSubmitKeys[name]; !ok {
					continue
				}
				if isRawSubmitKey(name) {
					continue
				}
				markUsedExact(useName, name)
			}
		}

		imports := resolveImportedVars(block.WithItems, res.ImportSourceByName)
		if plan := res.StepImportByName[block.Name]; plan != nil {
			imports = resolveImportedVarsFromPlan(plan)
		}
		if spec := res.SubmitByName[block.Name]; spec != nil {
			for _, helper := range spec.Helpers {
				imports[helper.Original] = append(imports[helper.Original], importedVar{
					Name:      helper.Original,
					SourceVar: helper.Original,
					Paramset:  helper.UseName,
					Kind:      SourceKindLet,
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
	}
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.AnalyseBlock)
		if !ok {
			continue
		}
		imports := resolveImportedVars(block.WithItems, res.ImportSourceByName)
		for _, origins := range imports {
			for _, origin := range origins {
				if origin.Kind != SourceKindLet {
					continue
				}
				sourceVar := origin.SourceVar
				if sourceVar == "" {
					sourceVar = origin.Name
				}
				markUsedExact(origin.Paramset, sourceVar)
			}
		}
	}

	for sourceName, byVar := range exposedBySource {
		src := res.ImportSourceByName[sourceName]
		for varName, origin := range byVar {
			if used[sourceName][varName] {
				continue
			}
			message := fmt.Sprintf("exposed variable '%s' from param '%s' is never used in any do/submit block", varName, sourceName)
			hint := fmt.Sprintf("remove it from the final expression or reference it with $%s/${%s} in a step", varName, varName)
			if src != nil && src.Kind == SourceKindLet {
				message = fmt.Sprintf("exposed variable '%s' from let '%s' is never used in any do/submit/analyse block", varName, sourceName)
				hint = fmt.Sprintf("remove it from the let block or reference it with %s via with-imports", varName)
			}
			diags.AddWarning(
				diag.CodeW310,
				message,
				origin,
				hint,
			)
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
			for _, arg := range n.Args {
				walk(arg)
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
			for _, arg := range n.Args {
				walk(arg)
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

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
	SourceKey  BindingVersionKey
	Display    string
	ImportSpan diag.Span
}

type sourceCandidate struct {
	SourceKey BindingVersionKey
	Source    string
	Display   string
	SourceVar string
	Origin    diag.Span
}

type warningSource struct {
	Key        BindingVersionKey
	Name       string
	Display    string
	Span       diag.Span
	Order      []string
	VarOrigins map[string]diag.Span
	DependsOn  []BindingVersionKey
	depNames   []string
}

type warningCatalog struct {
	byKey      map[BindingVersionKey]*warningSource
	keyByExact map[string]BindingVersionKey
	order      []BindingVersionKey
}

func newWarningCatalog() *warningCatalog {
	return &warningCatalog{
		byKey:      make(map[BindingVersionKey]*warningSource),
		keyByExact: make(map[string]BindingVersionKey),
		order:      make([]BindingVersionKey, 0),
	}
}

func buildWarningCatalog(res *Result) *warningCatalog {
	catalog := newWarningCatalog()
	if res == nil {
		return catalog
	}
	for _, binding := range res.Bindings {
		catalog.addBinding(binding, "")
	}
	for _, name := range slices.Sorted(maps.Keys(res.BindingsByName)) {
		catalog.addBinding(res.BindingsByName[name], name)
	}
	for _, index := range slices.Sorted(maps.Keys(res.ScopeSnapshotsByIndex)) {
		catalog.addSnapshot(res.ScopeSnapshotsByIndex[index])
	}
	for _, key := range slices.Sorted(maps.Keys(res.ScopeSnapshotsByBlock)) {
		catalog.addSnapshot(res.ScopeSnapshotsByBlock[key])
	}
	catalog.finalizeDeps()
	return catalog
}

func (c *warningCatalog) addSnapshot(snap *ScopeSnapshot) {
	if c == nil || snap == nil {
		return
	}
	for _, binding := range snap.Bindings {
		c.addBinding(binding, "")
	}
	for _, name := range slices.Sorted(maps.Keys(snap.BindingsByName)) {
		c.addBinding(snap.BindingsByName[name], name)
	}
}

func (c *warningCatalog) addBinding(binding *GlobalBinding, fallback string) {
	if c == nil {
		return
	}
	if binding == nil {
		return
	}
	key := BindingVersionKeyForBinding(binding, fallback)
	if key == (BindingVersionKey{}) {
		return
	}
	exact := binding.Name
	if exact == "" {
		exact = fallback
	}
	if exact != "" {
		c.keyByExact[exact] = key
	}
	if _, exists := c.byKey[key]; exists {
		return
	}
	order := planutil.SourceVarNames(binding.Order, binding.Vars)
	if len(order) == 0 {
		return
	}
	c.byKey[key] = &warningSource{
		Key:        key,
		Name:       exact,
		Display:    key.Display(),
		Span:       binding.Span,
		Order:      order,
		VarOrigins: warningVarOrigins(binding, order),
		DependsOn:  append([]BindingVersionKey(nil), binding.DependsOnKeys...),
		depNames:   append([]string(nil), binding.DependsOn...),
	}
	c.order = append(c.order, key)
}

func (c *warningCatalog) finalizeDeps() {
	if c == nil {
		return
	}
	for _, key := range c.order {
		src := c.byKey[key]
		if src == nil || len(src.DependsOn) > 0 {
			continue
		}
		seen := make(map[BindingVersionKey]struct{}, len(src.depNames))
		for _, depName := range src.depNames {
			dep := c.keyForSource(nil, depName)
			if dep == (BindingVersionKey{}) || dep == key {
				continue
			}
			if _, exists := seen[dep]; exists {
				continue
			}
			seen[dep] = struct{}{}
			src.DependsOn = append(src.DependsOn, dep)
		}
		slices.SortFunc(src.DependsOn, compareBindingVersionKey)
	}
}

func (c *warningCatalog) sources() []warningSource {
	if c == nil {
		return nil
	}
	out := make([]warningSource, 0, len(c.order))
	for _, key := range c.order {
		src := c.byKey[key]
		if src == nil {
			continue
		}
		out = append(out, *src)
	}
	return out
}

func (c *warningCatalog) keyForSource(bindings map[string]*GlobalBinding, source string) BindingVersionKey {
	if source == "" {
		return BindingVersionKey{}
	}
	if binding := bindings[source]; binding != nil {
		return BindingVersionKeyForBinding(binding, source)
	}
	if c != nil {
		if key, ok := c.keyByExact[source]; ok {
			return key
		}
	}
	return BindingVersionKey{Public: source, Version: source}
}

func warningVarOrigins(binding *GlobalBinding, order []string) map[string]diag.Span {
	origins := make(map[string]diag.Span, len(order))
	for _, name := range order {
		origin := diag.Span{}
		if binding == nil {
			continue
		}
		origin = binding.Origins[name]
		if origin.IsZero() {
			origin = binding.Span
		}
		origins[name] = origin
	}
	return origins
}

func buildWarningSources(res *Result) []warningSource {
	return buildWarningCatalog(res).sources()
}

func buildGlobalSourceDeps(catalog *warningCatalog) map[BindingVersionKey][]BindingVersionKey {
	out := make(map[BindingVersionKey][]BindingVersionKey)
	seen := make(map[BindingVersionKey]map[BindingVersionKey]struct{})
	if catalog == nil {
		return out
	}
	for _, from := range catalog.order {
		src := catalog.byKey[from]
		if src == nil || len(src.DependsOn) == 0 {
			continue
		}
		for _, to := range src.DependsOn {
			if to == (BindingVersionKey{}) || to == from {
				continue
			}
			if catalog.byKey[to] == nil {
				continue
			}
			if _, ok := seen[from]; !ok {
				seen[from] = make(map[BindingVersionKey]struct{})
			}
			if _, ok := seen[from][to]; ok {
				continue
			}
			seen[from][to] = struct{}{}
			out[from] = append(out[from], to)
		}
	}
	for key := range out {
		slices.SortFunc(out[key], compareBindingVersionKey)
	}
	return out
}

type usedBySource map[BindingVersionKey]map[string]bool

func (u usedBySource) mark(key BindingVersionKey, sourceVar string) {
	if key == (BindingVersionKey{}) || sourceVar == "" {
		return
	}
	if _, ok := u[key]; !ok {
		u[key] = make(map[string]bool)
	}
	u[key][sourceVar] = true
}

func (u usedBySource) has(key BindingVersionKey, sourceVar string) bool {
	if key == (BindingVersionKey{}) || sourceVar == "" {
		return false
	}
	return u[key][sourceVar]
}

func cloneUsedBySource(used usedBySource) usedBySource {
	out := make(usedBySource, len(used))
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

func propagateUsedByGlobalDeps(used usedBySource, catalog *warningCatalog, deps map[BindingVersionKey][]BindingVersionKey) {
	if len(used) == 0 || len(deps) == 0 {
		return
	}
	queue := make([]BindingVersionKey, 0, len(used))
	seen := make(map[BindingVersionKey]bool, len(used))
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
			src := catalog.byKey[dep]
			if src == nil || len(src.Order) == 0 {
				continue
			}
			for _, varName := range src.Order {
				used.mark(dep, varName)
			}
			if !seen[dep] {
				seen[dep] = true
				queue = append(queue, dep)
			}
		}
	}
}

func versionImports(imports map[string][]importedVar, catalog *warningCatalog, bindings map[string]*GlobalBinding) map[string][]importedVar {
	if len(imports) == 0 {
		return map[string][]importedVar{}
	}
	out := make(map[string][]importedVar, len(imports))
	for visible, origins := range imports {
		for _, origin := range origins {
			sourceVar := origin.SourceVar
			if sourceVar == "" {
				sourceVar = origin.Name
			}
			key := origin.SourceKey
			if key == (BindingVersionKey{}) {
				key = catalog.keyForSource(bindings, origin.Source)
			}
			display := origin.Display
			if display == "" {
				display = key.Display()
			}
			origin.SourceVar = sourceVar
			origin.SourceKey = key
			origin.Display = display
			out[visible] = append(out[visible], origin)
		}
	}
	return out
}

func stepWarningCandidates(res *Result, catalog *warningCatalog, stepName string, snap *ScopeSnapshot) map[string][]sourceCandidate {
	candidates := make(map[string][]sourceCandidate)
	seen := make(map[string]struct{})
	addKey := func(key BindingVersionKey) {
		src := catalog.byKey[key]
		if src == nil {
			return
		}
		for _, name := range src.Order {
			dedupe := key.Public + "\x00" + key.Version + "\x00" + name
			if _, exists := seen[dedupe]; exists {
				continue
			}
			seen[dedupe] = struct{}{}
			candidates[name] = append(candidates[name], sourceCandidate{
				SourceKey: key,
				Source:    src.Name,
				Display:   src.Display,
				SourceVar: name,
				Origin:    src.VarOrigins[name],
			})
		}
	}
	addBinding := func(binding *GlobalBinding) {
		if binding == nil {
			return
		}
		addKey(BindingVersionKeyForBinding(binding, binding.Name))
	}
	addSource := func(bindings map[string]*GlobalBinding, source string) {
		addKey(catalog.keyForSource(bindings, source))
	}

	if snap != nil {
		for _, binding := range snap.Bindings {
			addBinding(binding)
		}
		if len(snap.Bindings) == 0 {
			for _, name := range slices.Sorted(maps.Keys(snap.BindingsByName)) {
				addSource(snap.BindingsByName, name)
			}
		}
	} else {
		for _, key := range catalog.order {
			addKey(key)
		}
	}
	if res != nil {
		if plan := res.StepScopeByName[stepName]; plan != nil {
			for _, name := range slices.Sorted(maps.Keys(plan.Inherited)) {
				origin := plan.Inherited[name]
				addSource(res.BindingsByName, origin.Source)
			}
		}
	}
	return candidates
}

func validateStepVarReferences(res *Result, diags *diag.Diagnostics) {
	catalog := buildWarningCatalog(res)
	warningSources := catalog.sources()
	used := make(usedBySource)
	stepUnused := make(map[string]stepUnusedImport)

	markUsedExact := func(bindings map[string]*GlobalBinding, source string, sourceVar string) {
		used.mark(catalog.keyForSource(bindings, source), sourceVar)
	}

	markUsedByImports := func(imports []importedVar) {
		for _, imp := range imports {
			sourceVar := imp.SourceVar
			if sourceVar == "" {
				sourceVar = imp.Name
			}
			key := imp.SourceKey
			if key == (BindingVersionKey{}) {
				key = catalog.keyForSource(nil, imp.Source)
			}
			used.mark(key, sourceVar)
		}
	}

	markUsedCandidates := func(candidates []sourceCandidate) {
		for _, cand := range candidates {
			used.mark(cand.SourceKey, cand.SourceVar)
		}
	}

	warnMissing := func(stepName string, ref varRef, candidates []sourceCandidate) {
		if len(candidates) == 0 {
			return
		}
		source := candidates[0]
		related := []diag.RelatedSpan{}
		if !source.Origin.IsZero() {
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", source.Display),
				Span:    source.Origin,
			})
		}
		diags.AddWarning(
			diag.CodeW311,
			fmt.Sprintf("variable '%s' is referenced in step '%s' but is not visible through `with` or predecessor inheritance", ref.Name, stepName),
			ref.Span,
			"add `with <source>` or `with <source>[<variable>]`, or inherit it from a predecessor via `after`",
			related...,
		)
	}

	processStepWithImports := func(stepName string, imports map[string][]importedVar, refs []varRef, candidatesByVar map[string][]sourceCandidate) {
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
	resolveEffectiveImports := func(stepName string, bindings map[string]*GlobalBinding) map[string][]importedVar {
		return versionImports(importsFromStepPlan(res.StepScopeByName[stepName]), catalog, bindings)
	}
	resolveExplicitImports := func(stepName string, bindings map[string]*GlobalBinding) map[string][]importedVar {
		return versionImports(explicitImportsFromStepPlan(res.StepScopeByName[stepName], bindings), catalog, bindings)
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
				sourceKey := origin.SourceKey
				if sourceKey == (BindingVersionKey{}) {
					sourceKey = catalog.keyForSource(nil, origin.Source)
				}
				key := stepName + "::" + visible + "::" + sourceKey.Public + "::" + sourceKey.Version + "::" + sourceVar
				if _, ok := stepUnused[key]; ok {
					continue
				}
				stepUnused[key] = stepUnusedImport{
					StepName:   stepName,
					Visible:    visible,
					Source:     origin.Source,
					SourceVar:  sourceVar,
					SourceKey:  sourceKey,
					Display:    origin.Display,
					ImportSpan: origin.Span,
				}
			}
		}
	}

	for _, block := range res.DoBlocks {
		snap := snapshotForDoBlock(res, block)
		bindings := snapshotBindingsWithResult(res, snap)
		base := block.BodyStart
		if base.Line == 0 {
			base = block.Span.Start
		}
		refs := collectShellLikeRefs(block.Body, base, block.Span.File)
		effectiveImports := resolveEffectiveImports(block.Name, bindings)
		candidatesByVar := stepWarningCandidates(res, catalog, block.Name, snap)
		processStepWithImports(block.Name, effectiveImports, refs, candidatesByVar)
		explicitImports := resolveExplicitImports(block.Name, bindings)
		collectStepUnusedImports(block.Name, explicitImports, refs)
	}
	for _, block := range res.Submits {
		snap := snapshotForSubmitBlock(res, block)
		bindings := snapshotBindingsWithResult(res, snap)
		namespaces := snapshotNamespaces(res, snap)
		for _, useName := range block.UseNames {
			markSubmitUseBindingRefs(bindings, namespaces, useName, func(binding *GlobalBinding, sourceVar string) {
				if binding == nil {
					return
				}
				used.mark(BindingVersionKeyForBinding(binding, binding.Name), sourceVar)
			})
		}

		imports := resolveEffectiveImports(block.Name, bindings)
		explicitImports := resolveExplicitImports(block.Name, bindings)
		if spec := res.SubmitByName[block.Name]; spec != nil {
			for _, helper := range spec.Helpers {
				source := helper.Source
				if source == "" {
					source = helper.UseName
					if binding := bindings[helper.UseName+"."+helper.Original]; binding != nil {
						source = binding.Name
					}
				}
				sourceVar := helper.SourceVar
				if sourceVar == "" {
					sourceVar = helper.Original
				}
				sourceKey := catalog.keyForSource(bindings, source)
				imports[helper.Original] = append(imports[helper.Original], importedVar{
					Name:      helper.Original,
					SourceVar: sourceVar,
					Source:    source,
					SourceKey: sourceKey,
					Display:   helper.UseName,
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
		candidatesByVar := stepWarningCandidates(res, catalog, block.Name, snap)
		processStepWithImports(block.Name, imports, refs, candidatesByVar)
		collectStepUnusedImports(block.Name, explicitImports, refs)
	}
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.AnalyseBlock)
		if !ok {
			continue
		}
		snap := snapshotForAnalyseBlock(res, block)
		bindings := snapshotBindings(res, snap)
		imports := resolveAnalyseImportsCanonical(block.WithItems, bindings, snapshotGlobals(res, snap), snapshotNamespaces(res, snap), nil, analyseImportOptions{
			EmitDiagnostics: false,
		})
		for _, origin := range imports {
			markUsedExact(bindings, origin.Source, origin.SourceVar)
		}
	}
	usedForW310 := cloneUsedBySource(used)
	propagateUsedByGlobalDeps(usedForW310, catalog, buildGlobalSourceDeps(catalog))

	for _, src := range warningSources {
		for _, varName := range src.Order {
			if usedForW310.has(src.Key, varName) {
				continue
			}
			message := fmt.Sprintf("exposed variable '%s' from global '%s' is never used in any do/submit/analyse block", varName, src.Display)
			hint := fmt.Sprintf("remove it from the global binding or reference it with %s via imports", varName)
			diags.AddWarning(
				diag.CodeW310,
				message,
				src.VarOrigins[varName],
				hint,
			)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(stepUnused)) {
		item := stepUnused[key]
		if item.SourceKey == (BindingVersionKey{}) || item.SourceVar == "" {
			continue
		}
		if !used.has(item.SourceKey, item.SourceVar) {
			continue
		}
		sourceSpan := diag.Span{}
		if src := catalog.byKey[item.SourceKey]; src != nil {
			sourceSpan = src.VarOrigins[item.SourceVar]
		}
		span := item.ImportSpan
		if span.IsZero() {
			span = sourceSpan
		}
		related := []diag.RelatedSpan{}
		if !sourceSpan.IsZero() {
			display := item.Display
			if display == "" {
				display = item.SourceKey.Display()
			}
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", display),
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

func markSubmitUseBindingRefs(bindings map[string]*GlobalBinding, namespaces map[string]*Namespace, useName string, mark func(*GlobalBinding, string)) {
	if binding := bindings[useName]; binding != nil {
		for _, name := range planutil.SourceVarNames(binding.Order, binding.Vars) {
			mark(binding, name)
		}
		return
	}
	ns := namespaces[useName]
	if ns == nil {
		return
	}
	for _, bindingName := range ns.Bindings {
		rest := strings.TrimPrefix(bindingName, useName+".")
		if rest == bindingName || strings.Contains(rest, ".") {
			continue
		}
		binding := bindings[bindingName]
		if binding == nil || !binding.Supports(ImportIntoSubmitUse) {
			continue
		}
		for _, name := range planutil.SourceVarNames(binding.Order, binding.Vars) {
			mark(binding, name)
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
		case ast.MemberExpr:
			walk(n.Base)
		case ast.ListExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.TupleExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg.Expr)
			}
		case ast.FunctionExpr:
			for _, param := range n.Params {
				walk(param.Default)
			}
			walkFuncBodyIdentRefs(n.Body, walk)
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

func walkFuncBodyIdentRefs(body []ast.FuncBodyStmt, walk func(ast.Expr)) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			walk(node.Expr)
		case ast.ReturnStmt:
			walk(node.Expr)
		case ast.ExprStmt:
			walk(node.Expr)
		case ast.FuncIfStmt:
			walk(node.Cond)
			walkFuncBodyIdentRefs(node.Then, walk)
			walkFuncBodyIdentRefs(node.Else, walk)
		}
	}
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
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg.Expr)
			}
		case ast.FunctionExpr:
			for _, param := range n.Params {
				walk(param.Default)
			}
			walkFuncBodyStringRefs(n.Body, walk)
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

func walkFuncBodyStringRefs(body []ast.FuncBodyStmt, walk func(ast.Expr)) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			walk(node.Expr)
		case ast.ReturnStmt:
			walk(node.Expr)
		case ast.ExprStmt:
			walk(node.Expr)
		case ast.FuncIfStmt:
			walk(node.Cond)
			walkFuncBodyStringRefs(node.Then, walk)
			walkFuncBodyStringRefs(node.Else, walk)
		}
	}
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

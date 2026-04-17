// implement `use <file>` and `use <name> from <file>` jbs imports
//
// resolve embedded and local file modules, parse them, expand
// selective imports, detect import cycles/collisions, and return a
// single expanded program plus per-file source text for diagnostics
package imports

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/parser"
	"jbs/shared"
)

type LoadResult struct {
	Program ast.Program
	Sources map[string]string
	Entry   ModuleRef
	Aliases map[string]ModuleRef
	Modules map[string]*ModuleInfo
}

type ModuleRef struct {
	ID    string
	Label string
}

type ExportKind string

const (
	ExportGlobal ExportKind = "global"
	ExportDo     ExportKind = "do"
	ExportSubmit ExportKind = "submit"
	ExportOther  ExportKind = "other"
)

type ModuleExport struct {
	Name       string
	Kind       ExportKind
	Stmt       ast.Stmt
	Span       diag.Span
	ModuleID   string
	Importable bool
	Imported   bool
}

type ModuleInfo struct {
	Ref     ModuleRef
	BaseDir string
	Program ast.Program
	Exports map[string]ModuleExport
	Aliases map[string]ModuleRef
}

type rawModule struct {
	Ref     ModuleRef
	Source  string
	Program ast.Program
	// BaseDir anchors quoted-path imports from this module.
	BaseDir string
}

type expandedModule struct {
	Ref ModuleRef
	// BaseDir is preserved during expansion for importer-relative path resolution.
	BaseDir string
	Stmts   []ast.Stmt
	Exports map[string]ModuleExport
	Aliases map[string]ModuleRef
}

type resolver struct {
	cwd       string
	diags     *diag.Diagnostics
	raw       map[string]*rawModule
	expanded  map[string]*expandedModule
	expanding map[string]bool
	sources   map[string]string
}

func LoadAndExpand(entryPath string, cwd string, diags *diag.Diagnostics) (*LoadResult, error) {
	r, err := newResolver(cwd, diags)
	if err != nil {
		return nil, err
	}
	entryAbs := entryPath
	if !filepath.IsAbs(entryAbs) {
		entryAbs = filepath.Join(r.cwd, entryPath)
	}
	entryAbs = filepath.Clean(entryAbs)
	entryRef, err := r.loadFileModule(entryAbs)
	if err != nil {
		return nil, err
	}
	return r.loadResult(entryRef)
}

// LoadAndExpandSource expands `use` imports for an in-memory entry module.
//
// The source is parsed under entryLabel and uses baseDir as importer base for
// quoted path imports. Bare module resolution still uses cwd.
func LoadAndExpandSource(entryLabel string, source string, baseDir string, cwd string, diags *diag.Diagnostics) (*LoadResult, error) {
	r, err := newResolver(cwd, diags)
	if err != nil {
		return nil, err
	}
	entryRef, err := r.loadSourceModule(entryLabel, source, baseDir)
	if err != nil {
		return nil, err
	}
	return r.loadResult(entryRef)
}

func newResolver(cwd string, diags *diag.Diagnostics) (*resolver, error) {
	if diags == nil {
		diags = &diag.Diagnostics{}
	}
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd = wd
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	return &resolver{
		cwd:       absCwd,
		diags:     diags,
		raw:       make(map[string]*rawModule),
		expanded:  make(map[string]*expandedModule),
		expanding: make(map[string]bool),
		sources:   make(map[string]string),
	}, nil
}

func (r *resolver) loadResult(entryRef ModuleRef) (*LoadResult, error) {
	expanded := r.expandModule(entryRef)
	if expanded == nil {
		return nil, fmt.Errorf("failed to expand entry module")
	}
	r.expandAliasClosure(entryRef, map[string]struct{}{})
	prog := ast.Program{File: entryRef.Label, Stmts: append([]ast.Stmt(nil), expanded.Stmts...)}
	if len(prog.Stmts) > 0 {
		prog.Span = diag.Merge(prog.Stmts[0].GetSpan(), prog.Stmts[len(prog.Stmts)-1].GetSpan())
	}
	outSources := make(map[string]string, len(r.sources))
	for k, v := range r.sources {
		outSources[k] = v
	}
	modules := make(map[string]*ModuleInfo, len(r.expanded))
	for id, mod := range r.expanded {
		if mod == nil {
			continue
		}
		moduleProg := ast.Program{File: mod.Ref.Label, Stmts: append([]ast.Stmt(nil), mod.Stmts...)}
		if len(moduleProg.Stmts) > 0 {
			moduleProg.Span = diag.Merge(moduleProg.Stmts[0].GetSpan(), moduleProg.Stmts[len(moduleProg.Stmts)-1].GetSpan())
		}
		exports := make(map[string]ModuleExport, len(mod.Exports))
		for name, decl := range mod.Exports {
			exports[name] = decl
		}
		aliases := make(map[string]ModuleRef, len(mod.Aliases))
		for alias, ref := range mod.Aliases {
			aliases[alias] = ref
		}
		modules[id] = &ModuleInfo{
			Ref:     mod.Ref,
			BaseDir: mod.BaseDir,
			Program: moduleProg,
			Exports: exports,
			Aliases: aliases,
		}
	}
	aliases := make(map[string]ModuleRef, len(expanded.Aliases))
	for alias, ref := range expanded.Aliases {
		aliases[alias] = ref
	}
	return &LoadResult{
		Program: prog,
		Sources: outSources,
		Entry:   entryRef,
		Aliases: aliases,
		Modules: modules,
	}, nil
}

func (r *resolver) expandAliasClosure(ref ModuleRef, seen map[string]struct{}) {
	if r == nil || ref.ID == "" {
		return
	}
	if _, ok := seen[ref.ID]; ok {
		return
	}
	seen[ref.ID] = struct{}{}
	mod := r.expandModule(ref)
	if mod == nil {
		return
	}
	for _, alias := range mod.Aliases {
		r.expandAliasClosure(alias, seen)
	}
}

func (r *resolver) loadSourceModule(label string, source string, baseDir string) (ModuleRef, error) {
	entryLabel := strings.TrimSpace(label)
	if entryLabel == "" {
		entryLabel = "<source>"
	}
	normBaseDir := strings.TrimSpace(baseDir)
	if normBaseDir == "" {
		normBaseDir = r.cwd
	}
	if !filepath.IsAbs(normBaseDir) {
		normBaseDir = filepath.Join(r.cwd, normBaseDir)
	}
	normBaseDir = filepath.Clean(normBaseDir)

	id := "source:" + entryLabel
	if raw, ok := r.raw[id]; ok {
		return raw.Ref, nil
	}
	prog := parser.Parse(entryLabel, source, r.diags)
	ref := ModuleRef{ID: id, Label: entryLabel}
	r.raw[id] = &rawModule{
		Ref:     ref,
		Source:  source,
		Program: prog,
		BaseDir: normBaseDir,
	}
	r.sources[entryLabel] = source
	return ref, nil
}

func (r *resolver) loadFileModule(absPath string) (ModuleRef, error) {
	absPath = filepath.Clean(absPath)
	id := "file:" + absPath
	label := absPath
	if raw, ok := r.raw[id]; ok {
		return raw.Ref, nil
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return ModuleRef{}, err
	}
	src := string(data)
	prog := parser.Parse(label, src, r.diags)
	ref := ModuleRef{ID: id, Label: label}
	r.raw[id] = &rawModule{
		Ref:     ref,
		Source:  src,
		Program: prog,
		BaseDir: filepath.Dir(absPath),
	}
	r.sources[label] = src
	return ref, nil
}

func (r *resolver) loadEmbeddedModule(name string) (ModuleRef, error) {
	norm := normalizeEmbeddedName(name)
	id := "embed:" + norm
	label := filepath.ToSlash(filepath.Join("shared", norm))
	if raw, ok := r.raw[id]; ok {
		return raw.Ref, nil
	}
	src, err := shared.Read(norm)
	if err != nil {
		return ModuleRef{}, err
	}
	prog := parser.Parse(label, src, r.diags)
	ref := ModuleRef{ID: id, Label: label}
	r.raw[id] = &rawModule{
		Ref:     ref,
		Source:  src,
		Program: prog,
		BaseDir: r.cwd,
	}
	r.sources[label] = src
	return ref, nil
}

func normalizeEmbeddedName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	if !strings.HasSuffix(n, ".jbs") {
		n += ".jbs"
	}
	return n
}

func (r *resolver) resolveBareModule(name string) (ModuleRef, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return ModuleRef{}, fmt.Errorf("empty module name")
	}
	if shared.Has(n) {
		return r.loadEmbeddedModule(n)
	}
	local := filepath.Join(r.cwd, n+".jbs")
	return r.loadFileModule(local)
}

func (r *resolver) resolvePathModule(path string, importerBaseDir string, at diag.Span) (ModuleRef, error) {
	p := strings.TrimSpace(path)
	if !strings.HasSuffix(p, ".jbs") {
		r.diags.AddError(
			diag.CodeE535,
			fmt.Sprintf("quoted use path '%s' must end with .jbs", path),
			at,
			"use syntax: use \"path/to/file.jbs\" as alias or use name from \"path/to/file.jbs\"",
		)
		return ModuleRef{}, fmt.Errorf("invalid .jbs path: %s", path)
	}
	abs := p
	if !filepath.IsAbs(abs) {
		base := importerBaseDir
		if strings.TrimSpace(base) == "" {
			base = r.cwd
		}
		abs = filepath.Join(base, abs)
	}
	abs = filepath.Clean(abs)
	return r.loadFileModule(abs)
}

func (r *resolver) resolveUseSource(current *expandedModule, src ast.UseSource) (ModuleRef, error) {
	switch src.Kind {
	case ast.UseSourcePath:
		baseDir := r.cwd
		if current != nil && strings.TrimSpace(current.BaseDir) != "" {
			baseDir = current.BaseDir
		}
		return r.resolvePathModule(src.Value, baseDir, src.Span)
	case ast.UseSourceBare:
		if current != nil {
			if ref, ok := current.Aliases[src.Value]; ok {
				return ref, nil
			}
		}
		return r.resolveBareModule(src.Value)
	default:
		return ModuleRef{}, fmt.Errorf("unknown use source kind")
	}
}

func (r *resolver) expandModule(ref ModuleRef) *expandedModule {
	if exp, ok := r.expanded[ref.ID]; ok {
		return exp
	}
	if r.expanding[ref.ID] {
		r.diags.AddError(
			diag.CodeE530,
			fmt.Sprintf("module import cycle detected involving '%s'", ref.Label),
			diag.NewSpan(ref.Label, diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 1)),
			"break the module cycle by removing one of the recursive imports",
		)
		return nil
	}
	raw := r.raw[ref.ID]
	if raw == nil {
		return nil
	}
	r.expanding[ref.ID] = true
	defer delete(r.expanding, ref.ID)

	mod := &expandedModule{
		Ref:     ref,
		BaseDir: raw.BaseDir,
		Stmts:   make([]ast.Stmt, 0, len(raw.Program.Stmts)),
		Exports: make(map[string]ModuleExport),
		Aliases: make(map[string]ModuleRef),
	}
	inserted := make(map[string]struct{})

	for _, stmt := range raw.Program.Stmts {
		if useStmt, ok := stmt.(ast.UseStmt); ok {
			r.processUseStmt(mod, useStmt, inserted)
			continue
		}
		r.addLocalStmt(mod, stmt)
	}

	r.expanded[ref.ID] = mod
	return mod
}

func (r *resolver) processUseStmt(mod *expandedModule, stmt ast.UseStmt, inserted map[string]struct{}) {
	if len(stmt.Names) == 0 {
		ref, err := r.resolveUseSource(mod, stmt.Source)
		if err != nil {
			r.diags.AddError(
				diag.CodeE531,
				fmt.Sprintf("failed to resolve module '%s': %v", stmt.Source.Value, err),
				stmt.Span,
				"check module name/path and file availability",
			)
			return
		}
		alias := stmt.Alias
		if alias == "" {
			alias = stmt.Source.Value
		}
		if prev, ok := mod.Aliases[alias]; ok {
			if prev.ID != ref.ID {
				r.diags.AddError(
					diag.CodeE536,
					fmt.Sprintf("import alias collision for '%s'", alias),
					stmt.Span,
					"use a unique alias name",
				)
			}
			return
		}
		if sym, ok := mod.Exports[alias]; ok {
			r.diags.AddError(
				diag.CodeE534,
				fmt.Sprintf("import name collision: alias '%s' conflicts with symbol '%s'", alias, sym.Name),
				stmt.Span,
				"rename alias or conflicting symbol",
				diag.RelatedSpan{Message: "conflicting symbol", Span: sym.Span},
			)
			return
		}
		mod.Aliases[alias] = ref
		return
	}

	sourceRef, err := r.resolveUseSource(mod, stmt.Source)
	if err != nil {
		r.diags.AddError(
			diag.CodeE531,
			fmt.Sprintf("failed to resolve import source '%s': %v", stmt.Source.Value, err),
			stmt.Span,
			"check module name/path and file availability",
		)
		return
	}
	source := r.expandModule(sourceRef)
	if source == nil {
		return
	}
	for _, name := range stmt.Names {
		r.importSymbol(mod, source, name, stmt.Span, inserted, make(map[string]struct{}))
	}
}

func (r *resolver) addLocalStmt(mod *expandedModule, stmt ast.Stmt) {
	name, kind, importable, ok := stmtExport(stmt)
	if ok {
		if _, aliasExists := mod.Aliases[name]; aliasExists {
			r.diags.AddError(
				diag.CodeE534,
				fmt.Sprintf("import name collision: local symbol '%s' conflicts with import alias", name),
				stmt.GetSpan(),
				"rename symbol or alias",
			)
		} else if prev, exists := mod.Exports[name]; exists {
			if prev.Imported {
				r.diags.AddError(
					diag.CodeE534,
					fmt.Sprintf("import name collision: local symbol '%s' conflicts with imported symbol", name),
					stmt.GetSpan(),
					"rename local symbol or adjust imports",
					diag.RelatedSpan{Message: "imported symbol", Span: prev.Span},
				)
			}
		} else {
			mod.Exports[name] = ModuleExport{
				Name:       name,
				Kind:       kind,
				Stmt:       stmt,
				Span:       stmt.GetSpan(),
				ModuleID:   mod.Ref.ID,
				Importable: importable,
				Imported:   false,
			}
		}
	}
	mod.Stmts = append(mod.Stmts, stmt)
}

func (r *resolver) importSymbol(target *expandedModule, source *expandedModule, name string, at diag.Span, inserted map[string]struct{}, visiting map[string]struct{}) {
	sym, ok := source.Exports[name]
	if !ok {
		r.diags.AddError(
			diag.CodeE532,
			fmt.Sprintf("unknown symbol '%s' in module '%s'", name, source.Ref.Label),
			at,
			"import a symbol that exists in the source module",
		)
		return
	}
	if !sym.Importable {
		r.diags.AddError(
			diag.CodeE533,
			fmt.Sprintf("symbol '%s' in module '%s' is not importable", name, source.Ref.Label),
			at,
			"only globals, do blocks, and submit blocks are importable",
		)
		return
	}
	key := source.Ref.ID + "::" + name
	if _, ok := inserted[key]; ok {
		return
	}
	if _, ok := visiting[key]; ok {
		return
	}
	visiting[key] = struct{}{}
	deps := r.symbolDependencies(source, sym.Stmt)
	for _, dep := range deps {
		depSource := r.expandModule(dep.Source)
		if depSource == nil {
			continue
		}
		r.importSymbol(target, depSource, dep.Name, dep.Span, inserted, visiting)
	}
	delete(visiting, key)

	if !r.addImportedSymbol(target, source, sym, at) {
		return
	}
	inserted[key] = struct{}{}
	target.Stmts = append(target.Stmts, sym.Stmt)
}

func (r *resolver) addImportedSymbol(target *expandedModule, source *expandedModule, sym ModuleExport, at diag.Span) bool {
	if _, aliasExists := target.Aliases[sym.Name]; aliasExists {
		r.diags.AddError(
			diag.CodeE534,
			fmt.Sprintf("import name collision: symbol '%s' conflicts with alias", sym.Name),
			at,
			"rename alias or imported symbol",
			diag.RelatedSpan{Message: "imported symbol", Span: sym.Span},
		)
		return false
	}
	if prev, exists := target.Exports[sym.Name]; exists {
		if prev.ModuleID == source.Ref.ID && prev.Name == sym.Name {
			return false
		}
		r.diags.AddError(
			diag.CodeE534,
			fmt.Sprintf("import name collision: '%s' from '%s' conflicts with existing declaration", sym.Name, source.Ref.Label),
			at,
			"rename symbols or adjust imports",
			diag.RelatedSpan{Message: "existing declaration", Span: prev.Span},
			diag.RelatedSpan{Message: "imported declaration", Span: sym.Span},
		)
		return false
	}
	target.Exports[sym.Name] = ModuleExport{
		Name:       sym.Name,
		Kind:       sym.Kind,
		Stmt:       sym.Stmt,
		Span:       sym.Span,
		ModuleID:   source.Ref.ID,
		Importable: sym.Importable,
		Imported:   true,
	}
	return true
}

type depRef struct {
	Source ModuleRef
	Name   string
	Span   diag.Span
}

func (r *resolver) symbolDependencies(mod *expandedModule, stmt ast.Stmt) []depRef {
	deps := make([]depRef, 0)
	add := func(source ModuleRef, name string, span diag.Span) {
		if name == "" || source.ID == "" {
			return
		}
		for _, dep := range deps {
			if dep.Source.ID == source.ID && dep.Name == name {
				return
			}
		}
		deps = append(deps, depRef{Source: source, Name: name, Span: span})
	}
	resolveName := func(name string) (ModuleRef, string, bool) {
		name = strings.TrimSpace(name)
		if name == "" {
			return ModuleRef{}, "", false
		}
		if alias, tail, ok := strings.Cut(name, "."); ok && alias != "" && tail != "" {
			if ref, exists := mod.Aliases[alias]; exists {
				return ref, tail, true
			}
		}
		if sym, ok := mod.Exports[name]; ok {
			ref, exists := r.moduleRefByID(sym.ModuleID)
			if !exists {
				return ModuleRef{}, "", false
			}
			return ref, sym.Name, true
		}
		return ModuleRef{}, "", false
	}

	addExprDeps := func(expr ast.Expr) {}
	addExprDeps = func(expr ast.Expr) {
		if expr == nil {
			return
		}
		switch n := expr.(type) {
		case ast.IdentExpr:
			if ref, depName, ok := resolveName(n.Name); ok {
				add(ref, depName, n.Span)
			}
		case ast.QualifiedIdentExpr:
			if ref, depName, ok := resolveName(n.Namespace + "." + n.Name); ok {
				add(ref, depName, n.Span)
			} else if ref, depName, ok := resolveName(n.Namespace); ok {
				add(ref, depName, n.Span)
			}
		case ast.ModeExpr:
			addExprDeps(n.Expr)
		case ast.ListExpr:
			for _, it := range n.Items {
				addExprDeps(it)
			}
		case ast.TupleExpr:
			for _, it := range n.Items {
				addExprDeps(it)
			}
		case ast.ConvertExpr:
			addExprDeps(n.Expr)
		case ast.CallExpr:
			addExprDeps(n.Callee)
			for _, arg := range n.Args {
				addExprDeps(arg)
			}
		case ast.AliasExpr:
			addExprDeps(n.Expr)
		case ast.IndexExpr:
			addExprDeps(n.Base)
			for _, item := range n.Items {
				addExprDeps(item)
			}
		case ast.UnaryExpr:
			addExprDeps(n.Expr)
		case ast.BinaryExpr:
			addExprDeps(n.Left)
			addExprDeps(n.Right)
		case ast.CompareExpr:
			addExprDeps(n.Left)
			addExprDeps(n.Right)
		case ast.ConditionalExpr:
			addExprDeps(n.Then)
			addExprDeps(n.Cond)
			addExprDeps(n.Else)
		}
	}

	importWithItems := func(items []ast.WithItem) {
		for _, item := range items {
			if item.SourceExpr != "" && len(item.SourceSlice) > 0 {
				if ref, depName, ok := resolveName(item.SourceExpr); ok {
					add(ref, depName, item.Span)
					continue
				}
				for _, sel := range item.SourceSlice {
					if ref, depName, ok := resolveName(sel); ok {
						add(ref, depName, item.Span)
					}
				}
				continue
			}
			if item.From == "" {
				if ref, depName, ok := resolveName(item.Name); ok {
					add(ref, depName, item.Span)
				}
				continue
			}
			if ref, depName, ok := resolveName(item.From); ok {
				add(ref, depName, item.Span)
				continue
			}
			if ref, depName, ok := resolveName(item.Name); ok {
				add(ref, depName, item.Span)
			}
		}
	}

	switch n := stmt.(type) {
	case ast.GlobalAssign:
		addExprDeps(n.Expr)
	case ast.DoBlock:
		for _, dep := range n.After {
			if ref, depName, ok := resolveName(dep); ok {
				add(ref, depName, n.Span)
			}
		}
		importWithItems(n.WithItems)
	case ast.SubmitBlock:
		for _, dep := range n.After {
			if ref, depName, ok := resolveName(dep); ok {
				add(ref, depName, n.Span)
			}
		}
		for _, useName := range n.UseNames {
			if ref, depName, ok := resolveName(useName); ok {
				add(ref, depName, n.Span)
			}
		}
		importWithItems(n.WithItems)
	case ast.AnalyseBlock:
		importWithItems(n.WithItems)
	}

	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Source.ID == deps[j].Source.ID {
			return deps[i].Name < deps[j].Name
		}
		return deps[i].Source.ID < deps[j].Source.ID
	})
	return deps
}

func (r *resolver) moduleRefByID(id string) (ModuleRef, bool) {
	if raw, ok := r.raw[id]; ok && raw != nil {
		return raw.Ref, true
	}
	if exp, ok := r.expanded[id]; ok && exp != nil {
		return exp.Ref, true
	}
	return ModuleRef{}, false
}

func stmtExport(stmt ast.Stmt) (string, ExportKind, bool, bool) {
	switch n := stmt.(type) {
	case ast.DoBlock:
		return n.Name, ExportDo, true, true
	case ast.SubmitBlock:
		return n.Name, ExportSubmit, true, true
	case ast.GlobalAssign:
		return n.Name, ExportGlobal, true, true
	case ast.AnalyseBlock:
		return n.StepName, ExportOther, true, false
	default:
		return "", ExportOther, false, false
	}
}

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
}

type moduleRef struct {
	ID    string
	Label string
}

type symbolKind string

const (
	symbolKindLet    symbolKind = "let"
	symbolKindParam  symbolKind = "param"
	symbolKindDo     symbolKind = "do"
	symbolKindSubmit symbolKind = "submit"
	symbolKindGlobal symbolKind = "global"
	symbolKindOther  symbolKind = "other"
)

type symbolDecl struct {
	Name       string
	Kind       symbolKind
	Stmt       ast.Stmt
	Span       diag.Span
	ModuleID   string
	Importable bool
	Imported   bool
}

type rawModule struct {
	Ref     moduleRef
	Source  string
	Program ast.Program
	// BaseDir anchors quoted-path imports from this module.
	BaseDir string
}

type expandedModule struct {
	Ref moduleRef
	// BaseDir is preserved during expansion for importer-relative path resolution.
	BaseDir string
	Stmts   []ast.Stmt
	Symbols map[string]symbolDecl
	Aliases map[string]moduleRef
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
	entryAbs := entryPath
	if !filepath.IsAbs(entryAbs) {
		entryAbs = filepath.Join(absCwd, entryPath)
	}
	entryAbs = filepath.Clean(entryAbs)

	r := &resolver{
		cwd:       absCwd,
		diags:     diags,
		raw:       make(map[string]*rawModule),
		expanded:  make(map[string]*expandedModule),
		expanding: make(map[string]bool),
		sources:   make(map[string]string),
	}
	entryRef, err := r.loadFileModule(entryAbs)
	if err != nil {
		return nil, err
	}
	expanded := r.expandModule(entryRef)
	if expanded == nil {
		return nil, fmt.Errorf("failed to expand entry module")
	}
	prog := ast.Program{File: entryRef.Label, Stmts: append([]ast.Stmt(nil), expanded.Stmts...)}
	if len(prog.Stmts) > 0 {
		prog.Span = diag.Merge(prog.Stmts[0].GetSpan(), prog.Stmts[len(prog.Stmts)-1].GetSpan())
	}
	outSources := make(map[string]string, len(r.sources))
	for k, v := range r.sources {
		outSources[k] = v
	}
	return &LoadResult{Program: prog, Sources: outSources}, nil
}

func (r *resolver) loadFileModule(absPath string) (moduleRef, error) {
	absPath = filepath.Clean(absPath)
	id := "file:" + absPath
	label := absPath
	if raw, ok := r.raw[id]; ok {
		return raw.Ref, nil
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return moduleRef{}, err
	}
	src := string(data)
	prog := parser.Parse(label, src, r.diags)
	ref := moduleRef{ID: id, Label: label}
	r.raw[id] = &rawModule{
		Ref:     ref,
		Source:  src,
		Program: prog,
		BaseDir: filepath.Dir(absPath),
	}
	r.sources[label] = src
	return ref, nil
}

func (r *resolver) loadEmbeddedModule(name string) (moduleRef, error) {
	norm := normalizeEmbeddedName(name)
	id := "embed:" + norm
	label := filepath.ToSlash(filepath.Join("shared", norm))
	if raw, ok := r.raw[id]; ok {
		return raw.Ref, nil
	}
	src, err := shared.Read(norm)
	if err != nil {
		return moduleRef{}, err
	}
	prog := parser.Parse(label, src, r.diags)
	ref := moduleRef{ID: id, Label: label}
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

func (r *resolver) resolveBareModule(name string) (moduleRef, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return moduleRef{}, fmt.Errorf("empty module name")
	}
	if shared.Has(n) {
		return r.loadEmbeddedModule(n)
	}
	local := filepath.Join(r.cwd, n+".jbs")
	return r.loadFileModule(local)
}

func (r *resolver) resolvePathModule(path string, importerBaseDir string, at diag.Span) (moduleRef, error) {
	p := strings.TrimSpace(path)
	if !strings.HasSuffix(p, ".jbs") {
		r.diags.AddError(
			"E535",
			fmt.Sprintf("quoted use path '%s' must end with .jbs", path),
			at,
			"use syntax: use \"path/to/file.jbs\" as alias or use name from \"path/to/file.jbs\"",
		)
		return moduleRef{}, fmt.Errorf("invalid .jbs path: %s", path)
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

func (r *resolver) resolveUseSource(current *expandedModule, src ast.UseSource) (moduleRef, error) {
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
		return moduleRef{}, fmt.Errorf("unknown use source kind")
	}
}

func (r *resolver) expandModule(ref moduleRef) *expandedModule {
	if exp, ok := r.expanded[ref.ID]; ok {
		return exp
	}
	if r.expanding[ref.ID] {
		r.diags.AddError(
			"E530",
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
		Symbols: make(map[string]symbolDecl),
		Aliases: make(map[string]moduleRef),
	}
	inserted := make(map[string]struct{})

	for _, stmt := range raw.Program.Stmts {
		if useStmt, ok := stmt.(ast.UseStmt); ok {
			r.processUseStmt(mod, useStmt, inserted)
			continue
		}
		stmt = r.normalizeWithRefs(mod, stmt, inserted)
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
				"E531",
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
					"E536",
					fmt.Sprintf("import alias collision for '%s'", alias),
					stmt.Span,
					"use a unique alias name",
				)
			}
			return
		}
		if sym, ok := mod.Symbols[alias]; ok {
			r.diags.AddError(
				"E534",
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
			"E531",
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
	name, kind, importable, ok := stmtSymbol(stmt)
	if ok {
		if _, aliasExists := mod.Aliases[name]; aliasExists {
			r.diags.AddError(
				"E534",
				fmt.Sprintf("import name collision: local symbol '%s' conflicts with import alias", name),
				stmt.GetSpan(),
				"rename symbol or alias",
			)
		} else if prev, exists := mod.Symbols[name]; exists {
			if prev.Imported {
				r.diags.AddError(
					"E534",
					fmt.Sprintf("import name collision: local symbol '%s' conflicts with imported symbol", name),
					stmt.GetSpan(),
					"rename local symbol or adjust imports",
					diag.RelatedSpan{Message: "imported symbol", Span: prev.Span},
				)
			}
		} else {
			mod.Symbols[name] = symbolDecl{
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

func (r *resolver) normalizeWithRefs(mod *expandedModule, stmt ast.Stmt, inserted map[string]struct{}) ast.Stmt {
	normalizeItems := func(items []ast.WithItem) []ast.WithItem {
		if len(items) == 0 {
			return items
		}
		out := make([]ast.WithItem, len(items))
		for i, item := range items {
			n := item
			n.Name = r.normalizeWithRef(mod, item.Name, item.Span, inserted)
			if item.From != "" {
				n.From = r.normalizeWithRef(mod, item.From, item.Span, inserted)
			}
			out[i] = n
		}
		return out
	}

	switch n := stmt.(type) {
	case ast.ParamBlock:
		n.WithItems = normalizeItems(n.WithItems)
		return n
	case ast.DoBlock:
		n.WithItems = normalizeItems(n.WithItems)
		return n
	case ast.SubmitBlock:
		n.WithItems = normalizeItems(n.WithItems)
		return n
	case ast.AnalyseBlock:
		n.WithItems = normalizeItems(n.WithItems)
		return n
	default:
		return stmt
	}
}

func (r *resolver) normalizeWithRef(mod *expandedModule, ref string, at diag.Span, inserted map[string]struct{}) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || !strings.Contains(ref, ".") {
		return ref
	}
	parts := strings.Split(ref, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		r.diags.AddError(
			"E537",
			fmt.Sprintf("invalid qualified with reference '%s'", ref),
			at,
			"use syntax alias.symbol in with clauses",
		)
		return ref
	}
	alias := parts[0]
	name := parts[1]
	sourceRef, ok := mod.Aliases[alias]
	if !ok {
		r.diags.AddError(
			"E537",
			fmt.Sprintf("unknown with alias '%s' in qualified reference '%s'", alias, ref),
			at,
			"declare alias first with `use <module>` or `use \"path\" as <alias>`",
		)
		return ref
	}
	source := r.expandModule(sourceRef)
	if source == nil {
		return name
	}
	r.importSymbol(mod, source, name, at, inserted, make(map[string]struct{}))
	return name
}

func (r *resolver) importSymbol(target *expandedModule, source *expandedModule, name string, at diag.Span, inserted map[string]struct{}, visiting map[string]struct{}) {
	sym, ok := source.Symbols[name]
	if !ok {
		r.diags.AddError(
			"E532",
			fmt.Sprintf("unknown symbol '%s' in module '%s'", name, source.Ref.Label),
			at,
			"import a symbol that exists in the source module",
		)
		return
	}
	if !sym.Importable {
		r.diags.AddError(
			"E533",
			fmt.Sprintf("symbol '%s' in module '%s' is not importable", name, source.Ref.Label),
			at,
			"only let/param/do/submit/global symbols are importable",
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

func (r *resolver) addImportedSymbol(target *expandedModule, source *expandedModule, sym symbolDecl, at diag.Span) bool {
	if _, aliasExists := target.Aliases[sym.Name]; aliasExists {
		r.diags.AddError(
			"E534",
			fmt.Sprintf("import name collision: symbol '%s' conflicts with alias", sym.Name),
			at,
			"rename alias or imported symbol",
			diag.RelatedSpan{Message: "imported symbol", Span: sym.Span},
		)
		return false
	}
	if prev, exists := target.Symbols[sym.Name]; exists {
		if prev.ModuleID == source.Ref.ID && prev.Name == sym.Name {
			return false
		}
		r.diags.AddError(
			"E534",
			fmt.Sprintf("import name collision: '%s' from '%s' conflicts with existing declaration", sym.Name, source.Ref.Label),
			at,
			"rename symbols or adjust imports",
			diag.RelatedSpan{Message: "existing declaration", Span: prev.Span},
			diag.RelatedSpan{Message: "imported declaration", Span: sym.Span},
		)
		return false
	}
	target.Symbols[sym.Name] = symbolDecl{
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
	Source moduleRef
	Name   string
	Span   diag.Span
}

func (r *resolver) symbolDependencies(mod *expandedModule, stmt ast.Stmt) []depRef {
	deps := make([]depRef, 0)
	add := func(source moduleRef, name string, span diag.Span) {
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
	resolveLocal := func(name string) (moduleRef, string, bool) {
		if sym, ok := mod.Symbols[name]; ok {
			ref, exists := r.moduleRefByID(sym.ModuleID)
			if !exists {
				return moduleRef{}, "", false
			}
			return ref, sym.Name, true
		}
		return moduleRef{}, "", false
	}

	importWithItems := func(items []ast.WithItem) {
		for _, item := range items {
			if item.From == "" {
				if ref, depName, ok := resolveLocal(item.Name); ok {
					add(ref, depName, item.Span)
				}
				continue
			}
			if aliasRef, ok := mod.Aliases[item.From]; ok {
				add(aliasRef, item.Name, item.Span)
				continue
			}
			if ref, depName, ok := resolveLocal(item.From); ok {
				add(ref, depName, item.Span)
				continue
			}
			if ref, depName, ok := resolveLocal(item.Name); ok {
				add(ref, depName, item.Span)
			}
		}
	}

	switch n := stmt.(type) {
	case ast.ParamBlock:
		importWithItems(n.WithItems)
	case ast.DoBlock:
		for _, dep := range n.After {
			if ref, depName, ok := resolveLocal(dep); ok {
				add(ref, depName, n.Span)
			}
		}
		importWithItems(n.WithItems)
	case ast.SubmitBlock:
		for _, dep := range n.After {
			if ref, depName, ok := resolveLocal(dep); ok {
				add(ref, depName, n.Span)
			}
		}
		for _, useName := range n.UseNames {
			if ref, depName, ok := resolveLocal(useName); ok {
				add(ref, depName, n.Span)
			}
		}
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

func (r *resolver) moduleRefByID(id string) (moduleRef, bool) {
	if raw, ok := r.raw[id]; ok && raw != nil {
		return raw.Ref, true
	}
	if exp, ok := r.expanded[id]; ok && exp != nil {
		return exp.Ref, true
	}
	return moduleRef{}, false
}

func stmtSymbol(stmt ast.Stmt) (string, symbolKind, bool, bool) {
	switch n := stmt.(type) {
	case ast.LetBlock:
		return n.Name, symbolKindLet, true, true
	case ast.ParamBlock:
		return n.Name, symbolKindParam, true, true
	case ast.DoBlock:
		return n.Name, symbolKindDo, true, true
	case ast.SubmitBlock:
		return n.Name, symbolKindSubmit, true, true
	case ast.GlobalAssign:
		return n.Name, symbolKindGlobal, true, true
	case ast.AnalyseBlock:
		return n.StepName, symbolKindOther, true, false
	default:
		return "", symbolKindOther, false, false
	}
}

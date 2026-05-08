// implement top-level `use` imports by resolving modules into a graph.
//
// The loader resolves quoted file modules, parses them, resolves
// top-level `use` statements into namespace/selective edges, detects module
// cycles, and returns per-module source text for diagnostics. Semantic import
// behavior is handled later in sema; the loader no longer flattens imported
// statements into the importer.
package imports

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

type LoadResult struct {
	Sources map[string]string
	Entry   ModuleRef
	Modules map[string]*ModuleInfo
}

type ModuleRef struct {
	ID    string
	Label string
}

type ResolvedUseKind string

const (
	UseNamespace ResolvedUseKind = "namespace"
	UseSelective ResolvedUseKind = "selective"
)

type ResolvedUse struct {
	Kind   ResolvedUseKind
	Span   diag.Span
	Source ModuleRef
	Alias  string
	Names  []string
	Index  int
}

type ModuleInfo struct {
	Ref     ModuleRef
	BaseDir string
	Program ast.Program
	Uses    []ResolvedUse
}

type rawModule struct {
	Ref     ModuleRef
	Source  string
	Program ast.Program
	BaseDir string
}

type resolver struct {
	cwd     string
	diags   *diag.Diagnostics
	raw     map[string]*rawModule
	modules map[string]*ModuleInfo
	loading map[string]bool
	sources map[string]string
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

// LoadAndExpandSource resolves `use` imports for an in-memory entry module.
//
// The source is parsed under entryLabel and uses baseDir as importer base for
// quoted path imports. Bare import names are rejected.
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
		cwd:     absCwd,
		diags:   diags,
		raw:     make(map[string]*rawModule),
		modules: make(map[string]*ModuleInfo),
		loading: make(map[string]bool),
		sources: make(map[string]string),
	}, nil
}

func (r *resolver) loadResult(entryRef ModuleRef) (*LoadResult, error) {
	r.resolveModule(entryRef, nil, diag.Span{})
	outSources := make(map[string]string, len(r.sources))
	for k, v := range r.sources {
		outSources[k] = v
	}
	modules := make(map[string]*ModuleInfo, len(r.modules))
	for id, info := range r.modules {
		if info == nil {
			continue
		}
		uses := make([]ResolvedUse, 0, len(info.Uses))
		for _, use := range info.Uses {
			next := use
			next.Names = append([]string(nil), use.Names...)
			uses = append(uses, next)
		}
		modules[id] = &ModuleInfo{
			Ref:     info.Ref,
			BaseDir: info.BaseDir,
			Program: info.Program,
			Uses:    uses,
		}
	}
	return &LoadResult{
		Sources: outSources,
		Entry:   entryRef,
		Modules: modules,
	}, nil
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

type bareModuleResolutionError struct {
	Name      string
	LocalPath string
}

func (e *bareModuleResolutionError) Error() string {
	if e == nil {
		return "bare module resolution failed"
	}
	if e.LocalPath != "" {
		return fmt.Sprintf("bare import '%s' matches local file '%s'", e.Name, e.LocalPath)
	}
	return fmt.Sprintf("bare import '%s' is not supported", e.Name)
}

func (r *resolver) resolveBareModule(name string, importerBaseDir string) (ModuleRef, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return ModuleRef{}, fmt.Errorf("empty module name")
	}
	if localPath, ok := sameNamedLocalModule(importerBaseDir, r.cwd, n); ok {
		return ModuleRef{}, &bareModuleResolutionError{Name: n, LocalPath: localPath}
	}
	return ModuleRef{}, &bareModuleResolutionError{Name: n}
}

func sameNamedLocalModule(importerBaseDir string, cwd string, name string) (string, bool) {
	baseDir := strings.TrimSpace(importerBaseDir)
	if baseDir == "" {
		baseDir = cwd
	}
	if baseDir == "" {
		return "", false
	}
	candidate := filepath.Clean(filepath.Join(baseDir, name+".jbs"))
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return "", false
	}
	return candidate, true
}

func bareImportReplacement(useStmt ast.UseStmt) string {
	path := "./" + useStmt.Source.Value + ".jbs"
	if len(useStmt.Names) == 0 {
		alias := strings.TrimSpace(useStmt.Alias)
		if alias == "" {
			alias = useStmt.Source.Value
		}
		return fmt.Sprintf("use %q as %s", path, alias)
	}
	return fmt.Sprintf("use %s from %q", strings.Join(useStmt.Names, ", "), path)
}

func bareImportHint(useStmt ast.UseStmt, localPath string) string {
	replacement := bareImportReplacement(useStmt)
	if localPath != "" {
		return fmt.Sprintf("rewrite it as `%s` for the local file", replacement)
	}
	return fmt.Sprintf("use `%s` for local files", replacement)
}

func (r *resolver) reportResolveUseError(useStmt ast.UseStmt, err error) {
	var bareErr *bareModuleResolutionError
	if errors.As(err, &bareErr) {
		code := diag.CodeE531
		if bareErr.LocalPath != "" {
			code = diag.CodeE537
		}
		r.diags.AddError(
			code,
			fmt.Sprintf("bare import '%s' is not supported", bareErr.Name),
			useStmt.Span,
			bareImportHint(useStmt, bareErr.LocalPath),
		)
		return
	}
	r.diags.AddError(
		diag.CodeE531,
		fmt.Sprintf("failed to resolve import source '%s': %v", useStmt.Source.Value, err),
		useStmt.Span,
		"check module name/path and file availability",
	)
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

func (r *resolver) resolveUseSource(currentAliases map[string]ModuleRef, importerBaseDir string, src ast.UseSource) (ModuleRef, error) {
	switch src.Kind {
	case ast.UseSourcePath:
		baseDir := importerBaseDir
		if strings.TrimSpace(baseDir) == "" {
			baseDir = r.cwd
		}
		return r.resolvePathModule(src.Value, baseDir, src.Span)
	case ast.UseSourceBare:
		if currentAliases != nil {
			if ref, ok := currentAliases[src.Value]; ok {
				return ref, nil
			}
		}
		return r.resolveBareModule(src.Value, importerBaseDir)
	default:
		return ModuleRef{}, fmt.Errorf("unknown use source kind")
	}
}

func (r *resolver) resolveModule(ref ModuleRef, stack []ModuleRef, at diag.Span) *ModuleInfo {
	if info := r.modules[ref.ID]; info != nil {
		return info
	}
	if r.loading[ref.ID] {
		r.reportCycle(stack, ref, at)
		return nil
	}
	raw := r.raw[ref.ID]
	if raw == nil {
		return nil
	}

	r.loading[ref.ID] = true
	defer delete(r.loading, ref.ID)

	localSymbols := collectModuleLocalSymbols(raw.Program)
	aliases := make(map[string]ModuleRef)
	uses := make([]ResolvedUse, 0)
	childStack := append(append([]ModuleRef(nil), stack...), ref)

	for index, stmt := range raw.Program.Stmts {
		useStmt, ok := stmt.(ast.UseStmt)
		if !ok {
			continue
		}
		sourceRef, err := r.resolveUseSource(aliases, raw.BaseDir, useStmt.Source)
		if err != nil {
			r.reportResolveUseError(useStmt, err)
			continue
		}
		if len(useStmt.Names) == 0 {
			alias := useStmt.Alias
			if alias == "" {
				alias = useStmt.Source.Value
			}
			if localSpan, ok := localSymbols[alias]; ok {
				r.diags.AddError(
					diag.CodeE534,
					fmt.Sprintf("import name collision: alias '%s' conflicts with local symbol", alias),
					useStmt.Span,
					"rename alias or conflicting symbol",
					diag.RelatedSpan{Message: "conflicting symbol", Span: localSpan},
				)
				continue
			}
			if prev, ok := aliases[alias]; ok {
				if prev.ID != sourceRef.ID {
					r.diags.AddError(
						diag.CodeE536,
						fmt.Sprintf("import alias collision for '%s'", alias),
						useStmt.Span,
						"use a unique alias name",
					)
				}
				continue
			}
			aliases[alias] = sourceRef
			uses = append(uses, ResolvedUse{
				Kind:   UseNamespace,
				Span:   useStmt.Span,
				Source: sourceRef,
				Alias:  alias,
				Index:  index,
			})
		} else {
			uses = append(uses, ResolvedUse{
				Kind:   UseSelective,
				Span:   useStmt.Span,
				Source: sourceRef,
				Names:  append([]string(nil), useStmt.Names...),
				Index:  index,
			})
		}
		r.resolveModule(sourceRef, childStack, useStmt.Span)
	}

	sort.Slice(uses, func(i, j int) bool {
		if uses[i].Index == uses[j].Index {
			if uses[i].Kind == uses[j].Kind {
				return uses[i].Alias < uses[j].Alias
			}
			return uses[i].Kind < uses[j].Kind
		}
		return uses[i].Index < uses[j].Index
	})

	info := &ModuleInfo{
		Ref:     ref,
		BaseDir: raw.BaseDir,
		Program: raw.Program,
		Uses:    uses,
	}
	r.modules[ref.ID] = info
	return info
}

func (r *resolver) reportCycle(stack []ModuleRef, ref ModuleRef, at diag.Span) {
	chain := make([]string, 0, len(stack)+1)
	start := 0
	for i, item := range stack {
		if item.ID == ref.ID {
			start = i
			break
		}
	}
	for _, item := range stack[start:] {
		chain = append(chain, item.Label)
	}
	chain = append(chain, ref.Label)
	span := at
	if span.IsZero() {
		span = diag.NewSpan(ref.Label, diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 1))
	}
	r.diags.AddError(
		diag.CodeE530,
		fmt.Sprintf("module import cycle detected: %s", strings.Join(chain, " -> ")),
		span,
		"break the cycle by removing one of the recursive imports",
	)
}

func collectModuleLocalSymbols(prog ast.Program) map[string]diag.Span {
	out := make(map[string]diag.Span)
	for _, stmt := range prog.Stmts {
		collectStmtLocalSymbols(stmt, out)
	}
	return out
}

func collectStmtLocalSymbols(stmt ast.Stmt, out map[string]diag.Span) {
	name, span, ok := stmtLocalSymbol(stmt)
	if ok && strings.TrimSpace(name) != "" {
		if _, exists := out[name]; !exists {
			out[name] = span
		}
	}
	if ifStmt, ok := stmt.(ast.IfStmt); ok {
		for _, child := range ifStmt.Then {
			collectStmtLocalSymbols(child, out)
		}
		for _, branch := range ifStmt.Elifs {
			for _, child := range branch.Body {
				collectStmtLocalSymbols(child, out)
			}
		}
		for _, child := range ifStmt.Else {
			collectStmtLocalSymbols(child, out)
		}
	}
	if forStmt, ok := stmt.(ast.ForStmt); ok {
		if strings.TrimSpace(forStmt.Target) != "" {
			if _, exists := out[forStmt.Target]; !exists {
				out[forStmt.Target] = forStmt.Span
			}
		}
		for _, child := range forStmt.Body {
			collectStmtLocalSymbols(child, out)
		}
	}
	if whileStmt, ok := stmt.(ast.WhileStmt); ok {
		for _, child := range whileStmt.Body {
			collectStmtLocalSymbols(child, out)
		}
	}
}

func stmtLocalSymbol(stmt ast.Stmt) (string, diag.Span, bool) {
	switch n := stmt.(type) {
	case ast.GlobalAssign:
		return n.Name, n.Span, true
	case ast.DoBlock:
		return n.Name, n.Span, true
	default:
		return "", diag.Span{}, false
	}
}

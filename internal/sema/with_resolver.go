package sema

import (
	"fmt"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/runtimevar"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/shellvar"
)

type ExpandedWithVar struct {
	Visible   string
	SourceVar string
}

type ResolveIssueKind int

const (
	IssueUnknownSource ResolveIssueKind = iota
	IssueUnknownVar
	IssueDisallowedBinding
	IssueUnsupportedExpression
)

type DisallowedBindingReason int

const (
	DisallowedBindingNone DisallowedBindingReason = iota
	DisallowedBindingNotData
	DisallowedBindingAnalyseTable
	DisallowedBindingAnalyseMultiColumn
	DisallowedBindingAnalyseNonString
	DisallowedBindingDoUnsupported
)

type ResolveIssue struct {
	Kind                ResolveIssueKind
	Item                ast.WithItem
	Source              string
	Variable            string
	Span                diag.Span
	DisallowedReason    DisallowedBindingReason
	DisallowedShape     BindingShape
	DisallowedColumns   int
	DisallowedValueKind eval.Kind
}

type BindingResolver struct {
	Bindings   map[string]*GlobalBinding
	Globals    map[string]eval.Value
	Namespaces map[string]*Namespace
}

type withRef struct {
	Source    string
	Columns   []string
	Bare      bool
	Alias     string
	AliasSpan diag.Span
	Span      diag.Span
}

func (r BindingResolver) ResolveDoWithItems(items []ast.WithItem, diags *diag.Diagnostics) []WithExpansion {
	out := make([]WithExpansion, 0, len(items))
	issues := make([]ResolveIssue, 0)
	for itemID, item := range items {
		ref, ok := r.resolveDoWithRef(item, diags)
		if !ok {
			continue
		}
		binding, issue := r.resolveBinding(ref.Source, item)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		expansion, ok := r.expandDoBinding(itemID, item, ref, binding, diags)
		if !ok {
			continue
		}
		out = append(out, expansion)
	}
	emitWithIssues(diags, stepValidateWithDiagPolicy(), issues)
	return out
}

func (r BindingResolver) ResolveAnalyseWithItems(items []ast.WithItem, diags *diag.Diagnostics) (map[string]analyseBindingImport, []ResolveIssue) {
	out := make(map[string]analyseBindingImport)
	issues := make([]ResolveIssue, 0)
	tracker := newImportConflictTracker()
	for _, item := range items {
		if !validateWithAlias(item.Alias, item.AliasSpan, item.Span, diags) {
			continue
		}
		ref, ok := analyseBareRef(item)
		if !ok {
			issues = append(issues, ResolveIssue{
				Kind:             IssueUnsupportedExpression,
				Item:             item,
				Span:             item.Span,
				DisallowedReason: DisallowedBindingAnalyseNonString,
			})
			continue
		}
		binding, issue := r.resolveBinding(ref.Source, item)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		if binding.Value.Kind != eval.KindString {
			issues = append(issues, ResolveIssue{
				Kind:                IssueDisallowedBinding,
				Item:                item,
				Source:              ref.Source,
				Span:                item.Span,
				DisallowedReason:    analyseDisallowedReasonForValue(binding.Value),
				DisallowedShape:     binding.Shape,
				DisallowedColumns:   len(binding.Order),
				DisallowedValueKind: binding.Value.Kind,
			})
			continue
		}
		sourceVar := analyseSourceVar(ref.Source, binding)
		visible := ref.Source
		if ref.Alias != "" {
			visible = ref.Alias
		}
		prev, conflict, first := tracker.Add(visible, binding.Name+"\x00"+sourceVar, item.Span)
		if conflict {
			if first {
				diags.AddError(
					diag.CodeE214,
					fmt.Sprintf("conflicting analyse import '%s'", visible),
					item.Span,
					"import each analyse variable from only one global binding",
					diag.RelatedSpan{Message: "first conflicting import", Span: prev.Span},
				)
			}
			continue
		}
		out[visible] = analyseBindingImport{
			Source:    binding.Name,
			SourceVar: sourceVar,
			Span:      item.Span,
		}
	}
	return out, issues
}

func analyseDisallowedReasonForValue(value eval.Value) DisallowedBindingReason {
	switch value.Kind {
	case eval.KindComb:
		return DisallowedBindingAnalyseTable
	case eval.KindString:
		return DisallowedBindingNone
	default:
		return DisallowedBindingAnalyseNonString
	}
}

func analyseSourceVar(source string, binding *GlobalBinding) string {
	if binding == nil {
		return source
	}
	if _, ok := binding.Vars[source]; ok {
		return source
	}
	if len(binding.Order) == 1 {
		return binding.Order[0]
	}
	return source
}

func (r BindingResolver) resolveDoWithRef(item ast.WithItem, diags *diag.Diagnostics) (withRef, bool) {
	if item.Expr == nil {
		return withRef{}, false
	}
	if !validateWithAlias(item.Alias, item.AliasSpan, item.Span, diags) {
		return withRef{}, false
	}
	source, ok := withBareName(item.Expr)
	if ok {
		return withRef{Source: source, Bare: true, Alias: item.Alias, AliasSpan: item.AliasSpan, Span: item.Span}, true
	}
	idx, ok := item.Expr.(ast.IndexExpr)
	if !ok {
		diags.AddError(diag.CodeE023, "unsupported with-clause expression", item.Span, `use a variable name or table projection such as cases["x"]`)
		return withRef{}, false
	}
	source, ok = withBareName(idx.Base)
	if !ok {
		diags.AddError(diag.CodeE023, "unsupported with-clause expression", item.Span, "table projections in with clauses must start from a global binding")
		return withRef{}, false
	}
	columns, ok := r.evalWithColumns(idx.Items, item.Span, diags)
	if !ok {
		return withRef{}, false
	}
	return withRef{Source: source, Columns: columns, Alias: item.Alias, AliasSpan: item.AliasSpan, Span: item.Span}, true
}

func analyseBareRef(item ast.WithItem) (withRef, bool) {
	source, ok := withBareName(item.Expr)
	if !ok {
		return withRef{}, false
	}
	return withRef{Source: source, Bare: true, Alias: item.Alias, AliasSpan: item.AliasSpan, Span: item.Span}, true
}

func validateWithAlias(alias string, aliasSpan, itemSpan diag.Span, diags *diag.Diagnostics) bool {
	if alias == "" {
		return true
	}
	if shellvar.ValidName(alias) {
		return true
	}
	if aliasSpan.IsZero() {
		aliasSpan = itemSpan
	}
	if diags != nil {
		diags.AddError(
			diag.CodeE023,
			fmt.Sprintf("invalid with-clause alias %q", alias),
			aliasSpan,
			"use a shell variable name such as x, system_name, or _tmp",
		)
	}
	return false
}

func validateDoWithVisibleName(name string, at diag.Span, diags *diag.Diagnostics) bool {
	if reason, ok := runtimevar.ReservedName(name); ok {
		if diags != nil {
			diags.AddError(
				diag.CodeE023,
				fmt.Sprintf("with-clause variable %q is reserved for JBS runtime metadata", name),
				at,
				fmt.Sprintf("choose another name; %s is set by JBS as the %s", name, reason),
			)
		}
		return false
	}
	return true
}

func withBareName(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case ast.IdentExpr:
		return e.Name, e.Name != ""
	case ast.QualifiedIdentExpr:
		name := e.Namespace + "." + e.Name
		return name, e.Namespace != "" && e.Name != ""
	default:
		return "", false
	}
}

func (r BindingResolver) evalWithColumns(items []ast.Expr, at diag.Span, diags *diag.Diagnostics) ([]string, bool) {
	if len(items) == 0 {
		diags.AddError(diag.CodeE023, "empty with table projection", at, `select at least one column such as cases["x"]`)
		return nil, false
	}
	columns := make([]string, 0, len(items))
	for _, item := range items {
		value := eval.EvalExprWithOptions(item, r.Globals, diags, eval.ExprOptions{})
		if value.Kind != eval.KindString {
			diags.AddError(diag.CodeE023, "with table projection selector must evaluate to a string", item.GetSpan(), `use quoted column names such as cases["x"] or a string selector variable`)
			return nil, false
		}
		columns = append(columns, value.S)
	}
	return columns, true
}

func (r BindingResolver) resolveBinding(name string, item ast.WithItem) (*GlobalBinding, *ResolveIssue) {
	src := r.Bindings[name]
	if src == nil {
		if r.isExpressionVisibleOnly(name) {
			return nil, &ResolveIssue{
				Kind:             IssueDisallowedBinding,
				Item:             item,
				Source:           name,
				Span:             item.Span,
				DisallowedReason: DisallowedBindingNotData,
			}
		}
		return nil, &ResolveIssue{
			Kind:   IssueUnknownSource,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	return src, nil
}

func (r BindingResolver) expandDoBinding(itemID int, item ast.WithItem, ref withRef, binding *GlobalBinding, diags *diag.Diagnostics) (WithExpansion, bool) {
	norm, ok := normalizeDoWithBinding(ref.Source, binding, item.Span, diags)
	if !ok {
		return WithExpansion{}, false
	}
	if len(ref.Columns) > 0 {
		if norm.Shape != BindingTable {
			diags.AddError(diag.CodeE420, fmt.Sprintf("with-clause projection requires a table or dictionary source; '%s' is %s-valued", ref.Source, binding.Value.Kind), item.Span, `use projection syntax only with table-like values`)
			return WithExpansion{}, false
		}
		vars := make(map[string][]eval.Value, len(ref.Columns))
		projections := make(map[string][]eval.ProjectionKey, len(ref.Columns))
		order := make([]string, 0, len(ref.Columns))
		for _, column := range ref.Columns {
			if _, ok := norm.Vars[column]; !ok {
				diags.AddError(diag.CodeE021, fmt.Sprintf("unknown variable '%s' in source '%s'", column, ref.Source), item.Span, "select an existing table column")
				return WithExpansion{}, false
			}
			if !slices.Contains(order, column) {
				order = append(order, column)
				vars[column] = slices.Clone(norm.Vars[column])
				projections[column] = slices.Clone(norm.ProjectionByName[column])
			}
		}
		norm.Order = order
		norm.Vars = vars
		norm.ProjectionByName = projections
		norm.Full = false
	}

	sourceVars := planutil.SourceVarNames(norm.Order, norm.Vars)
	vars := make([]ExpandedWithVar, 0, len(sourceVars))
	if ref.Alias != "" {
		if len(sourceVars) != 1 {
			aliasSpan := ref.AliasSpan
			if aliasSpan.IsZero() {
				aliasSpan = item.Span
			}
			diags.AddError(diag.CodeE023, fmt.Sprintf("with-clause alias %q can rename only one imported variable", ref.Alias), aliasSpan, "select one column or remove the alias")
			return WithExpansion{}, false
		}
		vars = append(vars, ExpandedWithVar{Visible: ref.Alias, SourceVar: sourceVars[0]})
	} else {
		for _, name := range sourceVars {
			vars = append(vars, ExpandedWithVar{Visible: name, SourceVar: name})
		}
	}
	for _, v := range vars {
		span := item.Span
		if ref.Alias != "" && v.Visible == ref.Alias && !ref.AliasSpan.IsZero() {
			span = ref.AliasSpan
		}
		if !validateDoWithVisibleName(v.Visible, span, diags) {
			return WithExpansion{}, false
		}
	}
	sourceKey := BindingVersionKeyForBinding(binding, ref.Source)
	displaySource := sourceKey.Display()
	if displaySource == "" {
		displaySource = ref.Source
	}
	return WithExpansion{
		ItemID:           itemID,
		Source:           displaySource,
		SourceKey:        sourceKey,
		DisplaySource:    displaySource,
		Vars:             vars,
		VarsByName:       cloneSeriesMap(norm.Vars),
		ProjectionByName: cloneProjectionMap(norm.ProjectionByName),
		RowCount:         norm.RowCount,
		Full:             norm.Full,
		Span:             item.Span,
	}, true
}

type normalizedWithBinding struct {
	Shape            BindingShape
	Order            []string
	Vars             map[string][]eval.Value
	ProjectionByName map[string][]eval.ProjectionKey
	RowCount         int
	Full             bool
}

func normalizeDoWithBinding(source string, binding *GlobalBinding, at diag.Span, diags *diag.Diagnostics) (normalizedWithBinding, bool) {
	if binding == nil {
		return normalizedWithBinding{}, false
	}
	value := binding.Value
	switch {
	case value.IsScalar():
		return normalizedWithBinding{
			Shape:    BindingScalar,
			Order:    []string{sourceVarNameForScalar(source, binding)},
			Vars:     map[string][]eval.Value{sourceVarNameForScalar(source, binding): {eval.CloneValue(value)}},
			RowCount: 1,
			Full:     true,
		}, true
	case value.Kind == eval.KindList || value.Kind == eval.KindTuple:
		name := sourceVarNameForScalar(source, binding)
		values := make([]eval.Value, 0, len(value.L))
		warned := make(map[eval.Kind]bool)
		for _, item := range value.L {
			if item.IsScalar() {
				values = append(values, eval.CloneValue(item))
				continue
			}
			if !warned[item.Kind] {
				diags.AddWarning(diag.CodeW314, fmt.Sprintf("with-clause value '%s' contains non-scalar element of type %s", source, item.Kind), at, "the element will be exported using str(value)")
				warned[item.Kind] = true
			}
			values = append(values, eval.String(item.String()))
		}
		return normalizedWithBinding{
			Shape:    BindingScalar,
			Order:    []string{name},
			Vars:     map[string][]eval.Value{name: values},
			RowCount: len(values),
			Full:     true,
		}, true
	case eval.IsComb(value):
		order, vars, projections := varsAndProjectionsFromTable(value)
		return normalizedWithBinding{
			Shape:            BindingTable,
			Order:            order,
			Vars:             vars,
			ProjectionByName: projections,
			RowCount:         len(value.C.Rows),
			Full:             true,
		}, true
	case value.Kind == eval.KindDict:
		table, ok := eval.TableFromDictValue(value, at, diags)
		if !ok {
			return normalizedWithBinding{}, false
		}
		order, vars, projections := varsAndProjectionsFromTable(table)
		rowCount := 0
		if table.C != nil {
			rowCount = len(table.C.Rows)
		}
		return normalizedWithBinding{
			Shape:            BindingTable,
			Order:            order,
			Vars:             vars,
			ProjectionByName: projections,
			RowCount:         rowCount,
			Full:             true,
		}, true
	default:
		diags.AddError(diag.CodeE420, fmt.Sprintf("with-clause cannot import %s-valued global '%s'", value.Kind, source), at, "use int, float, string, bool, list, tuple, table, or dict values")
		return normalizedWithBinding{}, false
	}
}

func sourceVarNameForScalar(source string, binding *GlobalBinding) string {
	if binding != nil {
		for _, name := range planutil.SourceVarNames(binding.Order, binding.Vars) {
			return name
		}
	}
	return source
}

func varsAndProjectionsFromTable(value eval.Value) ([]string, map[string][]eval.Value, map[string][]eval.ProjectionKey) {
	if !eval.IsComb(value) {
		return nil, nil, nil
	}
	order := append([]string(nil), value.C.Order...)
	vars := make(map[string][]eval.Value, len(order))
	projections := make(map[string][]eval.ProjectionKey, len(order))
	for _, name := range order {
		col, ok := eval.CombColumn(value, name)
		if !ok {
			continue
		}
		vars[name] = slices.Clone(col)
		keys, ok := eval.CombColumnProjections(value, name)
		if ok {
			projections[name] = slices.Clone(keys)
		}
	}
	return order, vars, projections
}

func (r BindingResolver) isExpressionVisibleOnly(name string) bool {
	if name == "" {
		return false
	}
	if _, exists := r.Bindings[name]; exists {
		return false
	}
	if _, exists := r.Globals[name]; exists {
		return true
	}
	return r.Namespaces[name] != nil
}

package lower

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
	"jbs/internal/sema"
)

// ReservedSeparator keeps grouped source-row IDs opaque in synthetic _jr__
// helpers until inherited row-context expansion is explicitly requested.
const ReservedSeparator = "####"
const escapedAliasPrefix = "_ja__"

type Literal string

func (l Literal) MarshalYAML() (interface{}, error) {
	n := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(l), Style: yaml.LiteralStyle}
	return &n, nil
}

type SingleQuoted string

func (s SingleQuoted) MarshalYAML() (interface{}, error) {
	n := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(s), Style: yaml.SingleQuotedStyle}
	return &n, nil
}

type Document struct {
	Name         string         `yaml:"name"`
	Outpath      string         `yaml:"outpath"`
	Comment      string         `yaml:"comment,omitempty"`
	ParameterSet []ParameterSet `yaml:"parameterset,omitempty"`
	PatternSet   []PatternSet   `yaml:"patternset,omitempty"`
	Step         []Step         `yaml:"step,omitempty"`
	Analyser     []Analyser     `yaml:"analyser,omitempty"`
	Result       *ResultObject  `yaml:"result,omitempty"`
	Meta         DocumentMeta   `yaml:"-"`
}

type DocumentMeta struct{}

type ParameterSetKind string

const (
	ParameterSetKindParam      ParameterSetKind = "param"
	ParameterSetKindSubset     ParameterSetKind = "subset"
	ParameterSetKindSubmitInit ParameterSetKind = "submit_system"
)

type ParameterSetMeta struct {
	Kind   ParameterSetKind
	Source string
	Step   string
}

type ParameterSet struct {
	Name      string           `yaml:"name"`
	InitWith  string           `yaml:"init_with,omitempty"`
	Parameter []Parameter      `yaml:"parameter,omitempty"`
	Meta      ParameterSetMeta `yaml:"-"`
}

type Parameter struct {
	Name      string      `yaml:"name"`
	Type      string      `yaml:"type,omitempty"`
	Mode      string      `yaml:"mode,omitempty"`
	Separator string      `yaml:"separator,omitempty"`
	Value     interface{} `yaml:"_"`
}

type PatternSetKind string

const (
	PatternSetKindLet    PatternSetKind = "let"
	PatternSetKindInline PatternSetKind = "analyse_inline"
)

type PatternSetMeta struct {
	Kind   PatternSetKind
	Source string
}

type PatternSet struct {
	Name     string         `yaml:"name"`
	InitWith string         `yaml:"init_with,omitempty"`
	Pattern  []Pattern      `yaml:"pattern,omitempty"`
	Meta     PatternSetMeta `yaml:"-"`
}

type Pattern struct {
	Name  string      `yaml:"name"`
	Type  string      `yaml:"type,omitempty"`
	Value interface{} `yaml:"_"`
	Meta  PatternMeta `yaml:"-"`
}

type PatternMeta struct {
	IsAnalyseAlias bool
	AnalyseStep    string
	AliasName      string
	PatternRef     string
}

type Step struct {
	Name       string        `yaml:"name"`
	Depend     string        `yaml:"depend,omitempty"`
	MaxAsync   *int          `yaml:"max_async,omitempty"`
	Iterations *int          `yaml:"iterations,omitempty"`
	Use        []interface{} `yaml:"use,omitempty"`
	Do         []interface{} `yaml:"do,omitempty"`
	Meta       StepMeta      `yaml:"-"`
}

type StepKind string

const (
	StepKindDo     StepKind = "do"
	StepKindSubmit StepKind = "submit"
)

type StepMeta struct {
	Kind          StepKind
	Source        string
	InheritsFrom  []string
	InheritedVars []string
}

type UseEntry struct {
	From  string `yaml:"from,omitempty"`
	Value string `yaml:"_"`
}

type SubmitOperation struct {
	DoneFile  string `yaml:"done_file"`
	ErrorFile string `yaml:"error_file"`
	Command   string `yaml:"_"`
}

type AnalyserMeta struct {
	Source string
}

type Analyser struct {
	Name    string        `yaml:"name"`
	Use     string        `yaml:"use,omitempty"`
	Analyse []AnalyseItem `yaml:"analyse"`
	Meta    AnalyserMeta  `yaml:"-"`
}

type AnalyseItem struct {
	Step string        `yaml:"step"`
	File []AnalyseFile `yaml:"file"`
}

type AnalyseFile struct {
	Use   string `yaml:"use,omitempty"`
	Value string `yaml:"_"`
}

type ResultMeta struct{}

type ResultObject struct {
	Use   []string      `yaml:"use"`
	Table []ResultTable `yaml:"table"`
	Meta  ResultMeta    `yaml:"-"`
}

type ResultTableMeta struct {
	Source string
}

type ResultTable struct {
	Name   string          `yaml:"name"`
	Style  string          `yaml:"style"`
	Column []ResultColumn  `yaml:"column"`
	Meta   ResultTableMeta `yaml:"-"`
}

type ResultColumn struct {
	Title string `yaml:"title,omitempty"`
	Expr  string `yaml:"_"`
}

type subsetKey struct {
	Step          string
	Source        string
	Vars          string
	InheritedRows string
}

type subsetInfo struct {
	Name    string
	RowsVar string
}

type lowerContext struct {
	res                    *sema.Result
	doc                    Document
	diags                  *diag.Diagnostics
	names                  map[string]struct{}
	subsetNames            map[subsetKey]subsetInfo
	stepSourceRows         map[string]map[string]string
	patternSetIndexByGroup map[string]int
	analyserNames          map[string]string
}

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

func globalString(globals sema.GlobalState, name, fallback string) string {
	v, ok := globals.Values[name]
	if !ok {
		return fallback
	}
	if v.Kind == eval.KindString {
		return v.S
	}
	s := v.String()
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func lowerParamset(ps *sema.Paramset, diags *diag.Diagnostics) ParameterSet {
	out := ParameterSet{
		Name:      ps.Name,
		Parameter: make([]Parameter, 0),
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindParam,
			Source: ps.Name,
		},
	}

	rowCount := len(ps.Rows)
	if rowCount == 0 {
		for _, name := range ps.Order {
			if n := len(ps.Vars[name]); n > rowCount {
				rowCount = n
			}
		}
	}
	if rowCount == 0 {
		diags.AddError(
			diag.CodeE230,
			fmt.Sprintf("parameterset '%s' evaluates to zero rows", ps.Name),
			ps.Block.Span,
			"ensure final expression yields at least one row",
		)
		rowCount = 1
	}

	valuesByName := make(map[string][]eval.Value, len(ps.Order))
	for _, name := range ps.Order {
		valuesByName[name] = valuesFor(ps, name, rowCount)
	}

	indices := sequentialIndices(rowCount)
	out.Parameter = lowerIndexedParameters(ps.Order, valuesByName, ps.Modes, indices, indexVariableName(ps.Name), func(name string) diag.Span {
		return originFor(ps, name)
	}, diags)
	return out
}

func lowerIndexedParameters(
	order []string,
	valuesByName map[string][]eval.Value,
	modes map[string]string,
	indices []int,
	idxName string,
	origin func(name string) diag.Span,
	diags *diag.Diagnostics,
) []Parameter {
	if len(indices) == 0 {
		indices = []int{0}
	}
	if idxName == "" {
		idxName = indexVariableName("set")
	}
	idxRef := "$" + idxName

	params := make([]Parameter, 0, len(order)+1)
	params = append(params, Parameter{
		Name:  idxName,
		Type:  "int",
		Mode:  "text",
		Value: joinIntIndices(indices),
	})
	params = append(params, lowerIndexedPayloadParameters(order, valuesByName, modes, indices, idxRef, origin, diags)...)
	return params
}

func lowerIndexedPayloadParameters(
	order []string,
	valuesByName map[string][]eval.Value,
	modes map[string]string,
	indices []int,
	idxRef string,
	origin func(name string) diag.Span,
	diags *diag.Diagnostics,
) []Parameter {
	params := make([]Parameter, 0, len(order))
	for _, name := range order {
		fullValues := valuesByName[name]
		selectedValues := pickValuesAtIndices(fullValues, indices)
		if len(fullValues) == 0 {
			fullValues = []eval.Value{eval.Null()}
		}
		if len(selectedValues) == 0 {
			selectedValues = []eval.Value{fullValues[0]}
		}

		if mode := modes[name]; mode != "" {
			param := Parameter{Name: name, Mode: mode}
			switch mode {
			case "python":
				if allEqualValues(selectedValues) {
					param.Value = SingleQuoted(asString(selectedValues[0]))
				} else {
					param.Value = SingleQuoted(pythonIndexExpr(fullValues, idxRef))
				}
			case "shell":
				if !allEqualValues(selectedValues) {
					diags.AddError(
						diag.CodeE231,
						fmt.Sprintf("%s(...) parameter '%s' cannot vary across indexed rows", mode, name),
						origin(name),
						"use a single expression value for mode-declared parameters",
					)
				}
				param.Value = asString(selectedValues[0])
			default:
				param.Value = asString(selectedValues[0])
			}
			params = append(params, param)
			continue
		}

		params = append(params, Parameter{
			Name:  name,
			Mode:  "python",
			Value: SingleQuoted(pythonIndexExpr(fullValues, idxRef)),
		})
	}
	return params
}

func lowerContextualPayloadParameters(
	order []string,
	valuesByName map[string][]eval.Value,
	modes map[string]string,
	idxRef string,
	origin func(name string) diag.Span,
	diags *diag.Diagnostics,
) []Parameter {
	params := make([]Parameter, 0, len(order))
	for _, name := range order {
		fullValues := valuesByName[name]
		if len(fullValues) == 0 {
			fullValues = []eval.Value{eval.Null()}
		}
		if mode := modes[name]; mode != "" {
			param := Parameter{Name: name, Mode: mode}
			switch mode {
			case "python":
				if allEqualValues(fullValues) {
					param.Value = SingleQuoted(asString(fullValues[0]))
				} else {
					param.Value = SingleQuoted(pythonIndexExpr(fullValues, idxRef))
				}
			case "shell":
				if !allEqualValues(fullValues) {
					diags.AddError(
						diag.CodeE231,
						fmt.Sprintf("%s(...) parameter '%s' cannot vary across indexed rows", mode, name),
						origin(name),
						"use a single expression value for mode-declared parameters",
					)
				}
				param.Value = asString(fullValues[0])
			default:
				param.Value = asString(fullValues[0])
			}
			params = append(params, param)
			continue
		}

		params = append(params, Parameter{
			Name:  name,
			Mode:  "python",
			Value: SingleQuoted(pythonIndexExpr(fullValues, idxRef)),
		})
	}
	return params
}

func sequentialIndices(n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := range n {
		out[i] = i
	}
	return out
}

func joinIntIndices(indices []int) string {
	if len(indices) == 0 {
		return ""
	}
	out := make([]string, len(indices))
	for i, idx := range indices {
		out[i] = strconv.Itoa(idx)
	}
	return strings.Join(out, ",")
}

func pickValuesAtIndices(values []eval.Value, indices []int) []eval.Value {
	if len(indices) == 0 {
		return nil
	}
	out := make([]eval.Value, 0, len(indices))
	for _, idx := range indices {
		if idx >= 0 && idx < len(values) {
			out = append(out, values[idx])
			continue
		}
		out = append(out, eval.Null())
	}
	return out
}

func originFor(ps *sema.Paramset, name string) diag.Span {
	if s, ok := ps.Origins[name]; ok {
		return s
	}
	return ps.Block.Span
}

func valuesFor(ps *sema.Paramset, name string, rowCount int) []eval.Value {
	values := make([]eval.Value, 0, rowCount)
	if len(ps.Rows) > 0 {
		for _, row := range ps.Rows {
			if cell, ok := row.Values[name]; ok {
				values = append(values, cell.Value)
			}
		}
		if len(values) == rowCount {
			return values
		}
	}

	base := ps.Vars[name]
	if len(base) == 0 {
		for range rowCount {
			values = append(values, eval.Null())
		}
		return values
	}
	values = values[:0]
	for i := range rowCount {
		values = append(values, base[i%len(base)])
	}
	return values
}

func inferType(values []eval.Value) string {
	allInt := true
	allNumber := true
	for _, v := range values {
		switch v.Kind {
		case eval.KindInt:
		case eval.KindFloat:
			allInt = false
		default:
			allInt = false
			allNumber = false
		}
	}
	if allInt {
		return "int"
	}
	if allNumber {
		return "float"
	}
	return ""
}

func allEqualValues(values []eval.Value) bool {
	if len(values) <= 1 {
		return true
	}
	first := values[0]
	for i := 1; i < len(values); i++ {
		if !eval.Equal(first, values[i]) {
			return false
		}
	}
	return true
}

func asString(v eval.Value) string {
	if v.Kind == eval.KindString {
		return v.S
	}
	return v.String()
}

func templateValue(v eval.Value) string {
	switch v.Kind {
	case eval.KindInt:
		return strconv.FormatInt(v.I, 10)
	case eval.KindFloat:
		return strconv.FormatFloat(v.F, 'g', -1, 64)
	case eval.KindString:
		return v.S
	case eval.KindBool:
		if v.B {
			return "true"
		}
		return "false"
	default:
		return pythonLiteral(v)
	}
}

func pythonIndexExpr(values []eval.Value, indexVar string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, pythonLiteral(value))
	}
	return "[" + strings.Join(parts, ",") + "][" + indexVar + "]"
}

func pythonStringMapLookupExpr(keys []int, values []string, varName string) string {
	parts := make([]string, 0, len(keys))
	for i := range keys {
		key := strconv.Quote(strconv.Itoa(keys[i]))
		value := ""
		if i < len(values) {
			value = values[i]
		}
		parts = append(parts, key+":"+strconv.Quote(value))
	}
	return "{" + strings.Join(parts, ",") + "}" + "[\"${" + varName + "}\"]"
}

func pythonLiteral(v eval.Value) string {
	switch v.Kind {
	case eval.KindNull:
		return "None"
	case eval.KindInt:
		return strconv.FormatInt(v.I, 10)
	case eval.KindFloat:
		return strconv.FormatFloat(v.F, 'g', -1, 64)
	case eval.KindString:
		return strconv.Quote(v.S)
	case eval.KindBool:
		if v.B {
			return "True"
		}
		return "False"
	case eval.KindList:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, pythonLiteral(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return strconv.Quote(v.String())
	}
}

func patternTemplateKey(group, name string) string {
	return group + "." + name
}

func (ctx *lowerContext) ensurePatternSet(groupName, analyseStep string) {
	if idx, ok := ctx.patternSetIndexByGroup[groupName]; ok {
		if idx >= 0 && idx < len(ctx.doc.PatternSet) {
			return
		}
	}
	meta := PatternSetMeta{
		Kind:   PatternSetKindInline,
		Source: analyseStep,
	}
	if _, ok := ctx.res.LetByName[groupName]; ok {
		meta.Kind = PatternSetKindLet
		meta.Source = groupName
	}
	ps := PatternSet{
		Name:    groupName,
		Pattern: make([]Pattern, 0),
		Meta:    meta,
	}
	ctx.doc.PatternSet = append(ctx.doc.PatternSet, ps)
	ctx.patternSetIndexByGroup[groupName] = len(ctx.doc.PatternSet) - 1
	ctx.names[groupName] = struct{}{}
}

func (ctx *lowerContext) lowerAnalyseAndResult() {
	if len(ctx.res.Analyse) == 0 {
		return
	}

	result := &ResultObject{
		Use:   make([]string, 0, len(ctx.res.Analyse)),
		Table: make([]ResultTable, 0, len(ctx.res.Analyse)),
	}

	for _, spec := range ctx.res.Analyse {
		if spec == nil {
			continue
		}
		analyserName := ctx.uniqueName("analyser_" + sanitize(spec.Block.StepName))
		ctx.analyserNames[spec.Block.StepName] = analyserName
		files := make([]AnalyseFile, 0, len(spec.Assignments))
		assignmentResultExpr := make(map[string]string, len(spec.Assignments))
		usedGroups := make([]string, 0, len(spec.Assignments))
		seenFile := make(map[string]struct{}, len(spec.Assignments))
		for _, assign := range spec.Assignments {
			groupName := assign.Group
			ctx.ensurePatternSet(groupName, spec.Block.StepName)
			if !slices.Contains(usedGroups, groupName) {
				usedGroups = append(usedGroups, groupName)
			}

			fileKey := groupName + "\x00" + assign.File
			if _, ok := seenFile[fileKey]; !ok {
				files = append(files, AnalyseFile{
					Use:   groupName,
					Value: assign.File,
				})
				seenFile[fileKey] = struct{}{}
			}

			aliasVar := analyseAliasPatternName(assign.Group, assign.Pattern, spec.Block.StepName, assign.Name)
			ctx.appendAliasPattern(spec.Block.StepName, assign.Name, aliasVar, assign.Template)
			assignmentResultExpr[assign.Name] = aliasVar
		}
		analyserUse := strings.Join(usedGroups, ", ")
		ctx.doc.Analyser = append(ctx.doc.Analyser, Analyser{
			Name: analyserName,
			Use:  analyserUse,
			Analyse: []AnalyseItem{
				{
					Step: spec.Block.StepName,
					File: files,
				},
			},
			Meta: AnalyserMeta{Source: spec.Block.StepName},
		})
		if !slices.Contains(result.Use, analyserName) {
			result.Use = append(result.Use, analyserName)
		}

		columns := make([]ResultColumn, 0, len(spec.Columns))
		for _, col := range spec.Columns {
			title := col.Title
			if title == "" {
				title = col.Name
			}
			expr := col.Source
			if expr == "" {
				expr = col.Name
			}
			if mapped, ok := assignmentResultExpr[col.Name]; ok && mapped != "" {
				expr = mapped
			}
			columns = append(columns, ResultColumn{
				Title: title,
				Expr:  expr,
			})
		}
		result.Table = append(result.Table, ResultTable{
			Name:   ctx.uniqueName("result_" + sanitize(spec.Block.StepName)),
			Style:  "csv",
			Column: columns,
			Meta:   ResultTableMeta{Source: spec.Block.StepName},
		})
	}

	ctx.doc.Result = result
}

func analyseAliasPatternName(group, pattern, step, alias string) string {
	return shortPatternAliasName(group, pattern, step, alias)
}

func (ctx *lowerContext) appendAliasPattern(analyseStep, aliasName, internalName string, tmpl sema.PatternTemplate) {
	idx, ok := ctx.patternSetIndexByGroup[tmpl.Group]
	if !ok || idx < 0 || idx >= len(ctx.doc.PatternSet) {
		return
	}
	ps := &ctx.doc.PatternSet[idx]
	for _, existing := range ps.Pattern {
		if existing.Name == internalName {
			return
		}
	}
	ps.Pattern = append(ps.Pattern, Pattern{
		Name:  internalName,
		Type:  tmpl.Type,
		Value: SingleQuoted(tmpl.Regex),
		Meta: PatternMeta{
			IsAnalyseAlias: true,
			AnalyseStep:    analyseStep,
			AliasName:      aliasName,
			PatternRef:     patternTemplateKey(tmpl.Group, tmpl.Name),
		},
	})
}

func (ctx *lowerContext) lowerDo(block ast.DoBlock) Step {
	inherits := make([]string, 0)
	inheritVars := make([]string, 0)
	if plan := ctx.res.StepImportByName[block.Name]; plan != nil && len(plan.InheritedSteps) > 0 {
		inherits = append(inherits, plan.InheritedSteps...)
		inheritVars = slices.Sorted(maps.Keys(plan.Inherited))
	}
	step := Step{
		Name:       block.Name,
		MaxAsync:   block.MaxAsync,
		Iterations: block.Iterations,
		Meta: StepMeta{
			Kind:          StepKindDo,
			Source:        block.Name,
			InheritsFrom:  inherits,
			InheritedVars: inheritVars,
		},
	}
	if len(block.After) > 0 {
		step.Depend = strings.Join(block.After, ",")
	}
	aliases := ctx.stepAliasMap(block.Name, false)
	resolution := ctx.resolveStepUsesForStep(block.Name, block.WithItems, aliases)
	step.Use = resolution.Use
	ctx.stepSourceRows[block.Name] = cloneStringMap(resolution.SourceRows)

	body := normalizeRawLiteral(block.Body)
	body = rewriteShellRefs(body, aliases)
	step.Do = []interface{}{Literal(body)}
	return step
}

func (ctx *lowerContext) addSubmitParameterSet(block ast.SubmitBlock, aliases map[string]string) string {
	name := ctx.uniqueName(fmt.Sprintf("%s__submit_params", block.Name))
	params := make([]Parameter, 0)
	explicitNonRaw := make(map[string]struct{}, len(block.Fields))
	for _, field := range block.Fields {
		if field.IsRaw {
			continue
		}
		explicitNonRaw[field.Name] = struct{}{}
	}
	if spec := ctx.res.SubmitByName[block.Name]; spec != nil {
		for _, field := range spec.Values {
			if field.IsRaw {
				raw := normalizeRawLiteral(field.Raw)
				raw = rewriteShellRefs(raw, aliases)
				params = append(params, Parameter{
					Name:  field.Name,
					Mode:  "text",
					Value: Literal(raw),
				})
				continue
			}

			param := Parameter{Name: field.Name}
			if t := submitParameterType(field.Name); t != "" {
				param.Type = t
			}
			value := field.Value
			if _, ok := explicitNonRaw[field.Name]; ok {
				value = rewriteShellRefsInEvalValue(value, aliases)
			}
			if field.Mode != "" {
				param.Mode = field.Mode
				if field.Mode == "python" {
					param.Value = SingleQuoted(asString(value))
				} else {
					param.Value = asString(value)
				}
			} else {
				switch value.Kind {
				case eval.KindList, eval.KindNull:
					param.Value = pythonLiteral(value)
				default:
					param.Value = templateValue(value)
				}
			}
			params = append(params, param)
		}
	}

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		InitWith:  "platform.xml:systemParameter",
		Parameter: params,
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindSubmitInit,
			Source: block.Name,
		},
	})
	ctx.names[name] = struct{}{}
	return name
}

func (ctx *lowerContext) lowerSubmit(block ast.SubmitBlock, submitSet string, aliases map[string]string) Step {
	inherits := make([]string, 0)
	inheritVars := make([]string, 0)
	if plan := ctx.res.StepImportByName[block.Name]; plan != nil && len(plan.InheritedSteps) > 0 {
		inherits = append(inherits, plan.InheritedSteps...)
		inheritVars = slices.Sorted(maps.Keys(plan.Inherited))
	}
	step := Step{
		Name:       block.Name,
		MaxAsync:   block.MaxAsync,
		Iterations: block.Iterations,
		Meta: StepMeta{
			Kind:          StepKindSubmit,
			Source:        block.Name,
			InheritsFrom:  inherits,
			InheritedVars: inheritVars,
		},
	}
	if len(block.After) > 0 {
		step.Depend = strings.Join(block.After, ",")
	}
	resolution := ctx.resolveStepUsesForStep(block.Name, block.WithItems, aliases)
	ctx.stepSourceRows[block.Name] = cloneStringMap(resolution.SourceRows)
	use := append([]interface{}{}, resolution.Use...)
	use = append(use,
		submitSet,
		UseEntry{From: "platform.xml", Value: "jobfiles"},
		UseEntry{From: "platform.xml", Value: "executesub"},
		UseEntry{From: "platform.xml", Value: "executeset"},
	)
	step.Use = use
	step.Do = []interface{}{
		SubmitOperation{
			DoneFile:  "$done_file",
			ErrorFile: "$error_file",
			Command:   `${submit} --parsable ${submit_script} > run.jobid`,
		},
		`echo "true" > success`,
	}
	return step
}

func submitParameterType(name string) string {
	switch name {
	case "nodes", "tasks", "threadspertask":
		return "int"
	default:
		return ""
	}
}

type stepUseResolution struct {
	Use        []interface{}
	SourceRows map[string]string
}

type subsetVarSpec struct {
	Visible   string
	SourceVar string
	Emitted   string
}

func (ctx *lowerContext) resolveStepUsesForStep(stepName string, fallback []ast.WithItem, aliases map[string]string) stepUseResolution {
	inheritedSteps := make([]string, 0)
	if plan := ctx.res.StepImportByName[stepName]; plan != nil {
		inheritedSteps = append(inheritedSteps, plan.InheritedSteps...)
		return ctx.resolveStepUses(stepName, inheritedSteps, plan.ExplicitDelta, aliases)
	}
	return ctx.resolveStepUsesLegacy(stepName, inheritedSteps, fallback, aliases)
}

func (ctx *lowerContext) resolveStepUsesLegacy(stepName string, inheritedSteps []string, items []ast.WithItem, aliases map[string]string) stepUseResolution {
	planned := make([]sema.PlannedImport, 0, len(items))
	for _, item := range items {
		if item.From == "" {
			planned = append(planned, sema.PlannedImport{
				Source: item.Name,
				Kind:   sema.SourceKindParam,
				Full:   true,
				Span:   item.Span,
			})
			continue
		}
		planned = append(planned, sema.PlannedImport{
			Source:    item.From,
			Kind:      sema.SourceKindParam,
			Visible:   item.Name,
			SourceVar: item.Name,
			Span:      item.Span,
		})
	}
	return ctx.resolveStepUses(stepName, inheritedSteps, planned, aliases)
}

func (ctx *lowerContext) resolveStepUses(stepName string, inheritedSteps []string, items []sema.PlannedImport, aliases map[string]string) stepUseResolution {
	uses := make([]interface{}, 0)
	grouped := make(map[string][]subsetVarSpec)
	groupOrder := make([]string, 0)
	seenDirect := make(map[string]struct{})
	sourceRows := ctx.inheritedRowsForStep(stepName, inheritedSteps)
	sources := ctx.res.ImportSourceByName

	for _, item := range items {
		if item.Full {
			if src := sources[item.Source]; src != nil {
				if item.Kind == sema.SourceKindParam && sourceRows[item.Source] == "" && !sourceNeedsAlias(src, aliases) {
					if _, seen := seenDirect[item.Source]; !seen {
						seenDirect[item.Source] = struct{}{}
						uses = append(uses, item.Source)
					}
					continue
				}
				if _, ok := grouped[item.Source]; !ok {
					grouped[item.Source] = make([]subsetVarSpec, 0)
					groupOrder = append(groupOrder, item.Source)
				}
				for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
					if slices.ContainsFunc(grouped[item.Source], func(v subsetVarSpec) bool { return v.Visible == name }) {
						continue
					}
					emitted := name
					if alias, ok := aliases[name]; ok && alias != "" {
						emitted = alias
					}
					grouped[item.Source] = append(grouped[item.Source], subsetVarSpec{
						Visible:   name,
						SourceVar: name,
						Emitted:   emitted,
					})
				}
			}
			continue
		}
		if _, ok := grouped[item.Source]; !ok {
			grouped[item.Source] = make([]subsetVarSpec, 0)
			groupOrder = append(groupOrder, item.Source)
		}
		if !slices.ContainsFunc(grouped[item.Source], func(v subsetVarSpec) bool { return v.Visible == item.Visible }) {
			sourceVar := item.SourceVar
			if sourceVar == "" {
				sourceVar = item.Visible
			}
			emitted := item.Visible
			if alias, ok := aliases[item.Visible]; ok && alias != "" {
				emitted = alias
			}
			grouped[item.Source] = append(grouped[item.Source], subsetVarSpec{
				Visible:   item.Visible,
				SourceVar: sourceVar,
				Emitted:   emitted,
			})
		}
	}

	for _, source := range groupOrder {
		src := sources[source]
		if src != nil && src.Kind == sema.SourceKindLet {
			subset, rowsVar := ctx.ensureScalarLetSubsetParameterSetForStep(stepName, source, grouped[source])
			if subset != "" {
				uses = append(uses, subset)
			}
			if rowsVar != "" {
				sourceRows[source] = rowsVar
			}
			continue
		}
		subset, rowsVar := ctx.ensureSubsetParameterSetForStep(stepName, source, grouped[source], sourceRows[source])
		if subset != "" {
			uses = append(uses, subset)
		}
		if rowsVar != "" {
			sourceRows[source] = rowsVar
		}
	}
	return stepUseResolution{
		Use:        uses,
		SourceRows: sourceRows,
	}
}

func (ctx *lowerContext) stepAliasMap(stepName string, forSubmit bool) map[string]string {
	if !forSubmit {
		return map[string]string{}
	}
	plan := ctx.res.StepImportByName[stepName]
	if plan == nil {
		return map[string]string{}
	}
	out := make(map[string]string)
	for name, origin := range plan.Effective {
		if origin.Kind != sema.SourceKindParam && origin.Kind != sema.SourceKindLet {
			continue
		}
		if sema.IsSubmitKey(name) {
			out[name] = escapedAliasPrefix + name
		}
	}
	return out
}

func sourceNeedsAlias(src *sema.ImportSource, aliases map[string]string) bool {
	if src == nil || len(aliases) == 0 {
		return false
	}
	for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
		if _, ok := aliases[name]; ok {
			return true
		}
	}
	return false
}

func (ctx *lowerContext) inheritedRowsForStep(stepName string, inheritedSteps []string) map[string]string {
	out := make(map[string]string)
	conflicts := make(map[string]struct{})
	for _, dep := range inheritedSteps {
		depRows := ctx.stepSourceRows[dep]
		if len(depRows) == 0 {
			continue
		}
		for source, rowsVar := range depRows {
			if rowsVar == "" {
				continue
			}
			if prev, exists := out[source]; exists && prev != rowsVar {
				if _, reported := conflicts[source]; !reported {
					ctx.diags.AddError(
						diag.CodeE232,
						fmt.Sprintf("conflicting inherited row context for source '%s' in step '%s'", source, stepName),
						ctx.stepSpan(stepName),
						"ensure dependencies constrain the same source consistently",
						diag.RelatedSpan{Message: fmt.Sprintf("dependency '%s'", dep), Span: ctx.stepSpan(dep)},
					)
				}
				conflicts[source] = struct{}{}
				delete(out, source)
				continue
			}
			if _, bad := conflicts[source]; bad {
				continue
			}
			out[source] = rowsVar
		}
	}
	return out
}

func (ctx *lowerContext) stepSpan(stepName string) diag.Span {
	for _, block := range ctx.res.DoBlocks {
		if block.Name == stepName {
			return block.Span
		}
	}
	for _, block := range ctx.res.Submits {
		if block.Name == stepName {
			return block.Span
		}
	}
	return diag.Span{}
}

func cloneStringMap(src map[string]string) map[string]string {
	return maps.Clone(src)
}

func (ctx *lowerContext) ensureSubsetParameterSetForStep(stepName, source string, vars []subsetVarSpec, inheritedRowsVar string) (string, string) {
	varKeys := make([]string, 0, len(vars))
	for _, v := range vars {
		varKeys = append(varKeys, v.Visible+"="+v.SourceVar+"=>"+subsetEmittedName(v))
	}
	k := subsetKey{
		Step:          stepName,
		Source:        source,
		Vars:          strings.Join(varKeys, ","),
		InheritedRows: inheritedRowsVar,
	}
	if existing, ok := ctx.subsetNames[k]; ok {
		return existing.Name, existing.RowsVar
	}

	src := ctx.res.ImportSourceByName[source]
	if src == nil {
		// Semantic analysis already reports unknown parameter set imports with
		// precise spans. Skip lower-stage duplicate diagnostics.
		return "", ""
	}

	rowCount := sourceRowCountFromSource(src)
	if rowCount == 0 {
		rowCount = 1
	}

	visibleNames := make([]string, 0, len(vars))
	emittedByVisible := make(map[string]string, len(vars))
	for _, v := range vars {
		visibleNames = append(visibleNames, v.Visible)
		emittedByVisible[v.Visible] = subsetEmittedName(v)
	}

	baseName := shortSubsetBaseName(stepName, source, visibleNames)
	name := ctx.uniqueName(baseName)
	suffix := strings.TrimPrefix(name, baseName)
	rowsVar := shortSubsetRowsName(stepName, source, visibleNames) + suffix
	idxName := shortSubsetIndexName(stepName, source, visibleNames) + suffix
	idxRef := "$" + idxName

	valuesByName := make(map[string][]eval.Value, len(vars))
	modeByName := make(map[string]string, len(vars))
	sourceVarByVisible := make(map[string]string, len(vars))
	for _, variable := range vars {
		sourceVar := variable.SourceVar
		if sourceVar == "" {
			sourceVar = variable.Visible
		}
		sourceVarByVisible[variable.Visible] = sourceVar
		valuesByName[variable.Visible] = sourceValuesFor(src, sourceVar, rowCount)
		if mode, ok := src.Modes[sourceVar]; ok {
			modeByName[variable.Visible] = mode
		}
	}

	params := make([]Parameter, 0, len(vars)+2)
	if inheritedRowsVar == "" {
		groups := planutil.BuildRowGroups(visibleNames, valuesByName, rowCount, pythonLiteral)
		repIndices := make([]int, 0, len(groups))
		rowGroupStrings := make([]string, 0, len(groups))
		for _, group := range groups {
			repIndices = append(repIndices, group.Rep)
			rowGroupStrings = append(rowGroupStrings, joinIntIndices(group.Rows))
		}
		if len(repIndices) == 0 {
			repIndices = []int{0}
			rowGroupStrings = []string{"0"}
		}
		params = append(params, Parameter{
			Name:  idxName,
			Type:  "int",
			Mode:  "text",
			Value: joinIntIndices(repIndices),
		})
		params = append(params, Parameter{
			Name: rowsVar,
			Mode: "python",
			// Keep row groups like "0,1" opaque at this stage so they are
			// transported as one value across step dependencies.
			Separator: ReservedSeparator,
			Value:     SingleQuoted(pythonStringMapLookupExpr(repIndices, rowGroupStrings, idxName)),
		})
		payload := lowerIndexedPayloadParameters(visibleNames, valuesByName, modeByName, repIndices, idxRef, func(varName string) diag.Span {
			sourceVar := sourceVarByVisible[varName]
			if span, ok := src.Origins[sourceVar]; ok {
				return span
			}
			return src.Span
		}, ctx.diags)
		params = append(params, applyEmittedNames(payload, emittedByVisible)...)
	} else {
		params = append(params, Parameter{
			Name: idxName,
			Type: "int",
			Mode: "text",
			// In inherited context we intentionally split grouped row IDs.
			Separator: ",",
			Value:     "$" + inheritedRowsVar,
		})
		params = append(params, Parameter{
			Name:  rowsVar,
			Mode:  "text",
			Value: "${" + idxName + "}",
		})
		payload := lowerContextualPayloadParameters(visibleNames, valuesByName, modeByName, idxRef, func(varName string) diag.Span {
			sourceVar := sourceVarByVisible[varName]
			if span, ok := src.Origins[sourceVar]; ok {
				return span
			}
			return src.Span
		}, ctx.diags)
		params = append(params, applyEmittedNames(payload, emittedByVisible)...)
	}

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		Parameter: params,
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindSubset,
			Source: source,
			Step:   stepName,
		},
	})
	ctx.names[name] = struct{}{}
	ctx.subsetNames[k] = subsetInfo{Name: name, RowsVar: rowsVar}
	return name, rowsVar
}

func (ctx *lowerContext) ensureScalarLetSubsetParameterSetForStep(stepName, source string, vars []subsetVarSpec) (string, string) {
	varKeys := make([]string, 0, len(vars))
	for _, v := range vars {
		varKeys = append(varKeys, v.Visible+"="+v.SourceVar+"=>"+subsetEmittedName(v))
	}
	k := subsetKey{
		Step:          stepName,
		Source:        source,
		Vars:          strings.Join(varKeys, ","),
		InheritedRows: "",
	}
	if existing, ok := ctx.subsetNames[k]; ok {
		return existing.Name, existing.RowsVar
	}

	src := ctx.res.ImportSourceByName[source]
	if src == nil {
		return "", ""
	}

	visibleNames := make([]string, 0, len(vars))
	for _, v := range vars {
		visibleNames = append(visibleNames, v.Visible)
	}
	baseName := shortSubsetBaseName(stepName, source, visibleNames)
	name := ctx.uniqueName(baseName)

	params := make([]Parameter, 0, len(vars))
	for _, variable := range vars {
		sourceVar := variable.SourceVar
		if sourceVar == "" {
			sourceVar = variable.Visible
		}
		vals := sourceValuesFor(src, sourceVar, 1)
		value := eval.Null()
		if len(vals) > 0 {
			value = vals[0]
		}
		param := Parameter{Name: subsetEmittedName(variable)}
		if mode, ok := src.Modes[sourceVar]; ok && mode != "" {
			param.Mode = mode
			switch mode {
			case "python":
				param.Value = SingleQuoted(asString(value))
			default:
				param.Value = asString(value)
			}
		} else {
			param.Mode = "text"
			param.Value = asString(value)
		}
		params = append(params, param)
	}

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		Parameter: params,
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindSubset,
			Source: source,
			Step:   stepName,
		},
	})
	ctx.names[name] = struct{}{}
	ctx.subsetNames[k] = subsetInfo{Name: name, RowsVar: ""}
	return name, ""
}

func sourceRowCount(ps *sema.Paramset) int {
	if ps == nil {
		return 0
	}
	if n := len(ps.Rows); n > 0 {
		return n
	}
	rowCount := 0
	for _, name := range ps.Order {
		if n := len(ps.Vars[name]); n > rowCount {
			rowCount = n
		}
	}
	return rowCount
}

func sourceRowCountFromSource(src *sema.ImportSource) int {
	if src == nil {
		return 0
	}
	return planutil.SourceRowCount(src.Order, src.Vars)
}

func sourceValuesFor(src *sema.ImportSource, name string, rowCount int) []eval.Value {
	if src == nil {
		return nil
	}
	return planutil.ExpandValues(src.Vars[name], rowCount)
}

func (ctx *lowerContext) uniqueName(base string) string {
	if _, exists := ctx.names[base]; !exists {
		ctx.names[base] = struct{}{}
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if _, exists := ctx.names[candidate]; !exists {
			ctx.names[candidate] = struct{}{}
			return candidate
		}
	}
}

func normalizeRawLiteral(body string) string {
	trimmed := normalizeRawBlock(body)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, "\n") {
		return trimmed
	}
	return trimmed + "\n"
}

func normalizeRawBlock(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")

	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}

	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingIndent(line)
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent < 0 {
		minIndent = 0
	}

	for i, line := range lines {
		lines[i] = strings.TrimRight(stripIndent(line, minIndent), " \t")
	}
	return strings.Join(lines, "\n")
}

func leadingIndent(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' || r == '\t' {
			n++
			continue
		}
		break
	}
	return n
}

func stripIndent(s string, n int) string {
	if n <= 0 {
		return s
	}
	i := 0
	for _, r := range s {
		if i >= n {
			break
		}
		if r != ' ' && r != '\t' {
			break
		}
		i++
	}
	return s[i:]
}

func sanitize(name string) string {
	if name == "" {
		return "x"
	}
	b := strings.Builder{}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func subsetEmittedName(v subsetVarSpec) string {
	if v.Emitted != "" {
		return v.Emitted
	}
	return v.Visible
}

func applyEmittedNames(params []Parameter, emittedByVisible map[string]string) []Parameter {
	if len(params) == 0 || len(emittedByVisible) == 0 {
		return params
	}
	out := make([]Parameter, len(params))
	copy(out, params)
	for i := range out {
		if alias, ok := emittedByVisible[out[i].Name]; ok && alias != "" {
			out[i].Name = alias
		}
	}
	return out
}

func rewriteShellRefsInEvalValue(v eval.Value, aliases map[string]string) eval.Value {
	if len(aliases) == 0 {
		return v
	}
	switch v.Kind {
	case eval.KindString:
		return eval.String(rewriteShellRefs(v.S, aliases))
	case eval.KindList:
		items := make([]eval.Value, len(v.L))
		for i := range v.L {
			items[i] = rewriteShellRefsInEvalValue(v.L[i], aliases)
		}
		return eval.List(items)
	default:
		return v
	}
}

func rewriteShellRefs(text string, aliases map[string]string) string {
	if text == "" || len(aliases) == 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		ch := text[i]
		if ch == '\\' && i+1 < len(text) && text[i+1] == '$' {
			out.WriteByte('\\')
			out.WriteByte('$')
			i += 2
			continue
		}
		if ch != '$' {
			out.WriteByte(ch)
			i++
			continue
		}
		if i+1 >= len(text) {
			out.WriteByte('$')
			i++
			continue
		}
		next := text[i+1]
		if next == '{' {
			if alias, consumed, ok := rewriteBracedShellRef(text[i:], aliases); ok {
				out.WriteString(alias)
				i += consumed
				continue
			}
			out.WriteByte('$')
			i++
			continue
		}
		if isShellVarStart(next) {
			j := i + 2
			for j < len(text) && isShellVarChar(text[j]) {
				j++
			}
			name := text[i+1 : j]
			if alias, ok := aliases[name]; ok && alias != "" {
				out.WriteByte('$')
				out.WriteString(alias)
			} else {
				out.WriteByte('$')
				out.WriteString(name)
			}
			i = j
			continue
		}
		out.WriteByte('$')
		i++
	}
	return out.String()
}

func rewriteBracedShellRef(fragment string, aliases map[string]string) (string, int, bool) {
	if len(fragment) < 4 || fragment[0] != '$' || fragment[1] != '{' {
		return "", 0, false
	}
	if !isShellVarStart(fragment[2]) {
		return "", 0, false
	}
	j := 3
	for j < len(fragment) && isShellVarChar(fragment[j]) {
		j++
	}
	if j >= len(fragment) || fragment[j] != '}' {
		return "", 0, false
	}
	name := fragment[2:j]
	alias := name
	if mapped, ok := aliases[name]; ok && mapped != "" {
		alias = mapped
	}
	return "${" + alias + "}", j + 1, true
}

func isShellVarStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isShellVarChar(ch byte) bool {
	return isShellVarStart(ch) || (ch >= '0' && ch <= '9')
}

func indexVariableName(context string) string {
	return shortParamIndexName(context)
}

// Synthetic naming contract: _ji_<ctx>, _js__<step>__<source>__<vars>, _ji__<step>__<source>__<vars>, _jr__<step>__<source>__<vars>, _jp__<group>_<pattern>__<step>__<alias>.
func shortParamIndexName(context string) string {
	name := sanitize(context)
	if name == "" {
		name = "set"
	}
	return "_ji_" + name
}

func shortSubsetBaseName(step, source string, vars []string) string {
	return "_js__" + sanitize(step) + "__" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_"))
}

func shortSubsetIndexName(step, source string, vars []string) string {
	return "_ji__" + sanitize(step) + "__" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_"))
}

func shortSubsetRowsName(step, source string, vars []string) string {
	return "_jr__" + sanitize(step) + "__" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_"))
}

func shortPatternAliasName(group, pattern, step, alias string) string {
	return "_jp__" + sanitize(group) + "_" + sanitize(pattern) +
		"__" + sanitize(step) + "__" + sanitize(alias)
}

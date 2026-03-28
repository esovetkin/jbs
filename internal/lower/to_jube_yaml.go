package lower

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

const ReservedSeparator = "####"

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
	ParameterSet []ParameterSet `yaml:"parameterset,omitempty"`
	Step         []Step         `yaml:"step,omitempty"`
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

type Step struct {
	Name   string        `yaml:"name"`
	Depend string        `yaml:"depend,omitempty"`
	Use    []interface{} `yaml:"use,omitempty"`
	Do     []interface{} `yaml:"do,omitempty"`
	Meta   StepMeta      `yaml:"-"`
}

type StepKind string

const (
	StepKindDo     StepKind = "do"
	StepKindSubmit StepKind = "submit"
)

type StepMeta struct {
	Kind   StepKind
	Source string
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

type Options struct {
	BenchmarkName string
	Outpath       string
	InputPath     string
}

type subsetKey struct {
	Source string
	Vars   string
}

type lowerContext struct {
	res         *sema.Result
	doc         Document
	diags       *diag.Diagnostics
	names       map[string]struct{}
	subsetNames map[subsetKey]string
}

func ToJUBEYAML(res *sema.Result, opts Options, diags *diag.Diagnostics) Document {
	ctx := &lowerContext{
		res:         res,
		diags:       diags,
		names:       make(map[string]struct{}),
		subsetNames: make(map[subsetKey]string),
	}
	ctx.doc = Document{
		Name:    globalString(res.Globals, "jbs_name", "jbs_benchmark"),
		Outpath: globalString(res.Globals, "jbs_outpath", "out"),
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
			submitSetName := ctx.addSubmitParameterSet(node)
			ctx.doc.Step = append(ctx.doc.Step, ctx.lowerSubmit(node, submitSetName))
		}
	}

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
			"E230",
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
						"E216",
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

func (ctx *lowerContext) lowerDo(block ast.DoBlock) Step {
	step := Step{
		Name: block.Name,
		Meta: StepMeta{
			Kind:   StepKindDo,
			Source: block.Name,
		},
	}
	if len(block.After) > 0 {
		step.Depend = strings.Join(block.After, ",")
	}
	step.Use = ctx.resolveStepUses(block.WithItems)

	body := withPrelude(block.Body)
	step.Do = []interface{}{Literal(body)}
	return step
}

func (ctx *lowerContext) addSubmitParameterSet(block ast.SubmitBlock) string {
	name := ctx.uniqueName(fmt.Sprintf("%s__submit_params", block.Name))
	params := make([]Parameter, 0)
	if spec := ctx.res.SubmitByName[block.Name]; spec != nil {
		for _, field := range spec.Values {
			if field.IsRaw {
				raw := normalizeRawBlock(field.Raw)
				if field.Name == "preprocess" {
					raw = withPrelude(raw)
				}
				params = append(params, Parameter{
					Name:      field.Name,
					Mode:      "text",
					Separator: "|",
					Value:     Literal(raw),
				})
				continue
			}

			param := Parameter{Name: field.Name}
			if t := submitParameterType(field.Name); t != "" {
				param.Type = t
			}
			if field.Mode != "" {
				param.Mode = field.Mode
				if field.Mode == "python" {
					param.Value = SingleQuoted(asString(field.Value))
				} else {
					param.Value = asString(field.Value)
				}
			} else {
				switch field.Value.Kind {
				case eval.KindList, eval.KindNull:
					param.Value = pythonLiteral(field.Value)
				default:
					param.Value = templateValue(field.Value)
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

func (ctx *lowerContext) lowerSubmit(block ast.SubmitBlock, submitSet string) Step {
	step := Step{
		Name: block.Name,
		Meta: StepMeta{
			Kind:   StepKindSubmit,
			Source: block.Name,
		},
	}
	if len(block.After) > 0 {
		step.Depend = strings.Join(block.After, ",")
	}
	use := ctx.resolveStepUses(block.WithItems)
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

func (ctx *lowerContext) resolveStepUses(items []ast.WithItem) []interface{} {
	uses := make([]interface{}, 0)
	grouped := make(map[string][]string)
	groupOrder := make([]string, 0)
	seenDirect := make(map[string]struct{})

	for _, item := range items {
		if item.From == "" {
			if _, seen := seenDirect[item.Name]; seen {
				continue
			}
			seenDirect[item.Name] = struct{}{}
			uses = append(uses, item.Name)
			continue
		}

		// Mixed form support:
		// with x from p1, p2
		// If p2 is not variable in p1 but is an existing parameterset, treat it
		// as full parameterset import.
		if src := ctx.res.ParamByName[item.From]; src != nil {
			if _, ok := src.Vars[item.Name]; !ok {
				if _, isParamset := ctx.res.ParamByName[item.Name]; isParamset {
					if _, seen := seenDirect[item.Name]; seen {
						continue
					}
					seenDirect[item.Name] = struct{}{}
					uses = append(uses, item.Name)
					continue
				}
			}
		}

		if _, ok := grouped[item.From]; !ok {
			grouped[item.From] = make([]string, 0)
			groupOrder = append(groupOrder, item.From)
		}
		if !contains(grouped[item.From], item.Name) {
			grouped[item.From] = append(grouped[item.From], item.Name)
		}
	}

	for _, source := range groupOrder {
		subset := ctx.ensureSubsetParameterSet(source, grouped[source])
		if subset != "" {
			uses = append(uses, subset)
		}
	}
	return uses
}

func (ctx *lowerContext) ensureSubsetParameterSet(source string, vars []string) string {
	k := subsetKey{Source: source, Vars: strings.Join(vars, ",")}
	if existing, ok := ctx.subsetNames[k]; ok {
		return existing
	}

	src := ctx.res.ParamByName[source]
	if src == nil {
		// Semantic analysis already reports unknown parameter set imports with
		// precise spans. Skip lower-stage duplicate diagnostics.
		return ""
	}
	rowCount := len(src.Rows)
	if rowCount == 0 {
		for _, name := range vars {
			if n := len(src.Vars[name]); n > rowCount {
				rowCount = n
			}
		}
	}
	if rowCount == 0 {
		rowCount = 1
	}

	name := ctx.uniqueName("_jbs__subset_" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_")))
	valuesByName := make(map[string][]eval.Value, len(vars))
	for _, variable := range vars {
		valuesByName[variable] = valuesFor(src, variable, rowCount)
	}

	mask := firstOccurrenceMaskIndices(vars, valuesByName, rowCount)
	if len(mask) == 0 {
		mask = []int{0}
	}
	params := lowerIndexedParameters(vars, valuesByName, src.Modes, mask, indexVariableName(name), func(varName string) diag.Span {
		return originFor(src, varName)
	}, ctx.diags)

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		Parameter: params,
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindSubset,
			Source: source,
		},
	})
	ctx.names[name] = struct{}{}
	ctx.subsetNames[k] = name
	return name
}

func firstOccurrenceMaskIndices(vars []string, valuesByName map[string][]eval.Value, rowCount int) []int {
	if rowCount <= 0 {
		return nil
	}
	if len(vars) == 0 {
		return sequentialIndices(rowCount)
	}

	seen := make(map[string]struct{})
	indices := make([]int, 0, rowCount)
	for row := 0; row < rowCount; row++ {
		key := tupleKeyAt(vars, valuesByName, row)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		indices = append(indices, row)
	}
	return indices
}

func tupleKeyAt(vars []string, valuesByName map[string][]eval.Value, row int) string {
	var b strings.Builder
	for _, name := range vars {
		values := valuesByName[name]
		value := eval.Null()
		if row >= 0 && row < len(values) {
			value = values[row]
		}
		lit := pythonLiteral(value)
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(strconv.Itoa(len(lit)))
		b.WriteByte(':')
		b.WriteString(lit)
		b.WriteByte('|')
	}
	return b.String()
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

func withPrelude(body string) string {
	b := strings.Builder{}
	b.WriteString("set -euo pipefail\n")
	b.WriteString("cd \"${jube_benchmark_home}\"\n")
	trimmed := normalizeRawBlock(body)
	if trimmed != "" {
		b.WriteString(trimmed)
		if !strings.HasSuffix(trimmed, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
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

func contains(items []string, item string) bool {
	for _, x := range items {
		if x == item {
			return true
		}
	}
	return false
}

func indexVariableName(context string) string {
	name := sanitize(context)
	if name == "" {
		name = "set"
	}
	return "_jbs__idx_" + name
}

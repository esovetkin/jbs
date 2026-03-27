package lower

import (
	"fmt"
	"path/filepath"
	"sort"
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
}

type ParameterSet struct {
	Name      string      `yaml:"name"`
	InitWith  string      `yaml:"init_with,omitempty"`
	Parameter []Parameter `yaml:"parameter,omitempty"`
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
		Name:    chooseBenchmarkName(opts),
		Outpath: chooseOutpath(opts),
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

func chooseBenchmarkName(opts Options) string {
	if strings.TrimSpace(opts.BenchmarkName) != "" {
		return opts.BenchmarkName
	}
	if strings.TrimSpace(opts.InputPath) != "" {
		base := filepath.Base(opts.InputPath)
		ext := filepath.Ext(base)
		return strings.TrimSuffix(base, ext)
	}
	return "jbs_benchmark"
}

func chooseOutpath(opts Options) string {
	if strings.TrimSpace(opts.Outpath) != "" {
		return opts.Outpath
	}
	return "out"
}

func lowerParamset(ps *sema.Paramset, diags *diag.Diagnostics) ParameterSet {
	out := ParameterSet{Name: ps.Name, Parameter: make([]Parameter, 0)}
	if ps.HasPlus {
		return lowerGroupedParamset(ps, diags)
	}

	for _, name := range ps.Order {
		values := ps.Vars[name]
		if len(values) == 0 {
			diags.AddError(
				"E230",
				fmt.Sprintf("parameter '%s' has no values", name),
				ps.Block.Span,
				"ensure final expression yields at least one row",
			)
			continue
		}
		parts := make([]string, 0, len(values))
		for _, value := range values {
			part := templateValue(value)
			if strings.Contains(part, ReservedSeparator) {
				diags.AddError(
					"E053",
					fmt.Sprintf("value for '%s' contains reserved separator '%s'", name, ReservedSeparator),
					originFor(ps, name),
					"change parameter values to avoid the reserved separator",
				)
			}
			parts = append(parts, part)
		}
		param := Parameter{Name: name}
		if t := inferType(values); t != "" {
			param.Type = t
		}
		if len(parts) > 1 {
			param.Mode = "text"
			param.Separator = ReservedSeparator
			param.Value = strings.Join(parts, ReservedSeparator)
		} else {
			param.Value = parts[0]
		}
		out.Parameter = append(out.Parameter, param)
	}
	return out
}

func lowerGroupedParamset(ps *sema.Paramset, diags *diag.Diagnostics) ParameterSet {
	out := ParameterSet{Name: ps.Name, Parameter: make([]Parameter, 0)}
	rowCount := len(ps.Rows)
	if rowCount == 0 {
		diags.AddError(
			"E230",
			fmt.Sprintf("parameterset '%s' evaluates to zero rows", ps.Name),
			ps.Block.Span,
			"ensure direct-sum operands are non-empty",
		)
		rowCount = 1
	}

	indices := make([]string, rowCount)
	for i := range rowCount {
		indices[i] = strconv.Itoa(i)
	}
	out.Parameter = append(out.Parameter, Parameter{
		Name:  "i",
		Type:  "int",
		Mode:  "text",
		Value: strings.Join(indices, ","),
	})

	for _, name := range ps.Order {
		values := valuesFor(ps, name, rowCount)
		out.Parameter = append(out.Parameter, Parameter{
			Name:  name,
			Mode:  "python",
			Value: SingleQuoted(pythonIndexExpr(values, "$i")),
		})
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
	case eval.KindDict:
		keys := make([]string, 0, len(v.D))
		for k := range v.D {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, strconv.Quote(key)+":"+pythonLiteral(v.D[key]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		return strconv.Quote(v.String())
	}
}

func (ctx *lowerContext) lowerDo(block ast.DoBlock) Step {
	step := Step{Name: block.Name}
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
	visible := ctx.visibleNames(block.WithItems)

	params := make([]Parameter, 0)
	for _, spec := range BuiltinGlobals() {
		if spec.Target == "" {
			continue
		}
		value := spec.DefaultExpr
		mode := spec.Mode
		if visible[spec.Name] {
			value = "$" + spec.Name
			mode = ""
		}
		p := Parameter{Name: spec.Target, Value: value}
		if mode != "" {
			p.Mode = mode
			if mode == "python" {
				p.Value = SingleQuoted(value)
			}
		}
		if spec.Type != "" {
			p.Type = spec.Type
		}
		params = append(params, p)
	}

	params = append(params, Parameter{Name: "hint", Value: "nomultithread"})
	params = append(params, Parameter{
		Name:      "env",
		Mode:      "text",
		Separator: "|",
		Value:     Literal(trimOuterNewlines(block.EnvBody)),
	})
	params = append(params, Parameter{Name: "args_exec", Value: trimOuterNewlines(block.RunBody)})

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		InitWith:  "platform.xml:systemParameter",
		Parameter: params,
	})
	ctx.names[name] = struct{}{}
	return name
}

func (ctx *lowerContext) lowerSubmit(block ast.SubmitBlock, submitSet string) Step {
	step := Step{Name: block.Name}
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

	name := ctx.uniqueName("__subset_" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_")))
	indices := make([]string, rowCount)
	for i := range rowCount {
		indices[i] = strconv.Itoa(i)
	}
	params := []Parameter{{Name: "i", Type: "int", Mode: "text", Value: strings.Join(indices, ",")}}
	for _, variable := range vars {
		values := make([]eval.Value, 0, rowCount)
		if len(src.Rows) > 0 {
			for _, row := range src.Rows {
				if cell, ok := row.Values[variable]; ok {
					values = append(values, cell.Value)
				}
			}
		}
		if len(values) == 0 {
			base := src.Vars[variable]
			if len(base) == 0 {
				for range rowCount {
					values = append(values, eval.Null())
				}
			} else {
				for i := range rowCount {
					values = append(values, base[i%len(base)])
				}
			}
		}
		params = append(params, Parameter{Name: variable, Mode: "python", Value: SingleQuoted(pythonIndexExpr(values, "$i"))})
	}

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{Name: name, Parameter: params})
	ctx.names[name] = struct{}{}
	ctx.subsetNames[k] = name
	return name
}

func (ctx *lowerContext) visibleNames(items []ast.WithItem) map[string]bool {
	visible := make(map[string]bool)
	for _, item := range items {
		if item.From == "" {
			src := ctx.res.ParamByName[item.Name]
			if src == nil {
				continue
			}
			for _, name := range src.Order {
				visible[name] = true
			}
			continue
		}
		if src := ctx.res.ParamByName[item.From]; src != nil {
			if _, ok := src.Vars[item.Name]; !ok {
				if ps := ctx.res.ParamByName[item.Name]; ps != nil {
					for _, name := range ps.Order {
						visible[name] = true
					}
					continue
				}
			}
		}
		visible[item.Name] = true
	}
	return visible
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
	trimmed := trimOuterNewlines(body)
	if trimmed != "" {
		b.WriteString(trimmed)
		if !strings.HasSuffix(trimmed, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func trimOuterNewlines(s string) string {
	s = strings.TrimPrefix(s, "\n")
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\r")
	return s
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

// lowers `analyse` semantics into JUBE `patternset`, `analyser`,
// `result` sections
//
// emit/merge pattern groups, creates stable internal alias pattern
// names for analyse assignments, wires analyser file extraction per
// step, and builds result columns with correct source expressions and
// titles
package lower

import (
	"slices"
	"strings"

	"jbs/internal/sema"
)

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

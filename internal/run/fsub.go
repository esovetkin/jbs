package run

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsubutil"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/patternutil"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"
)

type FileSubstitutionPlan struct {
	SourcePath string
	DestName   string
	Rules      []FileSubstitutionRulePlan
	Span       diag.Span
}

type FileSubstitutionRulePlan struct {
	Pattern      string
	RegexPattern string
	Regex        *regexp.Regexp
	Expr         ast.Expr
	Span         diag.Span
}

type FileSubstitutionWarning struct {
	Step     string
	Row      int
	DestName string
	Pattern  string
	Matches  int
}

type fsubTemplateSnapshot struct {
	Data []byte
	Perm fs.FileMode
}

func fileSubPlansByStep(res *sema.Result) (map[string][]FileSubstitutionPlan, error) {
	out := make(map[string][]FileSubstitutionPlan)
	if res == nil {
		return out, nil
	}
	for _, block := range res.DoBlocks {
		if len(block.FSubs) == 0 {
			continue
		}
		base := fileSubBaseDir(res, block.Span)
		for _, fsub := range block.FSubs {
			sourcePath := resolveFSubTemplatePath(base, fsub.Path)
			destName := fsubutil.DestName(fsub.Path)
			rules, err := compileFSubRules(fsub.Rules)
			if err != nil {
				return nil, err
			}
			out[block.Name] = append(out[block.Name], FileSubstitutionPlan{
				SourcePath: sourcePath,
				DestName:   destName,
				Rules:      rules,
				Span:       fsub.Span,
			})
		}
	}
	return out, nil
}

func fileSubBaseDir(res *sema.Result, span diag.Span) string {
	if res != nil {
		if base := strings.TrimSpace(res.BaseDirByFile[span.File]); base != "" {
			return filepath.Clean(base)
		}
	}
	file := strings.TrimSpace(span.File)
	if file != "" && !strings.HasPrefix(file, "<") {
		return filepath.Dir(filepath.Clean(file))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return filepath.Clean(cwd)
}

func resolveFSubTemplatePath(base, raw string) string {
	raw = strings.TrimSpace(raw)
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	return filepath.Clean(filepath.Join(base, raw))
}

func compileFSubRules(rules []ast.FileSubstitutionRule) ([]FileSubstitutionRulePlan, error) {
	out := make([]FileSubstitutionRulePlan, 0, len(rules))
	for _, rule := range rules {
		normalized, ok := patternutil.NormalizePercentPattern(rule.Pattern)
		if !ok {
			return nil, fmt.Errorf("invalid placeholder in fsub regex %q: supported placeholders are %%d, %%f, %%w and %%%% for a literal percent", rule.Pattern)
		}
		re, err := regexp.Compile(normalized.Regex)
		if err != nil {
			return nil, fmt.Errorf("invalid fsub regex %q: %w", rule.Pattern, err)
		}
		out = append(out, FileSubstitutionRulePlan{
			Pattern:      rule.Pattern,
			RegexPattern: normalized.Regex,
			Regex:        re,
			Expr:         rule.Expr,
			Span:         rule.Span,
		})
	}
	return out, nil
}

func cloneFileSubstitutionPlans(in []FileSubstitutionPlan) []FileSubstitutionPlan {
	if len(in) == 0 {
		return nil
	}
	out := make([]FileSubstitutionPlan, len(in))
	for i, spec := range in {
		out[i] = spec
		out[i].Rules = append([]FileSubstitutionRulePlan(nil), spec.Rules...)
	}
	return out
}

func sourceHashWithFileSubs(sources map[string]string, fileSubs map[string][]FileSubstitutionPlan) (string, []TemplateHash, error) {
	bundle := maps.Clone(sources)
	if bundle == nil {
		bundle = make(map[string]string)
	}
	hashes := make([]TemplateHash, 0)
	for _, step := range slices.Sorted(maps.Keys(fileSubs)) {
		for _, spec := range fileSubs[step] {
			snap, err := readFSubTemplateSnapshot(spec.SourcePath)
			if err != nil {
				return "", nil, err
			}
			sourcePath := filepath.Clean(spec.SourcePath)
			label := "fsub:" + step + ":" + spec.DestName + ":" + sourcePath
			bundle[label] = string(snap.Data)
			hashes = append(hashes, TemplateHash{
				Step:       step,
				SourcePath: sourcePath,
				DestName:   spec.DestName,
				SHA256:     sha256Hex(snap.Data),
				Mode:       formatTemplatePerm(snap.Perm),
			})
		}
	}
	return SourceBundleHash(bundle), hashes, nil
}

func validateFSubTemplateSource(path string) error {
	_, err := readFSubTemplateSnapshot(path)
	return err
}

func readFSubTemplateSnapshot(path string) (fsubTemplateSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fsubTemplateSnapshot{}, fmt.Errorf("fsub template %s not found: %w", path, err)
		}
		return fsubTemplateSnapshot{}, fmt.Errorf("open fsub template %s: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fsubTemplateSnapshot{}, fmt.Errorf("stat fsub template %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fsubTemplateSnapshot{}, fmt.Errorf("fsub template %s is not a regular file", path)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return fsubTemplateSnapshot{}, fmt.Errorf("read fsub template %s: %w", path, err)
	}
	return fsubTemplateSnapshot{Data: data, Perm: info.Mode().Perm()}, nil
}

func formatTemplatePerm(perm fs.FileMode) string {
	return fmt.Sprintf("%04o", uint32(perm.Perm()))
}

func validateTemplateHashes(runDir string, stored, current []TemplateHash) error {
	storedByKey := templateHashMap(stored)
	currentByKey := templateHashMap(current)
	for key, want := range storedByKey {
		got, ok := currentByKey[key]
		if !ok {
			return fmt.Errorf("cannot continue %s: fsub template %s for step %s is no longer configured", runDir, want.DestName, want.Step)
		}
		if got.SHA256 != want.SHA256 {
			return fmt.Errorf("cannot continue %s: fsub template %s hash does not match current file", runDir, want.DestName)
		}
		if want.Mode != "" && got.Mode != "" && got.Mode != want.Mode {
			return fmt.Errorf("cannot continue %s: fsub template %s mode does not match current file (stored %s, current %s)", runDir, want.DestName, want.Mode, got.Mode)
		}
	}
	for key, got := range currentByKey {
		if _, ok := storedByKey[key]; !ok {
			return fmt.Errorf("cannot continue %s: fsub template %s for step %s was not part of the prepared run", runDir, got.DestName, got.Step)
		}
	}
	return nil
}

func templateHashMap(items []TemplateHash) map[string]TemplateHash {
	out := make(map[string]TemplateHash, len(items))
	for _, item := range items {
		key := item.Step + "\x00" + item.DestName + "\x00" + filepath.Clean(item.SourcePath)
		out[key] = item
	}
	return out
}

func workValuesByKey(wp workplan.Plan) map[string]map[string]eval.Value {
	out := make(map[string]map[string]eval.Value, len(wp.Work))
	for _, work := range wp.Work {
		out[workKey(work.StepName, work.ID.Row)] = work.Values
	}
	return out
}

func materializeFileSubstitutions(workDir string, work ManifestWork, specs []FileSubstitutionPlan, env map[string]eval.Value) ([]FileSubstitutionWarning, error) {
	warnings := make([]FileSubstitutionWarning, 0)
	for _, spec := range specs {
		snap, err := readFSubTemplateSnapshot(spec.SourcePath)
		if err != nil {
			return nil, err
		}
		text := string(snap.Data)
		for _, rule := range spec.Rules {
			next, matches, err := applyFSubRule(text, rule, env)
			if err != nil {
				return nil, fmt.Errorf("step %s row %s file %s: %w", work.Step, rowDir(work.Row), spec.DestName, err)
			}
			if matches > 1 {
				warnings = append(warnings, FileSubstitutionWarning{
					Step:     work.Step,
					Row:      work.Row,
					DestName: spec.DestName,
					Pattern:  rule.Pattern,
					Matches:  matches,
				})
			}
			text = next
		}
		out := filepath.Join(workDir, spec.DestName)
		if err := fsutil.WriteFileAtomic(out, []byte(text), snap.Perm, durableWrite); err != nil {
			return nil, fmt.Errorf("write fsub output %s: %w", out, err)
		}
	}
	return warnings, nil
}

func evalFSubReplacement(expr ast.Expr, env map[string]eval.Value) (eval.Value, error) {
	diags := &diag.Diagnostics{}
	value := eval.EvalExprWithOptions(expr, env, diags, eval.ExprOptions{
		ShellRunner: func(eval.ShellCommand) ([]byte, error) {
			return nil, fmt.Errorf("shell() is not supported in fsub replacement expressions")
		},
	})
	if diags.HasErrors() {
		return eval.Null(), fmt.Errorf("%s", diags.String())
	}
	return value, nil
}

func scalarReplacement(value eval.Value) (string, bool) {
	switch value.Kind {
	case eval.KindString, eval.KindInt, eval.KindFloat, eval.KindBool:
		return value.String(), true
	default:
		return "", false
	}
}

func replacementParts(value eval.Value) ([]string, bool) {
	switch value.Kind {
	case eval.KindList, eval.KindTuple:
		parts := make([]string, 0, len(value.L))
		for _, item := range value.L {
			text, ok := scalarReplacement(item)
			if !ok {
				return nil, false
			}
			parts = append(parts, text)
		}
		return parts, true
	default:
		text, ok := scalarReplacement(value)
		if !ok {
			return nil, false
		}
		return []string{text}, true
	}
}

func applyFSubRule(text string, rule FileSubstitutionRulePlan, env map[string]eval.Value) (string, int, error) {
	value, err := evalFSubReplacement(rule.Expr, env)
	if err != nil {
		return "", 0, err
	}
	parts, ok := replacementParts(value)
	if !ok {
		return "", 0, fmt.Errorf("fsub replacement for %q must be scalar or tuple/list of scalars", rule.Pattern)
	}
	if rule.Regex.NumSubexp() == 0 {
		if len(parts) != 1 {
			return "", 0, fmt.Errorf("fsub regex %q has no capture groups but replacement has %d values", rule.Pattern, len(parts))
		}
		matches := rule.Regex.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			return "", 0, fmt.Errorf("fsub regex %q did not match", rule.Pattern)
		}
		return rule.Regex.ReplaceAllLiteralString(text, parts[0]), len(matches), nil
	}
	if len(parts) != rule.Regex.NumSubexp() {
		return "", 0, fmt.Errorf("fsub regex %q has %d capture groups but replacement has %d values", rule.Pattern, rule.Regex.NumSubexp(), len(parts))
	}
	return replaceCaptureGroups(text, rule.Regex, parts)
}

func replaceCaptureGroups(text string, re *regexp.Regexp, parts []string) (string, int, error) {
	matches := re.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return "", 0, fmt.Errorf("fsub regex %q did not match", re.String())
	}
	var b strings.Builder
	last := 0
	for _, match := range matches {
		b.WriteString(text[last:match[0]])
		cursor := match[0]
		for group := 1; group <= len(parts); group++ {
			start := match[group*2]
			end := match[group*2+1]
			if start < 0 || end < 0 {
				return "", 0, fmt.Errorf("fsub regex %q matched without capture group %d", re.String(), group)
			}
			if start < cursor {
				return "", 0, fmt.Errorf("fsub regex %q has overlapping capture groups", re.String())
			}
			b.WriteString(text[cursor:start])
			b.WriteString(parts[group-1])
			cursor = end
		}
		b.WriteString(text[cursor:match[1]])
		last = match[1]
	}
	b.WriteString(text[last:])
	return b.String(), len(matches), nil
}

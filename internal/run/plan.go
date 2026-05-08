package run

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"
)

type runtimePlan struct {
	WorkPlan            workplan.Plan
	Manifest            Manifest
	Bodies              map[string]string
	Analyses            map[string]AnalysePlan
	SourceDir           string
	NoStrict            bool
	AnalyseDatabase     string
	AnalyseDatabasePath string
}

func buildRuntimePlan(opts Options, diags *diag.Diagnostics) (runtimePlan, error) {
	res := opts.Result
	if res == nil {
		return runtimePlan{}, fmt.Errorf("missing analysis result")
	}
	name, err := benchmarkName(res)
	if err != nil {
		return runtimePlan{}, err
	}
	globalNProc, err := globalNProc(res)
	if err != nil {
		return runtimePlan{}, err
	}
	defaultNProc := availableNProcForRun()
	globalNProc, err = resolveNProc(globalNProc, defaultNProc)
	if err != nil {
		return runtimePlan{}, err
	}
	database, err := globalAnalyseDatabase(res)
	if err != nil {
		return runtimePlan{}, err
	}
	hash := SourceBundleHash(opts.Sources)
	wp := workplan.Build(res, diags)
	wp.BenchmarkName = name
	wp.SourceHash = hash
	wp.GlobalNProc = globalNProc
	if len(wp.Steps) == 0 {
		return runtimePlan{}, fmt.Errorf("jbs run requires at least one do block")
	}
	sourceDir, err := sourceDirForRun(opts)
	if err != nil {
		return runtimePlan{}, err
	}

	usedDirs := make(map[string]struct{})
	stepDirs := make(map[string]string, len(wp.Steps))
	bodies := make(map[string]string, len(wp.Steps))
	for i := range wp.Steps {
		step := &wp.Steps[i]
		step.DirName = stepDirName(step.Name, usedDirs)
		stepDirs[step.Name] = step.DirName
		bodies[step.Name] = step.Body
		stepNProc, err := resolveNProc(step.NProc, defaultNProc)
		if err != nil {
			return runtimePlan{}, fmt.Errorf("do step %q has invalid nproc=%d", step.Name, step.NProc)
		}
		step.NProc = stepNProc
	}

	analyses, err := analysePlansByStep(res)
	if err != nil {
		return runtimePlan{}, err
	}
	if database.Path != "" {
		for step, plan := range analyses {
			if dup := duplicateHeader(plan.Header); dup != "" {
				return runtimePlan{}, fmt.Errorf("analyse step %q cannot be written to SQLite: duplicate result column %q", step, dup)
			}
		}
	}
	manifest := Manifest{
		Schema:              1,
		SourceHash:          hash,
		BenchmarkName:       name,
		GlobalNProc:         globalNProc,
		AnalyseDatabase:     database.Display,
		AnalyseDatabasePath: database.Path,
		Steps:               make([]ManifestStep, 0, len(wp.Steps)),
		Work:                make([]ManifestWork, 0, len(wp.Work)),
	}
	for _, step := range wp.Steps {
		ms := ManifestStep{Name: step.Name, Dir: step.DirName, NProc: step.NProc}
		if _, ok := analyses[step.Name]; ok {
			if database.Path == "" {
				ms.AnalyseCSV = "analyse.csv"
			} else {
				ms.AnalyseTable = step.Name
			}
		}
		manifest.Steps = append(manifest.Steps, ms)
	}
	for _, work := range wp.Work {
		values := make(map[string]string, len(work.Values))
		for _, name := range slices.Sorted(maps.Keys(work.Values)) {
			if !shellName.MatchString(name) {
				return runtimePlan{}, fmt.Errorf("variable %q cannot be emitted as a shell assignment", name)
			}
			values[name] = work.Values[name].String()
		}
		deps := make([]ManifestWorkRef, 0, len(work.Deps))
		usedLinks := make(map[string]struct{}, len(work.Deps))
		for i, dep := range work.Deps {
			link := safePathComponent(dep.Step)
			if link == "" {
				link = "dep"
			}
			if _, ok := usedLinks[link]; ok {
				link = fmt.Sprintf("dep_%d_%s", i, link)
			}
			usedLinks[link] = struct{}{}
			deps = append(deps, ManifestWorkRef{Step: dep.Step, Row: dep.Row, Link: link})
		}
		manifest.Work = append(manifest.Work, ManifestWork{
			Step:   work.StepName,
			Row:    work.ID.Row,
			Dir:    rowDir(work.ID.Row),
			Deps:   deps,
			Values: values,
		})
	}
	return runtimePlan{
		WorkPlan:            wp,
		Manifest:            manifest,
		Bodies:              bodies,
		Analyses:            analyses,
		SourceDir:           sourceDir,
		NoStrict:            opts.NoStrict,
		AnalyseDatabase:     database.Display,
		AnalyseDatabasePath: database.Path,
	}, nil
}

func sourceDirForRun(opts Options) (string, error) {
	path := strings.TrimSpace(opts.ProgramFile)
	if path == "" {
		path = strings.TrimSpace(opts.Input)
	}
	if path == "" || strings.HasPrefix(path, "<") {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determine source directory: %w", err)
		}
		return filepath.Clean(cwd), nil
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve source file %q: %w", path, err)
		}
		path = abs
	}
	return filepath.Dir(filepath.Clean(path)), nil
}

func benchmarkName(res *sema.Result) (string, error) {
	value := res.Globals.Values["jbs_name"]
	if value.Kind == "" {
		value = eval.String("jbs_benchmark")
	}
	if value.Kind != eval.KindString {
		return "", fmt.Errorf("jbs_name must be a string")
	}
	name := safePathComponent(value.S)
	if name == "" {
		return "", fmt.Errorf("jbs_name does not produce a valid directory name")
	}
	return name, nil
}

func globalNProc(res *sema.Result) (int, error) {
	value := res.Globals.Values["jbs_nproc"]
	if value.Kind == "" {
		return 0, nil
	}
	if value.Kind != eval.KindInt {
		return 0, fmt.Errorf("jbs_nproc must be an integer >= 0")
	}
	if value.I < 0 {
		return 0, fmt.Errorf("jbs_nproc must be >= 0")
	}
	return int(value.I), nil
}

type analyseDatabaseConfig struct {
	Display string
	Path    string
}

func globalAnalyseDatabase(res *sema.Result) (analyseDatabaseConfig, error) {
	value := res.Globals.Values["jbs_database"]
	if value.Kind == "" {
		return analyseDatabaseConfig{}, nil
	}
	if value.Kind != eval.KindString {
		return analyseDatabaseConfig{}, fmt.Errorf("jbs_database must be a string")
	}
	return resolveAnalyseDatabasePath(value.S)
}

func resolveAnalyseDatabasePath(raw string) (analyseDatabaseConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return analyseDatabaseConfig{}, nil
	}
	display := filepath.Clean(raw)
	if display == "." || filepath.Base(display) == "." || filepath.Base(display) == ".." {
		return analyseDatabaseConfig{}, fmt.Errorf("jbs_database must name a database file")
	}
	path := display
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return analyseDatabaseConfig{}, fmt.Errorf("resolve jbs_database %q: %w", raw, err)
		}
		path = abs
	}
	return analyseDatabaseConfig{Display: display, Path: filepath.Clean(path)}, nil
}

func analysePlansByStep(res *sema.Result) (map[string]AnalysePlan, error) {
	out := make(map[string]AnalysePlan)
	for _, spec := range res.Analyse {
		if spec == nil {
			continue
		}
		if spec.StepKind != "do" {
			return nil, fmt.Errorf("analyse %q targets unsupported step kind %q", spec.Name, spec.StepKind)
		}
		if _, ok := out[spec.Name]; ok {
			return nil, fmt.Errorf("multiple analyse blocks target step %q", spec.Name)
		}
		plan, err := buildAnalysePlan(spec)
		if err != nil {
			return nil, err
		}
		out[spec.Name] = plan
	}
	return out, nil
}

func buildAnalysePlan(spec *sema.AnalyseSpec) (AnalysePlan, error) {
	selected := selectedPatternNames(spec)
	patterns := make(map[string]AnalysePatternPlan, len(selected))
	for _, assign := range spec.Assignments {
		if _, ok := selected[assign.Name]; !ok {
			continue
		}
		re, err := regexp.Compile(assign.Template.Regex)
		if err != nil {
			return AnalysePlan{}, fmt.Errorf("analyse %q pattern %q is invalid: %w", spec.Name, assign.Name, err)
		}
		groups := re.NumSubexp()
		if groups == 0 {
			return AnalysePlan{}, fmt.Errorf("analyse %q pattern %q must contain at least one capture group", spec.Name, assign.Name)
		}
		patterns[assign.Name] = AnalysePatternPlan{
			Name:         assign.Name,
			File:         assign.File,
			Regex:        assign.Template.Regex,
			GroupCount:   groups,
			CompiledExpr: re,
		}
	}

	plan := AnalysePlan{
		Step:     spec.Name,
		CSV:      "analyse.csv",
		Header:   []string{"run_id"},
		Columns:  make([]AnalyseColumnPlan, 0, len(spec.Columns)),
		Patterns: patterns,
	}
	for _, col := range spec.Columns {
		source := col.Source
		if source == "" {
			source = col.Name
		}
		title := col.Title
		if title == "" {
			title = col.Name
		}
		if p, ok := patterns[source]; ok {
			plan.Columns = append(plan.Columns, AnalyseColumnPlan{
				Kind:       analyseColumnPattern,
				Source:     source,
				Title:      title,
				GroupCount: p.GroupCount,
			})
			plan.Header = appendExpandedHeader(plan.Header, title, p.GroupCount)
			continue
		}
		plan.Columns = append(plan.Columns, AnalyseColumnPlan{
			Kind:       analyseColumnWorkValue,
			Source:     source,
			Title:      title,
			GroupCount: 1,
		})
		plan.Header = append(plan.Header, title)
	}
	return plan, nil
}

func selectedPatternNames(spec *sema.AnalyseSpec) map[string]struct{} {
	assignments := make(map[string]struct{}, len(spec.Assignments))
	for _, assign := range spec.Assignments {
		assignments[assign.Name] = struct{}{}
	}
	out := make(map[string]struct{})
	for _, col := range spec.Columns {
		source := col.Source
		if source == "" {
			source = col.Name
		}
		if _, ok := assignments[source]; ok {
			out[source] = struct{}{}
		}
	}
	return out
}

func appendExpandedHeader(header []string, title string, groupCount int) []string {
	if groupCount <= 1 {
		return append(header, title)
	}
	for i := 0; i < groupCount; i++ {
		header = append(header, fmt.Sprintf("%s.%d", title, i))
	}
	return header
}

func duplicateHeader(header []string) string {
	seen := make(map[string]struct{}, len(header))
	for _, col := range header {
		if _, ok := seen[col]; ok {
			return col
		}
		seen[col] = struct{}{}
	}
	return ""
}

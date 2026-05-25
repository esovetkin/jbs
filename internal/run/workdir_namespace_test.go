package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestCreateRunDirectoryRejectsWorkDirNamespaceCollisions(t *testing.T) {
	t.Run("dependency link collides with reserved artifact before staging", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		plan := namespaceCollisionPlan("stdout", nil)
		_, _, err := CreateRunDirectoryWithInitial(root, plan, StatusNotStarted)
		if err == nil || !strings.Contains(err.Error(), `work-directory entry "stdout"`) ||
			!strings.Contains(err.Error(), `dependency link for step "stdout"`) ||
			!strings.Contains(err.Error(), "runtime stdout file") {
			t.Fatalf("error = %v, want stdout dependency namespace collision", err)
		}
		if matches, globErr := filepath.Glob(filepath.Join(root, ".creating-*")); globErr != nil {
			t.Fatal(globErr)
		} else if len(matches) != 0 {
			t.Fatalf("staging directory was created despite validation failure: %v", matches)
		}
	})

	t.Run("fsub output collides with dependency link", func(t *testing.T) {
		plan := namespaceCollisionPlan("prep", map[string][]FileSubstitutionPlan{
			"child": {{SourcePath: "prep", DestName: "prep"}},
		})
		_, _, err := CreateRunDirectoryWithInitial(filepath.Join(t.TempDir(), "bench"), plan, StatusNotStarted)
		if err == nil || !strings.Contains(err.Error(), `work-directory entry "prep"`) ||
			!strings.Contains(err.Error(), `fsub output for template "prep"`) ||
			!strings.Contains(err.Error(), `dependency link for step "prep"`) {
			t.Fatalf("error = %v, want fsub/dependency namespace collision", err)
		}
	})

	for _, name := range []string{"run.sh", "stdout", "stderr", "status", "exitcode"} {
		t.Run("fsub output collides with reserved "+name, func(t *testing.T) {
			plan := namespaceCollisionPlan("prep", map[string][]FileSubstitutionPlan{
				"child": {{SourcePath: name, DestName: name}},
			})
			_, _, err := CreateRunDirectoryWithInitial(filepath.Join(t.TempDir(), "bench"), plan, StatusNotStarted)
			if err == nil || !strings.Contains(err.Error(), `work-directory entry "`+name+`"`) ||
				!strings.Contains(err.Error(), `fsub output for template "`+name+`"`) ||
				!strings.Contains(err.Error(), "runtime ") {
				t.Fatalf("error = %v, want reserved fsub namespace collision for %s", err, name)
			}
		})
	}

	t.Run("duplicate fsub destinations", func(t *testing.T) {
		plan := singleStepNamespacePlan(map[string][]FileSubstitutionPlan{
			"step": {
				{SourcePath: "a.tpl", DestName: "config"},
				{SourcePath: "b.tpl", DestName: "config"},
			},
		})
		_, _, err := CreateRunDirectoryWithInitial(filepath.Join(t.TempDir(), "bench"), plan, StatusNotStarted)
		if err == nil || !strings.Contains(err.Error(), `work-directory entry "config"`) ||
			!strings.Contains(err.Error(), `fsub output for template "b.tpl"`) ||
			!strings.Contains(err.Error(), `fsub output for template "a.tpl"`) {
			t.Fatalf("error = %v, want duplicate fsub destination collision", err)
		}
	})
}

func TestValidateWorkDirNamespacesRejectsEmptyEntries(t *testing.T) {
	manifest := Manifest{
		Work: []ManifestWork{{
			Step: "child",
			Row:  0,
			Deps: []ManifestWorkRef{{Step: "prep"}},
		}},
	}
	if err := validateWorkDirNamespaces(manifest, nil); err == nil || !strings.Contains(err.Error(), "empty work-directory entry") {
		t.Fatalf("error = %v, want empty dependency link error", err)
	}

	manifest.Work[0].Deps = nil
	if err := validateWorkDirNamespaces(manifest, map[string][]FileSubstitutionPlan{
		"child": {{SourcePath: "empty.tpl"}},
	}); err == nil || !strings.Contains(err.Error(), "empty work-directory entry") {
		t.Fatalf("error = %v, want empty fsub destination error", err)
	}
}

func TestBuildRuntimePlanRejectsWorkDirNamespaceCollisions(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"

do stdout {
  echo parent
}

do child after stdout {
  echo child
}
`)
	if err == nil || !strings.Contains(err.Error(), `work-directory entry "stdout"`) ||
		!strings.Contains(err.Error(), `dependency link for step "stdout"`) {
		t.Fatalf("error = %v, want stdout dependency namespace collision", err)
	}

	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "prep"), []byte("TOKEN\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = buildPlanFromSourceErrInDir(t, `
jbs_name = "bench"

do prep {
  echo parent > out.txt
}

do child after prep
  fsub "prep" { "TOKEN": "ok" }
{
  test -d prep
}
`, cwd)
	if err == nil || !strings.Contains(err.Error(), `work-directory entry "prep"`) ||
		!strings.Contains(err.Error(), `fsub output`) ||
		!strings.Contains(err.Error(), `dependency link for step "prep"`) {
		t.Fatalf("error = %v, want fsub/dependency namespace collision", err)
	}
}

func namespaceCollisionPlan(depName string, fileSubs map[string][]FileSubstitutionPlan) runtimePlan {
	return runtimePlan{
		Manifest: Manifest{
			Schema:        1,
			SourceHash:    "hash",
			BenchmarkName: "bench",
			GlobalNProc:   1,
			Steps: []ManifestStep{
				{Name: depName, Dir: depName, NProc: 1},
				{Name: "child", Dir: "child", NProc: 1},
			},
			Work: []ManifestWork{
				{Step: depName, Row: 0, Dir: "000000"},
				{
					Step: "child",
					Row:  0,
					Dir:  "000000",
					Deps: []ManifestWorkRef{{Step: depName, Row: 0, Link: depName}},
				},
			},
		},
		Bodies:    map[string]string{depName: "echo parent", "child": "echo child"},
		FileSubs:  fileSubs,
		SourceDir: ".",
	}
}

func singleStepNamespacePlan(fileSubs map[string][]FileSubstitutionPlan) runtimePlan {
	return runtimePlan{
		Manifest: Manifest{
			Schema:        1,
			SourceHash:    "hash",
			BenchmarkName: "bench",
			GlobalNProc:   1,
			Steps:         []ManifestStep{{Name: "step", Dir: "step", NProc: 1}},
			Work:          []ManifestWork{{Step: "step", Row: 0, Dir: "000000"}},
		},
		Bodies:    map[string]string{"step": "true"},
		FileSubs:  fileSubs,
		SourceDir: ".",
	}
}

func buildPlanFromSourceErrInDir(t *testing.T, source, cwd string) (runtimePlan, error) {
	t.Helper()
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return buildRuntimePlan(Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: "test.jbs",
	}, diags)
}

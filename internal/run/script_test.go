package run

import (
	"strings"
	"testing"
)

func TestRenderRunScriptUsesFinalAbsolutePathsAndSourceDir(t *testing.T) {
	script, err := renderRunScript(runScriptSpec{
		RunDir:    "/tmp/project/bench/000000",
		WorkDir:   "/tmp/project/bench/000000/s/000001",
		SourceDir: "/tmp/project/cases",
		StepName:  "s",
		Work: ManifestWork{
			Step:   "s",
			Row:    1,
			Values: map[string]string{"x": "42"},
		},
		Body: "echo \"$x\"\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, ".creating-") {
		t.Fatalf("script leaked staging path:\n%s", script)
	}
	if !strings.Contains(script, "\nset -euo pipefail\n\n") {
		t.Fatalf("script missing strict mode:\n%s", script)
	}
	for _, want := range []string{
		"export JBS_RUN_DIR='/tmp/project/bench/000000'",
		"export JBS_WORK_DIR='/tmp/project/bench/000000/s/000001'",
		"export JBS_SRC_DIR='/tmp/project/cases'",
		"export JBS_ROW='000001'",
		"export JBS_STEP='s'",
		"export x='42'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestRenderRunScriptExportsQuotedWorkValues(t *testing.T) {
	script, err := renderRunScript(runScriptSpec{
		RunDir:    "/tmp/project/bench/000000",
		WorkDir:   "/tmp/project/bench/000000/s/000000",
		SourceDir: "/tmp/project",
		StepName:  "s",
		Work: ManifestWork{
			Step: "s",
			Row:  0,
			Values: map[string]string{
				"empty": "",
				"quote": "a'b",
			},
		},
		Body: "true\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"export empty=''",
		"export quote='a'\"'\"'b'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestRenderRunScriptExportsAliasedWorkValuesOnly(t *testing.T) {
	script, err := renderRunScript(runScriptSpec{
		RunDir:    "/tmp/project/bench/000000",
		WorkDir:   "/tmp/project/bench/000000/s/000000",
		SourceDir: "/tmp/project",
		StepName:  "s",
		Work: ManifestWork{
			Step:   "s",
			Row:    0,
			Values: map[string]string{"y": "7"},
		},
		Body: "echo \"$y\"\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "export y='7'") {
		t.Fatalf("script missing alias export:\n%s", script)
	}
	if strings.Contains(script, "export x=") {
		t.Fatalf("script exported original source name:\n%s", script)
	}
}

func TestRenderRunScriptRejectsRelativePathVariables(t *testing.T) {
	_, err := renderRunScript(runScriptSpec{
		RunDir:    "bench/000000",
		WorkDir:   "/tmp/project/bench/000000/s/000000",
		SourceDir: "/tmp/project",
		StepName:  "s",
		Work:      ManifestWork{Step: "s", Row: 0, Values: map[string]string{}},
		Body:      "true\n",
	})
	if err == nil {
		t.Fatalf("expected relative JBS_RUN_DIR to be rejected")
	}
}

func TestRenderRunScriptNoStrictOmitsStrictMode(t *testing.T) {
	script, err := renderRunScript(runScriptSpec{
		RunDir:    "/tmp/project/bench/000000",
		WorkDir:   "/tmp/project/bench/000000/s/000000",
		SourceDir: "/tmp/project",
		StepName:  "s",
		Work:      ManifestWork{Step: "s", Row: 0, Values: map[string]string{}},
		Body:      "true\n",
		NoStrict:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "set -euo pipefail") {
		t.Fatalf("script should not contain strict mode:\n%s", script)
	}
}

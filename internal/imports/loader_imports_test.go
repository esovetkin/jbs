package imports

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"jbs/internal/diag"
)

func TestLoadResultBuildsResolvedUseGraph(t *testing.T) {
	dir := t.TempDir()
	childPath := writeTestFile(t, dir, "child.jbs", "child_value = 2\n")
	aPath := writeTestFile(t, dir, "a.jbs", strings.Join([]string{
		"use \"./child.jbs\" as child",
		"a_value = child.child_value + 1",
	}, "\n"))
	entry := writeTestFile(t, dir, "entry.jbs", strings.Join([]string{
		"use a",
		"use a_value from a",
		"root = a_value",
	}, "\n"))

	diags := &diag.Diagnostics{}
	res, err := LoadAndExpand(entry, dir, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if res.Entry.Label != entry {
		t.Fatalf("unexpected entry ref: %#v", res.Entry)
	}
	if _, ok := res.Sources[entry]; !ok {
		t.Fatalf("expected entry source in load result")
	}
	if _, ok := res.Sources[aPath]; !ok {
		t.Fatalf("expected imported module source in load result")
	}
	if _, ok := res.Sources[childPath]; !ok {
		t.Fatalf("expected nested imported module source in load result")
	}

	entryInfo := res.Modules[res.Entry.ID]
	if entryInfo == nil {
		t.Fatalf("expected entry module info")
	}
	if got, want := len(entryInfo.Uses), 2; got != want {
		t.Fatalf("unexpected entry use count: got=%d want=%d", got, want)
	}
	if entryInfo.Uses[0].Kind != UseNamespace || entryInfo.Uses[0].Alias != "a" {
		t.Fatalf("unexpected first use: %#v", entryInfo.Uses[0])
	}
	if entryInfo.Uses[1].Kind != UseSelective || !reflect.DeepEqual(entryInfo.Uses[1].Names, []string{"a_value"}) {
		t.Fatalf("unexpected second use: %#v", entryInfo.Uses[1])
	}

	var aInfo *ModuleInfo
	for _, info := range res.Modules {
		if info != nil && info.Ref.Label == aPath {
			aInfo = info
			break
		}
	}
	if aInfo == nil {
		t.Fatalf("expected module info for a.jbs")
	}
	if got, want := len(aInfo.Uses), 1; got != want {
		t.Fatalf("unexpected a.jbs use count: got=%d want=%d", got, want)
	}
	if aInfo.Uses[0].Kind != UseNamespace || aInfo.Uses[0].Alias != "child" {
		t.Fatalf("unexpected nested namespace use: %#v", aInfo.Uses[0])
	}
	if aInfo.Uses[0].Source.Label != childPath {
		t.Fatalf("expected nested source label %q, got %#v", childPath, aInfo.Uses[0].Source)
	}
}

func TestLoadResultDetectsAliasLocalSymbolCollision(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.jbs", "value = 1\n")
	entry := writeTestFile(t, dir, "entry.jbs", strings.Join([]string{
		"dup = 0",
		"use \"./a.jbs\" as dup",
	}, "\n"))

	diags := &diag.Diagnostics{}
	_, err := LoadAndExpand(entry, dir, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if !hasDiagCode(diags, "E534") {
		t.Fatalf("expected E534 alias/local symbol collision, got: %s", diags.String())
	}
}

func TestLoadResultDetectsDuplicateAliasCollision(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.jbs", "value = 1\n")
	writeTestFile(t, dir, "b.jbs", "value = 2\n")
	entry := writeTestFile(t, dir, "entry.jbs", strings.Join([]string{
		"use \"./a.jbs\" as dup",
		"use \"./b.jbs\" as dup",
	}, "\n"))

	diags := &diag.Diagnostics{}
	_, err := LoadAndExpand(entry, dir, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if !hasDiagCode(diags, "E536") {
		t.Fatalf("expected E536 duplicate alias collision, got: %s", diags.String())
	}
}

func TestLoadResultReportsModuleCyclesWithChain(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.jbs", "use b\n")
	entry := writeTestFile(t, dir, "b.jbs", "use a\n")

	diags := &diag.Diagnostics{}
	_, err := LoadAndExpand(entry, dir, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if !hasDiagCode(diags, "E530") {
		t.Fatalf("expected E530 cycle diagnostic, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), filepath.Join(dir, "b.jbs")+" -> "+filepath.Join(dir, "a.jbs")+" -> "+filepath.Join(dir, "b.jbs")) {
		t.Fatalf("expected readable cycle chain, got: %s", diags.String())
	}
}

func TestResolvedUseOrderPreservesStatementOrder(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "lib.jbs", "x = 1\ny = 2\n")
	entry := writeTestFile(t, dir, "entry.jbs", strings.Join([]string{
		"use lib",
		"use x, y from lib",
		"use \"./lib.jbs\" as again",
	}, "\n"))

	diags := &diag.Diagnostics{}
	res, err := LoadAndExpand(entry, dir, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	info := res.Modules[res.Entry.ID]
	if info == nil {
		t.Fatalf("expected entry module info")
	}
	if got, want := len(info.Uses), 3; got != want {
		t.Fatalf("unexpected use count: got=%d want=%d", got, want)
	}
	if info.Uses[0].Kind != UseNamespace || info.Uses[0].Alias != "lib" || info.Uses[0].Index != 0 {
		t.Fatalf("unexpected first use: %#v", info.Uses[0])
	}
	if info.Uses[1].Kind != UseSelective || !reflect.DeepEqual(info.Uses[1].Names, []string{"x", "y"}) || info.Uses[1].Index != 1 {
		t.Fatalf("unexpected second use: %#v", info.Uses[1])
	}
	if info.Uses[2].Kind != UseNamespace || info.Uses[2].Alias != "again" || info.Uses[2].Index != 2 {
		t.Fatalf("unexpected third use: %#v", info.Uses[2])
	}
}

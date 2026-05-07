package run

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/imports"
	"jbs/internal/sema"
)

func TestAvailableNProcIsPositive(t *testing.T) {
	if got := availableNProc(); got < 1 {
		t.Fatalf("availableNProc() = %d, want positive", got)
	}
}

func TestResolveNProc(t *testing.T) {
	tests := []struct {
		name    string
		raw     int
		def     int
		want    int
		wantErr bool
	}{
		{name: "zero uses default", raw: 0, def: 12, want: 12},
		{name: "positive stays positive", raw: 4, def: 12, want: 4},
		{name: "default guarded", raw: 0, def: 0, want: 1},
		{name: "negative errors", raw: -1, def: 12, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveNProc(tt.raw, tt.def)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveNProc(%d, %d) returned nil error", tt.raw, tt.def)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveNProc(%d, %d) returned error: %v", tt.raw, tt.def, err)
			}
			if got != tt.want {
				t.Fatalf("resolveNProc(%d, %d) = %d, want %d", tt.raw, tt.def, got, tt.want)
			}
		})
	}
}

func TestRuntimePlanResolvesNProcDefaultsToCPUCount(t *testing.T) {
	withAvailableNProc(t, 7)

	tests := []struct {
		name       string
		source     string
		wantGlobal int
		wantStep   int
	}{
		{
			name: "implicit global and step",
			source: `
jbs_name = "bench"
do run {
    echo ok
}
`,
			wantGlobal: 7,
			wantStep:   7,
		},
		{
			name: "explicit zero global and step",
			source: `
jbs_name = "bench"
jbs_nproc = 0
cases = table(x=[1, 2])
do run with cases nproc 0 {
    echo "$x"
}
`,
			wantGlobal: 7,
			wantStep:   7,
		},
		{
			name: "positive global and step",
			source: `
jbs_name = "bench"
jbs_nproc = 5
do run nproc 2 {
    echo ok
}
`,
			wantGlobal: 5,
			wantStep:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := buildPlanFromSource(t, tt.source)
			if plan.WorkPlan.GlobalNProc != tt.wantGlobal {
				t.Fatalf("workplan global nproc = %d, want %d", plan.WorkPlan.GlobalNProc, tt.wantGlobal)
			}
			if plan.Manifest.GlobalNProc != tt.wantGlobal {
				t.Fatalf("manifest global nproc = %d, want %d", plan.Manifest.GlobalNProc, tt.wantGlobal)
			}
			if len(plan.Manifest.Steps) != 1 {
				t.Fatalf("manifest steps = %d, want 1", len(plan.Manifest.Steps))
			}
			if got := plan.Manifest.Steps[0].NProc; got != tt.wantStep {
				t.Fatalf("manifest step nproc = %d, want %d", got, tt.wantStep)
			}
			if got := plan.WorkPlan.Steps[0].NProc; got != tt.wantStep {
				t.Fatalf("workplan step nproc = %d, want %d", got, tt.wantStep)
			}
			if plan.Manifest.GlobalNProc <= 0 || plan.Manifest.Steps[0].NProc <= 0 {
				t.Fatalf("manifest contains non-positive nproc values: %#v", plan.Manifest)
			}
		})
	}
}

func TestLimiterRequiresPositiveLimit(t *testing.T) {
	l := newLimiter(0)
	if !l.canAcquire() {
		t.Fatal("fresh limiter should allow first acquire")
	}
	l.acquire()
	if l.canAcquire() {
		t.Fatal("newLimiter(0) should defensively behave as limit 1")
	}
	l.release()
	if !l.canAcquire() {
		t.Fatal("released limiter should allow acquire")
	}
}

func TestNewSchedulerNormalizesZeroManifestLimits(t *testing.T) {
	s := NewScheduler(&Store{Manifest: Manifest{
		GlobalNProc: 0,
		Steps:       []ManifestStep{{Name: "run", NProc: 0}},
	}}, nil)
	if s.global.limit != 1 {
		t.Fatalf("global limit = %d, want 1", s.global.limit)
	}
	if got := s.steps["run"].limit; got != 1 {
		t.Fatalf("step limit = %d, want 1", got)
	}
}

func TestSchedulerLimitersUsePositiveLimits(t *testing.T) {
	s := &Scheduler{
		global: newLimiter(2),
		steps: map[string]*limiter{
			"a": limiterPtr(newLimiter(1)),
			"b": limiterPtr(newLimiter(0)),
		},
	}
	ready := []ManifestWork{{Step: "a", Row: 0}, {Step: "a", Row: 1}, {Step: "b", Row: 0}}
	if got := s.firstStartable(ready); got != 0 {
		t.Fatalf("firstStartable = %d, want 0", got)
	}
	s.global.acquire()
	s.steps["a"].acquire()
	if got := s.firstStartable(ready[1:]); got != 1 {
		t.Fatalf("firstStartable after step a full = %d, want 1", got)
	}
	s.global.acquire()
	s.steps["b"].acquire()
	if got := s.firstStartable(ready[1:]); got != -1 {
		t.Fatalf("firstStartable after global full = %d, want -1", got)
	}
}

func withAvailableNProc(t *testing.T, n int) {
	t.Helper()
	old := availableNProcForRun
	availableNProcForRun = func() int { return n }
	t.Cleanup(func() { availableNProcForRun = old })
}

func buildPlanFromSource(t *testing.T, source string) runtimePlan {
	t.Helper()
	diags := &diag.Diagnostics{}
	cwd := t.TempDir()
	loadRes, err := imports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	plan, err := buildRuntimePlan(Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: "test.jbs",
	}, diags)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected runtime diagnostics: %s", diags.String())
	}
	return plan
}

func limiterPtr(l limiter) *limiter {
	return &l
}

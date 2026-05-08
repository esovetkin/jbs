package run

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
)

type Store struct {
	RunDir   string
	Manifest Manifest
	steps    map[string]ManifestStep
	work     map[string]ManifestWork
	bodies   map[string]string
}

var durableWrite = fsutil.AtomicWriteOptions{SyncDir: true}

func CreateRunDirectory(root string, plan runtimePlan) (*Store, error) {
	unlock, err := acquireRootLock(root)
	if err != nil {
		return nil, err
	}
	defer unlock()

	runID, err := nextRunID(root)
	if err != nil {
		return nil, err
	}
	final := filepath.Join(root, runID)
	staging := filepath.Join(root, fmt.Sprintf(".creating-%s-%d", runID, os.Getpid()))
	finalAbs, err := filepath.Abs(final)
	if err != nil {
		return nil, fmt.Errorf("resolve run directory %q: %w", final, err)
	}
	sourceDirAbs := plan.SourceDir
	if sourceDirAbs == "" {
		sourceDirAbs, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("determine source directory: %w", err)
		}
	}
	if !filepath.IsAbs(sourceDirAbs) {
		sourceDirAbs, err = filepath.Abs(sourceDirAbs)
		if err != nil {
			return nil, fmt.Errorf("resolve source directory %q: %w", plan.SourceDir, err)
		}
	}
	sourceDirAbs = filepath.Clean(sourceDirAbs)
	if err := os.Mkdir(staging, 0o755); err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(staging)
		}
	}()

	manifest := plan.Manifest
	manifest.CreatedAt = time.Now().UTC()
	if err := populateRunTree(staging, finalAbs, sourceDirAbs, manifest, plan.Bodies, plan.Analyses); err != nil {
		return nil, err
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(staging, "manifest.json"), manifest, 0o644, durableWrite); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rootStatus := RootStatus{
		Schema:     1,
		Status:     StatusRunning,
		SourceHash: manifest.SourceHash,
		PID:        os.Getpid(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(staging, "status"), rootStatus, 0o644, durableWrite); err != nil {
		return nil, err
	}
	if err := os.Rename(staging, final); err != nil {
		return nil, err
	}
	cleanup = false
	if err := fsutil.SyncDir(root); err != nil {
		return nil, err
	}
	return NewStore(final, manifest, plan.Bodies), nil
}

func OpenStore(runDir string, manifest Manifest, bodies map[string]string) *Store {
	return NewStore(runDir, manifest, bodies)
}

func NewStore(runDir string, manifest Manifest, bodies map[string]string) *Store {
	steps := make(map[string]ManifestStep, len(manifest.Steps))
	for _, step := range manifest.Steps {
		steps[step.Name] = step
	}
	work := make(map[string]ManifestWork, len(manifest.Work))
	for _, w := range manifest.Work {
		work[workKey(w.Step, w.Row)] = w
	}
	return &Store{RunDir: runDir, Manifest: manifest, steps: steps, work: work, bodies: bodies}
}

func (s *Store) RunManifest() Manifest {
	return s.Manifest
}

func LoadManifest(path string) (Manifest, error) {
	var manifest Manifest
	err := fsutil.ReadJSON(path, &manifest)
	return manifest, err
}

func LoadRootStatus(path string) (RootStatus, error) {
	var status RootStatus
	err := fsutil.ReadJSON(path, &status)
	return status, err
}

func populateRunTree(stagingRunDir, finalRunDir, sourceDir string, manifest Manifest, bodies map[string]string, analyses map[string]AnalysePlan) error {
	steps := make(map[string]ManifestStep, len(manifest.Steps))
	for _, step := range manifest.Steps {
		steps[step.Name] = step
		stepDir := filepath.Join(stagingRunDir, step.Dir)
		if err := os.MkdirAll(stepDir, 0o755); err != nil {
			return err
		}
		if step.AnalyseCSV != "" {
			plan, ok := analyses[step.Name]
			if !ok {
				return fmt.Errorf("missing analyse plan for step %q", step.Name)
			}
			if err := writeAnalyseHeader(filepath.Join(stepDir, step.AnalyseCSV), plan.Header); err != nil {
				return err
			}
		}
	}
	workMap := make(map[string]ManifestWork, len(manifest.Work))
	for _, work := range manifest.Work {
		workMap[workKey(work.Step, work.Row)] = work
	}
	for _, work := range manifest.Work {
		step, ok := steps[work.Step]
		if !ok {
			return fmt.Errorf("unknown step %q in manifest work", work.Step)
		}
		workDir := filepathForWork(stagingRunDir, step, work)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			return err
		}
		for _, dep := range work.Deps {
			depStep, ok := steps[dep.Step]
			if !ok {
				return fmt.Errorf("unknown dependency step %q", dep.Step)
			}
			depWork, ok := workMap[workKey(dep.Step, dep.Row)]
			if !ok {
				return fmt.Errorf("unknown dependency workpackage %s", workKey(dep.Step, dep.Row))
			}
			target, err := filepath.Rel(workDir, filepathForWork(stagingRunDir, depStep, depWork))
			if err != nil {
				return err
			}
			if err := os.Symlink(target, filepath.Join(workDir, dep.Link)); err != nil {
				return err
			}
		}
		script, err := renderRunScript(runScriptSpec{
			RunDir:    finalRunDir,
			WorkDir:   filepathForWork(finalRunDir, step, work),
			SourceDir: sourceDir,
			StepName:  work.Step,
			Work:      work,
			Body:      bodies[work.Step],
		})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(workDir, "run.sh"), []byte(script), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(workDir, "stdout"), nil, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(workDir, "stderr"), nil, 0o644); err != nil {
			return err
		}
		status := WorkStatus{Schema: 1, Status: StatusNotStarted, Step: work.Step, Row: work.Row}
		if err := fsutil.WriteJSONAtomic(filepath.Join(workDir, "status"), status, 0o644, durableWrite); err != nil {
			return err
		}
	}
	return nil
}

func filepathForWork(runDir string, step ManifestStep, work ManifestWork) string {
	return filepath.Join(runDir, step.Dir, work.Dir)
}

func (s *Store) WorkDir(work ManifestWork) string {
	return filepathForWork(s.RunDir, s.steps[work.Step], work)
}

func (s *Store) WorkStatusPath(work ManifestWork) string {
	return filepath.Join(s.WorkDir(work), "status")
}

func (s *Store) LoadWorkStatus(work ManifestWork) (WorkStatus, error) {
	var status WorkStatus
	err := fsutil.ReadJSON(s.WorkStatusPath(work), &status)
	return status, err
}

func (s *Store) WriteWorkStatus(work ManifestWork, status WorkStatus) error {
	return fsutil.WriteJSONAtomic(s.WorkStatusPath(work), status, 0o644, durableWrite)
}

func (s *Store) LoadRootStatus() (RootStatus, error) {
	return LoadRootStatus(filepath.Join(s.RunDir, "status"))
}

func (s *Store) MarkRootRunning() error {
	status, err := s.LoadRootStatus()
	if err != nil {
		return err
	}
	status.Status = StatusRunning
	status.PID = os.Getpid()
	status.UpdatedAt = time.Now().UTC()
	status.Error = ""
	return fsutil.WriteJSONAtomic(filepath.Join(s.RunDir, "status"), status, 0o644, durableWrite)
}

func (s *Store) MarkRootFinal(final Status, message string) error {
	status, err := s.LoadRootStatus()
	if err != nil {
		return err
	}
	status.Status = final
	status.PID = 0
	status.UpdatedAt = time.Now().UTC()
	status.Error = message
	return fsutil.WriteJSONAtomic(filepath.Join(s.RunDir, "status"), status, 0o644, durableWrite)
}

func (s *Store) NormalizeStaleRunning() error {
	for _, work := range s.Manifest.Work {
		status, err := s.LoadWorkStatus(work)
		if err != nil {
			return err
		}
		if status.Status != StatusRunning {
			continue
		}
		now := time.Now().UTC()
		status.Status = StatusInterrupted
		status.FinishedAt = &now
		status.Error = "stale RUNNING status from interrupted run"
		if err := s.WriteWorkStatus(work, status); err != nil {
			return err
		}
	}
	return nil
}

func writeAnalyseHeader(path string, header []string) error {
	var rows [][]string
	if len(header) == 0 {
		return fsutil.WriteCSVAtomic(path, rows, 0o644, durableWrite)
	}
	rows = [][]string{header}
	return fsutil.WriteCSVAtomic(path, rows, 0o644, durableWrite)
}

func acquireRootLock(root string) (func(), error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return acquireLockFile(filepath.Join(root, ".jbs.lock"))
}

func acquireExistingRootLock(root string) (func(), error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot lock benchmark root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cannot lock benchmark root %s: not a directory", root)
	}
	return acquireLockFile(filepath.Join(root, ".jbs.lock"))
}

func acquireLockFile(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("benchmark root is locked: %w", err)
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		os.Remove(lockPath)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(lockPath)
		return nil, err
	}
	return func() { _ = os.Remove(lockPath) }, nil
}

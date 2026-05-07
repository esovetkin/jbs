package run

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type processResult struct {
	Status   Status
	ExitCode *int
	Err      error
}

func runProcess(ctx context.Context, workDir string) processResult {
	stdout, err := os.Create(filepath.Join(workDir, "stdout"))
	if err != nil {
		return processResult{Status: StatusError, Err: err}
	}
	defer stdout.Close()

	stderr, err := os.Create(filepath.Join(workDir, "stderr"))
	if err != nil {
		return processResult{Status: StatusError, Err: err}
	}
	defer stderr.Close()

	cmd := exec.Command("/usr/bin/env", "bash", "./run.sh")
	cmd.Dir = workDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return processResult{Status: StatusError, Err: err}
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return finishProcess(workDir, cmd, err)
	case <-ctx.Done():
		terminateProcessGroup(cmd.Process.Pid)
		err := <-done
		result := finishProcess(workDir, cmd, err)
		result.Status = StatusInterrupted
		result.Err = ctx.Err()
		return result
	}
}

func finishProcess(workDir string, cmd *exec.Cmd, err error) processResult {
	code := 1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	_ = writeFileAtomic(filepath.Join(workDir, "exitcode"), []byte(formatExitCode(code)), 0o644)
	if err != nil || code != 0 {
		return processResult{Status: StatusError, ExitCode: &code, Err: err}
	}
	return processResult{Status: StatusFinished, ExitCode: &code}
}

func formatExitCode(code int) string {
	return strconv.Itoa(code) + "\n"
}

func terminateProcessGroup(pid int) {
	pgid := -pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
	timer := time.NewTimer(5 * time.Second)
	<-timer.C
	_ = syscall.Kill(pgid, syscall.SIGKILL)
}

package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
)

type processResult struct {
	Status   Status
	ExitCode *int
	Err      error
}

var processTerminationGrace = 5 * time.Second

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
		err := waitAfterCancellation(cmd, done)
		result := finishProcess(workDir, cmd, err)
		result.Status = StatusInterrupted
		result.Err = errors.Join(result.Err, ctx.Err())
		return result
	}
}

var writeExitCodeFile = func(workDir string, code int) error {
	return fsutil.WriteFileAtomic(filepath.Join(workDir, "exitcode"), []byte(formatExitCode(code)), 0o644, durableWrite)
}

func finishProcess(workDir string, cmd *exec.Cmd, err error) processResult {
	code := 1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	exitCode := &code
	if writeErr := writeExitCodeFile(workDir, code); writeErr != nil {
		return processResult{Status: StatusError, ExitCode: exitCode, Err: errors.Join(err, fmt.Errorf("write exitcode: %w", writeErr))}
	}
	if err != nil || code != 0 {
		return processResult{Status: StatusError, ExitCode: exitCode, Err: err}
	}
	return processResult{Status: StatusFinished, ExitCode: exitCode}
}

func formatExitCode(code int) string {
	return strconv.Itoa(code) + "\n"
}

func signalProcessGroup(pid int, sig syscall.Signal) {
	_ = syscall.Kill(-pid, sig)
}

func waitAfterCancellation(cmd *exec.Cmd, done <-chan error) error {
	signalProcessGroup(cmd.Process.Pid, syscall.SIGTERM)
	timer := time.NewTimer(processTerminationGrace)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		return <-done
	}
}

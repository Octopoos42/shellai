// Package shell provides non-interactive shell command execution with process
// group isolation and context-based cancellation.
package shell

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"sync"
	"syscall"
)

// ErrSudoBanned is returned when a command contains the word "sudo".
var ErrSudoBanned = errors.New("sudo is not allowed")

// sudoPattern matches "sudo" as a whole word to avoid false positives like "sudoedit".
var sudoPattern = regexp.MustCompile(`\bsudo\b`)

// Runner executes shell commands, streaming output to provided writers.
type Runner interface {
	Run(ctx context.Context, command string, stdout, stderr io.Writer) (exitCode int, err error)
}

// Executor runs shell commands via bash -c in a non-interactive, sandboxed
// environment. Each command runs in its own process group so that context
// cancellation can cleanly terminate the entire process tree.
type Executor struct{}

// ValidateCommand returns ErrSudoBanned if the command contains "sudo", nil otherwise.
func ValidateCommand(command string) error {
	if sudoPattern.MatchString(command) {
		return ErrSudoBanned
	}
	return nil
}

// Run executes command via bash -c with stdin closed (non-interactive).
// It returns (exitCode, nil) for command failures (non-zero exit status), and
// (-1, err) for exec-level failures such as context cancellation before start.
// When the context is cancelled, the entire process group is killed so that
// child processes spawned by the command are also terminated.
func (Executor) Run(ctx context.Context, command string, stdout, stderr io.Writer) (int, error) {
	if err := ValidateCommand(command); err != nil {
		return -1, err
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Stdin = nil // non-interactive: stdin is /dev/null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return -1, err
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	// When the context is cancelled, kill the entire process group (negative
	// PID sends the signal to all members of the group).
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-done:
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdout, stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stderr, stderrPipe)
	}()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

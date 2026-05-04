package shell_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Octopoos42/shellai/server/internal/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCommand(t *testing.T) {
	cases := []struct {
		command string
		banned  bool
	}{
		{"echo hello", false},
		{"sudo rm -rf /", true},
		{"bash -c 'sudo ls'", true},
		{"echo 'sudo is banned'", true},
		{"sudoedit /etc/hosts", false}, // not the word "sudo"
		{"cat /etc/sudoers", false},    // "sudoers" contains but is not "sudo"
		{"ls -la", false},
	}
	for _, tc := range cases {
		err := shell.ValidateCommand(tc.command)
		if tc.banned {
			assert.ErrorIs(t, err, shell.ErrSudoBanned, "expected banned: %q", tc.command)
		} else {
			assert.NoError(t, err, "expected allowed: %q", tc.command)
		}
	}
}

var ex = shell.Executor{}

func TestExecutor_StdoutStreamed(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := ex.Run(context.Background(), "echo hello", &out, &errOut)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", out.String())
	assert.Empty(t, errOut.String())
}

func TestExecutor_StderrStreamed(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := ex.Run(context.Background(), "echo err-msg >&2", &out, &errOut)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "err-msg")
}

func TestExecutor_NonZeroExit(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := ex.Run(context.Background(), "exit 42", &out, &errOut)
	require.NoError(t, err)
	assert.Equal(t, 42, code)
}

func TestExecutor_SudoBanned(t *testing.T) {
	var out, errOut bytes.Buffer
	_, err := ex.Run(context.Background(), "sudo ls", &out, &errOut)
	assert.ErrorIs(t, err, shell.ErrSudoBanned)
}

func TestExecutor_StdinClosed(t *testing.T) {
	// cat with no stdin input should get EOF immediately and exit cleanly.
	var out, errOut bytes.Buffer
	code, err := ex.Run(context.Background(), "cat", &out, &errOut)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Empty(t, out.String())
}

func TestExecutor_MultilineOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := ex.Run(context.Background(), "printf 'line1\\nline2\\nline3\\n'", &out, &errOut)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	assert.Equal(t, []string{"line1", "line2", "line3"}, lines)
}

func TestExecutor_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var out, errOut bytes.Buffer
	start := time.Now()
	_, _ = ex.Run(ctx, "sleep 30", &out, &errOut)
	assert.Less(t, time.Since(start), 3*time.Second, "command should be cancelled promptly")
}

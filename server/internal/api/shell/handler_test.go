package shell_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	shellapi "github.com/Octopoos42/shellai/server/internal/api/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner is a test double for shell.Runner.
type mockRunner struct {
	fn func(ctx context.Context, command string, stdout, stderr io.Writer) (int, error)
}

func (m *mockRunner) Run(ctx context.Context, command string, stdout, stderr io.Writer) (int, error) {
	if m.fn != nil {
		return m.fn(ctx, command, stdout, stderr)
	}
	return 0, nil
}

func newApp(runner *mockRunner) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/api/shell/exec", shellapi.HandleExec(runner))
	return app
}

// parseSSEEvents splits a raw SSE body into a slice of (event, data) pairs.
func parseSSEEvents(body string) []struct{ Event, Data string } {
	var events []struct{ Event, Data string }
	for block := range strings.SplitSeq(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var ev struct{ Event, Data string }
		for line := range strings.SplitSeq(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.Data = strings.TrimPrefix(line, "data: ")
			}
		}
		events = append(events, ev)
	}
	return events
}

func TestHandleExec_InvalidJSON(t *testing.T) {
	app := newApp(&mockRunner{})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestHandleExec_EmptyCommand(t *testing.T) {
	app := newApp(&mockRunner{})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader(`{"command":""}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestHandleExec_WhitespaceCommand(t *testing.T) {
	app := newApp(&mockRunner{})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader(`{"command":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestHandleExec_SudoBanned(t *testing.T) {
	app := newApp(&mockRunner{})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader(`{"command":"sudo ls"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestHandleExec_SSEHeaders(t *testing.T) {
	app := newApp(&mockRunner{fn: func(_ context.Context, _ string, _, _ io.Writer) (int, error) {
		return 0, nil
	}})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader(`{"command":"echo hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
}

func TestHandleExec_SuccessExitEvent(t *testing.T) {
	app := newApp(&mockRunner{fn: func(_ context.Context, _ string, stdout, _ io.Writer) (int, error) {
		_, _ = stdout.Write([]byte("hello output"))
		return 0, nil
	}})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader(`{"command":"echo hello"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	events := parseSSEEvents(string(body))
	require.NotEmpty(t, events)

	// Last event must be "exit" with code 0.
	last := events[len(events)-1]
	assert.Equal(t, "exit", last.Event)
	var exit shellapi.ExitEvent
	require.NoError(t, json.Unmarshal([]byte(last.Data), &exit))
	assert.Equal(t, 0, exit.Code)
	assert.Empty(t, exit.Error)
}

func TestHandleExec_StdoutEvent(t *testing.T) {
	app := newApp(&mockRunner{fn: func(_ context.Context, _ string, stdout, _ io.Writer) (int, error) {
		_, _ = stdout.Write([]byte("test output"))
		return 0, nil
	}})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader(`{"command":"echo test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	events := parseSSEEvents(string(body))

	var found bool
	for _, ev := range events {
		if ev.Event == "stdout" {
			var d struct {
				Text string `json:"text"`
			}
			require.NoError(t, json.Unmarshal([]byte(ev.Data), &d))
			assert.Equal(t, "test output", d.Text)
			found = true
		}
	}
	assert.True(t, found, "expected a stdout event")
}

func TestHandleExec_StderrEvent(t *testing.T) {
	app := newApp(&mockRunner{fn: func(_ context.Context, _ string, _, stderr io.Writer) (int, error) {
		_, _ = stderr.Write([]byte("err output"))
		return 1, nil
	}})
	req := httptest.NewRequest("POST", "/api/shell/exec", strings.NewReader(`{"command":"ls /nope"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	events := parseSSEEvents(string(body))

	var foundStderr bool
	var exitCode int
	for _, ev := range events {
		switch ev.Event {
		case "stderr":
			foundStderr = true
		case "exit":
			var exit shellapi.ExitEvent
			_ = json.Unmarshal([]byte(ev.Data), &exit)
			exitCode = exit.Code
		}
	}
	assert.True(t, foundStderr, "expected a stderr event")
	assert.Equal(t, 1, exitCode)
}

//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/Octopoos42/shellai/server/internal/api/admin"
	shellapi "github.com/Octopoos42/shellai/server/internal/api/shell"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/Octopoos42/shellai/server/internal/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupShellApp(queries db.Querier) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	adminGrp := app.Group("/api/admin", middleware.RequireAdmin(adminUser, adminPass))
	adminGrp.Post("/apikeys", admin.HandleCreateAPIKey(queries))

	app.Post("/api/shell/exec", middleware.RequireAPIKey(queries), shellapi.HandleExec(shell.Executor{}))
	return app
}

// createTestAPIKey creates a fresh API key via the admin endpoint and returns
// the plaintext key string.
func createTestAPIKey(t *testing.T, app *fiber.App) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"label": "shell-test"})
	req := httptest.NewRequest("POST", "/api/admin/apikeys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAuth(adminUser, adminPass))
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var created struct {
		Key string `json:"key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	return created.Key
}

// execShell sends a shell command to the test app and returns the HTTP response.
func execShell(t *testing.T, app *fiber.App, apiKey, command string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"command": command})
	req := httptest.NewRequest("POST", "/api/shell/exec", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	return resp
}

// parseSSE parses a raw SSE body into a map of event type → slice of data payloads.
func parseSSE(body string) map[string][]string {
	result := make(map[string][]string)
	for block := range strings.SplitSeq(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var event, data string
		for line := range strings.SplitSeq(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		if event != "" {
			result[event] = append(result[event], data)
		}
	}
	return result
}

func TestIntegration_Shell_Echo(t *testing.T) {
	pool := setupDB(t)
	app := setupShellApp(db.New(pool))
	key := createTestAPIKey(t, app)

	resp := execShell(t, app, key, "echo hello-world")
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	bodyBytes, _ := io.ReadAll(resp.Body)
	events := parseSSE(string(bodyBytes))

	require.NotEmpty(t, events["stdout"])
	var stdout struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal([]byte(events["stdout"][0]), &stdout))
	assert.Contains(t, stdout.Text, "hello-world")

	require.NotEmpty(t, events["exit"])
	var exit shellapi.ExitEvent
	require.NoError(t, json.Unmarshal([]byte(events["exit"][0]), &exit))
	assert.Equal(t, 0, exit.Code)
}

func TestIntegration_Shell_NonZeroExit(t *testing.T) {
	pool := setupDB(t)
	app := setupShellApp(db.New(pool))
	key := createTestAPIKey(t, app)

	resp := execShell(t, app, key, "exit 7")
	bodyBytes, _ := io.ReadAll(resp.Body)
	events := parseSSE(string(bodyBytes))

	require.NotEmpty(t, events["exit"])
	var exit shellapi.ExitEvent
	require.NoError(t, json.Unmarshal([]byte(events["exit"][0]), &exit))
	assert.Equal(t, 7, exit.Code)
}

func TestIntegration_Shell_Stderr(t *testing.T) {
	pool := setupDB(t)
	app := setupShellApp(db.New(pool))
	key := createTestAPIKey(t, app)

	resp := execShell(t, app, key, "echo err-line >&2")
	bodyBytes, _ := io.ReadAll(resp.Body)
	events := parseSSE(string(bodyBytes))

	require.NotEmpty(t, events["stderr"])
	var ev struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal([]byte(events["stderr"][0]), &ev))
	assert.Contains(t, ev.Text, "err-line")
}

func TestIntegration_Shell_SudoBanned(t *testing.T) {
	pool := setupDB(t)
	app := setupShellApp(db.New(pool))
	key := createTestAPIKey(t, app)

	resp := execShell(t, app, key, "sudo ls")
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestIntegration_Shell_RequiresAuth(t *testing.T) {
	pool := setupDB(t)
	app := setupShellApp(db.New(pool))

	body, _ := json.Marshal(map[string]string{"command": "echo hi"})
	req := httptest.NewRequest("POST", "/api/shell/exec", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No API key — should be rejected.
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestIntegration_Shell_LargeOutput(t *testing.T) {
	pool := setupDB(t)
	app := setupShellApp(db.New(pool))
	key := createTestAPIKey(t, app)

	resp := execShell(t, app, key, fmt.Sprintf("for i in $(seq 1 %d); do echo line$i; done", 1000))
	bodyBytes, _ := io.ReadAll(resp.Body)
	events := parseSSE(string(bodyBytes))

	require.NotEmpty(t, events["exit"])
	var exit shellapi.ExitEvent
	require.NoError(t, json.Unmarshal([]byte(events["exit"][0]), &exit))
	assert.Equal(t, 0, exit.Code)

	var allStdout strings.Builder
	for _, d := range events["stdout"] {
		var ev struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal([]byte(d), &ev)
		allStdout.WriteString(ev.Text)
	}
	assert.Contains(t, allStdout.String(), "line1\n")
	assert.Contains(t, allStdout.String(), "line1000\n")
}

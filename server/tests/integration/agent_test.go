//go:build integration

package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/agent"
	"github.com/Octopoos42/shellai/server/internal/api/admin"
	chatapi "github.com/Octopoos42/shellai/server/internal/api/chat"
	"github.com/Octopoos42/shellai/server/internal/config"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/Octopoos42/shellai/server/internal/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentDeepseekConfig wraps deepseekConfig and injects agent-loop settings.
func agentDeepseekConfig(t *testing.T, confirmTimeoutSecs int) *config.Config {
	t.Helper()
	cfg := deepseekConfig(t)
	cfg.Agent = config.AgentConfig{
		MaxIterations:          5,
		ToolConfirmTimeoutSecs: confirmTimeoutSecs,
	}
	return cfg
}

// setupAgentApp creates a Fiber app wired with the real LLM client.
func setupAgentApp(queries db.Querier, cfg *config.Config) (*fiber.App, *agent.Store) {
	store := agent.NewStore()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	adminGrp := app.Group("/api/admin", middleware.RequireAdmin(adminUser, adminPass))
	adminGrp.Post("/apikeys", admin.HandleCreateAPIKey(queries))

	app.Post("/api/sessions", middleware.RequireAPIKey(queries), chatapi.HandleCreateSession(queries))
	app.Post("/api/sessions/:id/chat", middleware.RequireAPIKey(queries),
		chatapi.HandleChat(queries, cfg, store, shell.Executor{}))
	app.Post("/api/sessions/:id/tool-confirm", middleware.RequireAPIKey(queries),
		chatapi.HandleToolConfirm(queries, store))

	return app, store
}

// startServer binds app to a random local port and returns the base URL.
// The listener is closed when the test ends.
func startServer(t *testing.T, app *fiber.App) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck
	go app.Listener(ln)              //nolint:errcheck
	return "http://" + ln.Addr().String()
}

// parseUUID converts a hyphenated UUID string to pgtype.UUID.
func parseUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	require.NoError(t, u.Scan(s))
	return u
}

// confirmToolHTTP posts a tool-confirm request to the real server.
func confirmToolHTTP(t *testing.T, baseURL, apiKey, sessionID, confirmID string, approved bool) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"confirm_id": confirmID, "approved": approved})
	req, err := http.NewRequest(http.MethodPost,
		baseURL+"/api/sessions/"+sessionID+"/tool-confirm",
		bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	return resp
}

// sendChatReal opens an SSE connection to the real HTTP server and returns the
// response; the body must be closed by the caller or drained via sseStream.
func sendChatReal(t *testing.T, baseURL, apiKey, sessionID, message string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"message": message})
	req, err := http.NewRequest(http.MethodPost,
		baseURL+"/api/sessions/"+sessionID+"/chat",
		bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	// No client-side timeout: the connection stays open while waiting for confirms.
	resp, err := new(http.Client).Do(req)
	require.NoError(t, err)
	return resp
}

// sseEvent is a single typed SSE event.
type sseEvent struct {
	Type string
	Data string
}

// sseStream reads SSE events from an HTTP response body in a background
// goroutine. Events are delivered through an internal channel.
type sseStream struct {
	ch <-chan sseEvent
}

// newSSEStream starts a background reader goroutine immediately.
func newSSEStream(resp *http.Response) *sseStream {
	ch := make(chan sseEvent, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		var ev, data string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "" && ev != "":
				ch <- sseEvent{Type: ev, Data: data}
				ev, data = "", ""
			}
		}
	}()
	return &sseStream{ch: ch}
}

// waitFor blocks until an event of the given type arrives or timeout fires.
func (s *sseStream) waitFor(t *testing.T, eventType string, timeout time.Duration) sseEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-s.ch:
			if !ok {
				t.Fatalf("SSE stream closed before %q event", eventType)
			}
			if ev.Type == eventType {
				return ev
			}
		case <-timer.C:
			t.Fatalf("timed out (%s) waiting for %q SSE event", timeout, eventType)
			return sseEvent{} // unreachable; satisfies compiler
		}
	}
}

// collect drains remaining events until the stream closes or timeout fires.
func (s *sseStream) collect(timeout time.Duration) map[string][]string {
	result := make(map[string][]string)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-s.ch:
			if !ok {
				return result
			}
			result[ev.Type] = append(result[ev.Type], ev.Data)
		case <-timer.C:
			return result
		}
	}
}

// --------------------------------------------------------------------------
// Agent loop tests (real LLM via DeepSeek)
// --------------------------------------------------------------------------

func TestIntegration_Agent_DirectResponse(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := agentDeepseekConfig(t, 30)
	app, _ := setupAgentApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sid := createSession(t, app, key)
	baseURL := startServer(t, app)

	// A simple factual question should not require any shell tool.
	resp := sendChatReal(t, baseURL, key, sid,
		"What is the capital of France? Reply with a single word.")
	stream := newSSEStream(resp)

	doneEv := stream.waitFor(t, "done", 90*time.Second)
	var done struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(doneEv.Data), &done))
	assert.NotEmpty(t, done.Content)

	// Both user and assistant messages must be persisted.
	msgs, err := queries.ListMessagesBySession(context.Background(), parseUUID(t, sid))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "assistant", msgs[len(msgs)-1].Role)
}

func TestIntegration_Agent_ToolCallApproved(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := agentDeepseekConfig(t, 60)
	app, _ := setupAgentApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sid := createSession(t, app, key)
	baseURL := startServer(t, app)

	// Explicit instruction so the planner reliably decides a shell tool is needed.
	resp := sendChatReal(t, baseURL, key, sid,
		`Use the shell tool to run the command "echo AGENT_TEST_OK" and report the output.`)
	stream := newSSEStream(resp)

	// Wait for the agent to emit tool_request.
	toolReqEv := stream.waitFor(t, "tool_request", 90*time.Second)
	var toolReq struct {
		ID          string         `json:"id"`
		Tool        string         `json:"tool"`
		Args        map[string]any `json:"args"`
		Explanation string         `json:"explanation"`
	}
	require.NoError(t, json.Unmarshal([]byte(toolReqEv.Data), &toolReq))
	assert.Equal(t, "shell", toolReq.Tool)
	assert.NotEmpty(t, toolReq.ID)

	// Approve the tool call.
	confirmResp := confirmToolHTTP(t, baseURL, key, sid, toolReq.ID, true)
	require.Equal(t, http.StatusNoContent, confirmResp.StatusCode, "confirm must succeed")
	confirmResp.Body.Close() //nolint:errcheck

	// Collect remaining events: expect tool_result then eventually done.
	rest := stream.collect(90 * time.Second)
	assert.NotEmpty(t, rest["tool_result"], "expected tool_result event")
	assert.NotEmpty(t, rest["done"], "expected done event")

	var toolResult agent.ToolResult
	require.NoError(t, json.Unmarshal([]byte(rest["tool_result"][0]), &toolResult))
	assert.Equal(t, 0, toolResult.ExitCode)
	assert.Contains(t, toolResult.Stdout, "AGENT_TEST_OK")
	assert.False(t, toolResult.Rejected)

	// DB must contain tool_call and tool_result messages.
	msgs, err := queries.ListMessagesBySession(context.Background(), parseUUID(t, sid))
	require.NoError(t, err)
	roles := make([]string, len(msgs))
	for i, m := range msgs {
		roles[i] = m.Role
	}
	assert.Contains(t, roles, "tool_call")
	assert.Contains(t, roles, "tool_result")
	assert.Contains(t, roles, "assistant")
}

func TestIntegration_Agent_ToolCallRejected(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := agentDeepseekConfig(t, 60)
	app, _ := setupAgentApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sid := createSession(t, app, key)
	baseURL := startServer(t, app)

	resp := sendChatReal(t, baseURL, key, sid,
		`Use the shell tool to run the command "echo REJECT_TEST" and report the output.`)
	stream := newSSEStream(resp)

	toolReqEv := stream.waitFor(t, "tool_request", 90*time.Second)
	var toolReq struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(toolReqEv.Data), &toolReq))

	// Reject the tool call.
	confirmResp := confirmToolHTTP(t, baseURL, key, sid, toolReq.ID, false)
	require.Equal(t, http.StatusNoContent, confirmResp.StatusCode, "confirm must succeed")
	confirmResp.Body.Close() //nolint:errcheck

	rest := stream.collect(90 * time.Second)
	assert.NotEmpty(t, rest["tool_rejected"], "expected tool_rejected event")
	assert.NotEmpty(t, rest["done"], "expected done event after rejection")

	// DB must record the rejection.
	msgs, err := queries.ListMessagesBySession(context.Background(), parseUUID(t, sid))
	require.NoError(t, err)
	var foundRejected bool
	for _, m := range msgs {
		if m.Role == "tool_result" {
			var tr agent.ToolResult
			_ = json.Unmarshal([]byte(m.Content), &tr)
			if tr.Rejected {
				foundRejected = true
			}
		}
	}
	assert.True(t, foundRejected, "expected a rejected=true tool_result in DB")
}

func TestIntegration_Agent_ConfirmTimeout(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	_ = queries
	cfg := agentDeepseekConfig(t, 3) // short 3-second confirm window
	app, _ := setupAgentApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sid := createSession(t, app, key)
	baseURL := startServer(t, app)

	resp := sendChatReal(t, baseURL, key, sid,
		`Use the shell tool to run "echo TIMEOUT_TEST".`)
	stream := newSSEStream(resp)

	// Capture tool_request but deliberately do not confirm.
	_ = stream.waitFor(t, "tool_request", 90*time.Second)

	// The stream should end with a CONFIRM_TIMEOUT error.
	rest := stream.collect(30 * time.Second)
	require.NotEmpty(t, rest["error"], "expected error event after timeout")
	var errPayload struct {
		ErrorCode string `json:"error_code"`
	}
	require.NoError(t, json.Unmarshal([]byte(rest["error"][0]), &errPayload))
	assert.Equal(t, "CONFIRM_TIMEOUT", errPayload.ErrorCode)
}

func TestIntegration_Agent_ConfirmLateAfterTimeout(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	_ = queries
	cfg := agentDeepseekConfig(t, 3)
	app, _ := setupAgentApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sid := createSession(t, app, key)
	baseURL := startServer(t, app)

	resp := sendChatReal(t, baseURL, key, sid,
		`Use the shell tool to run "echo LATE_CONFIRM".`)
	stream := newSSEStream(resp)

	toolReqEv := stream.waitFor(t, "tool_request", 90*time.Second)
	var toolReq struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(toolReqEv.Data), &toolReq))

	// Wait for the SSE to close (server-side timeout fires).
	_ = stream.collect(15 * time.Second)

	// Confirming after the agent has cleaned up must return 410 Gone.
	confirmResp := confirmToolHTTP(t, baseURL, key, sid, toolReq.ID, true)
	assert.Equal(t, http.StatusGone, confirmResp.StatusCode)
	confirmResp.Body.Close() //nolint:errcheck
}

// --------------------------------------------------------------------------
// Confirm endpoint HTTP semantics (no LLM calls needed)
// --------------------------------------------------------------------------

func TestIntegration_Agent_ConfirmExpiredOrUnknown(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := agentDeepseekConfig(t, 30)
	app, _ := setupAgentApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sid := createSession(t, app, key)

	// Posting a random UUID when no tool call is pending → 410 Gone.
	body, _ := json.Marshal(map[string]any{
		"confirm_id": "00000000-0000-4000-8000-000000000001",
		"approved":   true,
	})
	req := httptest.NewRequest("POST", "/api/sessions/"+sid+"/tool-confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)

	resp, err := app.Test(req, 5_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusGone, resp.StatusCode)

	var errResp struct {
		ErrorCode string `json:"error_code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	assert.Equal(t, "CONFIRM_EXPIRED", errResp.ErrorCode)
}

func TestIntegration_Agent_ConfirmMissingID(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := agentDeepseekConfig(t, 30)
	app, _ := setupAgentApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sid := createSession(t, app, key)

	body, _ := json.Marshal(map[string]any{"approved": true})
	req := httptest.NewRequest("POST", "/api/sessions/"+sid+"/tool-confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)

	resp, err := app.Test(req, 5_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestIntegration_Agent_ConfirmSessionIsolation(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := agentDeepseekConfig(t, 30)
	app, _ := setupAgentApp(queries, cfg)

	keyA := createTestAPIKey(t, app)
	keyB := createTestAPIKey(t, app)
	sidA := createSession(t, app, keyA)

	// User B tries to confirm on user A's session — must get 404.
	body, _ := json.Marshal(map[string]any{
		"confirm_id": "00000000-0000-4000-8000-000000000001",
		"approved":   true,
	})
	req := httptest.NewRequest("POST", "/api/sessions/"+sidA+"/tool-confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", keyB)

	resp, err := app.Test(req, 5_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestIntegration_Agent_ConfirmRequiresAuth(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	_ = queries
	cfg := agentDeepseekConfig(t, 30)
	app, _ := setupAgentApp(queries, cfg)

	body, _ := json.Marshal(map[string]any{
		"confirm_id": "00000000-0000-4000-8000-000000000001",
		"approved":   true,
	})
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/api/sessions/%s/tool-confirm", "00000000-0000-4000-8000-000000000002"),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-API-Key — must be rejected.

	resp, err := app.Test(req, 5_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

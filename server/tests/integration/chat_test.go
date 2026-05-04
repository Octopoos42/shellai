//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
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

// deepseekConfig builds a minimal config pointing at DeepSeek.
// The test is skipped if DEEPSEEK_API_KEY is unset.
func deepseekConfig(t *testing.T) *config.Config {
	t.Helper()
	_ = godotenv.Load("../../.env")
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Skip("DEEPSEEK_API_KEY not set — skipping LLM integration tests")
	}
	return &config.Config{
		DefaultModel: "deepseek",
		LLMs: []config.LLMConfig{{
			Name:     "deepseek",
			Endpoint: "https://api.deepseek.com/chat/completions",
			Model:    "deepseek-chat",
			EnvKey:   "DEEPSEEK_API_KEY",
			ContextK: 128,
			LimitK:   4, // keep max_tokens small for faster, cheaper tests
		}},
	}
}

func setupChatApp(queries db.Querier, cfg *config.Config) *fiber.App {
	store := agent.NewStore()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	adminGrp := app.Group("/api/admin", middleware.RequireAdmin(adminUser, adminPass))
	adminGrp.Post("/apikeys", admin.HandleCreateAPIKey(queries))

	app.Post("/api/sessions", middleware.RequireAPIKey(queries), chatapi.HandleCreateSession(queries))
	app.Get("/api/sessions", middleware.RequireAPIKey(queries), chatapi.HandleListSessions(queries))
	app.Get("/api/sessions/:id", middleware.RequireAPIKey(queries), chatapi.HandleGetSession(queries))
	app.Delete("/api/sessions/:id", middleware.RequireAPIKey(queries), chatapi.HandleDeleteSession(queries))
	app.Post("/api/sessions/:id/chat", middleware.RequireAPIKey(queries), chatapi.HandleChat(queries, cfg, store, shell.Executor{}))
	app.Post("/api/sessions/:id/tool-confirm", middleware.RequireAPIKey(queries), chatapi.HandleToolConfirm(queries, store))
	return app
}

// createSession creates a session and returns its ID.
func createSession(t *testing.T, app *fiber.App, apiKey string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader([]byte(`{"title":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var s chatapi.SessionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&s))
	return s.ID
}

// sendChat sends a chat message and returns the full HTTP response.
func sendChat(t *testing.T, app *fiber.App, apiKey, sessionID, message string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"message": message})
	req := httptest.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	resp, err := app.Test(req, 60_000) // generous timeout for LLM calls
	require.NoError(t, err)
	return resp
}

// parseChatDone extracts the "content" field from the SSE "done" event.
func parseChatDone(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	events := parseSSE(string(body))

	require.NotEmpty(t, events["done"], "expected a 'done' SSE event; got: %s", string(body))
	var done struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(events["done"][0]), &done))
	return done.Content
}

func TestIntegration_Chat_SessionCRUD(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)
	key := createTestAPIKey(t, app)

	// Create a session
	sessionID := createSession(t, app, key)
	assert.NotEmpty(t, sessionID)

	// List — should see exactly our session
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", key)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	var sessions []chatapi.SessionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sessions))
	require.Len(t, sessions, 1)
	assert.Equal(t, sessionID, sessions[0].ID)

	// Get with messages
	req = httptest.NewRequest("GET", "/api/sessions/"+sessionID, nil)
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Delete
	req = httptest.NewRequest("DELETE", "/api/sessions/"+sessionID, nil)
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)

	// Get after delete — 404
	req = httptest.NewRequest("GET", "/api/sessions/"+sessionID, nil)
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestIntegration_Chat_SessionIsolation(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)

	keyA := createTestAPIKey(t, app)
	keyB := createTestAPIKey(t, app)

	// User A creates a session
	sessionA := createSession(t, app, keyA)

	// User B lists sessions — should be empty
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-API-Key", keyB)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	var sessions []chatapi.SessionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sessions))
	assert.Empty(t, sessions, "user B should not see user A's sessions")

	// User B tries to get user A's session — 404
	req = httptest.NewRequest("GET", "/api/sessions/"+sessionA, nil)
	req.Header.Set("X-API-Key", keyB)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// User B tries to delete user A's session — 404
	req = httptest.NewRequest("DELETE", "/api/sessions/"+sessionA, nil)
	req.Header.Set("X-API-Key", keyB)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// User A's session still exists
	req = httptest.NewRequest("GET", "/api/sessions/"+sessionA, nil)
	req.Header.Set("X-API-Key", keyA)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestIntegration_Chat_BasicChat(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sessionID := createSession(t, app, key)

	resp := sendChat(t, app, key, sessionID, "Reply with exactly: PONG")
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	content := parseChatDone(t, resp)
	assert.NotEmpty(t, content, "LLM response should not be empty")

	// Messages should be persisted
	req := httptest.NewRequest("GET", "/api/sessions/"+sessionID, nil)
	req.Header.Set("X-API-Key", key)
	r, err := app.Test(req, 10_000)
	require.NoError(t, err)
	var full chatapi.SessionWithMessagesResponse
	require.NoError(t, json.NewDecoder(r.Body).Decode(&full))
	require.Len(t, full.Messages, 2, "user + assistant messages should be persisted")
	assert.Equal(t, "user", full.Messages[0].Role)
	assert.Equal(t, "assistant", full.Messages[1].Role)
}

func TestIntegration_Chat_Help(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sessionID := createSession(t, app, key)

	resp := sendChat(t, app, key, sessionID, "/help")
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	content := parseChatDone(t, resp)
	assert.Contains(t, content, "/compact")
	assert.Contains(t, content, "/interrupt")
}

func TestIntegration_Chat_Compact(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sessionID := createSession(t, app, key)

	// Build some conversation history first.
	r1 := sendChat(t, app, key, sessionID, "My name is TestBot.")
	require.Equal(t, fiber.StatusOK, r1.StatusCode)
	_, _ = io.ReadAll(r1.Body) // drain

	// Compact the conversation.
	resp := sendChat(t, app, key, sessionID, "/compact")
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	summary := parseChatDone(t, resp)
	assert.NotEmpty(t, summary)

	// After compacting, the session should have only the system summary message.
	req := httptest.NewRequest("GET", "/api/sessions/"+sessionID, nil)
	req.Header.Set("X-API-Key", key)
	r, err := app.Test(req, 10_000)
	require.NoError(t, err)
	var full chatapi.SessionWithMessagesResponse
	require.NoError(t, json.NewDecoder(r.Body).Decode(&full))
	require.Len(t, full.Messages, 1, "compact should leave exactly one system message")
	assert.Equal(t, "system", full.Messages[0].Role)
}

func TestIntegration_Chat_MultiTurn(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sessionID := createSession(t, app, key)

	// First turn.
	r1 := sendChat(t, app, key, sessionID, "Remember the number 42.")
	require.Equal(t, fiber.StatusOK, r1.StatusCode)
	_, _ = io.ReadAll(r1.Body)

	// Second turn referencing context.
	r2 := sendChat(t, app, key, sessionID, "What number did I ask you to remember? Reply with only the number.")
	require.Equal(t, fiber.StatusOK, r2.StatusCode)
	content := parseChatDone(t, r2)
	assert.Contains(t, content, "42", "LLM should recall the number from previous turn")
}

func TestIntegration_Chat_UnknownModel(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sessionID := createSession(t, app, key)

	body, _ := json.Marshal(map[string]string{"message": "hello", "model": "nonexistent"})
	req := httptest.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var errResp struct {
		ErrorCode string `json:"error_code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	assert.Equal(t, "MODEL_NOT_FOUND", errResp.ErrorCode)
}

func TestIntegration_Chat_SSEOnOtherUserSession(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)

	keyA := createTestAPIKey(t, app)
	keyB := createTestAPIKey(t, app)
	sessionA := createSession(t, app, keyA)

	// User B tries to chat in user A's session.
	body, _ := json.Marshal(map[string]string{"message": "hello"})
	req := httptest.NewRequest("POST", "/api/sessions/"+sessionA+"/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", keyB)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestIntegration_Chat_SSEHeadersPresent(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	cfg := deepseekConfig(t)
	app := setupChatApp(queries, cfg)
	key := createTestAPIKey(t, app)
	sessionID := createSession(t, app, key)

	body, _ := json.Marshal(map[string]string{"message": "/help"})
	req := httptest.NewRequest("POST", "/api/sessions/"+sessionID+"/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))

	// Drain and check for correct SSE format
	bodyBytes, _ := io.ReadAll(resp.Body)
	assert.True(t, strings.Contains(string(bodyBytes), "event: token"), "response should contain SSE token events")
	assert.True(t, strings.Contains(string(bodyBytes), "event: done"), "response should contain SSE done event")
}

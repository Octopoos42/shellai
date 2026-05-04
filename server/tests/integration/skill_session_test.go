//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/Octopoos42/shellai/server/internal/api/admin"
	chatapi "github.com/Octopoos42/shellai/server/internal/api/chat"
	skillapi "github.com/Octopoos42/shellai/server/internal/api/skill"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSkillSessionApp wires up skill and session-rename routes for integration tests.
func setupSkillSessionApp(queries db.Querier) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	adminGrp := app.Group("/api/admin", middleware.RequireAdmin(adminUser, adminPass))
	adminGrp.Post("/apikeys", admin.HandleCreateAPIKey(queries))

	app.Post("/api/sessions", middleware.RequireAPIKey(queries), chatapi.HandleCreateSession(queries))
	app.Patch("/api/sessions/:id", middleware.RequireAPIKey(queries), chatapi.HandleRenameSession(queries))
	app.Get("/api/sessions/:id", middleware.RequireAPIKey(queries), chatapi.HandleGetSession(queries))

	app.Post("/api/skills", middleware.RequireAPIKey(queries), skillapi.HandleCreateSkill(queries))
	app.Get("/api/skills", middleware.RequireAPIKey(queries), skillapi.HandleListMySkills(queries))
	app.Get("/api/skills/public", middleware.RequireAPIKey(queries), skillapi.HandleListPublicSkills(queries))
	app.Patch("/api/skills/:id", middleware.RequireAPIKey(queries), skillapi.HandleUpdateSkill(queries))
	app.Delete("/api/skills/:id", middleware.RequireAPIKey(queries), skillapi.HandleDeleteSkill(queries))

	return app
}

// ── Session rename ────────────────────────────────────────────────────────────

func TestIntegration_SessionRename_Success(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)
	key := createTestAPIKey(t, app)

	// Create a session
	body, _ := json.Marshal(map[string]string{"title": "original"})
	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var created chatapi.SessionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	assert.Equal(t, "original", created.Title)

	// Rename it
	body, _ = json.Marshal(map[string]string{"title": "renamed"})
	req = httptest.NewRequest("PATCH", "/api/sessions/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var renamed chatapi.SessionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&renamed))
	assert.Equal(t, "renamed", renamed.Title)
	assert.Equal(t, created.ID, renamed.ID)
}

func TestIntegration_SessionRename_Isolation(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)

	keyA := createTestAPIKey(t, app)
	keyB := createTestAPIKey(t, app)

	// User A creates a session
	body, _ := json.Marshal(map[string]string{"title": "A's session"})
	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", keyA)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	var sessionA chatapi.SessionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sessionA))

	// User B tries to rename A's session — should get 404
	body, _ = json.Marshal(map[string]string{"title": "hijacked"})
	req = httptest.NewRequest("PATCH", "/api/sessions/"+sessionA.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", keyB)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// A's session is untouched
	req = httptest.NewRequest("GET", "/api/sessions/"+sessionA.ID, nil)
	req.Header.Set("X-API-Key", keyA)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	var got chatapi.SessionWithMessagesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "A's session", got.Title)
}

func TestIntegration_SessionRename_NotFound(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)
	key := createTestAPIKey(t, app)

	body, _ := json.Marshal(map[string]string{"title": "whatever"})
	req := httptest.NewRequest("PATCH", "/api/sessions/00000000-0000-0000-0000-000000000001", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// ── Skills ────────────────────────────────────────────────────────────────────

func TestIntegration_Skills_CRUD(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)
	key := createTestAPIKey(t, app)

	// Create
	body, _ := json.Marshal(map[string]any{
		"name":        "list-files",
		"description": "lists files in a directory",
		"content":     "ls -la $1",
		"is_public":   false,
	})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var created skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	assert.Equal(t, "list-files", created.Name)
	assert.Equal(t, "ls -la $1", created.Content)
	assert.False(t, created.IsPublic)
	assert.NotEmpty(t, created.ID)

	// List own — should contain the skill
	req = httptest.NewRequest("GET", "/api/skills", nil)
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var listed []skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)

	// Update — make it public and rename
	body, _ = json.Marshal(map[string]any{
		"name":      "list-files-v2",
		"content":   "ls -la $1",
		"is_public": true,
	})
	req = httptest.NewRequest("PATCH", "/api/skills/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var updated skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	assert.Equal(t, "list-files-v2", updated.Name)
	assert.True(t, updated.IsPublic)

	// Delete
	req = httptest.NewRequest("DELETE", "/api/skills/"+created.ID, nil)
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)

	// List own after delete — empty
	req = httptest.NewRequest("GET", "/api/skills", nil)
	req.Header.Set("X-API-Key", key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	var afterDelete []skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&afterDelete))
	assert.Empty(t, afterDelete)
}

func TestIntegration_Skills_PublicMarketplace(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)

	keyA := createTestAPIKey(t, app)
	keyB := createTestAPIKey(t, app)

	createSkill := func(key, name string, public bool) skillapi.SkillResponse {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"name": name, "content": "x", "is_public": public})
		req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", key)
		resp, err := app.Test(req, 10_000)
		require.NoError(t, err)
		require.Equal(t, fiber.StatusCreated, resp.StatusCode)
		var s skillapi.SkillResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&s))
		return s
	}

	pubA := createSkill(keyA, "public-a", true)
	createSkill(keyA, "private-a", false) // not visible to B
	createSkill(keyB, "private-b", false) // not visible to A

	// A lists own skills: both appear
	req := httptest.NewRequest("GET", "/api/skills", nil)
	req.Header.Set("X-API-Key", keyA)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	var ownA []skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ownA))
	assert.Len(t, ownA, 2, "A should see both own skills")

	// B lists public skills: only public-a appears
	req = httptest.NewRequest("GET", "/api/skills/public", nil)
	req.Header.Set("X-API-Key", keyB)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	var public []skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&public))
	require.Len(t, public, 1, "only public-a should appear in public list")
	assert.Equal(t, pubA.ID, public[0].ID)
}

func TestIntegration_Skills_Isolation(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)

	keyA := createTestAPIKey(t, app)
	keyB := createTestAPIKey(t, app)

	// A creates a skill
	body, _ := json.Marshal(map[string]any{"name": "a-skill", "content": "echo"})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", keyA)
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	var skillA skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&skillA))

	// B tries to update A's skill — 404
	body, _ = json.Marshal(map[string]any{"name": "stolen", "content": "evil"})
	req = httptest.NewRequest("PATCH", "/api/skills/"+skillA.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", keyB)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// B tries to delete A's skill — 404
	req = httptest.NewRequest("DELETE", "/api/skills/"+skillA.ID, nil)
	req.Header.Set("X-API-Key", keyB)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// A's skill is still intact (appears in own list)
	req = httptest.NewRequest("GET", "/api/skills", nil)
	req.Header.Set("X-API-Key", keyA)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	var ownA []skillapi.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ownA))
	require.Len(t, ownA, 1)
	assert.Equal(t, "a-skill", ownA[0].Name)
}

func TestIntegration_Skills_ValidationErrors(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)
	key := createTestAPIKey(t, app)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing name", map[string]any{"content": "echo"}},
		{"missing content", map[string]any{"name": "x"}},
		{"empty name", map[string]any{"name": "", "content": "echo"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.body)
			req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", key)
			resp, err := app.Test(req, 10_000)
			require.NoError(t, err)
			assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestIntegration_Skills_RequiresAuth(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupSkillSessionApp(queries)

	req := httptest.NewRequest("GET", "/api/skills", nil) // no X-API-Key
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

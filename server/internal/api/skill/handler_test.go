package skill_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/api/skill"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/Octopoos42/shellai/server/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ownerID and otherID represent two distinct API key IDs.
var (
	ownerID = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	otherID = pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	skillID = pgtype.UUID{Bytes: [16]byte{0xab}, Valid: true}
)

// setupSkillApp builds a Fiber app with skill routes, injecting apiKeyID as the
// authenticated caller.
func setupSkillApp(q db.Querier, callerID pgtype.UUID) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	injectKey := func(c *fiber.Ctx) error {
		key := &db.ApiKey{ID: callerID}
		c.Locals(middleware.APIKeyLocalsKey, key)
		return c.Next()
	}

	app.Post("/api/skills", injectKey, skill.HandleCreateSkill(q))
	app.Get("/api/skills", injectKey, skill.HandleListMySkills(q))
	app.Get("/api/skills/public", injectKey, skill.HandleListPublicSkills(q))
	app.Patch("/api/skills/:id", injectKey, skill.HandleUpdateSkill(q))
	app.Delete("/api/skills/:id", injectKey, skill.HandleDeleteSkill(q))
	return app
}

func sampleSkill(owner pgtype.UUID) db.Skill {
	return db.Skill{
		ID:          skillID,
		ApiKeyID:    owner,
		Name:        "my-skill",
		Description: "does stuff",
		Content:     "echo hello",
		IsPublic:    false,
	}
}

// ── HandleCreateSkill ─────────────────────────────────────────────────────────

func TestHandleCreateSkill_Success(t *testing.T) {
	created := sampleSkill(ownerID)
	mock := &testutil.MockQuerier{
		CreateSkillFn: func(_ context.Context, arg db.CreateSkillParams) (db.Skill, error) {
			assert.Equal(t, ownerID, arg.ApiKeyID)
			assert.Equal(t, "my-skill", arg.Name)
			assert.Equal(t, "echo hello", arg.Content)
			return created, nil
		},
	}
	app := setupSkillApp(mock, ownerID)

	body, _ := json.Marshal(map[string]any{"name": "my-skill", "content": "echo hello"})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var got skill.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "my-skill", got.Name)
}

func TestHandleCreateSkill_MissingName(t *testing.T) {
	app := setupSkillApp(&testutil.MockQuerier{}, ownerID)

	body, _ := json.Marshal(map[string]any{"content": "echo hi"})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestHandleCreateSkill_MissingContent(t *testing.T) {
	app := setupSkillApp(&testutil.MockQuerier{}, ownerID)

	body, _ := json.Marshal(map[string]any{"name": "x"})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// ── HandleListMySkills ────────────────────────────────────────────────────────

func TestHandleListMySkills_ReturnsOwnerSkills(t *testing.T) {
	skills := []db.Skill{sampleSkill(ownerID)}
	mock := &testutil.MockQuerier{
		ListSkillsByOwnerFn: func(_ context.Context, apiKeyID pgtype.UUID) ([]db.Skill, error) {
			assert.Equal(t, ownerID, apiKeyID)
			return skills, nil
		},
	}
	app := setupSkillApp(mock, ownerID)

	req := httptest.NewRequest("GET", "/api/skills", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var got []skill.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Len(t, got, 1)
}

func TestHandleListMySkills_EmptyList(t *testing.T) {
	app := setupSkillApp(&testutil.MockQuerier{}, ownerID)

	req := httptest.NewRequest("GET", "/api/skills", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var got []skill.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Empty(t, got)
}

// ── HandleListPublicSkills ────────────────────────────────────────────────────

func TestHandleListPublicSkills_ReturnsPublic(t *testing.T) {
	pub := sampleSkill(otherID)
	pub.IsPublic = true
	mock := &testutil.MockQuerier{
		ListPublicSkillsFn: func(_ context.Context) ([]db.Skill, error) {
			return []db.Skill{pub}, nil
		},
	}
	app := setupSkillApp(mock, ownerID) // logged in as owner, but gets other's public skill

	req := httptest.NewRequest("GET", "/api/skills/public", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var got []skill.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.True(t, got[0].IsPublic)
}

// ── HandleUpdateSkill ─────────────────────────────────────────────────────────

func TestHandleUpdateSkill_Success(t *testing.T) {
	existing := sampleSkill(ownerID)
	updated := existing
	updated.Name = "renamed"
	updated.IsPublic = true

	mock := &testutil.MockQuerier{
		GetSkillFn: func(_ context.Context, id pgtype.UUID) (db.Skill, error) {
			assert.Equal(t, skillID, id)
			return existing, nil
		},
		UpdateSkillFn: func(_ context.Context, arg db.UpdateSkillParams) (db.Skill, error) {
			assert.Equal(t, "renamed", arg.Name)
			assert.True(t, arg.IsPublic)
			return updated, nil
		},
	}
	app := setupSkillApp(mock, ownerID)

	idStr := uuidStr(skillID)
	pub := true
	body, _ := json.Marshal(map[string]any{"name": "renamed", "is_public": pub})
	req := httptest.NewRequest("PATCH", "/api/skills/"+idStr, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var got skill.SkillResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "renamed", got.Name)
	assert.True(t, got.IsPublic)
}

func TestHandleUpdateSkill_NotFound(t *testing.T) {
	mock := &testutil.MockQuerier{
		GetSkillFn: func(_ context.Context, _ pgtype.UUID) (db.Skill, error) {
			return db.Skill{}, pgx.ErrNoRows
		},
	}
	app := setupSkillApp(mock, ownerID)

	body, _ := json.Marshal(map[string]any{"name": "x", "content": "y"})
	req := httptest.NewRequest("PATCH", "/api/skills/"+uuidStr(skillID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestHandleUpdateSkill_WrongOwner(t *testing.T) {
	existing := sampleSkill(otherID) // owned by someone else
	mock := &testutil.MockQuerier{
		GetSkillFn: func(_ context.Context, _ pgtype.UUID) (db.Skill, error) {
			return existing, nil
		},
	}
	app := setupSkillApp(mock, ownerID) // caller is owner, not other

	body, _ := json.Marshal(map[string]any{"name": "x", "content": "y"})
	req := httptest.NewRequest("PATCH", "/api/skills/"+uuidStr(skillID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode) // 404, not 403
}

// ── HandleDeleteSkill ─────────────────────────────────────────────────────────

func TestHandleDeleteSkill_Success(t *testing.T) {
	existing := sampleSkill(ownerID)
	deleted := false
	mock := &testutil.MockQuerier{
		GetSkillFn: func(_ context.Context, _ pgtype.UUID) (db.Skill, error) {
			return existing, nil
		},
		DeleteSkillFn: func(_ context.Context, id pgtype.UUID) error {
			assert.Equal(t, skillID, id)
			deleted = true
			return nil
		},
	}
	app := setupSkillApp(mock, ownerID)

	req := httptest.NewRequest("DELETE", "/api/skills/"+uuidStr(skillID), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
	assert.True(t, deleted)
}

func TestHandleDeleteSkill_NotFound(t *testing.T) {
	mock := &testutil.MockQuerier{
		GetSkillFn: func(_ context.Context, _ pgtype.UUID) (db.Skill, error) {
			return db.Skill{}, pgx.ErrNoRows
		},
	}
	app := setupSkillApp(mock, ownerID)

	req := httptest.NewRequest("DELETE", "/api/skills/"+uuidStr(skillID), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestHandleDeleteSkill_WrongOwner(t *testing.T) {
	existing := sampleSkill(otherID)
	mock := &testutil.MockQuerier{
		GetSkillFn: func(_ context.Context, _ pgtype.UUID) (db.Skill, error) {
			return existing, nil
		},
	}
	app := setupSkillApp(mock, ownerID)

	req := httptest.NewRequest("DELETE", "/api/skills/"+uuidStr(skillID), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func uuidStr(u pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

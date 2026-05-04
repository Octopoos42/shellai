package chat_test

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
	chatapi "github.com/Octopoos42/shellai/server/internal/api/chat"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/Octopoos42/shellai/server/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	renameOwnerID    = pgtype.UUID{Bytes: [16]byte{10}, Valid: true}
	renameStrangerID = pgtype.UUID{Bytes: [16]byte{11}, Valid: true}
	renameSessionID  = pgtype.UUID{Bytes: [16]byte{0xcc}, Valid: true}
)

func setupRenameApp(q db.Querier, callerID pgtype.UUID) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	injectKey := func(c *fiber.Ctx) error {
		c.Locals(middleware.APIKeyLocalsKey, &db.ApiKey{ID: callerID})
		return c.Next()
	}
	app.Patch("/api/sessions/:id", injectKey, chatapi.HandleRenameSession(q))
	return app
}

func renameUUIDStr(u pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

func TestHandleRenameSession_Success(t *testing.T) {
	session := db.Session{ID: renameSessionID, ApiKeyID: renameOwnerID, Title: "old"}
	updated := session
	updated.Title = "new title"

	mock := &testutil.MockQuerier{
		GetSessionFn: func(_ context.Context, id pgtype.UUID) (db.Session, error) {
			assert.Equal(t, renameSessionID, id)
			return session, nil
		},
		UpdateSessionTitleFn: func(_ context.Context, arg db.UpdateSessionTitleParams) (db.Session, error) {
			assert.Equal(t, renameSessionID, arg.ID)
			assert.Equal(t, "new title", arg.Title)
			return updated, nil
		},
	}
	app := setupRenameApp(mock, renameOwnerID)

	body, _ := json.Marshal(map[string]string{"title": "new title"})
	req := httptest.NewRequest("PATCH", "/api/sessions/"+renameUUIDStr(renameSessionID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var got chatapi.SessionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "new title", got.Title)
}

func TestHandleRenameSession_NotFound(t *testing.T) {
	mock := &testutil.MockQuerier{
		GetSessionFn: func(_ context.Context, _ pgtype.UUID) (db.Session, error) {
			return db.Session{}, pgx.ErrNoRows
		},
	}
	app := setupRenameApp(mock, renameOwnerID)

	body, _ := json.Marshal(map[string]string{"title": "x"})
	req := httptest.NewRequest("PATCH", "/api/sessions/"+renameUUIDStr(renameSessionID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestHandleRenameSession_WrongOwner(t *testing.T) {
	// session is owned by stranger, caller is owner
	session := db.Session{ID: renameSessionID, ApiKeyID: renameStrangerID, Title: "private"}
	mock := &testutil.MockQuerier{
		GetSessionFn: func(_ context.Context, _ pgtype.UUID) (db.Session, error) {
			return session, nil
		},
	}
	app := setupRenameApp(mock, renameOwnerID)

	body, _ := json.Marshal(map[string]string{"title": "hijack"})
	req := httptest.NewRequest("PATCH", "/api/sessions/"+renameUUIDStr(renameSessionID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode) // 404, not 403
}

func TestHandleRenameSession_InvalidUUID(t *testing.T) {
	app := setupRenameApp(&testutil.MockQuerier{}, renameOwnerID)

	body, _ := json.Marshal(map[string]string{"title": "x"})
	req := httptest.NewRequest("PATCH", "/api/sessions/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

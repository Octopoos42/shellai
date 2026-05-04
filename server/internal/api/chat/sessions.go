// Package chat provides HTTP handlers for session management and LLM chat streaming.
package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
)

// SessionResponse is the JSON shape returned for session operations.
type SessionResponse struct {
	ID        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Title     string `json:"title" example:"My chat session"`
	CreatedAt string `json:"created_at" example:"2026-01-01T00:00:00Z"`
	UpdatedAt string `json:"updated_at" example:"2026-01-01T00:00:00Z"`
}

// MessageResponse is the JSON shape for a single chat message.
type MessageResponse struct {
	ID        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Role      string `json:"role" example:"user"`
	Content   string `json:"content" example:"Hello!"`
	CreatedAt string `json:"created_at" example:"2026-01-01T00:00:00Z"`
}

// SessionWithMessagesResponse includes messages along with session metadata.
type SessionWithMessagesResponse struct {
	SessionResponse
	Messages []MessageResponse `json:"messages"`
}

func toSessionResponse(s db.Session) SessionResponse {
	return SessionResponse{
		ID:        uuidToString(s.ID),
		Title:     s.Title,
		CreatedAt: s.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt: s.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func toMessageResponse(m db.Message) MessageResponse {
	return MessageResponse{
		ID:        uuidToString(m.ID),
		Role:      m.Role,
		Content:   m.Content,
		CreatedAt: m.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func uuidToString(u pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

// currentAPIKey returns the authenticated API key stored by RequireAPIKey middleware.
func currentAPIKey(c *fiber.Ctx) *db.ApiKey {
	return c.Locals(middleware.APIKeyLocalsKey).(*db.ApiKey)
}

// HandleCreateSession godoc
//
//	@Summary		Create session
//	@Description	Creates a new chat session owned by the authenticated API key. Title is optional.
//	@Tags			chat
//	@Accept			json
//	@Produce		json
//	@Param			body	body		object					false	"Optional title"
//	@Success		201		{object}	SessionResponse			"Created session"
//	@Failure		401		{object}	apierr.ErrorResponse	"Unauthorized"
//	@Failure		500		{object}	apierr.ErrorResponse	"Internal error"
//	@Security		ApiKeyAuth
//	@Router			/api/sessions [post]
func HandleCreateSession(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			Title string `json:"title"`
		}
		_ = c.BodyParser(&body)

		apiKey := currentAPIKey(c)
		session, err := queries.CreateSession(context.Background(), db.CreateSessionParams{
			ApiKeyID: apiKey.ID,
			Title:    body.Title,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		return c.Status(fiber.StatusCreated).JSON(toSessionResponse(session))
	}
}

// HandleListSessions godoc
//
//	@Summary		List sessions
//	@Description	Returns sessions owned by the authenticated API key, ordered by most recently updated.
//	@Tags			chat
//	@Produce		json
//	@Success		200	{array}		SessionResponse			"Sessions list"
//	@Failure		401	{object}	apierr.ErrorResponse	"Unauthorized"
//	@Failure		500	{object}	apierr.ErrorResponse	"Internal error"
//	@Security		ApiKeyAuth
//	@Router			/api/sessions [get]
func HandleListSessions(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := currentAPIKey(c)
		sessions, err := queries.ListSessions(context.Background(), apiKey.ID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		resp := make([]SessionResponse, len(sessions))
		for i, s := range sessions {
			resp[i] = toSessionResponse(s)
		}
		return c.JSON(resp)
	}
}

// HandleGetSession godoc
//
//	@Summary		Get session
//	@Description	Returns a session and its full message history. Returns 404 if not found or not owned by the caller.
//	@Tags			chat
//	@Produce		json
//	@Param			id	path		string							true	"Session UUID"
//	@Success		200	{object}	SessionWithMessagesResponse		"Session with messages"
//	@Failure		400	{object}	apierr.ErrorResponse			"Invalid UUID"
//	@Failure		401	{object}	apierr.ErrorResponse			"Unauthorized"
//	@Failure		404	{object}	apierr.ErrorResponse			"Not found"
//	@Failure		500	{object}	apierr.ErrorResponse			"Internal error"
//	@Security		ApiKeyAuth
//	@Router			/api/sessions/{id} [get]
func HandleGetSession(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := parseSessionID(c)
		if !ok {
			return nil
		}

		session, err := queries.GetSession(context.Background(), id)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}

		// Return 404 (not 403) to avoid leaking session existence to other users.
		if session.ApiKeyID != currentAPIKey(c).ID {
			return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
		}

		msgs, err := queries.ListMessagesBySession(context.Background(), id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		msgResp := make([]MessageResponse, len(msgs))
		for i, m := range msgs {
			msgResp[i] = toMessageResponse(m)
		}
		return c.JSON(SessionWithMessagesResponse{
			SessionResponse: toSessionResponse(session),
			Messages:        msgResp,
		})
	}
}

// HandleRenameSession godoc
//
//	@Summary		Rename session
//	@Description	Updates the title of a session. Returns 404 if not found or not owned by the caller.
//	@Tags			chat
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Session UUID"
//	@Param			body	body		object					true	"New title"
//	@Success		200		{object}	SessionResponse
//	@Failure		400		{object}	apierr.ErrorResponse
//	@Failure		401		{object}	apierr.ErrorResponse
//	@Failure		404		{object}	apierr.ErrorResponse
//	@Security		ApiKeyAuth
//	@Router			/api/sessions/{id} [patch]
func HandleRenameSession(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := parseSessionID(c)
		if !ok {
			return nil
		}

		var body struct {
			Title string `json:"title"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(apierr.New("INVALID_INPUT", "invalid request body"))
		}

		session, err := queries.GetSession(context.Background(), id)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		if session.ApiKeyID != currentAPIKey(c).ID {
			return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
		}

		updated, err := queries.UpdateSessionTitle(context.Background(), db.UpdateSessionTitleParams{
			ID:    id,
			Title: body.Title,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		return c.JSON(toSessionResponse(updated))
	}
}

// HandleDeleteSession godoc
//
//	@Summary		Delete session
//	@Description	Permanently deletes a session and all its messages. Returns 404 if not found or not owned by the caller.
//	@Tags			chat
//	@Param			id	path	string	true	"Session UUID"
//	@Success		204	"No Content"
//	@Failure		400	{object}	apierr.ErrorResponse	"Invalid UUID"
//	@Failure		401	{object}	apierr.ErrorResponse	"Unauthorized"
//	@Failure		404	{object}	apierr.ErrorResponse	"Not found"
//	@Failure		500	{object}	apierr.ErrorResponse	"Internal error"
//	@Security		ApiKeyAuth
//	@Router			/api/sessions/{id} [delete]
func HandleDeleteSession(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := parseSessionID(c)
		if !ok {
			return nil
		}

		session, err := queries.GetSession(context.Background(), id)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		if session.ApiKeyID != currentAPIKey(c).ID {
			return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
		}

		if err := queries.DeleteSession(context.Background(), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

// parseSessionID parses the ":id" route param as a UUID, writing a 400 on failure.
func parseSessionID(c *fiber.Ctx) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(c.Params("id")); err != nil {
		_ = c.Status(fiber.StatusBadRequest).JSON(apierr.WithDetails(
			"INVALID_INPUT", "invalid session UUID",
			map[string]any{"field": "id", "provided_value": c.Params("id")},
		))
		return pgtype.UUID{}, false
	}
	return id, true
}

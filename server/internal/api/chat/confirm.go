package chat

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/agent"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/Octopoos42/shellai/server/internal/db"
)

// ToolConfirmRequest is the body for the tool-confirm endpoint.
type ToolConfirmRequest struct {
	ConfirmID string `json:"confirm_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Approved  bool   `json:"approved" example:"true"`
}

// HandleToolConfirm godoc
//
//	@Summary		Approve or reject a pending tool call
//	@Description	Delivers the user's decision to the agent goroutine that is waiting for tool-call confirmation. Returns 410 Gone if the confirmation has already expired.
//	@Tags			chat
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Session UUID"
//	@Param			body	body		ToolConfirmRequest		true	"Confirmation decision"
//	@Success		204		"Decision delivered"
//	@Failure		400		{object}	apierr.ErrorResponse	"Invalid input"
//	@Failure		401		{object}	apierr.ErrorResponse	"Unauthorized"
//	@Failure		404		{object}	apierr.ErrorResponse	"Session not found"
//	@Failure		410		{object}	apierr.ErrorResponse	"Confirmation expired or not found"
//	@Security		ApiKeyAuth
//	@Router			/api/sessions/{id}/tool-confirm [post]
func HandleToolConfirm(queries db.Querier, store *agent.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionID, ok := parseSessionID(c)
		if !ok {
			return nil
		}

		// Verify the session exists and belongs to the authenticated API key.
		session, err := queries.GetSession(context.Background(), sessionID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		if session.ApiKeyID != currentAPIKey(c).ID {
			return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
		}

		var req ToolConfirmRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(apierr.New("INVALID_INPUT", "invalid request body"))
		}
		if req.ConfirmID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("INVALID_INPUT", "confirm_id is required", map[string]any{"field": "confirm_id"}),
			)
		}

		var confirmID pgtype.UUID
		if err := confirmID.Scan(req.ConfirmID); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("INVALID_INPUT", "confirm_id must be a valid UUID",
					map[string]any{"field": "confirm_id", "provided_value": req.ConfirmID}),
			)
		}

		if err := store.Confirm(confirmID, sessionID, req.Approved); err != nil {
			if errors.Is(err, agent.ErrConfirmExpired) {
				return c.Status(fiber.StatusGone).JSON(
					apierr.New("CONFIRM_EXPIRED", "tool confirmation has expired or was already resolved"),
				)
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}

		return c.SendStatus(fiber.StatusNoContent)
	}
}

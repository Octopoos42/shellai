package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
)

// MeResponse is the response body for the authenticated key info endpoint.
type MeResponse struct {
	ID        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Label     string `json:"label" example:"my-service"`
	CreatedAt string `json:"created_at" example:"2026-01-01T00:00:00Z"`
}

// HandleMe godoc
//
//	@Summary		Get current API key info
//	@Description	Returns information about the authenticated API key. Useful for verifying key validity.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	MeResponse				"Authenticated key details"
//	@Failure		401	{object}	apierr.ErrorResponse	"Unauthorized"
//	@Security		ApiKeyAuth
//	@Router			/api/me [get]
func HandleMe(c *fiber.Ctx) error {
	key := c.Locals(middleware.APIKeyLocalsKey).(*db.ApiKey)
	return c.JSON(MeResponse{
		ID:        key.ID.String(),
		Label:     key.Label,
		CreatedAt: key.CreatedAt.Time.UTC().Format(time.RFC3339),
	})
}

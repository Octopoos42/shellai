package api

import (
	"github.com/gofiber/fiber/v2"
)

// HealthResponse is the response body for the health check endpoint.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// HandleHealth godoc
//
//	@Summary		Health check
//	@Description	Returns server health status.
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Router			/api/health [get]
func HandleHealth(c *fiber.Ctx) error {
	return c.JSON(HealthResponse{Status: "ok"})
}

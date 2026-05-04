// Package admin provides HTTP handlers for admin-only API key management.
package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/Octopoos42/shellai/server/internal/db"
)

// CreateAPIKeyRequest is the request body for creating a new API key.
type CreateAPIKeyRequest struct {
	Label string `json:"label" example:"my-service"`
}

// APIKeyResponse is the JSON shape returned for API key operations.
// The Key field is only populated at creation time.
type APIKeyResponse struct {
	ID        string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Label     string  `json:"label" example:"my-service"`
	Key       string  `json:"key,omitempty" example:"shellai_3f9a..."` // present only in CreateAPIKey response
	CreatedAt string  `json:"created_at" example:"2026-01-01T00:00:00Z"`
	RevokedAt *string `json:"revoked_at" example:"null"`
}

func marshalAPIKey(k db.ApiKey) APIKeyResponse {
	resp := APIKeyResponse{
		ID:        formatUUID(k.ID),
		Label:     k.Label,
		CreatedAt: k.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
	if k.RevokedAt != nil {
		revoked := k.RevokedAt.UTC().Format(time.RFC3339)
		resp.RevokedAt = &revoked
	}
	return resp
}

func formatUUID(u pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

// HandleCreateAPIKey godoc
//
//	@Summary		Create API key
//	@Description	Generates a new API key, stores its SHA-256 hash, and returns the plaintext key exactly once.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CreateAPIKeyRequest			true	"API key label"
//	@Success		201		{object}	APIKeyResponse				"Created key (plaintext key included)"
//	@Failure		400		{object}	apierr.ErrorResponse		"Invalid input"
//	@Failure		401		{object}	apierr.ErrorResponse		"Unauthorized"
//	@Failure		500		{object}	apierr.ErrorResponse		"Internal error"
//	@Security		BasicAuth
//	@Router			/api/admin/apikeys [post]
func HandleCreateAPIKey(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body CreateAPIKeyRequest
		if err := c.BodyParser(&body); err != nil || body.Label == "" {
			return c.Status(fiber.StatusBadRequest).JSON(apierr.WithDetails(
				"INVALID_INPUT", "label is required",
				map[string]any{"field": "label"},
			))
		}

		// Generate a cryptographically random 32-byte token.
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.New("INTERNAL_ERROR", "failed to generate key"))
		}

		// The displayed key uses hex encoding with an "shellai_" prefix for identification.
		plaintext := "shellai_" + hex.EncodeToString(raw[:])

		// Store the SHA-256 hash of the raw bytes (not the hex string) for lookups.
		sum := sha256.Sum256(raw[:])
		hash := hex.EncodeToString(sum[:])

		key, err := queries.CreateAPIKey(context.Background(), db.CreateAPIKeyParams{
			KeyHash: hash,
			Label:   body.Label,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}

		resp := marshalAPIKey(key)
		resp.Key = plaintext // included only here, never stored
		return c.Status(fiber.StatusCreated).JSON(resp)
	}
}

// HandleListAPIKeys godoc
//
//	@Summary		List API keys
//	@Description	Returns all API keys (including revoked), without exposing hashes.
//	@Tags			admin
//	@Produce		json
//	@Success		200	{array}		APIKeyResponse			"List of API keys"
//	@Failure		401	{object}	apierr.ErrorResponse	"Unauthorized"
//	@Failure		500	{object}	apierr.ErrorResponse	"Internal error"
//	@Security		BasicAuth
//	@Router			/api/admin/apikeys [get]
func HandleListAPIKeys(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		keys, err := queries.ListAPIKeys(context.Background())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}

		resp := make([]APIKeyResponse, len(keys))
		for i, k := range keys {
			resp[i] = toResponse(k)
		}
		return c.JSON(resp)
	}
}

// HandleRevokeAPIKey godoc
//
//	@Summary		Revoke API key
//	@Description	Soft-deletes an API key by setting its revoked_at timestamp.
//	@Tags			admin
//	@Produce		json
//	@Param			id	path		string					true	"API key UUID"
//	@Success		200	{object}	APIKeyResponse			"Revoked key"
//	@Failure		400	{object}	apierr.ErrorResponse	"Invalid UUID"
//	@Failure		401	{object}	apierr.ErrorResponse	"Unauthorized"
//	@Failure		404	{object}	apierr.ErrorResponse	"Not found or already revoked"
//	@Failure		500	{object}	apierr.ErrorResponse	"Internal error"
//	@Security		BasicAuth
//	@Router			/api/admin/apikeys/{id} [delete]
func HandleRevokeAPIKey(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		idStr := c.Params("id")
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(apierr.WithDetails(
				"INVALID_INPUT", "invalid UUID format",
				map[string]any{"field": "id", "provided_value": idStr},
			))
		}

		key, err := queries.RevokeAPIKey(context.Background(), id)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "API key not found or already revoked"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}

		return c.JSON(toResponse(key))
	}
}

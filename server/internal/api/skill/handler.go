// Package skill provides HTTP handlers for skill/interface management.
package skill

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
)

// SkillResponse is the JSON shape returned for skill operations.
type SkillResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	IsPublic    bool   `json:"is_public"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toSkillResponse(s db.Skill) SkillResponse {
	return SkillResponse{
		ID:          uuidToString(s.ID),
		Name:        s.Name,
		Description: s.Description,
		Content:     s.Content,
		IsPublic:    s.IsPublic,
		CreatedAt:   s.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:   s.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func uuidToString(u pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

func currentAPIKey(c *fiber.Ctx) *db.ApiKey {
	return c.Locals(middleware.APIKeyLocalsKey).(*db.ApiKey)
}

func parseSkillID(c *fiber.Ctx) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(c.Params("id")); err != nil {
		_ = c.Status(fiber.StatusBadRequest).JSON(apierr.WithDetails(
			"INVALID_INPUT", "invalid skill UUID",
			map[string]any{"field": "id", "provided_value": c.Params("id")},
		))
		return pgtype.UUID{}, false
	}
	return id, true
}

// HandleCreateSkill godoc
//
//	@Summary		Create skill
//	@Description	Creates a new skill or interface definition owned by the authenticated API key.
//	@Tags			skills
//	@Accept			json
//	@Produce		json
//	@Param			body	body		object				true	"name, description, content, is_public"
//	@Success		201		{object}	SkillResponse		"Created skill"
//	@Failure		400		{object}	apierr.ErrorResponse
//	@Failure		401		{object}	apierr.ErrorResponse
//	@Security		ApiKeyAuth
//	@Router			/api/skills [post]
func HandleCreateSkill(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Content     string `json:"content"`
			IsPublic    bool   `json:"is_public"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(apierr.New("INVALID_INPUT", "invalid request body"))
		}
		if strings.TrimSpace(body.Name) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("INVALID_INPUT", "name is required", map[string]any{"field": "name"}),
			)
		}
		if strings.TrimSpace(body.Content) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("INVALID_INPUT", "content is required", map[string]any{"field": "content"}),
			)
		}

		skill, err := queries.CreateSkill(context.Background(), db.CreateSkillParams{
			ApiKeyID:    currentAPIKey(c).ID,
			Name:        body.Name,
			Description: body.Description,
			Content:     body.Content,
			IsPublic:    body.IsPublic,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		return c.Status(fiber.StatusCreated).JSON(toSkillResponse(skill))
	}
}

// HandleListMySkills godoc
//
//	@Summary		List my skills
//	@Description	Returns all skills owned by the authenticated API key.
//	@Tags			skills
//	@Produce		json
//	@Success		200	{array}		SkillResponse
//	@Failure		401	{object}	apierr.ErrorResponse
//	@Security		ApiKeyAuth
//	@Router			/api/skills [get]
func HandleListMySkills(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		skills, err := queries.ListSkillsByOwner(context.Background(), currentAPIKey(c).ID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		resp := make([]SkillResponse, len(skills))
		for i, s := range skills {
			resp[i] = toSkillResponse(s)
		}
		return c.JSON(resp)
	}
}

// HandleListPublicSkills godoc
//
//	@Summary		List public skills
//	@Description	Returns all publicly shared skills from all users.
//	@Tags			skills
//	@Produce		json
//	@Success		200	{array}		SkillResponse
//	@Failure		401	{object}	apierr.ErrorResponse
//	@Security		ApiKeyAuth
//	@Router			/api/skills/public [get]
func HandleListPublicSkills(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		skills, err := queries.ListPublicSkills(context.Background())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		resp := make([]SkillResponse, len(skills))
		for i, s := range skills {
			resp[i] = toSkillResponse(s)
		}
		return c.JSON(resp)
	}
}

// HandleUpdateSkill godoc
//
//	@Summary		Update skill
//	@Description	Updates a skill's name, description, content, or visibility. Returns 404 if not found or not owned by the caller.
//	@Tags			skills
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Skill UUID"
//	@Param			body	body		object				true	"Fields to update"
//	@Success		200		{object}	SkillResponse
//	@Failure		400		{object}	apierr.ErrorResponse
//	@Failure		401		{object}	apierr.ErrorResponse
//	@Failure		404		{object}	apierr.ErrorResponse
//	@Security		ApiKeyAuth
//	@Router			/api/skills/{id} [patch]
func HandleUpdateSkill(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := parseSkillID(c)
		if !ok {
			return nil
		}

		existing, err := queries.GetSkill(context.Background(), id)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "skill not found"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		if existing.ApiKeyID != currentAPIKey(c).ID {
			return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "skill not found"))
		}

		// Start from existing values so callers can patch a single field.
		updated := struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Content     string `json:"content"`
			IsPublic    *bool  `json:"is_public"`
		}{
			Name:        existing.Name,
			Description: existing.Description,
			Content:     existing.Content,
		}
		if err := c.BodyParser(&updated); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(apierr.New("INVALID_INPUT", "invalid request body"))
		}

		isPublic := existing.IsPublic
		if updated.IsPublic != nil {
			isPublic = *updated.IsPublic
		}

		if strings.TrimSpace(updated.Name) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("INVALID_INPUT", "name cannot be empty", map[string]any{"field": "name"}),
			)
		}

		skill, err := queries.UpdateSkill(context.Background(), db.UpdateSkillParams{
			ID:          id,
			Name:        updated.Name,
			Description: updated.Description,
			Content:     updated.Content,
			IsPublic:    isPublic,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		return c.JSON(toSkillResponse(skill))
	}
}

// HandleDeleteSkill godoc
//
//	@Summary		Delete skill
//	@Description	Permanently deletes a skill. Returns 404 if not found or not owned by the caller.
//	@Tags			skills
//	@Param			id	path	string	true	"Skill UUID"
//	@Success		204	"No Content"
//	@Failure		400	{object}	apierr.ErrorResponse
//	@Failure		401	{object}	apierr.ErrorResponse
//	@Failure		404	{object}	apierr.ErrorResponse
//	@Security		ApiKeyAuth
//	@Router			/api/skills/{id} [delete]
func HandleDeleteSkill(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := parseSkillID(c)
		if !ok {
			return nil
		}

		existing, err := queries.GetSkill(context.Background(), id)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "skill not found"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		if existing.ApiKeyID != currentAPIKey(c).ID {
			return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "skill not found"))
		}

		if err := queries.DeleteSkill(context.Background(), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

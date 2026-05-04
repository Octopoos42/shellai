// Package main is the entry point for the ShellAI server.
//
//	@title					ShellAI Server API
//	@version				1.0
//	@description			Web-based AI shell server. All user-facing endpoints require an API key;
//	@description			admin endpoints require HTTP Basic Auth.
//	@host					localhost:8080
//	@BasePath				/
//
//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-API-Key
//
//	@securityDefinitions.basic	BasicAuth
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	fiberlog "github.com/gofiber/fiber/v2/log"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	fiberswagger "github.com/gofiber/swagger"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/Octopoos42/shellai/server/internal/agent"
	"github.com/Octopoos42/shellai/server/internal/api"
	"github.com/Octopoos42/shellai/server/internal/api/admin"
	chatapi "github.com/Octopoos42/shellai/server/internal/api/chat"
	shellapi "github.com/Octopoos42/shellai/server/internal/api/shell"
	skillapi "github.com/Octopoos42/shellai/server/internal/api/skill"
	"github.com/Octopoos42/shellai/server/internal/config"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/Octopoos42/shellai/server/internal/migrate"
	"github.com/Octopoos42/shellai/server/internal/shell"

	_ "github.com/Octopoos42/shellai/server/docs" // swagger generated docs
)

// corsAllowOrigins returns CORS origins from CORS_ALLOW_ORIGINS when set,
// otherwise sensible local-development defaults.
func getCorsOrigins() []string {
	if raw := strings.TrimSpace(os.Getenv("CORS_ALLOW_ORIGINS")); raw != "" {
		parts := strings.Split(raw, ",")
		origins := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
		if len(origins) > 0 {
			return origins
		}
	}

	return []string{
		"http://localhost:9000",
		"http://127.0.0.1:9000",
	}
}

func runMigrations(ctx context.Context, databasePool *pgxpool.Pool) error {
	for _, dir := range []string{"db/schema", "server/db/schema"} {
		if _, err := os.Stat(dir); err == nil {
			return migrate.Apply(ctx, databasePool, dir)
		}
	}
	return fmt.Errorf("schema directory not found (tried db/schema and server/db/schema)")
}

// requireEnv reads an environment variable and fatals if it is unset or empty.
func getRequiredEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fiberlog.Fatalf("required environment variable %q is not set", key)
	}
	return v
}

func main() {
	// Load .env if present (development convenience; in production use injected env vars).
	_ = godotenv.Load()

	configData, err := config.Load("config.yaml")
	if err != nil {
		fiberlog.Fatalf("load config: %v", err)
	}

	dbURL := getRequiredEnv("DATABASE_URL")
	adminUser := getRequiredEnv("ADMIN_USERNAME")
	adminPass := getRequiredEnv("ADMIN_PASSWORD")

	databasePool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		fiberlog.Fatalf("connect to database: %v", err)
	}
	defer databasePool.Close()

	if err := runMigrations(context.Background(), databasePool); err != nil {
		fiberlog.Fatalf("apply migrations: %v", err)
	}

	queries := db.New(databasePool)
	agentStore := agent.NewStore()

	webApp := fiber.New(fiber.Config{
		// Use structured JSON error responses matching the project error standard.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var fe *fiber.Error
			if errors.As(err, &fe) {
				code = fe.Code
			}
			if err != nil {
				fiberlog.Errorf("unhandled error: %v", err)
			}

			message := "internal server error"
			if code >= 400 && code < 500 && err != nil {
				message = err.Error()
			}
			return c.Status(code).JSON(fiber.Map{
				"error_code": "INTERNAL_ERROR",
				"message":    message,
			})
		},
	})

	webApp.Use(recover.New())
	webApp.Use(logger.New())
	webApp.Use(cors.New(cors.Config{
		AllowOrigins: strings.Join(getCorsOrigins(), ","),
		AllowMethods: "GET,POST,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-API-Key,Last-Event-ID",
	}))

	// Swagger UI available at /swagger/index.html
	webApp.Get("/swagger/*", fiberswagger.HandlerDefault)

	webApp.Get("/api/health", api.HandleHealth)
	webApp.Get("/api/me", middleware.RequireAPIKey(queries), api.HandleMe)
	webApp.Post("/api/shell/exec", middleware.RequireAPIKey(queries), shellapi.HandleExec(shell.Executor{}))

	webApp.Post("/api/sessions", middleware.RequireAPIKey(queries), chatapi.HandleCreateSession(queries))
	webApp.Get("/api/sessions", middleware.RequireAPIKey(queries), chatapi.HandleListSessions(queries))
	webApp.Get("/api/sessions/:id", middleware.RequireAPIKey(queries), chatapi.HandleGetSession(queries))
	webApp.Patch("/api/sessions/:id", middleware.RequireAPIKey(queries), chatapi.HandleRenameSession(queries))
	webApp.Delete("/api/sessions/:id", middleware.RequireAPIKey(queries), chatapi.HandleDeleteSession(queries))
	webApp.Post("/api/sessions/:id/chat", middleware.RequireAPIKey(queries), chatapi.HandleChat(queries, configData, agentStore, shell.Executor{}))
	webApp.Post("/api/sessions/:id/tool-confirm", middleware.RequireAPIKey(queries), chatapi.HandleToolConfirm(queries, agentStore))

	webApp.Post("/api/skills", middleware.RequireAPIKey(queries), skillapi.HandleCreateSkill(queries))
	webApp.Get("/api/skills", middleware.RequireAPIKey(queries), skillapi.HandleListMySkills(queries))
	webApp.Get("/api/skills/public", middleware.RequireAPIKey(queries), skillapi.HandleListPublicSkills(queries))
	webApp.Patch("/api/skills/:id", middleware.RequireAPIKey(queries), skillapi.HandleUpdateSkill(queries))
	webApp.Delete("/api/skills/:id", middleware.RequireAPIKey(queries), skillapi.HandleDeleteSkill(queries))

	adminGroup := webApp.Group("/api/admin", middleware.RequireAdmin(adminUser, adminPass))
	adminGroup.Post("/apikeys", admin.HandleCreateAPIKey(queries))
	adminGroup.Get("/apikeys", admin.HandleListAPIKeys(queries))
	adminGroup.Delete("/apikeys/:id", admin.HandleRevokeAPIKey(queries))

	port := configData.Server.Port
	if port == 0 {
		if p := os.Getenv("PORT"); p != "" {
			if _, err := fmt.Sscanf(p, "%d", &port); err != nil {
				fiberlog.Fatalf("invalid PORT value %q: %v", p, err)
			}
		}
	}
	if port == 0 {
		port = 8080
	}

	if err := webApp.Listen(fmt.Sprintf(":%d", port)); err != nil {
		fiberlog.Fatal(err)
	}
}

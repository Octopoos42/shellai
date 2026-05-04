//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Octopoos42/shellai/server/internal/api"
	"github.com/Octopoos42/shellai/server/internal/api/admin"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	adminUser = "testadmin"
	adminPass = "testpass"
)

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// setupDB starts a PostgreSQL container, runs the schema migration, and returns
// a connection pool. The container and pool are cleaned up via t.Cleanup.
func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("shellai_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { pgc.Terminate(ctx) }) //nolint:errcheck

	connStr, err := pgc.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	for _, f := range []string{"001_init.sql", "002_skills.sql"} {
		schema, err := os.ReadFile("../../db/schema/" + f)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(schema))
		require.NoError(t, err)
	}

	return pool
}

// setupApp builds a Fiber test app wired to the given query layer.
func setupApp(queries db.Querier) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/api/me", middleware.RequireAPIKey(queries), api.HandleMe)
	adminGroup := app.Group("/api/admin", middleware.RequireAdmin(adminUser, adminPass))
	adminGroup.Post("/apikeys", admin.HandleCreateAPIKey(queries))
	adminGroup.Get("/apikeys", admin.HandleListAPIKeys(queries))
	adminGroup.Delete("/apikeys/:id", admin.HandleRevokeAPIKey(queries))
	return app
}

func TestIntegration_APIKeyLifecycle(t *testing.T) {
	pool := setupDB(t)
	queries := db.New(pool)
	app := setupApp(queries)

	// 1. Create an API key
	body, _ := json.Marshal(map[string]string{"label": "integration-test"})
	req := httptest.NewRequest("POST", "/api/admin/apikeys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAuth(adminUser, adminPass))

	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var created admin.APIKeyResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	assert.Equal(t, "integration-test", created.Label)
	assert.NotEmpty(t, created.Key, "plaintext key must be present at creation")
	assert.Nil(t, created.RevokedAt)

	// 2. List keys — should include the created one
	req = httptest.NewRequest("GET", "/api/admin/apikeys", nil)
	req.Header.Set("Authorization", basicAuth(adminUser, adminPass))
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var listed []admin.APIKeyResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)
	assert.Empty(t, listed[0].Key, "plaintext key must not appear in list response")

	// 3. /api/me with the plaintext key -> authenticated
	req = httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("X-API-Key", created.Key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// 4. Revoke the key
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/admin/apikeys/%s", created.ID), nil)
	req.Header.Set("Authorization", basicAuth(adminUser, adminPass))
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var revoked admin.APIKeyResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&revoked))
	assert.NotNil(t, revoked.RevokedAt)

	// 5. /api/me with revoked key -> 401
	req = httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("X-API-Key", created.Key)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// 6. Double-revoke -> 404
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/admin/apikeys/%s", created.ID), nil)
	req.Header.Set("Authorization", basicAuth(adminUser, adminPass))
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestIntegration_AdminAuth(t *testing.T) {
	pool := setupDB(t)
	app := setupApp(db.New(pool))

	// Wrong password -> 401
	req := httptest.NewRequest("GET", "/api/admin/apikeys", nil)
	req.Header.Set("Authorization", basicAuth(adminUser, "wrongpass"))
	resp, err := app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// No auth -> 401
	req = httptest.NewRequest("GET", "/api/admin/apikeys", nil)
	resp, err = app.Test(req, 10_000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

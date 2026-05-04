package middleware_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/middleware"
	"github.com/Octopoos42/shellai/server/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeAPIKey generates a random shellai_ key and returns (plaintext, hash) so tests
// can set up the mock with the correct hash and send the correct plaintext.
func makeAPIKey(t *testing.T) (plaintext, hash string) {
	t.Helper()
	var raw [32]byte
	_, err := rand.Read(raw[:])
	require.NoError(t, err)
	plaintext = "shellai_" + hex.EncodeToString(raw[:])
	sum := sha256.Sum256(raw[:])
	hash = hex.EncodeToString(sum[:])
	return
}

// newApp builds a minimal Fiber app with the given middleware and a 200 OK handler.
func newApp(mw fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", mw, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	return app
}

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// ── RequireAPIKey ─────────────────────────────────────────────────────────────

func TestRequireAPIKey_MissingHeader(t *testing.T) {
	app := newApp(middleware.RequireAPIKey(&testutil.MockQuerier{}))
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAPIKey_InvalidKey(t *testing.T) {
	mock := &testutil.MockQuerier{
		GetAPIKeyByHashFn: func(_ context.Context, _ string) (db.ApiKey, error) {
			return db.ApiKey{}, pgx.ErrNoRows
		},
	}
	app := newApp(middleware.RequireAPIKey(mock))

	// Valid hex but unknown key → 401 from DB lookup.
	plaintext, _ := makeAPIKey(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", plaintext)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAPIKey_BadFormat(t *testing.T) {
	app := newApp(middleware.RequireAPIKey(&testutil.MockQuerier{}))

	// "shellai_" prefix followed by non-hex → 401 from hex decode failure.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "shellai_notvalidhex!!")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAPIKey_ValidKey(t *testing.T) {
	plaintext, expectedHash := makeAPIKey(t)

	fixedKey := db.ApiKey{Label: "test-key"}
	fixedKey.ID.Valid = true
	mock := &testutil.MockQuerier{
		GetAPIKeyByHashFn: func(_ context.Context, gotHash string) (db.ApiKey, error) {
			if gotHash == expectedHash {
				return fixedKey, nil
			}
			return db.ApiKey{}, pgx.ErrNoRows
		},
	}
	app := newApp(middleware.RequireAPIKey(mock))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", plaintext)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequireAPIKey_BearerToken(t *testing.T) {
	plaintext, expectedHash := makeAPIKey(t)

	fixedKey := db.ApiKey{Label: "bearer-key"}
	mock := &testutil.MockQuerier{
		GetAPIKeyByHashFn: func(_ context.Context, gotHash string) (db.ApiKey, error) {
			if gotHash == expectedHash {
				return fixedKey, nil
			}
			return db.ApiKey{}, pgx.ErrNoRows
		},
	}
	app := newApp(middleware.RequireAPIKey(mock))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// ── RequireAdmin ──────────────────────────────────────────────────────────────

func TestRequireAdmin_MissingCredentials(t *testing.T) {
	app := newApp(middleware.RequireAdmin("admin", "secret"))
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAdmin_WrongPassword(t *testing.T) {
	app := newApp(middleware.RequireAdmin("admin", "secret"))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("admin", "wrong"))
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAdmin_ValidCredentials(t *testing.T) {
	app := newApp(middleware.RequireAdmin("admin", "secret"))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequireAdmin_WWWAuthenticateHeaderOnFailure(t *testing.T) {
	app := newApp(middleware.RequireAdmin("admin", "secret"))
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, `Basic realm="admin"`, resp.Header.Get("WWW-Authenticate"))
}

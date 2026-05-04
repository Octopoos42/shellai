// Package middleware provides Fiber middleware for authentication.
package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/Octopoos42/shellai/server/internal/db"
)

// APIKeyLocalsKey is the Fiber locals key under which the verified ApiKey row is stored.
const APIKeyLocalsKey = "api_key"

// RequireAPIKey authenticates requests via the X-API-Key header or a Bearer token.
// On success it stores the verified *db.ApiKey in c.Locals(APIKeyLocalsKey).
func RequireAPIKey(queries db.Querier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		secret := c.Get("X-API-Key")
		if secret == "" {
			secret = strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		}
		if secret == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(apierr.New("UNAUTHORIZED", "missing API key"))
		}

		tokenHex := strings.TrimPrefix(secret, "shellai_")
		rawToken, err := hex.DecodeString(tokenHex)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(apierr.New("UNAUTHORIZED", "invalid API key format"))
		}
		sum := sha256.Sum256(rawToken)
		hash := hex.EncodeToString(sum[:])

		key, err := queries.GetAPIKeyByHash(context.Background(), hash)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusUnauthorized).JSON(apierr.New("UNAUTHORIZED", "invalid or revoked API key"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}

		c.Locals(APIKeyLocalsKey, &key)
		return c.Next()
	}
}

// RequireAdmin authenticates admin requests using HTTP Basic Auth.
// Credentials are compared using constant-time comparison to prevent timing attacks.
// The username and password are injected at startup (read from env by main).
func RequireAdmin(username, password string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, pass, ok := decodeBasicAuth(c.Get("Authorization"))
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			c.Set("WWW-Authenticate", `Basic realm="admin"`)
			return c.Status(fiber.StatusUnauthorized).JSON(apierr.New("UNAUTHORIZED", "invalid admin credentials"))
		}
		return c.Next()
	}
}

// decodeBasicAuth parses an "Authorization: Basic <b64>" header value.
func decodeBasicAuth(header string) (user, pass string, ok bool) {
	payload, found := strings.CutPrefix(header, "Basic ")
	if !found {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", "", false
	}
	user, pass, ok = strings.Cut(string(decoded), ":")
	return
}

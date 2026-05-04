package apierr_test

import (
	"testing"

	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	r := apierr.New("NOT_FOUND", "resource not found")
	assert.Equal(t, "NOT_FOUND", r.ErrorCode)
	assert.Equal(t, "resource not found", r.Message)
	assert.Nil(t, r.Details)
}

func TestWithDetails(t *testing.T) {
	r := apierr.WithDetails("INVALID_INPUT", "bad field", map[string]any{"field": "email"})
	assert.Equal(t, "INVALID_INPUT", r.ErrorCode)
	assert.Equal(t, "bad field", r.Message)
	assert.Equal(t, "email", r.Details["field"])
}

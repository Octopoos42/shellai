// Package apierr provides the canonical error response type used across all API endpoints.
package apierr

import fiberlog "github.com/gofiber/fiber/v2/log"

// ErrorResponse is the standard error body returned by all API endpoints on failure.
type ErrorResponse struct {
	ErrorCode string         `json:"error_code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}

// New constructs an ErrorResponse with the given code and message.
func New(code, message string) ErrorResponse {
	return ErrorResponse{ErrorCode: code, Message: message}
}

// WithDetails constructs an ErrorResponse with additional structured context.
func WithDetails(code, message string, details map[string]any) ErrorResponse {
	return ErrorResponse{ErrorCode: code, Message: message, Details: details}
}

// Internal logs the underlying error and returns a generic INTERNAL_ERROR response.
// Use this for unexpected server-side failures (DB/network/etc.) so implementation
// details are never leaked to API clients.
func Internal(err error) ErrorResponse {
	if err != nil {
		fiberlog.Errorf("internal error: %v", err)
	}
	return New("INTERNAL_ERROR", "internal server error")
}

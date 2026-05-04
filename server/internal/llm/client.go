// Package llm provides a provider-agnostic interface for streaming LLM completions.
// Provider detection is based on model name prefix: "claude-" → Anthropic Messages API;
// anything else → OpenAI-compatible Chat Completions API (works with DeepSeek, etc.).
package llm

import (
	"context"
	"strings"
)

// Message is a single conversation turn.
type Message struct {
	Role    string // "user", "assistant", or "system"
	Content string
}

// StreamFunc is called for each text delta received from the provider.
// Returning a non-nil error aborts the stream immediately.
type StreamFunc func(delta string) error

// Client streams a chat completion from an LLM provider.
type Client interface {
	// Stream sends the conversation history to the model and calls fn for each
	// response token. It returns when streaming is complete, ctx is done, or fn
	// returns an error. The caller should cancel ctx when the downstream writer
	// fails (client disconnect) so the HTTP request to the provider is released.
	Stream(ctx context.Context, messages []Message, fn StreamFunc) error

	// JSONCall makes a non-streaming request and returns the full model response.
	// The caller must include JSON-schema instructions in the prompt; for
	// OpenAI-compatible providers this also enables response_format=json_object
	// for higher reliability.
	JSONCall(ctx context.Context, messages []Message) (string, error)
}

// ClientConfig holds the resolved runtime parameters for a single LLM client.
type ClientConfig struct {
	Endpoint  string
	Model     string
	APIKey    string
	MaxTokens int // derived from config.LimitK * 1000
}

// sanitizeMaxTokens clamps provider max_tokens to a range accepted by the backend APIs.
func sanitizeMaxTokens(maxTokens int) int {
	if maxTokens <= 0 {
		return 8192
	}
	if maxTokens > 8192 {
		return 8192
	}
	return maxTokens
}

// New creates a Client for the given config.
// "claude-*" models use the Anthropic Messages API; everything else uses the
// OpenAI Chat Completions API (including DeepSeek and compatible services).
func New(cfg ClientConfig) Client {
	if strings.HasPrefix(cfg.Model, "claude-") {
		return &anthropicClient{cfg: cfg}
	}
	return &openaiClient{cfg: cfg}
}

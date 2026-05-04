package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/Octopoos42/shellai/server/internal/llm"
)

// MockLLMClient is a test double for llm.Client. Responses are served from
// pre-programmed queues; Stream calls fn once with the entire response string.
// Calls beyond the end of the queue return an error.
type MockLLMClient struct {
	mu sync.Mutex

	// StreamResponses is consumed in order by successive Stream calls.
	StreamResponses []string
	// JSONResponses is consumed in order by successive JSONCall calls.
	JSONResponses []string

	streamIdx int
	jsonIdx   int
}

// Stream implements llm.Client. It delivers the next queued StreamResponse as a
// single delta to fn.
func (m *MockLLMClient) Stream(_ context.Context, _ []llm.Message, fn llm.StreamFunc) error {
	m.mu.Lock()
	idx := m.streamIdx
	m.streamIdx++
	m.mu.Unlock()

	if idx >= len(m.StreamResponses) {
		return fmt.Errorf("mock: no more Stream responses (call %d)", idx)
	}
	return fn(m.StreamResponses[idx])
}

// JSONCall implements llm.Client. It returns the next queued JSONResponse.
func (m *MockLLMClient) JSONCall(_ context.Context, _ []llm.Message) (string, error) {
	m.mu.Lock()
	idx := m.jsonIdx
	m.jsonIdx++
	m.mu.Unlock()

	if idx >= len(m.JSONResponses) {
		return "", fmt.Errorf("mock: no more JSONCall responses (call %d)", idx)
	}
	return m.JSONResponses[idx], nil
}

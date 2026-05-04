// Package agent implements the agentic loop: planner → optional tool call →
// direct streaming response. The loop runs inside the SSE body-stream goroutine
// and hands off tool-call confirmations to a separate HTTP endpoint via the
// Store's channel-based mechanism.
package agent

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrConfirmExpired is returned by Store.Confirm when the pending confirmation
// has already timed out, been consumed, or was never registered.
var ErrConfirmExpired = errors.New("tool confirmation expired or not found")

// PlannerResponse is the JSON structure the planner LLM call must return.
type PlannerResponse struct {
	NeedsTools      bool   `json:"needs_tools"`
	PlanDescription string `json:"plan_description"` // shown to the user only when NeedsTools is true
}

// ToolCallDecision is the JSON structure the tool-selector LLM call must return.
type ToolCallDecision struct {
	Tool        string         `json:"tool"`        // currently only "shell"
	Args        map[string]any `json:"args"`        // tool-specific arguments
	Explanation string         `json:"explanation"` // user-visible reason, shown before confirmation
}

// ExternalTool describes a client-provided third-party API/tool definition.
type ExternalTool struct {
	Name                    string `json:"name"`
	Endpoint                string `json:"endpoint"`
	Description             string `json:"description"`
	WaitForUserConfirm      bool   `json:"waitForUserConfirm"`
	NeedClientProvideAPIKey bool   `json:"needClientProvideApiKey"`
	Request                 string `json:"request"`
	Response                string `json:"response"`
	CommandType             string `json:"commandType"`
	CommandTemplate         string `json:"commandTemplate"`
}

// ToolResult holds the outcome of executing a tool.
type ToolResult struct {
	Tool     string `json:"tool"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Rejected bool   `json:"rejected,omitempty"` // true when the user declined the tool call
}

// PendingConfirm represents an in-flight tool call awaiting user approval.
// ResultCh is an unbuffered channel; Store.Confirm performs a non-blocking send
// so that races between timeout and a late confirmation are handled safely.
type PendingConfirm struct {
	ID        pgtype.UUID
	SessionID pgtype.UUID
	ToolCall  ToolCallDecision
	ResultCh  chan bool // receives true (approved) or false (rejected)
	ExpiresAt time.Time
}

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/llm"
	"github.com/Octopoos42/shellai/server/internal/shell"
)

// RunnerConfig holds agent-loop operational parameters.
type RunnerConfig struct {
	MaxIterations          int
	ToolConfirmTimeoutSecs int
}

// WithDefaults returns RunnerConfig with sensible fallbacks applied.
func (rc RunnerConfig) WithDefaults() RunnerConfig {
	if rc.MaxIterations <= 0 {
		rc.MaxIterations = 10
	}
	if rc.ToolConfirmTimeoutSecs <= 0 {
		rc.ToolConfirmTimeoutSecs = 300
	}
	return rc
}

// Runner orchestrates the planner → optional tool call → direct-response loop.
type Runner struct {
	client  llm.Client
	shell   shell.Runner
	store   *Store
	queries db.Querier
	cfg     RunnerConfig
	tools   []ExternalTool
}

// NewRunner constructs a Runner from its dependencies.
func NewRunner(client llm.Client, shellRunner shell.Runner, store *Store, queries db.Querier, cfg RunnerConfig, tools []ExternalTool) *Runner {
	return &Runner{
		client:  client,
		shell:   shellRunner,
		store:   store,
		queries: queries,
		cfg:     cfg.WithDefaults(),
		tools:   tools,
	}
}

// Run executes the agentic loop for one chat turn. It:
//  1. Stores the user message in the DB.
//  2. Calls the planner LLM (non-streaming, JSON) to decide whether a tool is needed.
//  3. If no tool needed: streams the LLM response directly, stores it, sends done.
//  4. If a tool is needed: emits a plan event, calls the tool-selector LLM
//     (non-streaming, JSON), emits a tool_request event, and blocks waiting for
//     the user to approve/reject via Store.Confirm. After the outcome the loop
//     repeats from step 2, up to cfg.MaxIterations times.
//
// All SSE events are written to w. cancel should be called when a write fails
// (client disconnect) to release in-flight LLM requests.
func (r *Runner) Run(ctx context.Context, cancel context.CancelFunc, w *bufio.Writer, sessionID pgtype.UUID, userMessage string) {
	bgCtx := context.Background()

	if _, err := r.queries.CreateMessage(bgCtx, db.CreateMessageParams{
		SessionID: sessionID,
		Role:      "user",
		Content:   userMessage,
	}); err != nil {
		writeErrorEvent(w, "INTERNAL_ERROR", err.Error())
		return
	}

	for range r.cfg.MaxIterations {
		history, err := r.queries.ListMessagesBySession(bgCtx, sessionID)
		if err != nil {
			writeErrorEvent(w, "INTERNAL_ERROR", err.Error())
			return
		}

		plan, err := r.callPlanner(ctx, history)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			writeErrorEvent(w, "LLM_ERROR", fmt.Sprintf("planner: %s", err))
			return
		}

		if !plan.NeedsTools {
			r.streamDirect(ctx, cancel, w, sessionID, history)
			return
		}

		if plan.PlanDescription != "" {
			writeSSEEvent(w, "plan", map[string]string{"description": plan.PlanDescription})
		}

		toolCall, err := r.callToolSelector(ctx, history)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			writeErrorEvent(w, "LLM_ERROR", fmt.Sprintf("tool-selector: %s", err))
			return
		}

		approved, err := r.awaitConfirmation(ctx, w, sessionID, *toolCall)
		if err != nil {
			// awaitConfirmation already wrote the appropriate SSE event.
			return
		}

		var result ToolResult
		if approved {
			result = r.executeTool(ctx, *toolCall)
			writeSSEEvent(w, "tool_result", result)
		} else {
			result = ToolResult{Tool: toolCall.Tool, Rejected: true}
			writeSSEEvent(w, "tool_rejected", map[string]string{"tool": toolCall.Tool})
		}

		if err := r.saveToolMessages(bgCtx, sessionID, *toolCall, result); err != nil {
			writeErrorEvent(w, "INTERNAL_ERROR", err.Error())
			return
		}
		if result.Rejected {
			// rejected: stop and let the user rephrase/retry
			return
		}
	}

	writeErrorEvent(w, "AGENT_LIMIT",
		fmt.Sprintf("reached maximum of %d tool-call iterations", r.cfg.MaxIterations))
}

// callPlanner runs the planner LLM call (non-streaming JSON). It prepends a
// system prompt instructing the model to return a PlannerResponse JSON object.
func (r *Runner) callPlanner(ctx context.Context, history []db.Message) (*PlannerResponse, error) {
	systemPrompt := `You are a planning assistant. Decide if a shell command must be executed to answer the user's latest request.

Available tool: "shell" — runs a non-interactive bash command on the server. You can invoke a python script or other scripts in the shell tool.

Be concise. Respond with a single JSON object only, no markdown, no prose:
{"needs_tools": <true|false>, "plan_description": "<one-sentence plan, only relevant when needs_tools is true>"}`

	if len(r.tools) > 0 {
		systemPrompt += "\n\nClient-provided third-party tools (use when relevant):\n"
		for i, tool := range r.tools {
			systemPrompt += fmt.Sprintf(
				"%d) name=%q endpoint=%q description=%q request=%q response=%q commandType=%q commandTemplate=%q waitForUserConfirm=%t needClientProvideApiKey=%t\n",
				i+1,
				tool.Name,
				tool.Endpoint,
				tool.Description,
				tool.Request,
				tool.Response,
				tool.CommandType,
				tool.CommandTemplate,
				tool.WaitForUserConfirm,
				tool.NeedClientProvideAPIKey,
			)
		}
	}

	msgs := append([]llm.Message{{Role: "system", Content: systemPrompt}}, buildLLMMessages(history)...)
	raw, err := r.client.JSONCall(ctx, msgs)
	if err != nil {
		return nil, err
	}

	plan, err := parsePlannerResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse planner JSON: %w (raw: %q)", err, raw)
	}
	return plan, nil
}

// callToolSelector runs the tool-selector LLM call (non-streaming JSON).
func (r *Runner) callToolSelector(ctx context.Context, history []db.Message) (*ToolCallDecision, error) {
	var systemPrompt strings.Builder
	systemPrompt.WriteString(`You are a tool-selection assistant. Choose the exact shell command to run next.
If you want to call a python script or other scripts, first use echo to create the script file "tmp.py" with the appropriate content, 
and then call it with "python3 tmp.py" in the shell tool. YOU SHOULD NEVER USE "python -c"!
Use CURL for short API calls. Use Python scripts to call APIs if you need to pass a long payload or complicated parameters. 
Make sure that you first write the script content to a file, and then call it. 
Do NOT put lots of tasks in one single python script, which will make it hard for the user to understand.

Be concise. Respond with a single JSON object only, no markdown, no prose:
{"tool": "shell", "args": {"command": "<bash command>"}, "explanation": "<one sentence shown to the user before execution>"}`)

	if len(r.tools) > 0 {
		systemPrompt.WriteString("\n\nWhen useful, build shell commands using these client-provided tool definitions:\n")
		for i, tool := range r.tools {
			fmt.Fprintf(&systemPrompt, "%d) name=%q endpoint=%q description=%q request=%q response=%q commandType=%q commandTemplate=%q waitForUserConfirm=%t needClientProvideApiKey=%t\n",
				i+1,
				tool.Name,
				tool.Endpoint,
				tool.Description,
				tool.Request,
				tool.Response,
				tool.CommandType,
				tool.CommandTemplate,
				tool.WaitForUserConfirm,
				tool.NeedClientProvideAPIKey,
			)
		}
	}

	msgs := append([]llm.Message{{Role: "system", Content: systemPrompt.String()}}, buildLLMMessages(history)...)
	raw, err := r.client.JSONCall(ctx, msgs)
	if err != nil {
		return nil, err
	}

	decision, err := parseToolCallDecision(raw)
	if err != nil {
		return nil, fmt.Errorf("parse tool-selector JSON: %w (raw: %q)", err, raw)
	}
	if decision.Tool == "" {
		return nil, fmt.Errorf("tool-selector returned empty tool name")
	}
	return decision, nil
}

// awaitConfirmation registers a PendingConfirm, emits a tool_request SSE event,
// and blocks until the user approves/rejects (via Store.Confirm) or the timeout
// fires. Returns the user's decision and nil, or false and a non-nil error if
// the wait was abandoned (SSE error event already written).
func (r *Runner) awaitConfirmation(ctx context.Context, w *bufio.Writer, sessionID pgtype.UUID, toolCall ToolCallDecision) (bool, error) {
	timeout := time.Duration(r.cfg.ToolConfirmTimeoutSecs) * time.Second
	pc := r.store.Create(sessionID, toolCall, timeout)

	writeSSEEvent(w, "tool_request", map[string]any{
		"id":          UUIDString(pc.ID),
		"tool":        toolCall.Tool,
		"args":        toolCall.Args,
		"explanation": toolCall.Explanation,
	})

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case approved := <-pc.ResultCh:
		return approved, nil
	case <-timer.C:
		r.store.Delete(pc.ID)
		writeErrorEvent(w, "CONFIRM_TIMEOUT", "tool confirmation timed out; no user response received")
		return false, errors.New("confirmation timed out")
	case <-ctx.Done():
		r.store.Delete(pc.ID)
		return false, ctx.Err()
	}
}

// executeTool runs the approved tool call and returns its result.
func (r *Runner) executeTool(ctx context.Context, toolCall ToolCallDecision) ToolResult {
	result := ToolResult{Tool: toolCall.Tool}

	switch toolCall.Tool {
	case "shell":
		command, _ := toolCall.Args["command"].(string)
		if command == "" {
			result.Stderr = "missing or empty 'command' argument"
			result.ExitCode = -1
			return result
		}
		var stdoutBuf, stderrBuf bytes.Buffer
		exitCode, err := r.shell.Run(ctx, command, &stdoutBuf, &stderrBuf)
		if err != nil {
			result.Stderr = err.Error()
			result.ExitCode = -1
		} else {
			result.ExitCode = exitCode
			result.Stdout = stdoutBuf.String()
			result.Stderr = stderrBuf.String()
		}
	default:
		result.Stderr = fmt.Sprintf("unknown tool %q", toolCall.Tool)
		result.ExitCode = -1
	}
	return result
}

// streamDirect makes a streaming LLM call using the full conversation history
// and writes token/done SSE events.
func (r *Runner) streamDirect(ctx context.Context, cancel context.CancelFunc, w *bufio.Writer, sessionID pgtype.UUID, history []db.Message) {
	bgCtx := context.Background()
	messages := buildLLMMessages(history)
	answerPrompt := `Now, provide a helpful and concise answer to the user's original question based on the conversation history.`
	messages = append(messages, llm.Message{Role: "system", Content: answerPrompt})
	var buf strings.Builder
	streamErr := r.client.Stream(ctx, messages, func(delta string) error {
		buf.WriteString(delta)
		return writeTokenEventChecked(w, delta, cancel)
	})

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		writeErrorEvent(w, "LLM_ERROR", streamErr.Error())
		return
	}
	if errors.Is(streamErr, context.Canceled) {
		return
	}

	content := buf.String()
	_, _ = r.queries.CreateMessage(bgCtx, db.CreateMessageParams{
		SessionID: sessionID,
		Role:      "assistant",
		Content:   content,
	})
	_ = r.queries.TouchSession(bgCtx, sessionID)
	writeDoneEvent(w, content)
}

// saveToolMessages persists a tool_call and its corresponding tool_result to the
// DB and touches the session's updated_at timestamp.
func (r *Runner) saveToolMessages(ctx context.Context, sessionID pgtype.UUID, toolCall ToolCallDecision, result ToolResult) error {
	tcJSON, _ := json.Marshal(toolCall)
	if _, err := r.queries.CreateMessage(ctx, db.CreateMessageParams{
		SessionID: sessionID,
		Role:      "tool_call",
		Content:   string(tcJSON),
	}); err != nil {
		return err
	}

	trJSON, _ := json.Marshal(result)
	if _, err := r.queries.CreateMessage(ctx, db.CreateMessageParams{
		SessionID: sessionID,
		Role:      "tool_result",
		Content:   string(trJSON),
	}); err != nil {
		return err
	}

	return r.queries.TouchSession(ctx, sessionID)
}

// buildLLMMessages converts DB message history to the llm.Message slice that
// providers expect. tool_call and tool_result roles are rendered as text so any
// provider can understand them without native tool-use support.
func buildLLMMessages(history []db.Message) []llm.Message {
	msgs := make([]llm.Message, 0, len(history))
	for _, m := range history {
		switch m.Role {
		case "tool_call":
			var tc ToolCallDecision
			_ = json.Unmarshal([]byte(m.Content), &tc)
			msgs = append(msgs, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("[Tool Call: %s]\nArgs: %v\nExplanation: %s", tc.Tool, tc.Args, tc.Explanation),
			})
		case "tool_result":
			var tr ToolResult
			_ = json.Unmarshal([]byte(m.Content), &tr)
			var content string
			if tr.Rejected {
				content = fmt.Sprintf("[Tool Call Rejected: %s]", tr.Tool)
			} else {
				content = fmt.Sprintf("[Tool Result: %s, exit_code=%d]\nStdout:\n%s\nStderr:\n%s",
					tr.Tool, tr.ExitCode, tr.Stdout, tr.Stderr)
			}
			msgs = append(msgs, llm.Message{Role: "user", Content: content})
		default:
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	return msgs
}

// cleanJSON strips optional markdown code fences that some LLMs wrap JSON in.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

func parsePlannerResponse(raw string) (*PlannerResponse, error) {
	cleaned := cleanJSON(raw)

	var plan PlannerResponse
	if err := json.Unmarshal([]byte(cleaned), &plan); err == nil {
		return &plan, nil
	}

	if obj, ok := extractFirstJSONObject(cleaned); ok {
		if err := json.Unmarshal([]byte(obj), &plan); err == nil {
			return &plan, nil
		}
	}

	if tc, ok := parseBracketToolCall(cleaned); ok {
		desc := tc.Explanation
		if desc == "" {
			desc = fmt.Sprintf("Use %q tool to continue", tc.Tool)
		}
		return &PlannerResponse{
			NeedsTools:      true,
			PlanDescription: desc,
		}, nil
	}

	return nil, errors.New("invalid planner JSON")
}

func parseToolCallDecision(raw string) (*ToolCallDecision, error) {
	cleaned := cleanJSON(raw)

	var decision ToolCallDecision
	if err := json.Unmarshal([]byte(cleaned), &decision); err == nil {
		return &decision, nil
	}

	if obj, ok := extractFirstJSONObject(cleaned); ok {
		if err := json.Unmarshal([]byte(obj), &decision); err == nil {
			return &decision, nil
		}
	}

	if tc, ok := parseBracketToolCall(cleaned); ok {
		return &tc, nil
	}

	if tc, ok := parseRelaxedToolCallJSON(cleaned); ok {
		return &tc, nil
	}

	return nil, errors.New("invalid tool-selector JSON")
}

// parseRelaxedToolCallJSON parses JSON-like output where quoted string values
// may include raw newlines (invalid in strict JSON), which some models emit.
func parseRelaxedToolCallJSON(s string) (ToolCallDecision, bool) {
	tool, ok := extractRelaxedJSONStringValue(s, "tool")
	if !ok || strings.TrimSpace(tool) == "" {
		return ToolCallDecision{}, false
	}

	decision := ToolCallDecision{
		Tool: strings.TrimSpace(tool),
		Args: map[string]any{},
	}

	if command, ok := extractRelaxedJSONStringValue(s, "command"); ok {
		decision.Args["command"] = command
	}
	if explanation, ok := extractRelaxedJSONStringValue(s, "explanation"); ok {
		decision.Explanation = strings.TrimSpace(explanation)
	}

	return decision, true
}

func extractRelaxedJSONStringValue(s, key string) (string, bool) {
	needle := fmt.Sprintf("\"%s\"", key)
	keyIdx := strings.Index(s, needle)
	if keyIdx == -1 {
		return "", false
	}

	i := keyIdx + len(needle)
	i = skipASCIIWhitespace(s, i)
	if i >= len(s) || s[i] != ':' {
		return "", false
	}

	i++
	i = skipASCIIWhitespace(s, i)
	if i >= len(s) || s[i] != '"' {
		return "", false
	}

	value, ok := scanRelaxedJSONString(s, i)
	if !ok {
		return "", false
	}

	return value, true
}

func scanRelaxedJSONString(s string, quoteIdx int) (string, bool) {
	if quoteIdx >= len(s) || s[quoteIdx] != '"' {
		return "", false
	}

	escaped := false
	for i := quoteIdx + 1; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			return s[quoteIdx+1 : i], true
		}
	}

	return "", false
}

func skipASCIIWhitespace(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

// extractFirstJSONObject returns the first balanced JSON object found in s.
func extractFirstJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start == -1 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}

	return "", false
}

// parseBracketToolCall parses text like:
// [Tool Call: shell]
// Args: map[command:echo hi]
// Explanation: reason
func parseBracketToolCall(s string) (ToolCallDecision, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[Tool Call:") {
		return ToolCallDecision{}, false
	}

	endTool := strings.IndexByte(s, ']')
	if endTool == -1 {
		return ToolCallDecision{}, false
	}

	tool := strings.TrimSpace(s[len("[Tool Call:"):endTool])
	if tool == "" {
		return ToolCallDecision{}, false
	}

	body := strings.TrimSpace(s[endTool+1:])
	args := map[string]any{}
	explanation := ""

	argsIdx := strings.Index(body, "Args:")
	explIdx := strings.Index(body, "Explanation:")

	if argsIdx != -1 {
		argsStart := argsIdx + len("Args:")
		argsEnd := len(body)
		if explIdx != -1 && explIdx > argsStart {
			argsEnd = explIdx
		}
		argsPart := strings.TrimSpace(body[argsStart:argsEnd])

		const mapPrefix = "map[command:"
		if strings.HasPrefix(argsPart, mapPrefix) {
			cmd := strings.TrimSpace(strings.TrimPrefix(argsPart, mapPrefix))
			cmd = strings.TrimSuffix(cmd, "]")
			if cmd != "" {
				args["command"] = cmd
			}
		}
	}

	if explIdx != -1 {
		explStart := explIdx + len("Explanation:")
		explanation = strings.TrimSpace(body[explStart:])
	}

	return ToolCallDecision{Tool: tool, Args: args, Explanation: explanation}, true
}

// --- SSE helpers ---

func writeSSEEvent(w *bufio.Writer, event string, data any) {
	b, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	_ = w.Flush()
}

func writeErrorEvent(w *bufio.Writer, code, message string) {
	b, _ := json.Marshal(apierr.New(code, message))
	_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
	_ = w.Flush()
}

func writeTokenEventChecked(w *bufio.Writer, text string, cancel context.CancelFunc) error {
	data, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if _, err := fmt.Fprintf(w, "event: token\ndata: %s\n\n", data); err != nil {
		cancel()
		return err
	}
	if err := w.Flush(); err != nil {
		cancel()
		return err
	}
	return nil
}

func writeDoneEvent(w *bufio.Writer, content string) {
	data, _ := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: content})
	_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
	_ = w.Flush()
}

package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	fiberlog "github.com/gofiber/fiber/v2/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/agent"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	"github.com/Octopoos42/shellai/server/internal/config"
	"github.com/Octopoos42/shellai/server/internal/db"
	"github.com/Octopoos42/shellai/server/internal/llm"
	"github.com/Octopoos42/shellai/server/internal/shell"
)

const textHelp = `Available commands:
  /help               — show this help message
  /compact [prompt]   — summarize and replace the conversation history; reduces context size
  /interrupt          — close this connection to cancel a running response`

// RequestChat is the body for the chat endpoint.
type RequestChat struct {
	Message string               `json:"message" example:"Hello, how are you?"`
	Model   string               `json:"model,omitempty" example:"deepseek"` // uses config default_model if omitted
	Tools   []agent.ExternalTool `json:"tools,omitempty"`
}

func writeTokenEvent(w *bufio.Writer, text string) {
	data, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	_, _ = fmt.Fprintf(w, "event: token\ndata: %s\n\n", data)
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

func writeErrorEvent(w *bufio.Writer, code, message string) {
	data, _ := json.Marshal(apierr.New(code, message))
	_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
	_ = w.Flush()
}

// HandleChat godoc
//
//	@Summary		Chat with LLM (agentic)
//	@Description	Sends a message and streams the response as SSE events. Regular messages go through the agentic loop (planner → optional tool call → response). Slash commands /help and /compact are handled directly.
//	@Tags			chat
//	@Accept			json
//	@Produce		text/event-stream
//	@Param			id		path		string					true	"Session UUID"
//	@Param			body	body		RequestChat				true	"Chat message"
//	@Success		200		{string}	string					"SSE stream (plan / tool_request / tool_result / token / done / error events)"
//	@Failure		400		{object}	apierr.ErrorResponse	"Invalid input or model not found"
//	@Failure		401		{object}	apierr.ErrorResponse	"Unauthorized"
//	@Failure		404		{object}	apierr.ErrorResponse	"Session not found"
//	@Security		ApiKeyAuth
//	@Router			/api/sessions/{id}/chat [post]
func HandleChat(queries db.Querier, cfg *config.Config, store *agent.Store, shellRunner shell.Runner) fiber.Handler {
	return HandleChatWithFactory(queries, cfg, store, shellRunner, llm.New)
}

// HandleChatWithFactory is like HandleChat but accepts a custom LLM client
// constructor, which is useful for injecting mock clients in tests.
func HandleChatWithFactory(
	queries db.Querier,
	cfg *config.Config,
	store *agent.Store,
	shellRunner shell.Runner,
	newClient func(llm.ClientConfig) llm.Client,
) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionID, ok := parseSessionID(c)
		if !ok {
			return nil
		}

		var req RequestChat
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(apierr.New("INVALID_INPUT", "invalid request body"))
		}
		if strings.TrimSpace(req.Message) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("INVALID_INPUT", "message is required", map[string]any{"field": "message"}),
			)
		}

		modelName := req.Model
		if modelName == "" {
			modelName = cfg.DefaultModel
		}
		llmConfig, found := cfg.FindLLM(modelName)
		if !found {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("MODEL_NOT_FOUND", "model not configured",
					map[string]any{"model": modelName}),
			)
		}
		apiKey := os.Getenv(llmConfig.EnvKey)
		if apiKey == "" {
			missingErr := fmt.Errorf("API key env var %q is not set", llmCfg.EnvKey)
			return c.Status(fiber.StatusInternalServerError).JSON(
				apierr.Internal(missingErr),
			)
		}

		session, err := queries.GetSession(context.Background(), sessionID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(apierr.Internal(err))
		}
		if session.ApiKeyID != currentAPIKey(c).ID {
			return c.Status(fiber.StatusNotFound).JSON(apierr.New("NOT_FOUND", "session not found"))
		}

		client := newClient(llm.ClientConfig{
			Endpoint:  llmConfig.Endpoint,
			Model:     llmConfig.Model,
			APIKey:    apiKey,
			MaxTokens: llmConfig.LimitK * 1000,
		})

		agentCfg := cfg.Agent.WithDefaults()
		runner := agent.NewRunner(client, shellRunner, store, queries, agent.RunnerConfig{
			MaxIterations:          agentCfg.MaxIterations,
			ToolConfirmTimeoutSecs: agentCfg.ToolConfirmTimeoutSecs,
		}, req.Tools)

		userCtx := c.UserContext()
		message := strings.TrimSpace(req.Message)

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			streamCtx, cancel := context.WithCancel(userCtx)
			defer cancel()

			switch {
			case message == "/help":
				streamHelp(w, sessionID, queries)
			case strings.HasPrefix(message, "/compact"):
				prompt := strings.TrimSpace(strings.TrimPrefix(message, "/compact"))
				streamCompact(streamCtx, cancel, w, sessionID, queries, client, prompt)
			default:
				runner.Run(streamCtx, cancel, w, sessionID, message)
			}
		})
		return nil
	}
}

func streamHelp(w *bufio.Writer, sessionID pgtype.UUID, queries db.Querier) {
	writeTokenEvent(w, textHelp)
	writeDoneEvent(w, textHelp)

	bgCtx := context.Background()
	_, _ = queries.CreateMessage(bgCtx, db.CreateMessageParams{
		SessionID: sessionID, Role: "user", Content: "/help",
	})
	_, _ = queries.CreateMessage(bgCtx, db.CreateMessageParams{
		SessionID: sessionID, Role: "assistant", Content: textHelp,
	})
	_ = queries.TouchSession(bgCtx, sessionID)
}

func streamCompact(
	ctx context.Context, cancel context.CancelFunc,
	w *bufio.Writer, sessionID pgtype.UUID,
	queries db.Querier, client llm.Client, extraPrompt string,
) {
	bgCtx := context.Background()

	history, err := queries.ListMessagesBySession(bgCtx, sessionID)
	if err != nil {
		fiberlog.Errorf("streamCompact ListMessagesBySession failed: %v", err)
		writeErrorEvent(w, "INTERNAL_ERROR", "internal server error")
		return
	}

	var transcript strings.Builder
	for _, msg := range history {
		transcript.WriteString(msg.Role)
		transcript.WriteString(": ")
		transcript.WriteString(msg.Content)
		transcript.WriteString("\n\n")
	}
	summarizePrompt := "Please produce a concise summary of the following conversation that preserves all important context, decisions, and key information. Output only the summary — no preamble."
	if extraPrompt != "" {
		summarizePrompt += "\n\nAdditional instructions: " + extraPrompt
	}
	summarizePrompt += "\n\nConversation:\n" + transcript.String()

	_, _ = queries.CreateMessage(bgCtx, db.CreateMessageParams{
		SessionID: sessionID, Role: "user", Content: "/compact " + extraPrompt,
	})

	var buf strings.Builder
	streamErr := client.Stream(ctx, []llm.Message{{Role: "user", Content: summarizePrompt}}, func(delta string) error {
		buf.WriteString(delta)
		return writeTokenEventChecked(w, delta, cancel)
	})

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		writeErrorEvent(w, "LLM_ERROR", streamErr.Error())
		return
	}

	summary := buf.String()
	if summary == "" {
		writeErrorEvent(w, "LLM_ERROR", "empty summary returned")
		return
	}

	_ = queries.DeleteMessagesBySession(bgCtx, sessionID)
	_, _ = queries.CreateMessage(bgCtx, db.CreateMessageParams{
		SessionID: sessionID, Role: "system", Content: summary,
	})
	_ = queries.TouchSession(bgCtx, sessionID)
	writeDoneEvent(w, summary)
}

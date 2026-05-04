// Package shell provides the HTTP handler for non-interactive shell command
// execution, streaming stdout and stderr as Server-Sent Events.
package shell

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/Octopoos42/shellai/server/internal/apierr"
	shellex "github.com/Octopoos42/shellai/server/internal/shell"
)

// RequestExec is the request body for the shell execution endpoint.
type RequestExec struct {
	Command string `json:"command" example:"ls -la"`
}

// EventExit is the payload of the SSE "exit" event, sent after the command
// completes. Error is non-empty only when the process could not be started or
// was killed by context cancellation (i.e. not for non-zero exit codes).
type EventExit struct {
	Code  int    `json:"code"`
	Error string `json:"error,omitempty"`
}

// sseWriter serialises concurrent writes from stdout and stderr goroutines
// into SSE-formatted events on a shared bufio.Writer.
type sseWriter struct {
	w  *bufio.Writer
	mu sync.Mutex
}

func (s *sseWriter) writer(event string) *eventWriter {
	return &eventWriter{sse: s, event: event}
}

func (s *sseWriter) writeChunk(event string, p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: string(p)})
	_, _ = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, data)
	_ = s.w.Flush()
}

type eventWriter struct {
	sse   *sseWriter
	event string
}

func (e *eventWriter) Write(p []byte) (int, error) {
	e.sse.writeChunk(e.event, p)
	return len(p), nil
}

// HandleExec godoc
//
//	@Summary		Execute shell command
//	@Description	Executes a non-interactive bash command. sudo is strictly banned. Streams stdout and stderr as SSE events ("stdout"/"stderr"), and terminates with an "exit" event carrying the exit code.
//	@Tags			shell
//	@Accept			json
//	@Produce		text/event-stream
//	@Param			body	body		RequestExec				true	"Command to execute"
//	@Success		200		{string}	string					"SSE stream of stdout/stderr/exit events"
//	@Failure		400		{object}	apierr.ErrorResponse	"Empty or invalid command"
//	@Failure		403		{object}	apierr.ErrorResponse	"sudo is banned"
//	@Failure		401		{object}	apierr.ErrorResponse	"Unauthorized"
//	@Security		ApiKeyAuth
//	@Router			/api/shell/exec [post]
func HandleExec(runner shellex.Runner) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req RequestExec
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.New("INVALID_INPUT", "invalid request body"),
			)
		}
		cmd := strings.TrimSpace(req.Command)
		if cmd == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				apierr.WithDetails("INVALID_INPUT", "command is required", map[string]any{"field": "command"}),
			)
		}
		if err := shellex.ValidateCommand(cmd); err != nil {
			if errors.Is(err, shellex.ErrSudoBanned) {
				return c.Status(fiber.StatusForbidden).JSON(
					apierr.New("SUDO_BANNED", "sudo is not allowed"),
				)
			}
			return c.Status(fiber.StatusBadRequest).JSON(apierr.New("INVALID_INPUT", err.Error()))
		}

		ctx := c.UserContext()

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no") // disable nginx proxy buffering

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			sw := &sseWriter{w: w}
			exitCode, runErr := runner.Run(ctx, cmd, sw.writer("stdout"), sw.writer("stderr"))

			ev := EventExit{Code: exitCode}
			if runErr != nil {
				ev.Error = runErr.Error()
			}
			data, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "event: exit\ndata: %s\n\n", data)
			_ = w.Flush()
		})

		return nil
	}
}

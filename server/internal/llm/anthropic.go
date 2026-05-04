package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// anthropicClient calls the Anthropic Messages API.
type anthropicClient struct {
	cfg ClientConfig
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicNonStreamResponse is the minimal shape of a non-streaming completion.
type anthropicNonStreamResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// anthropicEvent is the minimal shape needed to parse Anthropic SSE data lines.
type anthropicEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *anthropicClient) Stream(ctx context.Context, messages []Message, fn StreamFunc) error {
	// Anthropic requires system messages in a dedicated top-level field, not in the
	// messages array. Extract and join them; the rest go into messages.
	var systemParts []string
	var turns []anthropicMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
		} else {
			turns = append(turns, anthropicMessage(m))
		}
	}
	if len(turns) == 0 {
		return fmt.Errorf("anthropic: messages must contain at least one user or assistant turn")
	}

	maxTok := c.cfg.MaxTokens
	maxTok = sanitizeMaxTokens(maxTok)

	reqBody := anthropicRequest{
		Model:     c.cfg.Model,
		MaxTokens: maxTok,
		System:    strings.Join(systemParts, "\n\n"),
		Messages:  turns,
		Stream:    true,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("provider error %d: %s", resp.StatusCode, b)
	}

	// Anthropic SSE format pairs "event: <type>" with "data: <json>" lines.
	// We only care about content_block_delta events with text_delta deltas.
	scanner := bufio.NewScanner(resp.Body)
	var eventType string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if eventType != "content_block_delta" {
				continue
			}
			var ev anthropicEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				continue
			}
			if ev.Type == "error" {
				return fmt.Errorf("anthropic stream error: %s", ev.Error.Message)
			}
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				if err := fn(ev.Delta.Text); err != nil {
					return err
				}
			}
		case line == "":
			eventType = "" // reset between blocks
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("read stream: %w", err)
	}
	return nil
}

func (c *anthropicClient) JSONCall(ctx context.Context, messages []Message) (string, error) {
	var systemParts []string
	var turns []anthropicMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
		} else {
			turns = append(turns, anthropicMessage(m))
		}
	}
	if len(turns) == 0 {
		return "", fmt.Errorf("anthropic: messages must contain at least one user or assistant turn")
	}

	reqBody := anthropicRequest{
		Model:     c.cfg.Model,
		MaxTokens: sanitizeMaxTokens(c.cfg.MaxTokens),
		System:    strings.Join(systemParts, "\n\n"),
		Messages:  turns,
		Stream:    false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("provider error %d: %s", resp.StatusCode, b)
	}

	var result anthropicNonStreamResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in response")
}

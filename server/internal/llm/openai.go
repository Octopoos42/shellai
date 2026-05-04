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

// openaiClient calls any OpenAI Chat Completions-compatible endpoint.
// This covers DeepSeek, standard OpenAI (/v1/chat/completions), and similar services.
type openaiClient struct {
	cfg ClientConfig
}

type oaiRequest struct {
	Model     string       `json:"model"`
	Messages  []oaiMessage `json:"messages"`
	MaxTokens int          `json:"max_tokens,omitempty"`
	Stream    bool         `json:"stream"`
}

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// oaiDelta is the minimal shape needed from a streaming chunk.
type oaiDelta struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// oaiNonStreamResponse is the minimal shape of a non-streaming completion.
type oaiNonStreamResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *openaiClient) Stream(ctx context.Context, messages []Message, fn StreamFunc) error {
	msgs := make([]oaiMessage, len(messages))
	for i, m := range messages {
		msgs[i] = oaiMessage(m)
	}

	body, err := json.Marshal(oaiRequest{
		Model:     c.cfg.Model,
		Messages:  msgs,
		MaxTokens: sanitizeMaxTokens(c.cfg.MaxTokens),
		Stream:    true,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
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

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		var chunk oaiDelta
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		if err := fn(delta); err != nil {
			return err
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

func (c *openaiClient) JSONCall(ctx context.Context, messages []Message) (string, error) {
	msgs := make([]oaiMessage, len(messages))
	for i, m := range messages {
		msgs[i] = oaiMessage(m)
	}

	reqBody := struct {
		Model          string       `json:"model"`
		Messages       []oaiMessage `json:"messages"`
		MaxTokens      int          `json:"max_tokens,omitempty"`
		Stream         bool         `json:"stream"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
	}{
		Model:     c.cfg.Model,
		Messages:  msgs,
		MaxTokens: sanitizeMaxTokens(c.cfg.MaxTokens),
		Stream:    false,
	}
	reqBody.ResponseFormat.Type = "json_object"

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
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

	var result oaiNonStreamResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

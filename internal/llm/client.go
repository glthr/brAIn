// Package llm provides an Ollama chat client that uses the /v1/chat/completions endpoint.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

type Client struct {
	BaseURL    string
	Model      string
	APIKey     string
	HTTPClient *http.Client
}

// DefaultHTTPTimeout is the HTTP client timeout. Ingest batch encoding can take 20+ minutes
// (encodingTimeout in ingest.go); the client must not cap that.
const DefaultHTTPTimeout = 25 * time.Minute

// NewClient creates a client. For Ollama use base URL "http://localhost:11434".
func NewClient(baseURL, model string) *Client {
	return &Client{
		BaseURL: baseURL,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
	}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream"`
	Think       bool      `json:"think"`
	Options     *options  `json:"options,omitempty"`
}

type options struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Chat returns the assistant reply. Transient errors (429, 502, 503, 504, connection) are retried with exponential backoff.
func (c *Client) Chat(ctx context.Context, messages []Message, temperature float64) (string, error) {
	return c.ChatWithMaxTokens(ctx, messages, temperature, 0)
}

// ChatWithMaxTokens is like Chat but limits output tokens. If maxTokens <= 0, 1024 is used.
// Use a lower value (e.g. 512) for short structured outputs to reduce latency.
func (c *Client) ChatWithMaxTokens(ctx context.Context, messages []Message, temperature float64, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	body := chatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: temperature,
		Stream:      false,
		Options:     &options{NumPredict: maxTokens},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	var result string
	op := func() error {
		var opErr error
		result, opErr = c.doChat(ctx, jsonBody)
		if opErr == nil {
			return nil
		}
		if !isRetryable(opErr) {
			return backoff.Permanent(opErr)
		}
		return opErr
	}
	bo := backoff.WithContext(backoff.NewExponentialBackOff(), ctx)
	if err := backoff.Retry(op, bo); err != nil {
		return "", err
	}
	return result, nil
}

func (c *Client) doChat(ctx context.Context, jsonBody []byte) (string, error) {
	u, err := joinPath(c.BaseURL, "v1", "chat", "completions")
	if err != nil {
		return "", fmt.Errorf("llm: invalid base URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	start := time.Now()
	logger := loggerFromContext(ctx)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "llm chat request failed",
				"action", "chat",
				"model", c.Model,
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err)
		}
		return "", fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "llm chat response read failed",
				"action", "chat",
				"model", c.Model,
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err)
		}
		return "", fmt.Errorf("llm: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if logger != nil {
			logger.WarnContext(ctx, "llm chat non-200 response",
				"action", "chat",
				"model", c.Model,
				"status", resp.StatusCode,
				"duration_ms", time.Since(start).Milliseconds())
		}
		return "", &httpError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("llm: unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		if logger != nil {
			logger.WarnContext(ctx, "llm chat API error",
				"action", "chat",
				"model", c.Model,
				"duration_ms", time.Since(start).Milliseconds(),
				"error", chatResp.Error.Message)
		}
		return "", fmt.Errorf("llm: API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		if logger != nil {
			logger.WarnContext(ctx, "llm chat empty response",
				"action", "chat",
				"model", c.Model,
				"duration_ms", time.Since(start).Milliseconds())
		}
		return "", fmt.Errorf("llm: empty response (no choices)")
	}

	if logger != nil {
		duration := time.Since(start)
		if duration > 30*time.Second {
			logger.InfoContext(ctx, "llm chat slow",
				"action", "chat",
				"model", c.Model,
				"duration_ms", duration.Milliseconds())
		} else {
			logger.DebugContext(ctx, "llm chat completed",
				"action", "chat",
				"model", c.Model,
				"duration_ms", duration.Milliseconds())
		}
	}

	return chatResp.Choices[0].Message.Content, nil
}

type httpError struct {
	StatusCode int
	Body       string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("llm: HTTP %d: %s", e.StatusCode, e.Body)
}

func isRetryable(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	return strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "request failed")
}

func joinPath(baseURL string, elem ...string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	u2 := u.JoinPath(elem...)
	return u2.String(), nil
}

func (c *Client) Available(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	u, err := joinPath(c.BaseURL, "v1", "models")
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

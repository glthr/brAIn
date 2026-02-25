package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type EmbedFunc func(ctx context.Context, text string) ([]float32, error)

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body := embedRequest{
		Model:  c.Model,
		Prompt: text,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("embed: marshal request: %w", err)
	}

	u, err := joinPath(c.BaseURL, "api", "embeddings")
	if err != nil {
		return nil, fmt.Errorf("embed: invalid base URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embed: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: HTTP %d: %s", resp.StatusCode, respBody)
	}

	var embedResp embedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("embed: unmarshal response: %w", err)
	}
	if embedResp.Error != "" {
		return nil, fmt.Errorf("embed: API error: %s", embedResp.Error)
	}
	if len(embedResp.Embedding) == 0 {
		return nil, fmt.Errorf("embed: empty embedding returned")
	}

	return embedResp.Embedding, nil
}

func NewEmbedFunc(c *Client) EmbedFunc {
	return c.Embed
}

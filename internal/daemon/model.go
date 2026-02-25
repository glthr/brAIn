package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// IsOllamaURL returns true if baseURL is an Ollama API endpoint (default port 11434).
// Used to decide whether to ensure required models are available at startup.
func IsOllamaURL(baseURL string) bool {
	return strings.Contains(baseURL, ":11434")
}

// IsLocalOllamaURL returns true only for localhost/127.0.0.1 on port 11434.
// StartOllamaIfNeeded should only be called when this is true (starting "ollama serve" locally).
func IsLocalOllamaURL(baseURL string) bool {
	return strings.Contains(baseURL, "localhost:11434") ||
		strings.Contains(baseURL, "127.0.0.1:11434")
}

// EnsureModelAvailable downloads the model from Ollama if missing. Connection errors are ignored (LLM call will fail with a clear error).
func EnsureModelAvailable(ctx context.Context, baseURL, model string) error {
	log := brainLogger()

	exists, err := modelExists(ctx, baseURL, model)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	log.Info("model not found, downloading", "model", model, "url", baseURL)
	return pullModel(ctx, baseURL, model, log)
}

func brainLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).With("action", "model_check")
}

func urlJoinPath(baseURL string, elem ...string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	return u.JoinPath(elem...).String(), nil
}

// modelExists returns (true,nil) if model is listed, (false,nil) if reachable but absent, (false,err) on connection failure.
func modelExists(ctx context.Context, baseURL, model string) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	u, err := urlJoinPath(baseURL, "api", "tags")
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("cannot reach Ollama at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	for _, m := range result.Models {
		if m.Name == model || strings.HasPrefix(m.Name, model+":") {
			return true, nil
		}
	}
	return false, nil
}

// pullModel streams model from Ollama /api/pull; returns nil only after Ollama sends final success.
func pullModel(ctx context.Context, baseURL, model string, log *slog.Logger) error {
	body := map[string]string{"name": model}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	pullCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	u, err := urlJoinPath(baseURL, "api", "pull")
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	req, err := http.NewRequestWithContext(pullCtx, http.MethodPost, u, strings.NewReader(string(jsonBody)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		log.Error("model download failed", "model", model, "error", err)
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error("model download failed", "model", model, "status", resp.StatusCode, "body", string(bodyBytes))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Ollama only registers the model after final {"status":"success"}; don't exit on blob completion alone.
	decoder := json.NewDecoder(resp.Body)
	lastProgress := time.Now()
	var lastPercent float64 = -1
	var lastCompleted int64 = -1
	lastStatus := ""
	for {
		select {
		case <-pullCtx.Done():
			log.Error("model download cancelled", "model", model, "error", pullCtx.Err())
			return fmt.Errorf("download cancelled: %w", pullCtx.Err())
		default:
		}

		var update struct {
			Status    string `json:"status"`
			Error     string `json:"error,omitempty"`
			Completed int64  `json:"completed,omitempty"`
			Total     int64  `json:"total,omitempty"`
		}
		if err := decoder.Decode(&update); err != nil {
			if err == io.EOF {
				return fmt.Errorf("pull stream ended without success status")
			}
			log.Error("model download decode error", "model", model, "error", err)
			return fmt.Errorf("decode response: %w", err)
		}

		if update.Error != "" {
			log.Error("model download error", "model", model, "error", update.Error)
			return fmt.Errorf("pull error: %s", update.Error)
		}

		if update.Total > 0 {
			now := time.Now()
			percent := float64(update.Completed) / float64(update.Total) * 100
			shouldLog := false
			if update.Completed >= update.Total {
				shouldLog = percent != lastPercent || update.Completed != lastCompleted
			} else if percent-lastPercent >= 1.0 {
				shouldLog = true
			} else if now.Sub(lastProgress) >= 10*time.Second && update.Completed != lastCompleted {
				shouldLog = true
			}
			if shouldLog {
				log.Info("model download progress", "model", model, "completed", update.Completed, "total", update.Total, "percent", fmt.Sprintf("%.1f%%", percent))
				lastProgress = now
				lastPercent = percent
				lastCompleted = update.Completed
			}
		} else if update.Status != "" {
			if update.Status != lastStatus {
				log.Info("model download status", "model", model, "status", update.Status)
				lastStatus = update.Status
			}
		}

		if update.Status == "success" {
			log.Info("model download completed", "model", model)
			return nil
		}
	}
}

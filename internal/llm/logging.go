package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

type loggerKey struct{}

// WithLogger attaches a logger to the context for LLM helpers to use.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey{}, logger)
}

func loggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return nil
}

const rawLLMLogMax = 2000

var (
	emailPattern   = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phonePattern   = regexp.MustCompile(`\b\+?\d[\d\s().-]{7,}\d\b`)
	secretPattern  = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|passwd)[\"\\s:=]+[A-Za-z0-9_\-]{8,}`)
	bearerPattern  = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9_\-\.=]{12,}`)
	longTokenRegex = regexp.MustCompile(`[A-Za-z0-9_\-]{32,}`)
)

// allowRawLLMLogs reports whether to log raw LLM request/response bodies (sanitized).
// Set BRAIN_LOG_RAW_LLM=1 (or true/yes/on) to enable; useful for debugging with go test -v.
func allowRawLLMLogs() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BRAIN_LOG_RAW_LLM"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func sanitizeLLMText(text string) string {
	if text == "" {
		return text
	}
	out := emailPattern.ReplaceAllString(text, "[redacted-email]")
	out = phonePattern.ReplaceAllString(out, "[redacted-phone]")
	out = secretPattern.ReplaceAllString(out, "[redacted-secret]")
	out = bearerPattern.ReplaceAllString(out, "${1}[redacted-token]")
	out = longTokenRegex.ReplaceAllString(out, "[redacted-token]")
	return out
}

func sanitizeRawLLM(raw string) (string, string, bool) {
	hash := sha256.Sum256([]byte(raw))
	sanitized := sanitizeLLMText(raw)
	truncated := false
	if len(sanitized) > rawLLMLogMax {
		sanitized = sanitized[:rawLLMLogMax] + "..."
		truncated = true
	}
	return sanitized, hex.EncodeToString(hash[:]), truncated
}

func sanitizeAndTruncate(text string, max int) (string, bool) {
	sanitized := sanitizeLLMText(text)
	if len(sanitized) <= max {
		return sanitized, false
	}
	return sanitized[:max] + "...", true
}

func logRawLLMResponse(ctx context.Context, logger *slog.Logger, action, raw string) {
	if logger == nil || !allowRawLLMLogs() {
		return
	}
	sanitized, sha, truncated := sanitizeRawLLM(raw)
	// Log at Info when BRAIN_LOG_RAW_LLM is set so it appears with go test -v and typical handlers.
	logger.InfoContext(ctx, "raw LLM response",
		"action", action,
		"raw_response", sanitized,
		"raw_response_length", len(raw),
		"raw_response_sha256", sha,
		"raw_response_truncated", truncated)
}

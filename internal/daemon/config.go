// Package daemon contains configuration and Ollama model management used by the
// brain-daemon background process. CLI commands should not import the LLM or
// Ollama concepts defined here -- those belong to the daemon only.
package daemon

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultOllamaURL = "http://localhost:11434"

// Config holds settings loaded from ~/.brain/config.
type Config struct {
	ConsolidationInterval time.Duration
	ProfileInterval       time.Duration
	LLMUrl                string
	LLMModel              string
	LLMApiKey             string
	LogLevel              string
	// EmbedModel is the Ollama model used for embedding-based recall (e.g. "nomic-embed-text").
	// Required for keyword search. Uses the same LLMUrl.
	EmbedModel string
	// TestLLMModel is the Ollama model used by integration tests. The daemon ensures it is available at startup so tests can run without extra pulls. Optional; defaults to DefaultTestOllamaModel or BRAIN_TEST_OLLAMA_MODEL.
	TestLLMModel string
	// SourcePath is the brAIn repo path used for auto-update (go build from here).
	SourcePath string
	// AutoUpdateOnStart, when true and SourcePath is set, build and replace the daemon binary on start, then exit so the service restarts with the new binary.
	AutoUpdateOnStart bool
}

// DefaultConfig returns a Config populated with built-in defaults.
func DefaultConfig() *Config {
	return &Config{
		ConsolidationInterval: 10 * time.Minute,
		ProfileInterval:       30 * time.Minute,
		LLMUrl:                defaultOllamaURL,
		LLMModel:              DefaultLLMModel,
		EmbedModel:            DefaultEmbedModel,
		LogLevel:              "info",
	}
}

// ConfigFilePath returns the path to ~/.brain/config.
func ConfigFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".brain", "config")
}

// LoadConfig reads ~/.brain/config and returns the parsed Config.
// A missing file is not an error -- defaults are returned instead.
// Environment variables (BRAIN_LLM_*) override config file values.
func LoadConfig() *Config {
	cfg := DefaultConfig()

	path := ConfigFilePath()
	if path == "" {
		applyEnvOverrides(cfg)
		return cfg
	}

	f, err := os.Open(path)
	if err != nil {
		applyEnvOverrides(cfg)
		return cfg
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if value == "" {
			continue
		}

		switch key {
		case "consolidation_interval":
			if d, err := time.ParseDuration(value); err == nil {
				cfg.ConsolidationInterval = d
			}
		case "profile_interval":
			if d, err := time.ParseDuration(value); err == nil {
				cfg.ProfileInterval = d
			}
		case "llm_url":
			cfg.LLMUrl = value
		case "llm_model":
			cfg.LLMModel = value
		case "llm_api_key":
			cfg.LLMApiKey = value
		case "log_level":
			cfg.LogLevel = value
		case "embed_model":
			cfg.EmbedModel = value
		case "test_llm_model":
			cfg.TestLLMModel = value
		case "source_path", "daemon_source_path":
			cfg.SourcePath = value
		case "auto_update_on_start":
			cfg.AutoUpdateOnStart = strings.EqualFold(value, "true") || value == "1"
		}
	}

	applyEnvOverrides(cfg)
	return cfg
}

// applyEnvOverrides lets BRAIN_* environment variables override config file values.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BRAIN_LLM_URL"); v != "" {
		cfg.LLMUrl = v
	}
	if v := os.Getenv("BRAIN_LLM_MODEL"); v != "" {
		cfg.LLMModel = v
	}
	if v := os.Getenv("BRAIN_LLM_API_KEY"); v != "" {
		cfg.LLMApiKey = v
	}
	if v := os.Getenv("BRAIN_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("BRAIN_EMBED_MODEL"); v != "" {
		cfg.EmbedModel = v
	}
	if v := os.Getenv("BRAIN_TEST_OLLAMA_MODEL"); v != "" {
		cfg.TestLLMModel = v
	}
	if v := os.Getenv("BRAIN_SOURCE_PATH"); v != "" {
		cfg.SourcePath = v
	}
	if v := os.Getenv("BRAIN_AUTO_UPDATE_ON_START"); v != "" {
		cfg.AutoUpdateOnStart = strings.EqualFold(v, "true") || v == "1"
	}
}

// DefaultConfigTemplate is the content written to a new ~/.brain/config file.
const DefaultConfigTemplate = `# brAIn configuration
# Location: ~/.brain/config
#
# Duration format: 5m, 1h, 30s, etc.
# Lines starting with # are comments.

# How often the daemon runs memory consolidation (decay, prune, summarize).
consolidation_interval = 10m

# How often the daemon checks for users needing LLM-powered trait analysis.
profile_interval = 30m

# LLM connection for consolidation, trait inference, and conversation ingest.
# Works with Ollama.
# Environment variables BRAIN_LLM_URL, BRAIN_LLM_MODEL, BRAIN_LLM_API_KEY
# override these values.
llm_url = http://localhost:11434
llm_model = qwen3:4b
# llm_api_key =

# Log verbosity for the daemon: debug, info, warn, error.
# Environment variable: BRAIN_LOG_LEVEL
log_level = info

# Embedding model for semantic vector search (Ollama only).
# Enables semantic gap coverage: "recall concurrency" can match a memory tagged "goroutine", etc.
# Alternative: mxbai-embed-large (higher quality, slower).
# Install with: ollama pull nomic-embed-text
# Environment variable: BRAIN_EMBED_MODEL
embed_model = nomic-embed-text

# Optional: model used by integration tests. Daemon ensures it is available at startup.
# Environment variable: BRAIN_TEST_OLLAMA_MODEL
test_llm_model = qwen2.5:1.5b

# Optional: auto-update daemon from source. When set, the daemon can build and replace itself.
# source_path = /path/to/brAIn
# auto_update_on_start = false
`

// ParseLogLevel converts a config string into a slog.Level.
func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

// WriteDefaultConfig creates ~/.brain/config with default values.
// Does NOT overwrite an existing file.
func WriteDefaultConfig() error {
	path := ConfigFilePath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	return os.WriteFile(path, []byte(DefaultConfigTemplate), 0o644)
}

package daemon

import (
	"os"
	"strings"
)

func defaultTestModelFromEnv() string {
	return strings.TrimSpace(os.Getenv("BRAIN_TEST_OLLAMA_MODEL"))
}

// Default Ollama model names. Used by config defaults, daemon startup, and tests.
// Override via ~/.brain/config (llm_model, embed_model, test_llm_model) or environment (BRAIN_LLM_MODEL, BRAIN_EMBED_MODEL, BRAIN_TEST_OLLAMA_MODEL).
const (
	DefaultLLMModel        = "qwen3:4b"
	DefaultEmbedModel      = "nomic-embed-text"
	DefaultTestOllamaModel = "qwen2.5:1.5b"
)

// RequiredOllamaModels returns the list of Ollama model names the daemon should ensure are available at startup (LLM, embed, and test model). No duplicates, no empty strings. Order: LLM first, then embed, then test.
func RequiredOllamaModels(cfg *Config) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(cfg.LLMModel)
	add(cfg.EmbedModel)
	testModel := cfg.TestLLMModel
	if testModel == "" {
		testModel = defaultTestModelFromEnv()
		if testModel == "" {
			testModel = DefaultTestOllamaModel
		}
	}
	add(testModel)
	return out
}

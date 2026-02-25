package brain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glthr/brAIn/internal/daemon"
)

func newTestBrain(t *testing.T, opts ...Option) *Brain {
	t.Helper()
	// LLM is mandatory, so always include a test LLM configuration.
	// Tests can override with mock functions via WithBatchEncoderFunc.
	ollamaURL := os.Getenv("BRAIN_TEST_OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	ollamaModel := os.Getenv("BRAIN_TEST_OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = daemon.DefaultTestOllamaModel
	}
	testOpts := make([]Option, 0, 1+len(opts))
	testOpts = append(testOpts, WithOllama(ollamaURL, ollamaModel))
	testOpts = append(testOpts, opts...)

	tmpDir := t.TempDir()
	brainPath := filepath.Join(tmpDir, "test.brain")
	b, err := New(brainPath, testOpts...)
	if err != nil {
		t.Fatalf("failed to create brain: %v", err)
	}
	t.Cleanup(func() {
		_ = b.Close()
		_ = os.Remove(brainPath)
		_ = os.Remove(brainPath + "-shm")
		_ = os.Remove(brainPath + "-wal")
	})
	return b
}

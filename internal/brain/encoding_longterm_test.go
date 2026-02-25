//go:build integration

package brain

import (
	"context"
	"os"
	"testing"
	"time"

	"log/slog"

	"github.com/glthr/brAIn/internal/daemon"
	"github.com/glthr/brAIn/internal/llm"
	"github.com/glthr/brAIn/internal/model"
)

// loggingBatchEncoderFunc returns a BatchEncoderFunc that logs the encoded input
// turns and the resulting long-term encoding (episode, facts, tags, outcome, valence)
// per turn via t.Logf so they appear when running tests with -v for manual review.
func loggingBatchEncoderFunc(t *testing.T) BatchEncoderFunc {
	url := os.Getenv("BRAIN_TEST_OLLAMA_URL")
	if url == "" {
		url = "http://localhost:11434"
	}
	modelName := os.Getenv("BRAIN_TEST_OLLAMA_MODEL")
	if modelName == "" {
		modelName = daemon.DefaultTestOllamaModel
	}
	client := NewLLMClient(url, modelName)
	realBatch := llm.NewBatchEncoderFunc(client)
	return func(ctx context.Context, turns []EncodingTurn) ([]EncodingResult, error) {
		t.Logf("=== Long-term encoding input (batch, %d turns) ===", len(turns))
		for i, turn := range turns {
			t.Logf("--- Turn %d (user=%s project=%s agent=%s) ---\nUser prompt:\n%s\n\nAgent response:\n%s\n--- End turn %d ---",
				i+1, turn.UserID, turn.Project, turn.Agent, turn.UserPrompt, turn.AgentResponse, i+1)
		}
		t.Logf("=== End batch input ===")
		results, err := realBatch(ctx, turns)
		if err != nil {
			return nil, err
		}
		t.Logf("=== Long-term encoding output (%d results) ===", len(results))
		for i, res := range results {
			t.Logf("--- Result %d ---\nEpisode: %s\nFacts: %v\nTags: %v\nOutcome: %s\nValence: %g\n--- End result %d ---",
				i+1, res.Episode, res.Facts, res.Tags, res.Outcome, res.Valence, i+1)
		}
		t.Logf("=== End batch output ===")
		return results, nil
	}
}

// TestShortTermToLongTerm uses the test harness to create a .brain, encodes several
// short-term (working) memories, calls ProcessEncodings once (one LLM call for all
// turns via batch encoder), then asserts long-term memories exist in the DB.
//
// Granular timing (with -v and RUN_INTEGRATION_DEBUG=1): newTestBrain, Encode, and
// ProcessEncodings up to "batch encode request" are all sub-second; the entire
// remaining time is the single Ollama Chat request (batch encode). If the test
// times out, Ollama is taking too long for that request (try fewer turns or a
// smaller/faster model).
//
// Run with: go test -tags integration -run TestShortTermToLongTerm -timeout 11m -v ./internal/brain/...
//
// To see LLM request/response in test output: set BRAIN_LOG_RAW_LLM=1 (and optionally RUN_INTEGRATION_DEBUG=1).
func TestShortTermToLongTerm(t *testing.T) {
	ctx := context.Background()
	opts := []Option{
		WithWorkingMemoryTTL(1 * time.Hour),
		WithBatchEncoderFunc(loggingBatchEncoderFunc(t)),
	}
	// Attach stderr logger when debugging so LLM request/response (and with BRAIN_LOG_RAW_LLM=1, raw body) appear in go test -v output.
	if os.Getenv("RUN_INTEGRATION_DEBUG") == "1" || os.Getenv("BRAIN_LOG_RAW_LLM") != "" {
		opts = append(opts, WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))))
	}
	t0 := time.Now()
	b := newTestBrain(t, opts...)
	t.Logf("[timing] newTestBrain: %v", time.Since(t0))

	// Register user so ProcessEncodings can record interactions.
	t0 = time.Now()
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID:   "user-llm",
		ContactName: "LLMTestUser",
		EntityType:  model.EntityTypeHuman,
		Type:        "greeting",
		Outcome:     "success",
		Valence:     0.7,
	})
	t.Logf("[timing] RecordInteraction: %v", time.Since(t0))

	// Encode several conversation turns (short-term working memory). ProcessEncodings will run the LLM once for all of them.
	t0 = time.Now()
	// Keep the batch small so lightweight test models reliably return a full JSON array.
	turns := []struct{ prompt, response string }{
		{"How do I run tests in Go?", "Use go test ./... or go test -v for verbose output."},
		{"What about code coverage?", "Run go test -cover ./... and optionally -coverprofile=coverage.out."},
		{"How do I fix a flaky test?", "Use deterministic inputs, mock the clock, or retry with backoff."},
		{"What's the best way to structure test files?", "Place _test.go next to the code; use table-driven tests for many cases."},
	}
	for _, turn := range turns {
		_, err := b.Encode(ctx, "user-llm", "LLMTestUser", "cursor", "/project", turn.prompt, turn.response)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	t.Logf("[timing] Encode %d turns: %v", len(turns), time.Since(t0))

	// Verify short-term memories are present.
	t0 = time.Now()
	wc, err := b.Working.Count(ctx)
	if err != nil {
		t.Fatalf("Working.Count: %v", err)
	}
	if wc != len(turns) {
		t.Fatalf("expected %d working memories before process, got %d", len(turns), wc)
	}
	t.Logf("[timing] Working.Count + assert: %v", time.Since(t0))

	// Run LLM to transform short-term into long-term (up to 10 minutes).
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	t0 = time.Now()
	stats, err := b.ProcessEncodings(runCtx)
	if err != nil {
		t.Fatalf("ProcessEncodings: %v", err)
	}
	if stats == nil {
		t.Fatal("ProcessEncodings returned nil stats")
	}
	t.Logf("[timing] ProcessEncodings (LLM batch + apply): %v", time.Since(t0))
	if stats.Processed != len(turns) {
		t.Errorf("expected %d processed, got %d (errors=%d)", len(turns), stats.Processed, stats.Errors)
	}

	// Short-term (working) should be cleared after processing.
	wc, _ = b.Working.Count(ctx)
	if wc != 0 {
		t.Errorf("expected 0 working memories after process, got %d", wc)
	}

	// Long-term: LLM may merge similar turns, so we only require at least one episodic memory.
	epCount, err := b.Episodic.Count(ctx)
	if err != nil {
		t.Fatalf("Episodic.Count: %v", err)
	}
	if epCount < 1 {
		t.Errorf("expected at least 1 episodic (long-term) memory (merge allowed), got %d", epCount)
	}
	episodes, err := b.Episodic.Recent(ctx, 20)
	if err != nil {
		t.Fatalf("Episodic.Recent: %v", err)
	}
	if len(episodes) < 1 {
		t.Fatalf("expected at least 1 episode from Episodic.Recent, got %d", len(episodes))
	}
	for i, ep := range episodes {
		if ep.Content == "" {
			t.Errorf("episode %d has empty content", i)
		}
		if ep.Type != model.MemoryTypeEpisodic {
			t.Errorf("episode %d type=%q, want episodic", i, ep.Type)
		}
	}

	var longTermCount int
	err = b.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memories WHERE memory_type IN ('episodic','semantic')`).Scan(&longTermCount)
	if err != nil {
		t.Fatalf("query long-term count: %v", err)
	}
	if longTermCount < 1 {
		t.Errorf("expected at least 1 long-term row (episodic or semantic), got %d", longTermCount)
	}
	if stats.Episodes < 1 {
		t.Errorf("expected at least 1 episode in stats, got %d", stats.Episodes)
	}
	_ = stats.Facts
}

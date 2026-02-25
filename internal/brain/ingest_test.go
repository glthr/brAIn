package brain

import (
	"context"
	"testing"
	"time"

	"github.com/glthr/brAIn/internal/model"
)

// mockBatchEncoderFunc returns a BatchEncoderFunc that produces one result per turn with the given episode/facts/outcome/valence.
func mockBatchEncoderFunc(episode string, facts []string, outcome string, valence float64) BatchEncoderFunc {
	return func(_ context.Context, turns []EncodingTurn) ([]EncodingResult, error) {
		results := make([]EncodingResult, len(turns))
		for i := range turns {
			results[i] = EncodingResult{
				Episode: episode,
				Facts:   facts,
				Tags:    []string{"test"},
				Outcome: outcome,
				Valence: valence,
			}
		}
		return results, nil
	}
}

func TestProcessEncodingsBasic(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t,
		WithWorkingMemoryTTL(1*time.Hour),
		WithBatchEncoderFunc(mockBatchEncoderFunc(
			"User asked about Go testing",
			[]string{"User prefers table-driven tests"},
			"success",
			0.8,
		)),
	)

	// Register a human user so interaction recording works.
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID:   "user-alice",
		ContactName: "Alice",
		EntityType:  model.EntityTypeHuman,
		Type:        "greeting",
		Outcome:     "success",
		Valence:     0.7,
	})

	// Encode a conversation turn (stores as working memory).
	_, err := b.Encode(ctx, "user-alice", "Alice", "test-agent", "", "How do I test in Go?", "Use table-driven tests...")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Verify the raw working memory exists.
	wc, _ := b.Working.Count(ctx)
	if wc != 1 {
		t.Fatalf("expected 1 working memory, got %d", wc)
	}

	// Process the encodings.
	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		t.Fatalf("process encodings: %v", err)
	}
	if stats.Processed != 1 {
		t.Errorf("expected 1 processed, got %d", stats.Processed)
	}
	if stats.Episodes != 1 {
		t.Errorf("expected 1 episode, got %d", stats.Episodes)
	}
	if stats.Facts != 1 {
		t.Errorf("expected 1 fact, got %d", stats.Facts)
	}
	if stats.Interactions != 1 {
		t.Errorf("expected 1 interaction, got %d", stats.Interactions)
	}
	if stats.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", stats.Errors)
	}

	// Raw working memory should be deleted.
	wc, _ = b.Working.Count(ctx)
	if wc != 0 {
		t.Errorf("expected 0 working memories after processing, got %d", wc)
	}

	// Episode should exist.
	episodes, _ := b.Episodic.Recent(ctx, 10)
	if len(episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(episodes))
	}
	if episodes[0].Content != "User asked about Go testing" {
		t.Errorf("episode content: %q", episodes[0].Content)
	}
	if episodes[0].UserID != "user-alice" {
		t.Errorf("episode user_id: %q", episodes[0].UserID)
	}
	if episodes[0].Agent != "test-agent" {
		t.Errorf("episode agent: got %q, want %q", episodes[0].Agent, "test-agent")
	}

	// Fact should exist.
	facts, _ := b.Semantic.All(ctx)
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Content != "User prefers table-driven tests" {
		t.Errorf("fact content: %q", facts[0].Content)
	}
	if facts[0].Agent != "test-agent" {
		t.Errorf("fact agent: got %q, want %q", facts[0].Agent, "test-agent")
	}
}

func TestProcessEncodingsNoPending(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t,
		WithBatchEncoderFunc(mockBatchEncoderFunc("ep", nil, "success", 0.5)),
	)

	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		t.Fatalf("process encodings: %v", err)
	}
	if stats.Processed != 0 {
		t.Errorf("expected 0 processed, got %d", stats.Processed)
	}
}

func TestProcessEncodingsNoFunc(t *testing.T) {
	ctx := context.Background()
	// Explicitly set batch encoder to nil to test the no-func path.
	b := newTestBrain(t, WithBatchEncoderFunc(nil))

	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		t.Fatalf("process encodings: %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil stats when no batch encoder, got %+v", stats)
	}
}

func TestProcessEncodingsMultiple(t *testing.T) {
	ctx := context.Background()
	var batchCallCount int
	b := newTestBrain(t,
		WithWorkingMemoryTTL(1*time.Hour),
		WithBatchEncoderFunc(func(_ context.Context, turns []EncodingTurn) ([]EncodingResult, error) {
			batchCallCount++
			results := make([]EncodingResult, len(turns))
			for i, turn := range turns {
				results[i] = EncodingResult{
					Episode: "episode " + turn.UserPrompt,
					Outcome: "success",
					Valence: 0.7,
				}
			}
			return results, nil
		}),
	)

	b.Encode(ctx, "user-bob", "Bob", "test-agent", "", "question 1", "answer 1")
	b.Encode(ctx, "user-bob", "Bob", "test-agent", "", "question 2", "answer 2")
	b.Encode(ctx, "user-bob", "Bob", "test-agent", "", "question 3", "answer 3")

	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		t.Fatalf("process encodings: %v", err)
	}
	if stats.Processed != 3 {
		t.Errorf("expected 3 processed, got %d", stats.Processed)
	}
	if batchCallCount != 1 {
		t.Errorf("expected batch encoder called 1 time, got %d", batchCallCount)
	}

	wc, _ := b.Working.Count(ctx)
	if wc != 0 {
		t.Errorf("expected 0 working memories after processing, got %d", wc)
	}
}

func TestProcessEncodingsSkipsNonConversation(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t,
		WithWorkingMemoryTTL(1*time.Hour),
		WithBatchEncoderFunc(mockBatchEncoderFunc("ep", nil, "success", 0.5)),
	)

	// Store a regular working memory (not a conversation).
	b.Working.Store(ctx, "just a note", nil)

	// Store a conversation via Encode.
	b.Encode(ctx, "user-x", "", "test-agent", "", "hello", "hi there")

	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		t.Fatalf("process encodings: %v", err)
	}
	if stats.Processed != 1 {
		t.Errorf("expected 1 processed (conversation only), got %d", stats.Processed)
	}

	// The non-conversation working memory should still exist.
	wc, _ := b.Working.Count(ctx)
	if wc != 1 {
		t.Errorf("expected 1 remaining working memory (the note), got %d", wc)
	}
}

func TestParseConversationContent(t *testing.T) {
	content := "## User prompt\nHow do I test?\n\n## Agent response\nUse table-driven tests."
	prompt, response := parseConversationContent(content)

	if prompt != "How do I test?" {
		t.Errorf("prompt: %q", prompt)
	}
	if response != "Use table-driven tests." {
		t.Errorf("response: %q", response)
	}
}

package brain

import (
	"context"
	"math"
	"testing"
	"time"
)

// --- Working Memory tests ---

func TestWorkingMemory(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Hour))

	id, err := b.Working.Store(ctx, "current task: write tests", nil)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	items, err := b.Working.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Content != "current task: write tests" {
		t.Errorf("content mismatch: %q", items[0].Content)
	}

	count, err := b.Working.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	m, err := b.Working.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.Retrievals != 1 {
		t.Errorf("expected retrievals 1 after Get, got %d", m.Retrievals)
	}

	err = b.Working.Clear(ctx)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	count, _ = b.Working.Count(ctx)
	if count != 0 {
		t.Errorf("expected 0 after clear, got %d", count)
	}
}

func TestWorkingMemoryExpiry(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Millisecond))

	id, _ := b.Working.Store(ctx, "ephemeral", nil)
	time.Sleep(10 * time.Millisecond)

	_, err := b.Working.Get(ctx, id)
	if err != ErrExpired {
		t.Errorf("expected ErrExpired, got %v", err)
	}
}

// --- Episodic Memory tests ---

func TestEpisodicMemory(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	id, err := b.Episodic.Record(ctx, Episode{
		Content: "helped user debug a deadlock",
		Tags:    []string{"debugging", "go"},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	m, err := b.Episodic.Recall(ctx, id)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if m.Content != "helped user debug a deadlock" {
		t.Errorf("content: %q", m.Content)
	}

	items, err := b.Episodic.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1, got %d", len(items))
	}

	results, err := b.Episodic.Search(ctx, EpisodicSearchOpts{
		Tags:  []string{"go"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("tag search: expected 1, got %d", len(results))
	}
}

// --- Semantic Memory tests ---

func TestSemanticMemory(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	_, err := b.Semantic.Store(ctx, Fact{
		Content: "Go uses goroutines for concurrency",
		Tags:    []string{"go", "concurrency"},
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_, err = b.Semantic.Store(ctx, Fact{
		Content: "Rust uses ownership for memory safety",
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	// Lookup requires EmbedFunc; test only Store and Count without embeddings
	count, err := b.Semantic.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

// --- Procedural Memory tests ---

func TestProceduralMemory(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	_, err := b.Procedural.Store(ctx, Procedure{
		Name:        "code_review",
		Description: "Review code changes",
		Steps:       []string{"read diff", "check style", "verify tests"},
		SuccessRate: 0.8,
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	proc, err := b.Procedural.Lookup(ctx, "code_review")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if proc.Name != "code_review" {
		t.Errorf("name: %q", proc.Name)
	}
	if len(proc.Steps) != 3 {
		t.Errorf("steps: %d", len(proc.Steps))
	}

	err = b.Procedural.UpdateSuccessRate(ctx, "code_review", true)
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	proc, _ = b.Procedural.Lookup(ctx, "code_review")
	if proc.UseCount != 1 {
		t.Errorf("use_count: %d", proc.UseCount)
	}
	if math.Abs(proc.SuccessRate-0.84) > 0.01 {
		t.Errorf("success_rate: %f, want ~0.84", proc.SuccessRate)
	}
}

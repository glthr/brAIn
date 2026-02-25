package brain

import (
	"context"
	"testing"
	"time"
)

func TestSemanticMemoryAll(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Store multiple facts
	b.Semantic.Store(ctx, Fact{Content: "fact 1", Tags: []string{"a"}})
	b.Semantic.Store(ctx, Fact{Content: "fact 2", Tags: []string{"b"}})
	b.Semantic.StoreWithImportance(ctx, Fact{Content: "important fact"}, 0.9)

	all, err := b.Semantic.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 facts, got %d", len(all))
	}

	// Should be ordered by salience (descending)
	if len(all) >= 2 && all[0].Salience < all[1].Salience {
		t.Error("facts should be ordered by salience descending")
	}
}

func TestSemanticMemoryCount(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	count, err := b.Semantic.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 initially, got %d", count)
	}

	b.Semantic.Store(ctx, Fact{Content: "fact 1"})
	b.Semantic.Store(ctx, Fact{Content: "fact 2"})

	count, err = b.Semantic.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestEpisodicMemoryCount(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	count, err := b.Episodic.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 initially, got %d", count)
	}

	b.Episodic.Record(ctx, Episode{Content: "episode 1"})
	b.Episodic.Record(ctx, Episode{Content: "episode 2"})

	count, err = b.Episodic.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestProceduralMemoryAll(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Procedural.Store(ctx, Procedure{
		Name:        "proc1",
		Description: "procedure 1",
		Steps:       []string{"step1", "step2"},
		SuccessRate: 0.8,
	})
	b.Procedural.Store(ctx, Procedure{
		Name:        "proc2",
		Description: "procedure 2",
		Steps:       []string{"step1"},
		SuccessRate: 0.9,
	})

	all, err := b.Procedural.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 procedures, got %d", len(all))
	}

	// Verify both procedures exist
	found1, found2 := false, false
	for _, p := range all {
		if p.Name == "proc1" {
			found1 = true
		}
		if p.Name == "proc2" {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Error("both procedures should be present")
	}

	// Verify content
	found := false
	for _, p := range all {
		if p.Name == "proc1" && len(p.Steps) == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Error("proc1 not found with correct steps")
	}
}

func TestProceduralMemoryCount(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	count, err := b.Procedural.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 initially, got %d", count)
	}

	b.Procedural.Store(ctx, Procedure{Name: "proc1", Steps: []string{"step1"}})
	b.Procedural.Store(ctx, Procedure{Name: "proc2", Steps: []string{"step1"}})

	count, err = b.Procedural.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestProceduralMemoryEmerging(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Procedural.Store(ctx, Procedure{
		Name:        "explicit_proc",
		Description: "explicitly stored",
		Steps:       []string{"a", "b"},
		SuccessRate: 0.7,
	})
	_, err := b.Procedural.StoreEmerging(ctx, Procedure{
		Name:        "convert_to_pdf",
		Description: "convert file to PDF",
		Steps:       []string{"open in LibreOffice", "File → Export as PDF"},
		SuccessRate: 0.8,
	})
	if err != nil {
		t.Fatalf("StoreEmerging: %v", err)
	}

	all, err := b.Procedural.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("All: expected 2 procedures, got %d", len(all))
	}

	emerging, err := b.Procedural.Emerging(ctx)
	if err != nil {
		t.Fatalf("Emerging: %v", err)
	}
	if len(emerging) != 1 {
		t.Errorf("Emerging: expected 1 skill, got %d", len(emerging))
	}
	if len(emerging) > 0 && emerging[0].Name != "convert_to_pdf" {
		t.Errorf("Emerging: got name %q", emerging[0].Name)
	}
}

func TestWorkingMemoryStoreWithTTL(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Hour))

	// Store with custom TTL (shorter than default, but long enough for an immediate Get to succeed)
	customTTL := 500 * time.Millisecond
	id, err := b.Working.StoreWithTTL(ctx, "short-lived", nil, customTTL)
	if err != nil {
		t.Fatalf("StoreWithTTL: %v", err)
	}

	// Should be accessible immediately
	mem, err := b.Working.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get immediately: %v", err)
	}
	if mem.Content != "short-lived" {
		t.Errorf("content: got %q", mem.Content)
	}

	// Wait for expiration
	time.Sleep(600 * time.Millisecond)
	_, err = b.Working.Get(ctx, id)
	if err != ErrExpired {
		t.Errorf("expected ErrExpired after TTL, got %v", err)
	}
}

func TestSemanticMemoryDuplicateContent(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Store the same fact twice
	id1, err := b.Semantic.Store(ctx, Fact{Content: "duplicate fact"})
	if err != nil {
		t.Fatalf("Store first: %v", err)
	}

	id2, err := b.Semantic.Store(ctx, Fact{Content: "duplicate fact"})
	if err != nil {
		t.Fatalf("Store second: %v", err)
	}

	// Should return the same ID (potentiation instead of duplicate)
	if id1 != id2 {
		t.Errorf("duplicate content should return same ID: got %d and %d", id1, id2)
	}

	// Count should still be 1
	count, _ := b.Semantic.Count(ctx)
	if count != 1 {
		t.Errorf("expected 1 fact (no duplicate), got %d", count)
	}
}

func TestEpisodicMemorySearchWithTimeRange(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	// Record episodes at different times
	b.Episodic.Record(ctx, Episode{Content: "old episode"})
	// Manually set created_at to past
	b.db.ExecContext(ctx, `UPDATE memories SET created_at = ? WHERE content = 'old episode'`,
		past.Format("2006-01-02 15:04:05"))

	b.Episodic.Record(ctx, Episode{Content: "recent episode"})

	// Search for episodes after past time (should find both)
	results, err := b.Episodic.Search(ctx, EpisodicSearchOpts{
		After: past.Add(-1 * time.Hour),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 episodes, got %d", len(results))
	}

	// Search for episodes after recent time (should find only recent)
	results, err = b.Episodic.Search(ctx, EpisodicSearchOpts{
		After: now.Add(-1 * time.Hour),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 episode, got %d", len(results))
	}
	if results[0].Content != "recent episode" {
		t.Errorf("expected 'recent episode', got %q", results[0].Content)
	}

	// Search with before time
	results, err = b.Episodic.Search(ctx, EpisodicSearchOpts{
		Before: future,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 episodes, got %d", len(results))
	}
}

func TestEpisodicMemorySearchWithMinSalience(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Episodic.RecordWithImportance(ctx, Episode{Content: "low importance"}, 0.3)
	b.Episodic.RecordWithImportance(ctx, Episode{Content: "high importance"}, 0.9)

	results, err := b.Episodic.Search(ctx, EpisodicSearchOpts{
		MinSalience: 0.5,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 episode with salience >= 0.5, got %d", len(results))
	}
	if results[0].Content != "high importance" {
		t.Errorf("expected 'high importance', got %q", results[0].Content)
	}
}

func TestSemanticMemorySearchWithTags(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Semantic.Store(ctx, Fact{Content: "fact with tag a", Tags: []string{"a", "b"}})
	b.Semantic.Store(ctx, Fact{Content: "fact with tag b", Tags: []string{"b", "c"}})
	b.Semantic.Store(ctx, Fact{Content: "fact with tag c", Tags: []string{"c"}})

	// Search for tag "b" (should find 2 facts)
	results, err := b.Semantic.Search(ctx, SemanticSearchOpts{
		Tags:  []string{"b"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 facts with tag 'b', got %d", len(results))
	}

	// Search for tag "c" (should find 2 facts)
	results, err = b.Semantic.Search(ctx, SemanticSearchOpts{
		Tags:  []string{"c"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 facts with tag 'c', got %d", len(results))
	}
}

func TestSemanticMemorySearchWithMinSalience(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Semantic.StoreWithImportance(ctx, Fact{Content: "low salience"}, 0.3)
	b.Semantic.StoreWithImportance(ctx, Fact{Content: "high salience"}, 0.9)

	results, err := b.Semantic.Search(ctx, SemanticSearchOpts{
		MinSalience: 0.5,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 fact with salience >= 0.5, got %d", len(results))
	}
	if results[0].Content != "high salience" {
		t.Errorf("expected 'high salience', got %q", results[0].Content)
	}
}

func TestProceduralMemoryUpdateSuccessRateMultiple(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Procedural.Store(ctx, Procedure{
		Name:        "test_proc",
		Description: "test",
		Steps:       []string{"step1"},
		SuccessRate: 0.5,
	})

	// Update multiple times
	b.Procedural.UpdateSuccessRate(ctx, "test_proc", true)
	b.Procedural.UpdateSuccessRate(ctx, "test_proc", true)
	b.Procedural.UpdateSuccessRate(ctx, "test_proc", false)

	proc, err := b.Procedural.Lookup(ctx, "test_proc")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if proc.UseCount != 3 {
		t.Errorf("expected use_count 3, got %d", proc.UseCount)
	}
	// Success rate should be between 0.5 and 1.0 (2 successes, 1 failure)
	if proc.SuccessRate <= 0.5 || proc.SuccessRate > 1.0 {
		t.Errorf("success_rate should be > 0.5 and <= 1.0, got %f", proc.SuccessRate)
	}
}

func TestWorkingMemoryRecentRespectsLimit(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Hour))

	// Store more items than limit
	for i := 0; i < 10; i++ {
		b.Working.Store(ctx, "item", nil)
	}

	// Request only 5
	recent, err := b.Working.Recent(ctx, 5)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 5 {
		t.Errorf("expected 5 items, got %d", len(recent))
	}
}

func TestWorkingMemoryCountExcludesExpired(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Millisecond))

	b.Working.Store(ctx, "ephemeral", nil)
	time.Sleep(10 * time.Millisecond)

	count, err := b.Working.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 (expired items excluded), got %d", count)
	}
}

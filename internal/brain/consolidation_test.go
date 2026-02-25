package brain

import (
	"context"
	"testing"
	"time"
)

func TestConsolidationExpiry(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Millisecond))

	b.Working.Store(ctx, "temp1", nil)
	b.Working.Store(ctx, "temp2", nil)
	time.Sleep(10 * time.Millisecond)

	stats, err := b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if stats.Expired != 2 {
		t.Errorf("expected 2 expired, got %d", stats.Expired)
	}
}

func TestConsolidationDecay(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithDecayRate(0.5), WithSalienceFloor(0.01))

	id, _ := b.Semantic.Store(ctx, Fact{Content: "old fact"})
	b.db.ExecContext(ctx, `UPDATE memories SET last_accessed = datetime('now', '-2 days') WHERE id = ?`, id)

	stats, err := b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if stats.Decayed != 1 {
		t.Errorf("expected 1 decayed, got %d", stats.Decayed)
	}

	m, _ := b.store.GetMemory(ctx, id)
	if m.Salience >= 0.6 {
		t.Errorf("salience should have decayed, got %f", m.Salience)
	}
}

func TestConsolidationPromotion(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Hour))

	id, _ := b.Working.Store(ctx, "frequently accessed", nil)
	b.db.ExecContext(ctx, `UPDATE memories SET retrievals = 3 WHERE id = ?`, id)

	stats, err := b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if stats.Transferred != 1 {
		t.Errorf("expected 1 transferred, got %d", stats.Transferred)
	}

	m, _ := b.store.GetMemory(ctx, id)
	if m.Type != MemoryTypeEpisodic {
		t.Errorf("should be transferred to episodic, got %s", m.Type)
	}
}

func TestConsolidationPrune(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithSalienceFloor(0.3))

	b.Semantic.Store(ctx, Fact{Content: "important"})

	b.db.ExecContext(ctx, `
		INSERT INTO memories (memory_type, content, salience, created_at, last_accessed)
		VALUES ('semantic', 'forgettable', 0.05, datetime('now'), datetime('now', '-2 days'))`)

	stats, err := b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if stats.Forgotten != 1 {
		t.Errorf("expected 1 forgotten, got %d", stats.Forgotten)
	}
}

func TestConsolidationSummarize(t *testing.T) {
	ctx := context.Background()

	abstractor := func(_ context.Context, memories []Memory) (*Memory, error) {
		combined := ""
		for _, m := range memories {
			combined += m.Content + "; "
		}
		return &Memory{
			Content:  "Summary: " + combined,
			Salience: 0.8,
		}, nil
	}

	b := newTestBrain(t,
		WithSemanticizationFunc(abstractor),
		WithSemanticizationAge(1*time.Millisecond),
	)

	b.Episodic.Record(ctx, Episode{Content: "event 1"})
	b.Episodic.Record(ctx, Episode{Content: "event 2"})
	b.db.ExecContext(ctx, `UPDATE memories SET created_at = datetime('now', '-7 days') WHERE memory_type = 'episodic'`)

	time.Sleep(5 * time.Millisecond)

	stats, err := b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("consolidate: %v", err)
	}
	if stats.Semanticized != 2 {
		t.Errorf("expected 2 semanticized, got %d", stats.Semanticized)
	}

	episodic, _ := b.Episodic.Recent(ctx, 10)
	if len(episodic) != 0 {
		t.Errorf("expected 0 episodic after abstraction, got %d", len(episodic))
	}
	semantic, _ := b.Semantic.All(ctx)
	if len(semantic) != 1 {
		t.Fatalf("expected 1 semantic abstraction, got %d", len(semantic))
	}
	if semantic[0].Content[:7] != "Summary" {
		t.Errorf("unexpected content: %q", semantic[0].Content)
	}
}

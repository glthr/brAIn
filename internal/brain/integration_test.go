package brain

import (
	"context"
	"testing"
	"time"
)

// TestMemoryLifecycle tests the complete lifecycle: working -> episodic -> semantic
func TestMemoryLifecycle(t *testing.T) {
	ctx := context.Background()
	semanticizer := func(_ context.Context, memories []Memory) (*Memory, error) {
		return &Memory{
			Content:  "Summary: " + memories[0].Content,
			Salience: 0.8,
			Type:     MemoryTypeSemantic,
		}, nil
	}
	b := newTestBrain(t,
		WithWorkingMemoryTTL(1*time.Hour),
		WithSemanticizationAge(1*time.Millisecond),
		WithSemanticizationFunc(semanticizer),
	)

	// 1. Store in working memory
	workingID, err := b.Working.Store(ctx, "working memory item", nil)
	if err != nil {
		t.Fatalf("Store working: %v", err)
	}

	// Verify it's in working memory
	wc, _ := b.Working.Count(ctx)
	if wc != 1 {
		t.Errorf("expected 1 working memory, got %d", wc)
	}

	// 2. Promote to episodic (simulate frequent access)
	b.db.ExecContext(ctx, `UPDATE memories SET retrievals = 5 WHERE id = ?`, workingID)
	stats, err := b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if stats.Transferred != 1 {
		t.Errorf("expected 1 transferred, got %d", stats.Transferred)
	}

	// Verify it's now episodic
	mem, _ := b.GetMemory(ctx, workingID)
	if mem.Type != MemoryTypeEpisodic {
		t.Errorf("expected episodic type, got %q", mem.Type)
	}

	ec, _ := b.Episodic.Count(ctx)
	if ec != 1 {
		t.Errorf("expected 1 episodic memory, got %d", ec)
	}

	// 3. Semanticize (age the episodic memory)
	b.db.ExecContext(ctx, `UPDATE memories SET created_at = datetime('now', '-8 days') WHERE id = ?`, workingID)

	time.Sleep(5 * time.Millisecond)
	stats, err = b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("Consolidate semanticization: %v", err)
	}
	if stats.Semanticized != 1 {
		t.Errorf("expected 1 semanticized, got %d", stats.Semanticized)
	}

	// Verify episodic is gone and semantic exists
	ec, _ = b.Episodic.Count(ctx)
	if ec != 0 {
		t.Errorf("expected 0 episodic after semanticization, got %d", ec)
	}

	sc, _ := b.Semantic.Count(ctx)
	if sc != 1 {
		t.Errorf("expected 1 semantic memory, got %d", sc)
	}
}

// TestMultiUserMemoryIsolation tests that memories are properly isolated by user
func TestMultiUserMemoryIsolation(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Register users
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "greeting", Outcome: "success", Valence: 0.7,
	})
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-bob", ContactName: "Bob", EntityType: EntityTypeHuman,
		Type: "greeting", Outcome: "success", Valence: 0.7,
	})

	// Store memories for each user
	b.Semantic.Store(ctx, Fact{Content: "Alice's fact", UserID: "user-alice"})
	b.Semantic.Store(ctx, Fact{Content: "Bob's fact", UserID: "user-bob"})
	b.Semantic.Store(ctx, Fact{Content: "shared fact"}) // no user_id

	b.Episodic.Record(ctx, Episode{Content: "Alice's episode", UserID: "user-alice"})
	b.Episodic.Record(ctx, Episode{Content: "Bob's episode", UserID: "user-bob"})

	// Verify isolation
	aliceMems, _ := b.MemoriesForUser(ctx, "user-alice", 50)
	if len(aliceMems) != 2 {
		t.Errorf("expected 2 memories for Alice, got %d", len(aliceMems))
	}
	for _, m := range aliceMems {
		if m.UserID != "user-alice" {
			t.Errorf("memory has wrong user_id: %q", m.UserID)
		}
	}

	bobMems, _ := b.MemoriesForUser(ctx, "user-bob", 50)
	if len(bobMems) != 2 {
		t.Errorf("expected 2 memories for Bob, got %d", len(bobMems))
	}
	for _, m := range bobMems {
		if m.UserID != "user-bob" {
			t.Errorf("memory has wrong user_id: %q", m.UserID)
		}
	}
}

// TestEncodeProcessEncodeCycle tests the full encode -> process -> encode cycle
func TestEncodeProcessEncodeCycle(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t,
		WithWorkingMemoryTTL(1*time.Hour),
		WithBatchEncoderFunc(func(_ context.Context, turns []EncodingTurn) ([]EncodingResult, error) {
			results := make([]EncodingResult, len(turns))
			for i, turn := range turns {
				results[i] = EncodingResult{
					Episode: "User asked: " + turn.UserPrompt,
					Facts:   []string{"Fact: " + turn.AgentResponse},
					Tags:    []string{"test"},
					Outcome: "success",
					Valence: 0.8,
				}
			}
			return results, nil
		}),
	)

	// Register user
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-charlie", ContactName: "Charlie", EntityType: EntityTypeHuman,
		Type: "greeting", Outcome: "success", Valence: 0.7,
	})

	// Encode conversation
	b.Encode(ctx, "user-charlie", "Charlie", "cursor", "/project", "How do I test?", "Use table-driven tests")

	// Process encodings
	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		t.Fatalf("ProcessEncodings: %v", err)
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

	// Verify episode was created
	episodes, _ := b.Episodic.Recent(ctx, 10)
	if len(episodes) != 1 {
		t.Errorf("expected 1 episode, got %d", len(episodes))
	}
	if episodes[0].UserID != "user-charlie" {
		t.Errorf("episode user_id: got %q, want %q", episodes[0].UserID, "user-charlie")
	}
	if episodes[0].Agent != "cursor" {
		t.Errorf("episode agent: got %q, want %q", episodes[0].Agent, "cursor")
	}
	if episodes[0].Metadata["project"] != "/project" {
		t.Errorf("episode project metadata: got %q, want %q", episodes[0].Metadata["project"], "/project")
	}

	// Verify fact was created
	facts, _ := b.Semantic.All(ctx)
	if len(facts) != 1 {
		t.Errorf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].UserID != "user-charlie" {
		t.Errorf("fact user_id: got %q, want %q", facts[0].UserID, "user-charlie")
	}

	// Verify working memory was deleted
	wc, _ := b.Working.Count(ctx)
	if wc != 0 {
		t.Errorf("expected 0 working memories after processing, got %d", wc)
	}

	// Encode another conversation
	b.Encode(ctx, "user-charlie", "Charlie", "cursor", "/project", "How do I deploy?", "Use Docker")

	// Process again
	stats, err = b.ProcessEncodings(ctx)
	if err != nil {
		t.Fatalf("ProcessEncodings second: %v", err)
	}
	if stats.Processed != 1 {
		t.Errorf("expected 1 processed second time, got %d", stats.Processed)
	}

	// Verify both episodes exist
	episodes, _ = b.Episodic.Recent(ctx, 10)
	if len(episodes) != 2 {
		t.Errorf("expected 2 episodes after second encode, got %d", len(episodes))
	}
}

// TestConsolidationAllPhases tests all consolidation phases together
func TestConsolidationAllPhases(t *testing.T) {
	ctx := context.Background()
	semanticizer := func(_ context.Context, memories []Memory) (*Memory, error) {
		return &Memory{
			Content:  "Summary: " + memories[0].Content,
			Salience: 0.8,
			Type:     MemoryTypeSemantic,
		}, nil
	}
	b := newTestBrain(t,
		WithWorkingMemoryTTL(1*time.Millisecond),
		WithDecayRate(0.5),
		WithSalienceFloor(0.3),
		WithSemanticizationAge(1*time.Millisecond),
		WithSemanticizationFunc(semanticizer),
	)

	// Create memories for different phases
	// 1. Working memory that will expire
	b.Working.Store(ctx, "expiring", nil)

	// 2. Semantic memory that will decay (salience > floor so decay applies, then forget may remove)
	b.Semantic.StoreWithImportance(ctx, Fact{Content: "will decay"}, 0.6)
	b.db.ExecContext(ctx, `UPDATE memories SET last_accessed = datetime('now', '-2 days') WHERE content = 'will decay'`)

	// 3. Working memory that will be promoted (use long TTL so it survives the sleep)
	promoteID, _ := b.Working.StoreWithTTL(ctx, "promote me", nil, 1*time.Hour)
	b.db.ExecContext(ctx, `UPDATE memories SET retrievals = 5 WHERE id = ?`, promoteID)

	// 4. Episodic memory that will be semanticized
	episodeID, _ := b.Episodic.Record(ctx, Episode{Content: "old episode"})
	b.db.ExecContext(ctx, `UPDATE memories SET created_at = datetime('now', '-8 days') WHERE id = ?`, episodeID)

	time.Sleep(10 * time.Millisecond)

	stats, err := b.Consolidate(ctx)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	// Verify all phases ran
	if stats.Expired == 0 {
		t.Error("expected expired memories")
	}
	if stats.Decayed == 0 {
		t.Error("expected decayed memories")
	}
	if stats.Transferred == 0 {
		t.Error("expected transferred memories")
	}
	if stats.Semanticized == 0 {
		t.Error("expected semanticized memories")
	}
}

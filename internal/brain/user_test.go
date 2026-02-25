package brain

import (
	"context"
	"testing"
	"time"
)

func TestFactWithUserID(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "interaction", Outcome: "success", Valence: 0.8,
	})

	id, err := b.Semantic.Store(ctx, Fact{
		Content: "Alice prefers Go for backend work",
		Tags:    []string{"go", "preference"},
		UserID:  "user-alice",
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	_, err = b.Semantic.Store(ctx, Fact{Content: "Go 1.22 was released"})
	if err != nil {
		t.Fatalf("store global fact: %v", err)
	}

	mems, err := b.MemoriesForUser(ctx, "user-alice", 50)
	if err != nil {
		t.Fatalf("MemoriesForUser: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory for user-alice, got %d", len(mems))
	}
	if mems[0].ID != id {
		t.Errorf("expected id %d, got %d", id, mems[0].ID)
	}
	if mems[0].UserID != "user-alice" {
		t.Errorf("expected user_id user-alice, got %q", mems[0].UserID)
	}
}

func TestEpisodeWithUserID(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	id, err := b.Episodic.Record(ctx, Episode{
		Content: "Helped Alice debug a goroutine leak",
		Tags:    []string{"debugging"},
		UserID:  "user-alice",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	mems, err := b.MemoriesForUser(ctx, "user-alice", 50)
	if err != nil {
		t.Fatalf("MemoriesForUser: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1, got %d", len(mems))
	}
	if mems[0].ID != id {
		t.Errorf("expected id %d, got %d", id, mems[0].ID)
	}
	if mems[0].Type != MemoryTypeEpisodic {
		t.Errorf("expected episodic, got %s", mems[0].Type)
	}
}

func TestWorkingMemoryForUser(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t, WithWorkingMemoryTTL(1*time.Hour))

	id, err := b.Working.StoreForUser(ctx, "reviewing PR for Alice", nil, "user-alice")
	if err != nil {
		t.Fatalf("StoreForUser: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	m, err := b.Working.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.UserID != "user-alice" {
		t.Errorf("expected user_id user-alice, got %q", m.UserID)
	}
}

func TestMemoriesForUserEmpty(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	mems, err := b.MemoriesForUser(ctx, "user-nobody", 50)
	if err != nil {
		t.Fatalf("MemoriesForUser: %v", err)
	}
	if len(mems) != 0 {
		t.Errorf("expected 0 memories, got %d", len(mems))
	}
}

func TestSchemaMigrationUserID(t *testing.T) {
	b := newTestBrain(t)
	ctx := context.Background()

	id, err := b.Semantic.Store(ctx, Fact{Content: "migrated", UserID: "user-test"})
	if err != nil {
		t.Fatalf("store after migration: %v", err)
	}
	m, err := b.GetMemory(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.UserID != "user-test" {
		t.Errorf("expected user-test, got %q", m.UserID)
	}
}

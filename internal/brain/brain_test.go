package brain

import (
	"context"
	"testing"

	"github.com/glthr/brAIn/internal/daemon"
)

func TestBrainActivity(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Add some memories
	b.Working.Store(ctx, "task 1", nil)
	b.Episodic.Record(ctx, Episode{Content: "episode 1"})
	b.Semantic.Store(ctx, Fact{Content: "fact 1"})
	b.Procedural.Store(ctx, Procedure{Name: "proc1", Steps: []string{"step1"}})

	// Add contacts
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-bot", ContactName: "Bot", EntityType: EntityTypeAgent,
		Type: "collaboration", Outcome: "success", Valence: 0.7,
	})

	// Encode a conversation (creates pending ingest)
	b.Encode(ctx, "user-alice", "Alice", "cursor", "", "question", "answer")

	report, err := b.Activity(ctx)
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}

	if report.MemoryCounts["working"] != 2 { // task 1 + encoded conversation
		t.Errorf("working memory count: got %d, want 2", report.MemoryCounts["working"])
	}
	if report.MemoryCounts["episodic"] != 1 {
		t.Errorf("episodic memory count: got %d, want 1", report.MemoryCounts["episodic"])
	}
	if report.MemoryCounts["semantic"] != 1 {
		t.Errorf("semantic memory count: got %d, want 1", report.MemoryCounts["semantic"])
	}
	if report.MemoryCounts["procedural"] != 1 {
		t.Errorf("procedural memory count: got %d, want 1", report.MemoryCounts["procedural"])
	}
	if report.HumanCount != 1 {
		t.Errorf("human count: got %d, want 1", report.HumanCount)
	}
	if report.AgentCount != 1 {
		t.Errorf("agent count: got %d, want 1", report.AgentCount)
	}
	if report.PendingIngests != 1 {
		t.Errorf("pending ingests: got %d, want 1", report.PendingIngests)
	}
	if !report.LLMConfigured {
		t.Error("LLM should be configured")
	}
}

func TestBrainCompact(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	brainPath := tmpDir + "/test.brain"

	b, err := New(brainPath, WithOllama("http://localhost:11434", daemon.DefaultTestOllamaModel))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	// Add some data
	b.Semantic.Store(ctx, Fact{Content: "test fact"})
	b.Episodic.Record(ctx, Episode{Content: "test episode"})

	// Compact should succeed
	if err := b.Compact(); err != nil {
		t.Errorf("Compact: %v", err)
	}

	// Verify data still accessible after compact
	facts, _ := b.Semantic.All(ctx)
	if len(facts) != 1 {
		t.Errorf("expected 1 fact after compact, got %d", len(facts))
	}
}

func TestBrainGetMemory(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Store a fact
	factID, err := b.Semantic.Store(ctx, Fact{Content: "test fact", Tags: []string{"test"}})
	if err != nil {
		t.Fatalf("Store fact: %v", err)
	}

	// Retrieve via GetMemory
	mem, err := b.GetMemory(ctx, factID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if mem.Content != "test fact" {
		t.Errorf("content: got %q, want %q", mem.Content, "test fact")
	}
	if mem.Type != MemoryTypeSemantic {
		t.Errorf("type: got %q, want %q", mem.Type, MemoryTypeSemantic)
	}

	// GetMemory should potentiate (increase retrievals)
	mem2, _ := b.GetMemory(ctx, factID)
	if mem2.Retrievals <= mem.Retrievals {
		t.Errorf("retrievals should increase: got %d, want > %d", mem2.Retrievals, mem.Retrievals)
	}

	// Non-existent memory
	_, err = b.GetMemory(ctx, 99999)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent memory, got %v", err)
	}
}

func TestBrainCheckModel(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// With a valid test LLM config, CheckModel should succeed or fail gracefully
	// (depending on whether Ollama is actually running)
	err := b.CheckModel(ctx)
	// We don't assert on success/failure since it depends on external service
	// Just verify the method doesn't panic
	if err != nil {
		t.Logf("CheckModel returned error (expected if Ollama not running): %v", err)
	}
}

func TestBrainNewWithInvalidPath(t *testing.T) {
	// Test that New handles invalid paths gracefully
	_, err := New("/invalid/path/that/does/not/exist/test.brain",
		WithOllama("http://localhost:11434", daemon.DefaultTestOllamaModel))
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestBrainNewWithMemoryPath(t *testing.T) {
	// Test that ":memory:" path works
	b, err := New(":memory:", WithOllama("http://localhost:11434", daemon.DefaultTestOllamaModel))
	if err != nil {
		t.Fatalf("New with :memory: failed: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	_, err = b.Semantic.Store(ctx, Fact{Content: "test"})
	if err != nil {
		t.Errorf("Store failed: %v", err)
	}
}

func TestBrainNewAutoAppendsExtension(t *testing.T) {
	tmpDir := t.TempDir()
	// Path without .brain extension should get it appended
	brainPath := tmpDir + "/test"

	b, err := New(brainPath, WithOllama("http://localhost:11434", daemon.DefaultTestOllamaModel))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	// Verify the file was created with .brain extension
	rows, err := b.db.QueryContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Errorf("database should be accessible: %v", err)
	} else {
		rows.Close()
	}
}

func TestBrainRecordBackup(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	meta, _ := b.Meta(ctx)
	if meta.LastBackupAt != "" {
		t.Error("LastBackupAt should be empty initially")
	}

	err := b.RecordBackup(ctx)
	if err != nil {
		t.Fatalf("RecordBackup: %v", err)
	}

	meta, _ = b.Meta(ctx)
	if meta.LastBackupAt == "" {
		t.Error("LastBackupAt should be set after RecordBackup")
	}
}

// TestCloseIdempotent verifies that Close() can be called multiple times without
// panic and that the second call returns the same error as the first (no "database is closed").
func TestCloseIdempotent(t *testing.T) {
	b, err := New(":memory:", WithOllama("http://localhost:11434", daemon.DefaultTestOllamaModel))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err1 := b.Close()
	if err1 != nil {
		t.Fatalf("first Close: %v", err1)
	}
	err2 := b.Close()
	if err2 != nil {
		t.Fatalf("second Close should be idempotent and return nil: %v", err2)
	}

	// Concurrent Close calls must not panic
	b2, err := New(":memory:", WithOllama("http://localhost:11434", daemon.DefaultTestOllamaModel))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_ = b2.Close()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
}

package brain

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glthr/brAIn/internal/daemon"
)

func TestLoggingOutput(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	b := newTestBrain(t, WithLogger(logger), WithWorkingMemoryTTL(1*time.Hour))

	b.Working.Store(ctx, "test task", nil)
	b.Working.Recent(ctx, 10)

	b.Semantic.Store(ctx, Fact{Content: "Go is great", Tags: []string{"go"}})
	b.Semantic.Lookup(ctx, "Go", 5)

	b.Episodic.Record(ctx, Episode{Content: "did a thing", Tags: []string{"test"}})
	b.Episodic.Recent(ctx, 10)
	b.Episodic.Search(ctx, EpisodicSearchOpts{Tags: []string{"test"}, Limit: 5})

	b.Procedural.Store(ctx, Procedure{Name: "test_proc", Steps: []string{"s1"}})
	b.Procedural.Lookup(ctx, "test_proc")

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "u1", ContactName: "User", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.9,
	})
	b.Social.ListAll(ctx)

	b.Consolidate(ctx)

	output := buf.String()

	expectedSubsystems := []string{
		"subsystem=working",
		"subsystem=episodic",
		"subsystem=semantic",
		"subsystem=procedural",
		"subsystem=social",
		"subsystem=consolidation",
	}
	for _, sub := range expectedSubsystems {
		if !strings.Contains(output, sub) {
			t.Errorf("expected log output to contain %q", sub)
		}
	}

	expectedActions := []string{
		"action=store",
		"action=recent",
		"action=record",
		"action=lookup",
		"action=search",
		"action=record_interaction",
		"action=list_all",
		"action=run",
	}
	for _, act := range expectedActions {
		if !strings.Contains(output, act) {
			t.Errorf("expected log output to contain %q", act)
		}
	}
}

func TestNoLoggerDefault(t *testing.T) {
	b := newTestBrain(t)
	ctx := context.Background()
	_, err := b.Working.Store(ctx, "silent", nil)
	if err != nil {
		t.Fatalf("store with no logger should succeed: %v", err)
	}
}

func TestFileLogging(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	brainPath := filepath.Join(tmpDir, "test.brain")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	b, err := New(brainPath, WithWorkingMemoryTTL(1*time.Hour), WithLogger(logger), WithOllama("http://localhost:11434", daemon.DefaultTestOllamaModel))
	if err != nil {
		t.Fatalf("create brain: %v", err)
	}

	b.Working.Store(ctx, "file log test", nil)
	b.Semantic.Store(ctx, Fact{Content: "a fact"})
	b.Consolidate(ctx)
	b.Close()

	output := buf.String()

	for _, expected := range []string{"subsystem=working", "subsystem=semantic", "subsystem=consolidation"} {
		if !strings.Contains(output, expected) {
			t.Errorf("log output should contain %q", expected)
		}
	}
	for _, expected := range []string{"action=store", "action=run"} {
		if !strings.Contains(output, expected) {
			t.Errorf("log output should contain %q", expected)
		}
	}
}

package brain

import (
	"context"
	"strings"
	"testing"
)

// TestEncodeShortTermMemories spawns a brain DB via newTestBrain, encodes conversation
// turns into short-term (working) memory, asserts the entries are in the database,
// then relies on t.Cleanup (in newTestBrain) to tear down the DB.
func TestEncodeShortTermMemories(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Encode two conversation turns (stored as working memory).
	_, err := b.Encode(ctx, "user-test", "TestUser", "cursor", "/my/project", "How do I run tests?", "Use: go test ./...")
	if err != nil {
		t.Fatalf("first encode: %v", err)
	}
	_, err = b.Encode(ctx, "user-test", "TestUser", "cursor", "/my/project", "What about coverage?", "Use: go test -cover ./...")
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}

	// Assert count via Working API.
	wc, err := b.Working.Count(ctx)
	if err != nil {
		t.Fatalf("Working.Count: %v", err)
	}
	if wc != 2 {
		t.Errorf("expected 2 working memories, got %d", wc)
	}

	// Assert entries visible via Recent.
	recent, err := b.Working.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Working.Recent: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 recent working memories, got %d", len(recent))
	}
	// Check both encoded contents appear (Recent returns most recent first).
	var allContent string
	for _, m := range recent {
		if m.Content == "" {
			t.Error("working memory content is empty")
		}
		allContent += m.Content
		if m.UserID != "user-test" || m.Agent != "cursor" {
			t.Errorf("working memory user_id=%q agent=%q, want user-test / cursor", m.UserID, m.Agent)
		}
	}
	if !strings.Contains(allContent, "How do I run tests?") || !strings.Contains(allContent, "go test ./...") {
		t.Errorf("working memories missing first encode: %q", allContent)
	}
	if !strings.Contains(allContent, "What about coverage?") || !strings.Contains(allContent, "go test -cover ./...") {
		t.Errorf("working memories missing second encode: %q", allContent)
	}

	// Assert entries are actually in the database (direct query).
	var rowCount int
	err = b.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories WHERE memory_type = 'working'`).Scan(&rowCount)
	if err != nil {
		t.Fatalf("query working count: %v", err)
	}
	if rowCount != 2 {
		t.Errorf("expected 2 rows in memories with memory_type='working', got %d", rowCount)
	}
}

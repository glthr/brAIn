package brain

import (
	"context"
	"testing"
)

func TestSocialMemory(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	_, err := b.Social.RecordInteraction(ctx, Interaction{
		ContactID:   "agent-42",
		ContactName: "Helper Bot",
		EntityType:  EntityTypeAgent,
		Type:        "collaboration",
		Outcome:     "success",
		Valence:     0.8,
		Notes:       "reliable for data tasks",
	})
	if err != nil {
		t.Fatalf("record agent: %v", err)
	}

	_, _ = b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-42",
		Type:      "question",
		Outcome:   "partial",
		Valence:   0.5,
	})

	_, err = b.Social.RecordInteraction(ctx, Interaction{
		ContactID:   "user-alice",
		ContactName: "Alice",
		EntityType:  EntityTypeHuman,
		Type:        "conversation",
		Outcome:     "success",
		Valence:     0.9,
		Notes:       "asked about Go channels",
	})
	if err != nil {
		t.Fatalf("record human: %v", err)
	}

	profile, err := b.Social.GetContactProfile(ctx, "agent-42")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if profile.Name != "Helper Bot" {
		t.Errorf("name: %q", profile.Name)
	}
	if profile.EntityType != EntityTypeAgent {
		t.Errorf("entity_type: %q, want %q", profile.EntityType, EntityTypeAgent)
	}
	if profile.InteractionCount != 2 {
		t.Errorf("interaction count: %d", profile.InteractionCount)
	}
	if profile.Trust <= 0 || profile.Trust > 1 {
		t.Errorf("trust out of range: %f", profile.Trust)
	}

	humanProfile, err := b.Social.GetContactProfile(ctx, "user-alice")
	if err != nil {
		t.Fatalf("human profile: %v", err)
	}
	if humanProfile.EntityType != EntityTypeHuman {
		t.Errorf("entity_type: %q, want %q", humanProfile.EntityType, EntityTypeHuman)
	}
	if humanProfile.Name != "Alice" {
		t.Errorf("human name: %q", humanProfile.Name)
	}

	err = b.Social.UpdateNotes(ctx, "agent-42", "prefers structured prompts")
	if err != nil {
		t.Fatalf("notes: %v", err)
	}
	profile, _ = b.Social.GetContactProfile(ctx, "agent-42")
	if profile.Notes != "prefers structured prompts" {
		t.Errorf("notes: %q", profile.Notes)
	}

	history, err := b.Social.InteractionHistory(ctx, "agent-42", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("history: %d", len(history))
	}
	if history[0].EntityType != EntityTypeAgent {
		t.Errorf("history entity_type: %q", history[0].EntityType)
	}

	agents, err := b.Social.ListAgents(ctx)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("agents: expected 1, got %d", len(agents))
	}

	humans, err := b.Social.ListHumans(ctx)
	if err != nil {
		t.Fatalf("list humans: %v", err)
	}
	if len(humans) != 1 {
		t.Errorf("humans: expected 1, got %d", len(humans))
	}

	all, err := b.Social.ListAll(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("all contacts: expected 2, got %d", len(all))
	}
}

func TestMostRecentHuman(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	_, err := b.Social.MostRecentHuman(ctx)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound with no humans, got %v", err)
	}

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-1", ContactName: "Bot", EntityType: EntityTypeAgent,
		Type: "chat", Outcome: "success", Valence: 0.7,
	})
	_, err = b.Social.MostRecentHuman(ctx)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound with only agents, got %v", err)
	}

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	profile, err := b.Social.MostRecentHuman(ctx)
	if err != nil {
		t.Fatalf("most recent human: %v", err)
	}
	if profile.ID != "user-alice" || profile.Name != "Alice" {
		t.Errorf("expected Alice, got id=%s name=%s", profile.ID, profile.Name)
	}

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-bob", ContactName: "Bob", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.9,
	})

	profile, err = b.Social.MostRecentHuman(ctx)
	if err != nil {
		t.Fatalf("most recent human after bob: %v", err)
	}
	if profile.ID != "user-bob" || profile.Name != "Bob" {
		t.Errorf("expected Bob as most recent, got id=%s name=%s", profile.ID, profile.Name)
	}
}

func TestRetroactiveUserIDAssignment(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Store some memories with empty user_id (simulating unknown user)
	_, err := b.Working.StoreForUserWithAgent(ctx, "First interaction", map[string]string{"type": "conversation"}, "", "cursor")
	if err != nil {
		t.Fatalf("store memory: %v", err)
	}
	_, err = b.Working.StoreForUserWithAgent(ctx, "Second interaction", map[string]string{"type": "conversation"}, "", "cursor")
	if err != nil {
		t.Fatalf("store memory: %v", err)
	}
	_, err = b.Semantic.Store(ctx, Fact{Content: "Some fact", UserID: ""})
	if err != nil {
		t.Fatalf("store fact: %v", err)
	}

	// Verify memories have empty user_id before registration
	workingMems, _ := b.Working.Recent(ctx, 10)
	emptyCountBefore := 0
	for _, m := range workingMems {
		if m.UserID == "" {
			emptyCountBefore++
		}
	}
	if emptyCountBefore == 0 {
		t.Fatal("expected some memories with empty user_id before registration")
	}

	// Register a new human user - this should retroactively assign user_id
	_, err = b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "greeting", Outcome: "success", Valence: 0.7,
	})
	if err != nil {
		t.Fatalf("record interaction: %v", err)
	}

	// Verify working memories now have user_id assigned
	workingMems, _ = b.Working.Recent(ctx, 10)
	assignedCount := 0
	for _, m := range workingMems {
		if m.UserID == "user-alice" {
			assignedCount++
		}
	}
	if assignedCount == 0 {
		t.Errorf("expected at least some working memories to have user_id=user-alice after retroactive assignment, got %d", assignedCount)
	}

	// Verify semantic memory has user_id assigned
	mems, err := b.MemoriesForUser(ctx, "user-alice", 50)
	if err != nil {
		t.Fatalf("MemoriesForUser: %v", err)
	}
	if len(mems) < 1 {
		t.Errorf("expected at least 1 semantic memory for user-alice after retroactive assignment, got %d", len(mems))
	}

	// Verify the memories have the correct user_id
	for _, m := range mems {
		if m.UserID != "user-alice" {
			t.Errorf("expected user_id=user-alice, got %q", m.UserID)
		}
	}
}

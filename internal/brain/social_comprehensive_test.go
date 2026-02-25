package brain

import (
	"context"
	"testing"
)

func TestSocialMemoryInteractionHistoryLimit(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Record multiple interactions
	for i := 0; i < 10; i++ {
		b.Social.RecordInteraction(ctx, Interaction{
			ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
			Type: "chat", Outcome: "success", Valence: 0.8,
		})
	}

	// Request only 5
	history, err := b.Social.InteractionHistory(ctx, "user-alice", 5)
	if err != nil {
		t.Fatalf("InteractionHistory: %v", err)
	}
	if len(history) != 5 {
		t.Errorf("expected 5 interactions, got %d", len(history))
	}
}

func TestSocialMemoryTrustCalculation(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Trust is only computed for agents. Record interactions with an agent.
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-bob", ContactName: "Bob", EntityType: EntityTypeAgent,
		Type: "chat", Outcome: "success", Valence: 0.9,
	})
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-bob", ContactName: "Bob", EntityType: EntityTypeAgent,
		Type: "chat", Outcome: "success", Valence: 0.7,
	})
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-bob", ContactName: "Bob", EntityType: EntityTypeAgent,
		Type: "chat", Outcome: "success", Valence: 0.5,
	})

	profile, err := b.Social.GetContactProfile(ctx, "agent-bob")
	if err != nil {
		t.Fatalf("GetContactProfile: %v", err)
	}

	// Trust should be calculated for agents (weighted average)
	if profile.Trust <= 0 || profile.Trust > 1 {
		t.Errorf("trust should be between 0 and 1, got %f", profile.Trust)
	}
	if profile.Trust < 0.5 {
		t.Errorf("trust should be > 0.5 with positive interactions, got %f", profile.Trust)
	}

	// Humans have no trust notion; profile should report 0
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-eve", ContactName: "Eve", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.9,
	})
	humanProfile, _ := b.Social.GetContactProfile(ctx, "user-eve")
	if humanProfile != nil && humanProfile.Trust != 0 {
		t.Errorf("human trust should be 0 (not used), got %f", humanProfile.Trust)
	}
}

func TestSocialMemoryUpdateNotes(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-charlie", ContactName: "Charlie", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	// Update notes
	err := b.Social.UpdateNotes(ctx, "user-charlie", "prefers detailed explanations")
	if err != nil {
		t.Fatalf("UpdateNotes: %v", err)
	}

	profile, err := b.Social.GetContactProfile(ctx, "user-charlie")
	if err != nil {
		t.Fatalf("GetContactProfile: %v", err)
	}
	if profile.Notes != "prefers detailed explanations" {
		t.Errorf("notes: got %q, want %q", profile.Notes, "prefers detailed explanations")
	}

	// Update notes again
	err = b.Social.UpdateNotes(ctx, "user-charlie", "updated preference")
	if err != nil {
		t.Fatalf("UpdateNotes second time: %v", err)
	}

	profile, _ = b.Social.GetContactProfile(ctx, "user-charlie")
	if profile.Notes != "updated preference" {
		t.Errorf("notes after update: got %q, want %q", profile.Notes, "updated preference")
	}
}

func TestSocialMemoryUpdateNotesForNonExistentContact(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	err := b.Social.UpdateNotes(ctx, "user-nonexistent", "some notes")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent contact, got %v", err)
	}
}

func TestSocialMemoryListContactsByType(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Add humans and agents
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-bob", ContactName: "Bob", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-bot1", ContactName: "Bot1", EntityType: EntityTypeAgent,
		Type: "collaboration", Outcome: "success", Valence: 0.7,
	})
	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "agent-bot2", ContactName: "Bot2", EntityType: EntityTypeAgent,
		Type: "collaboration", Outcome: "success", Valence: 0.7,
	})

	humans, err := b.Social.ListHumans(ctx)
	if err != nil {
		t.Fatalf("ListHumans: %v", err)
	}
	if len(humans) != 2 {
		t.Errorf("expected 2 humans, got %d", len(humans))
	}

	agents, err := b.Social.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}

	all, err := b.Social.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 total contacts, got %d", len(all))
	}
}

func TestSocialMemoryContactProfileFields(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-dave", ContactName: "Dave", EntityType: EntityTypeHuman,
		Type: "conversation", Outcome: "success", Valence: 0.9,
	})
	_ = b.Social.UpdateNotes(ctx, "user-dave", "initial note")

	profile, err := b.Social.GetContactProfile(ctx, "user-dave")
	if err != nil {
		t.Fatalf("GetContactProfile: %v", err)
	}

	if profile.ID != "user-dave" {
		t.Errorf("ID: got %q, want %q", profile.ID, "user-dave")
	}
	if profile.Name != "Dave" {
		t.Errorf("Name: got %q, want %q", profile.Name, "Dave")
	}
	if profile.EntityType != EntityTypeHuman {
		t.Errorf("EntityType: got %q, want %q", profile.EntityType, EntityTypeHuman)
	}
	if profile.InteractionCount != 1 {
		t.Errorf("InteractionCount: got %d, want 1", profile.InteractionCount)
	}
	if profile.Trust != 0 {
		t.Errorf("Trust (human): got %f, want 0 (trust not used for humans)", profile.Trust)
	}
	if profile.Notes != "initial note" {
		t.Errorf("Notes: got %q, want %q", profile.Notes, "initial note")
	}
}

func TestSocialMemoryMultipleInteractionsUpdateProfile(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	// Record multiple interactions
	for i := 0; i < 5; i++ {
		b.Social.RecordInteraction(ctx, Interaction{
			ContactID: "user-eve", ContactName: "Eve", EntityType: EntityTypeHuman,
			Type: "chat", Outcome: "success", Valence: 0.8,
		})
	}

	profile, err := b.Social.GetContactProfile(ctx, "user-eve")
	if err != nil {
		t.Fatalf("GetContactProfile: %v", err)
	}

	if profile.InteractionCount != 5 {
		t.Errorf("InteractionCount: got %d, want 5", profile.InteractionCount)
	}
}

func TestSocialMemoryGetContactProfileNotFound(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	_, err := b.Social.GetContactProfile(ctx, "user-nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent contact, got %v", err)
	}
}

func TestSocialMemoryInteractionHistoryEmpty(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-frank", ContactName: "Frank", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	// Get history for different contact
	history, err := b.Social.InteractionHistory(ctx, "user-different", 10)
	if err != nil {
		t.Fatalf("InteractionHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 interactions for different contact, got %d", len(history))
	}
}

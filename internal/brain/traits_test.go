package brain

import (
	"context"
	"testing"
)

func TestPersonSchemaSetAndGet(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	text := "Alice is a senior Go developer who prefers concise, direct responses. She works on distributed systems and values correctness over brevity in technical explanations."

	err := b.Social.SetSocialSchema(ctx, "user-alice", text)
	if err != nil {
		t.Fatalf("set social schema: %v", err)
	}

	got, err := b.Social.GetSocialSchema(ctx, "user-alice")
	if err != nil {
		t.Fatalf("get person schema: %v", err)
	}
	if got != text {
		t.Errorf("expected %q, got %q", text, got)
	}
}

func TestPersonSchemaUpdate(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-bob", ContactName: "Bob", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	b.Social.SetSocialSchema(ctx, "user-bob", "Bob is a Python developer.")
	b.Social.SetSocialSchema(ctx, "user-bob", "Bob is a Python developer who recently switched to Go.")

	got, _ := b.Social.GetSocialSchema(ctx, "user-bob")
	if got != "Bob is a Python developer who recently switched to Go." {
		t.Errorf("unexpected person schema after update: %q", got)
	}
}

func TestPersonSchemaEmptyByDefault(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-carol", ContactName: "Carol", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	got, err := b.Social.GetSocialSchema(ctx, "user-carol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty person schema, got %q", got)
	}
}

func TestPersonSchemaNotFoundForUnknownUser(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	_, err := b.Social.GetSocialSchema(ctx, "user-nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPersonSchemaInProfile(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-dave", ContactName: "Dave", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	text := "Dave is a product manager with a preference for bullet-point summaries."
	b.Social.SetSocialSchema(ctx, "user-dave", text)

	profile, err := b.Social.GetContactProfile(ctx, "user-dave")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if profile.SocialSchema != text {
		t.Errorf("profile.SocialSchema = %q, want %q", profile.SocialSchema, text)
	}
}

func TestBehavioralPriorsSetAndGet(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-eve", ContactName: "Eve", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	text := "Eve expects detailed, step-by-step explanations with code examples."

	err := b.Social.SetBehavioralPriors(ctx, "user-eve", text)
	if err != nil {
		t.Fatalf("set behavioral priors: %v", err)
	}

	got, err := b.Social.GetBehavioralPriors(ctx, "user-eve")
	if err != nil {
		t.Fatalf("get behavioral priors: %v", err)
	}
	if got != text {
		t.Errorf("expected %q, got %q", text, got)
	}
}

func TestBehavioralPriorsEmptyByDefault(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-frank", ContactName: "Frank", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	got, err := b.Social.GetBehavioralPriors(ctx, "user-frank")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty behavioral priors, got %q", got)
	}
}

func TestBehavioralPriorsInProfile(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-grace", ContactName: "Grace", EntityType: EntityTypeHuman,
		Type: "chat", Outcome: "success", Valence: 0.8,
	})

	pText := "Grace is a backend engineer."
	eText := "Grace prefers concise answers and dislikes unnecessary preamble."
	b.Social.SetSocialSchema(ctx, "user-grace", pText)
	b.Social.SetBehavioralPriors(ctx, "user-grace", eText)

	profile, err := b.Social.GetContactProfile(ctx, "user-grace")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if profile.SocialSchema != pText {
		t.Errorf("profile.SocialSchema = %q, want %q", profile.SocialSchema, pText)
	}
	if profile.BehavioralPriors != eText {
		t.Errorf("profile.BehavioralPriors = %q, want %q", profile.BehavioralPriors, eText)
	}
}

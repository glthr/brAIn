//go:build integration

package brain

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/glthr/brAIn/internal/daemon"
	"github.com/glthr/brAIn/internal/model"
)

// loggingInferProfileFunc returns an InferProfileFunc that logs the encoded summary
// (input) and the inferred trait (social_schema) and expectations (behavioral_priors)
// via t.Logf so they appear when running tests with -v for manual review.
func loggingInferProfileFunc(t *testing.T) InferProfileFunc {
	url := os.Getenv("BRAIN_TEST_OLLAMA_URL")
	if url == "" {
		url = "http://localhost:11434"
	}
	modelName := os.Getenv("BRAIN_TEST_OLLAMA_MODEL")
	if modelName == "" {
		modelName = daemon.DefaultTestOllamaModel
	}
	client := NewLLMClient(url, modelName)
	realInfer := NewInferProfileFunc(client, nil)
	return func(ctx context.Context, summary string, current *model.ProfileInference) (*model.ProfileInference, error) {
		t.Logf("=== Profile inference input (encoded summary) ===\n%s\n=== End summary ===", summary)
		out, err := realInfer(ctx, summary, current)
		if err != nil {
			return nil, err
		}
		if out != nil {
			t.Logf("=== Inferred trait (social_schema) ===\n%s\n=== End social_schema ===", out.SocialSchema)
			t.Logf("=== Inferred expectations (behavioral_priors) ===\n%s\n=== End behavioral_priors ===", out.BehavioralPriors)
		}
		return out, nil
	}
}

// TestTraitAnalysisTechnicalSenior seeds a user with interactions and memories
// that suggest a technical senior engineer (architecture, refactoring, concise
// preferences), runs profile analysis via the real Ollama LLM, and asserts
// that the inferred social schema and behavioral priors reflect that personality.
//
// Run with: go test -tags integration -run TestTraitAnalysisTechnicalSenior -timeout 5m -v ./internal/brain/...
func TestTraitAnalysisTechnicalSenior(t *testing.T) {
	ctx := context.Background()
	opts := []Option{WithInferProfileFunc(loggingInferProfileFunc(t))}
	if os.Getenv("RUN_INTEGRATION_DEBUG") == "1" {
		opts = append(opts, WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))))
	}
	b := newTestBrain(t, opts...)

	userID := "user-morgan"
	userName := "Morgan"

	// Interactions that suggest a senior, technical profile.
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID: userID, ContactName: userName, EntityType: model.EntityTypeHuman,
		Type: "coding", Outcome: "success", Valence: 0.8,
		Notes: "Asked how to structure service boundaries in a monorepo; wanted minimal hand-holding.",
	})
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID: userID, ContactName: userName, EntityType: model.EntityTypeHuman,
		Type: "coding", Outcome: "success", Valence: 0.9,
		Notes: "Refactoring legacy code; requested concise review feedback and performance tradeoffs only.",
	})
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID: userID, ContactName: userName, EntityType: model.EntityTypeHuman,
		Type: "testing", Outcome: "success", Valence: 0.7,
		Notes: "Discussed testing strategy for event-driven services; prefers table-driven tests and mocks.",
	})

	// Long-term memories that reinforce the profile.
	b.Semantic.Store(ctx, Fact{Content: "Morgan works on distributed systems and cares about consistency and latency.", UserID: userID})
	b.Episodic.Record(ctx, Episode{Content: "Morgan reviewed a design doc and asked for concrete tradeoffs rather than long explanations.", UserID: userID})

	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	results, err := b.AnalyzeProfiles(runCtx)
	if err != nil {
		t.Fatalf("AnalyzeProfiles: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].UserID != userID {
		t.Errorf("expected user %s, got %s", userID, results[0].UserID)
	}
	if results[0].Skipped {
		t.Fatalf("profile analysis was skipped: %s", results[0].SkipReason)
	}

	schema, _ := b.Social.GetSocialSchema(ctx, userID)
	priors, _ := b.Social.GetBehavioralPriors(ctx, userID)

	if schema == "" {
		t.Fatal("expected non-empty social schema")
	}

	// Expect themes consistent with a technical senior (wording may vary by model).
	schemaLower := strings.ToLower(schema)
	seniorKeywords := []string{"senior", "technical", "engineer", "architecture", "concise", "expert", "experienced", "systems"}
	foundSchema := false
	for _, k := range seniorKeywords {
		if strings.Contains(schemaLower, k) {
			foundSchema = true
			break
		}
	}
	if !foundSchema {
		t.Errorf("social schema should suggest technical/senior profile; got: %s", truncateForTest(schema, 300))
	}

	// Behavioral priors: assert only when the model returns them (small models may omit).
	if priors != "" {
		priorsLower := strings.ToLower(priors)
		priorsKeywords := []string{"concise", "minimal", "direct", "tradeoff", "hand-holding", "brief", "point"}
		foundPriors := false
		for _, k := range priorsKeywords {
			if strings.Contains(priorsLower, k) {
				foundPriors = true
				break
			}
		}
		if !foundPriors {
			t.Errorf("behavioral priors should suggest preference for concise/direct responses; got: %s", truncateForTest(priors, 300))
		}
	}
}

// TestTraitAnalysisBeginnerPrefersExamples seeds a user with interactions and
// memories that suggest a beginner who likes step-by-step guidance and
// examples, runs profile analysis via the real Ollama LLM, and asserts the
// inferred traits reflect that personality.
//
// Run with: go test -tags integration -run TestTraitAnalysisBeginnerPrefersExamples -timeout 5m -v ./internal/brain/...
func TestTraitAnalysisBeginnerPrefersExamples(t *testing.T) {
	ctx := context.Background()
	opts := []Option{WithInferProfileFunc(loggingInferProfileFunc(t))}
	if os.Getenv("RUN_INTEGRATION_DEBUG") == "1" {
		opts = append(opts, WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))))
	}
	b := newTestBrain(t, opts...)

	userID := "user-jordan"
	userName := "Jordan"

	// Interactions that suggest a beginner who wants examples and guidance.
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID: userID, ContactName: userName, EntityType: model.EntityTypeHuman,
		Type: "learning", Outcome: "success", Valence: 0.8,
		Notes: "Asked how to run Go tests for the first time; wanted a step-by-step and an example command.",
	})
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID: userID, ContactName: userName, EntityType: model.EntityTypeHuman,
		Type: "learning", Outcome: "success", Valence: 0.9,
		Notes: "Asked what code coverage is and requested a concrete example with go test -cover.",
	})
	b.Social.RecordInteraction(ctx, model.Interaction{
		ContactID: userID, ContactName: userName, EntityType: model.EntityTypeHuman,
		Type: "coding", Outcome: "success", Valence: 0.7,
		Notes: "Struggled with imports; asked for a full minimal example they could copy-paste.",
	})

	b.Semantic.Store(ctx, Fact{Content: "Jordan is learning Go and prefers examples and step-by-step instructions.", UserID: userID})
	b.Episodic.Record(ctx, Episode{Content: "Jordan thanked the agent for a clear example and said they learn better with code they can run.", UserID: userID})

	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	results, err := b.AnalyzeProfiles(runCtx)
	if err != nil {
		t.Fatalf("AnalyzeProfiles: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].UserID != userID {
		t.Errorf("expected user %s, got %s", userID, results[0].UserID)
	}
	if results[0].Skipped {
		t.Fatalf("profile analysis was skipped: %s", results[0].SkipReason)
	}

	schema, _ := b.Social.GetSocialSchema(ctx, userID)
	priors, _ := b.Social.GetBehavioralPriors(ctx, userID)

	if schema == "" {
		t.Fatal("expected non-empty social schema")
	}

	schemaLower := strings.ToLower(schema)
	// Expect themes: beginner, learning, prefers examples or step-by-step.
	schemaKeywords := []string{"beginner", "learning", "new", "novice", "learner", "go ", "example"}
	foundSchema := false
	for _, k := range schemaKeywords {
		if strings.Contains(schemaLower, k) {
			foundSchema = true
			break
		}
	}
	if !foundSchema {
		t.Errorf("social schema should suggest beginner/learner profile; got: %s", truncateForTest(schema, 300))
	}

	if priors != "" {
		priorsLower := strings.ToLower(priors)
		priorsKeywords := []string{"example", "step", "detailed", "guidance", "explain", "concrete", "copy"}
		foundPriors := false
		for _, k := range priorsKeywords {
			if strings.Contains(priorsLower, k) {
				foundPriors = true
				break
			}
		}
		if !foundPriors {
			t.Errorf("behavioral priors should suggest preference for examples or step-by-step; got: %s", truncateForTest(priors, 300))
		}
	}
}

func truncateForTest(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

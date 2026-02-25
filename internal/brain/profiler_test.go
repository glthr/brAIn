package brain

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glthr/brAIn/internal/model"
)

func TestProfilerRunsOnNewData(t *testing.T) {
	ctx := context.Background()
	inferCalled := false
	inferFunc := func(ctx context.Context, summary string, current *model.ProfileInference) (*model.ProfileInference, error) {
		inferCalled = true
		if !strings.Contains(summary, "Alice") {
			t.Errorf("summary should mention Alice, got: %s", summary)
		}
		return &model.ProfileInference{
			SocialSchema:     "Alice is a senior engineer who prefers direct answers.",
			BehavioralPriors: "Alice expects concise, technical responses without hand-holding.",
		}, nil
	}

	b := newTestBrain(t, WithInferProfileFunc(inferFunc))

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-alice", ContactName: "Alice", EntityType: EntityTypeHuman,
		Type: "interaction", Outcome: "success", Valence: 0.8, Notes: "helped with deployment",
	})
	b.Semantic.Store(ctx, Fact{Content: "Alice deploys to k8s", UserID: "user-alice"})

	results, err := b.AnalyzeProfiles(ctx)
	if err != nil {
		t.Fatalf("AnalyzeProfiles: %v", err)
	}
	if !inferCalled {
		t.Fatal("InferProfileFunc was not called")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].UserID != "user-alice" {
		t.Errorf("expected user-alice, got %s", results[0].UserID)
	}
	if !results[0].SocialSchemaUpdated {
		t.Error("expected SocialSchemaUpdated=true")
	}
	if !results[0].BehavioralPriorsUpdated {
		t.Error("expected BehavioralPriorsUpdated=true")
	}

	socialSchema, _ := b.Social.GetSocialSchema(ctx, "user-alice")
	if socialSchema == "" {
		t.Error("expected social schema to be stored")
	}

	behavioralPriors, _ := b.Social.GetBehavioralPriors(ctx, "user-alice")
	if behavioralPriors == "" {
		t.Error("expected behavioral priors to be stored")
	}
}

func TestProfilerSkipsAnalyzedUsers(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	inferFunc := func(ctx context.Context, summary string, current *model.ProfileInference) (*model.ProfileInference, error) {
		callCount++
		return nil, nil
	}

	b := newTestBrain(t, WithInferProfileFunc(inferFunc))

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-bob", ContactName: "Bob", EntityType: EntityTypeHuman,
		Type: "interaction", Outcome: "success", Valence: 0.8,
	})

	b.AnalyzeProfiles(ctx)
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	b.AnalyzeProfiles(ctx)
	if callCount != 1 {
		t.Fatalf("expected still 1 call (skipped), got %d", callCount)
	}

	b.Episodic.Record(ctx, Episode{Content: "fixed a bug with Bob", UserID: "user-bob"})
	b.AnalyzeProfiles(ctx)
	if callCount != 2 {
		t.Fatalf("expected 2 calls, got %d", callCount)
	}
}

func TestProfilerNoFuncIsNoop(t *testing.T) {
	ctx := context.Background()
	// Explicitly set InferProfileFunc to nil to test the no-func path.
	// (LLM is mandatory, so InferProfileFunc is set by default, but can be overridden.)
	b := newTestBrain(t, WithInferProfileFunc(nil))

	results, err := b.AnalyzeProfiles(ctx)
	if err != nil {
		t.Fatalf("AnalyzeProfiles: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results without InferProfileFunc, got %v", results)
	}
}

func TestHumansNeedingAnalysis(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	users, _ := b.Social.HumansNeedingAnalysis(ctx)
	if len(users) != 0 {
		t.Fatalf("expected 0, got %d", len(users))
	}

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-carol", ContactName: "Carol", EntityType: EntityTypeHuman,
		Type: "interaction", Outcome: "success", Valence: 0.9,
	})

	users, _ = b.Social.HumansNeedingAnalysis(ctx)
	if len(users) != 1 {
		t.Fatalf("expected 1, got %d", len(users))
	}

	b.Social.MarkAnalyzed(ctx, "user-carol")

	users, _ = b.Social.HumansNeedingAnalysis(ctx)
	if len(users) != 0 {
		t.Fatalf("expected 0 after mark, got %d", len(users))
	}

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-carol", ContactName: "Carol", EntityType: EntityTypeHuman,
		Type: "interaction", Outcome: "success", Valence: 0.8,
	})

	users, _ = b.Social.HumansNeedingAnalysis(ctx)
	if len(users) != 1 {
		t.Fatalf("expected 1 after new interaction, got %d", len(users))
	}
}

func TestProfilerWithBackground(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	inferFunc := func(ctx context.Context, summary string, current *model.ProfileInference) (*model.ProfileInference, error) {
		callCount++
		return nil, nil
	}

	b := newTestBrain(t,
		WithInferProfileFunc(inferFunc),
		WithProfileInterval(50*time.Millisecond),
	)

	b.Social.RecordInteraction(ctx, Interaction{
		ContactID: "user-dan", ContactName: "Dan", EntityType: EntityTypeHuman,
		Type: "interaction", Outcome: "success", Valence: 0.7,
	})

	time.Sleep(150 * time.Millisecond)

	b.Close()
	if callCount < 1 {
		t.Errorf("expected at least 1 profiler call from background loop, got %d", callCount)
	}
}

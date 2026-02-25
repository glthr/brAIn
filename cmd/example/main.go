package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/glthr/brAIn/internal/brain"
)

func main() {
	ctx := context.Background()

	// Create a brain persisted to disk, with consolidation every hour.
	// LLM connection is mandatory (prerequisite).
	b, err := brain.New("agent_brain.brain",
		brain.WithConsolidationInterval(1*time.Hour),
		brain.WithDecayRate(0.05),
		brain.WithSalienceFloor(0.1),
		brain.WithWorkingMemoryTTL(30*time.Minute),
		brain.WithOllama("http://localhost:11434", "qwen3:4b"), // Required!
		brain.WithSemanticizationFunc(exampleAbstractor),       // Override default semanticization
		brain.WithSemanticizationAge(72*time.Hour),
		brain.WithDescription("Demo brain for the brAIn example"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	fmt.Println("=== Working Memory (short-term context) ===")

	_, _ = b.Working.Store(ctx, "User is asking about Go concurrency patterns", nil)
	b.Working.Store(ctx, "Current task: explain channels vs mutexes", map[string]string{
		"priority": "high",
	})

	recent, _ := b.Working.Recent(ctx, 10)
	for _, m := range recent {
		fmt.Printf("  [WM] %s (salience=%.2f)\n", m.Content, m.Salience)
	}
	count, _ := b.Working.Count(ctx)
	fmt.Printf("  Active working memories: %d\n\n", count)

	fmt.Println("=== Episodic Memory (events & experiences) ===")

	_, _ = b.Episodic.Record(ctx, brain.Episode{
		Content: "Successfully helped user debug a goroutine leak using pprof",
		Tags:    []string{"debugging", "go", "goroutines", "success"},
	})
	_, _ = b.Episodic.RecordWithImportance(ctx, brain.Episode{
		Content: "Failed to explain monads clearly -- user got confused",
		Tags:    []string{"explanation", "functional-programming", "failure"},
	}, 0.8)

	episodes, _ := b.Episodic.Recent(ctx, 5)
	for _, e := range episodes {
		fmt.Printf("  [EP] %s (salience=%.2f, tags=%v)\n", e.Content, e.Salience, e.Tags)
	}
	fmt.Println()

	fmt.Println("=== Semantic Memory (facts & knowledge) ===")

	_, _ = b.Semantic.Store(ctx, brain.Fact{
		Content: "Go channels provide a way for goroutines to communicate and synchronize",
		Tags:    []string{"go", "channels", "concurrency"},
	})
	_, _ = b.Semantic.Store(ctx, brain.Fact{
		Content: "Mutexes protect shared state but can lead to deadlocks if not used carefully",
		Tags:    []string{"go", "mutex", "concurrency"},
	})

	lookup, _ := b.Semantic.Lookup(ctx, "deadlock", 5)
	if len(lookup) > 0 {
		fmt.Printf("  [SM] Keyword 'deadlock' found: %q\n", lookup[0].Content)
	}
	allFacts, _ := b.Semantic.All(ctx)
	fmt.Printf("  Total semantic facts: %d\n\n", len(allFacts))

	fmt.Println("=== Procedural Memory (learned strategies) ===")

	b.Procedural.Store(ctx, brain.Procedure{
		Name:        "code_review",
		Description: "Review a pull request systematically",
		Steps:       []string{"read the description", "check diff for logic errors", "verify tests exist", "look for style issues", "leave constructive comments"},
		SuccessRate: 0.85,
	})
	b.Procedural.Store(ctx, brain.Procedure{
		Name:        "debug_goroutine_leak",
		Description: "Find and fix goroutine leaks",
		Steps:       []string{"run pprof", "identify leaked goroutines", "check for missing channel closes", "add context cancellation", "verify with test"},
		SuccessRate: 0.9,
	})

	b.Procedural.UpdateSuccessRate(ctx, "code_review", true)
	b.Procedural.UpdateSuccessRate(ctx, "code_review", true)

	procs, _ := b.Procedural.All(ctx)
	for _, p := range procs {
		fmt.Printf("  [PR] %s: %d steps, success=%.2f, used=%d times\n",
			p.Name, len(p.Steps), p.SuccessRate, p.UseCount)
	}
	fmt.Println()

	fmt.Println("=== Social Memory (agents and humans) ===")

	// Agent-to-Agent interactions.
	b.Social.RecordInteraction(ctx, brain.Interaction{
		ContactID:   "agent-search",
		ContactName: "SearchBot",
		EntityType:  brain.EntityTypeAgent,
		Type:        "delegation",
		Outcome:     "success",
		Valence:     0.9,
		Notes:       "Returned accurate results quickly",
	})
	b.Social.RecordInteraction(ctx, brain.Interaction{
		ContactID:  "agent-search",
		EntityType: brain.EntityTypeAgent,
		Type:       "delegation",
		Outcome:    "partial",
		Valence:    0.5,
		Notes:      "Results were relevant but incomplete",
	})
	b.Social.RecordInteraction(ctx, brain.Interaction{
		ContactID:   "agent-code",
		ContactName: "CodeGenBot",
		EntityType:  brain.EntityTypeAgent,
		Type:        "collaboration",
		Outcome:     "success",
		Valence:     0.8,
		Notes:       "Generated clean Go code, good with interfaces",
	})

	// Agent-to-Human interactions (no valence/score for humans).
	b.Social.RecordInteraction(ctx, brain.Interaction{
		ContactID:   "user-alice",
		ContactName: "Alice",
		EntityType:  brain.EntityTypeHuman,
		Type:        "conversation",
		Outcome:     "success",
		Notes:       "Experienced Go developer, clear communicator",
	})
	b.Social.RecordInteraction(ctx, brain.Interaction{
		ContactID:   "user-bob",
		ContactName: "Bob",
		EntityType:  brain.EntityTypeHuman,
		Type:        "code_review",
		Outcome:     "success",
		Notes:       "Junior dev, needed extra explanation on concurrency",
	})

	b.Social.UpdateNotes(ctx, "agent-search", "Fast but sometimes incomplete. Best for simple queries.")

	profile, _ := b.Social.GetContactProfile(ctx, "agent-search")
	fmt.Printf("  [A2A] %s (trust=%.3f, interactions=%d, avg_valence=%.2f)\n",
		profile.Name, profile.Trust, profile.InteractionCount, profile.AvgValence)
	fmt.Printf("        Notes: %s\n", profile.Notes)

	humanProfile, _ := b.Social.GetContactProfile(ctx, "user-alice")
	fmt.Printf("  [A2H] %s (interactions=%d)\n",
		humanProfile.Name, humanProfile.InteractionCount)

	agents, _ := b.Social.ListAgents(ctx)
	fmt.Printf("  Known agents: %d\n", len(agents))
	humans, _ := b.Social.ListHumans(ctx)
	fmt.Printf("  Known humans: %d\n", len(humans))
	allContacts, _ := b.Social.ListAll(ctx)
	fmt.Printf("  Total contacts: %d\n\n", len(allContacts))

	fmt.Println("=== Consolidation (manual sleep cycle) ===")

	stats, err := b.Consolidate(ctx)
	if err != nil {
		log.Printf("  consolidation error: %v", err)
	} else {
		fmt.Printf("  Expired: %d, Decayed: %d, Transferred: %d\n", stats.Expired, stats.Decayed, stats.Transferred)
		fmt.Printf("  Forgotten: %d, Semanticized: %d\n", stats.Forgotten, stats.Semanticized)
		fmt.Printf("  Duration: %v\n", stats.Duration)
	}

	fmt.Println("\n=== Brain Metadata (identity & portability) ===")

	meta, _ := b.Meta(ctx)
	fmt.Printf("  Schema version:      %s\n", meta.SchemaVersion)
	fmt.Printf("  Created at:          %s\n", meta.CreatedAt)
	fmt.Printf("  Description:         %s\n", meta.Description)
	fmt.Printf("  Last backup:         %s\n", meta.LastBackupAt)

	b.RecordBackup(ctx)
	meta, _ = b.Meta(ctx)
	fmt.Printf("  Last backup (after): %s\n", meta.LastBackupAt)

	fmt.Println("\nDone. Brain state saved to agent_brain.brain")
}

func exampleAbstractor(ctx context.Context, memories []brain.Memory) (*brain.Memory, error) {
	combined := "Summary of events: "
	for i, m := range memories {
		if i > 0 {
			combined += " | "
		}
		combined += m.Content
	}
	return &brain.Memory{
		Content:  combined,
		Salience: 0.7,
	}, nil
}

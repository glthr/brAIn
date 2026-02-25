// Package profiler implements automatic user profile analysis.
//
// The Profiler runs as a background goroutine, periodically scanning for
// human contacts that have new interactions or memories since their last
// analysis. For each such user, it builds a structured summary (profile,
// interactions, memories) and calls the InferProfileFunc callback
// so an LLM can write or update person schema and behavioral priors.
package profiler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/glthr/brAIn/internal/memory"
	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// Profiler analyzes user profiles and infers person schema + behavioral priors from long-term memory.
type Profiler struct {
	Store  *store.Store
	Social *memory.SocialMemory
	Infer  model.InferProfileFunc
	Logger *slog.Logger
}

// Run performs a single analysis pass over all human contacts with new data.
// For each user that has new interactions or memories since last analysis,
// it builds a summary and calls InferProfileFunc. Returns one
// ProfileAnalysis per user inspected.
func (p *Profiler) Run(ctx context.Context) ([]model.ProfileAnalysis, error) {
	if p.Infer == nil {
		return nil, nil
	}

	p.Logger.InfoContext(ctx, "profile analysis started", "action", "run")

	users, err := p.Social.HumansNeedingAnalysis(ctx)
	if err != nil {
		return nil, fmt.Errorf("profiler: list users: %w", err)
	}

	if len(users) == 0 {
		p.Logger.InfoContext(ctx, "no users need analysis", "action", "run")
		return nil, nil
	}

	results := make([]model.ProfileAnalysis, 0, len(users))
	for _, user := range users {
		analysis, err := p.analyzeUser(ctx, user)
		if err != nil {
			p.Logger.WarnContext(ctx, "user analysis failed (non-fatal)",
				"action", "analyze_user", "user_id", user.ID, "error", err)
			results = append(results, model.ProfileAnalysis{
				UserID:     user.ID,
				Skipped:    true,
				SkipReason: err.Error(),
			})
			continue
		}
		results = append(results, analysis)
	}

	p.Logger.InfoContext(ctx, "profile analysis completed", "action", "run", "users", len(results))
	return results, nil
}

func (p *Profiler) analyzeUser(ctx context.Context, user model.ContactProfile) (model.ProfileAnalysis, error) {
	result := model.ProfileAnalysis{UserID: user.ID}

	summary, err := p.buildSummary(ctx, user)
	if err != nil {
		return result, err
	}

	currentSocialSchema, _ := p.Social.GetSocialSchema(ctx, user.ID)
	currentBehavioralPriors, _ := p.Social.GetBehavioralPriors(ctx, user.ID)

	mems, _ := p.Store.MemoriesByUser(ctx, user.ID, 200)
	result.NewMemories = len(mems)

	current := &model.ProfileInference{
		SocialSchema:     currentSocialSchema,
		BehavioralPriors: currentBehavioralPriors,
	}

	updated, err := p.Infer(ctx, summary, current)
	if err != nil {
		return result, fmt.Errorf("infer: %w", err)
	}

	if updated != nil {
		p.Logger.DebugContext(ctx, "profile inference result",
			"action", "infer_profile",
			"user_id", user.ID,
			"social_schema_provided", updated.SocialSchema != "",
			"behavioral_priors_provided", updated.BehavioralPriors != "",
			"social_schema_length", len(updated.SocialSchema),
			"behavioral_priors_length", len(updated.BehavioralPriors))
		if updated.SocialSchema != "" {
			if err := p.Social.SetSocialSchema(ctx, user.ID, updated.SocialSchema); err != nil {
				p.Logger.WarnContext(ctx, "set social schema failed",
					"action", "set_social_schema", "user_id", user.ID, "error", err)
			} else {
				result.SocialSchemaUpdated = true
			}
		}
		if updated.BehavioralPriors != "" {
			if err := p.Social.SetBehavioralPriors(ctx, user.ID, updated.BehavioralPriors); err != nil {
				p.Logger.WarnContext(ctx, "set behavioral priors failed",
					"action", "set_behavioral_priors", "user_id", user.ID, "error", err)
			} else {
				result.BehavioralPriorsUpdated = true
			}
		}
	} else {
		p.Logger.DebugContext(ctx, "profile inference returned nil",
			"action", "infer_profile",
			"user_id", user.ID)
	}

	if err := p.Social.MarkAnalyzed(ctx, user.ID); err != nil {
		p.Logger.WarnContext(ctx, "mark analyzed failed",
			"action", "mark_analyzed", "user_id", user.ID, "error", err)
	}

	p.Logger.InfoContext(ctx, "analyzed user profile",
		"action", "analyze_user", "user_id", user.ID,
		"social_schema_updated", result.SocialSchemaUpdated,
		"behavioral_priors_updated", result.BehavioralPriorsUpdated,
		"new_memories", result.NewMemories)

	return result, nil
}

func (p *Profiler) buildSummary(ctx context.Context, user model.ContactProfile) (string, error) {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# User: %s (id=%s)\n", user.Name, user.ID)
	fmt.Fprintf(&sb, "Interactions: %d\n\n", user.InteractionCount)

	history, _ := p.Social.InteractionHistory(ctx, user.ID, 30)
	if len(history) > 0 {
		sb.WriteString("## Recent interactions\n")
		for _, h := range history {
			fmt.Fprintf(&sb, "- [%s] outcome=%s: %s\n", h.Type, h.Outcome, h.Notes)
		}
		sb.WriteString("\n")
	}

	memories, _ := p.Store.MemoriesByUser(ctx, user.ID, 200)
	if len(memories) > 0 {
		sb.WriteString("## Memories associated with this user\n")
		for _, m := range memories {
			tags := ""
			if len(m.Tags) > 0 {
				tags = " [" + strings.Join(m.Tags, ", ") + "]"
			}
			fmt.Fprintf(&sb, "- [%s]%s %s\n", m.Type, tags, m.Content)
		}
		sb.WriteString("\n")
	}

	workingMems, _ := p.Store.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE user_id = ? AND memory_type = 'working'
		  AND (expires_at IS NULL OR expires_at > datetime('now'))
		ORDER BY created_at DESC
		LIMIT 50`, user.ID)
	var humanContext []string
	for _, m := range workingMems {
		if m.Metadata["type"] == "conversation" {
			// Extract only the user prompt; strip the agent response section.
			content := m.Content
			if idx := strings.Index(content, "\n\n## Agent response"); idx >= 0 {
				content = content[:idx]
			}
			content = strings.TrimPrefix(content, "## User prompt\n")
			content = strings.TrimSpace(content)
			if content != "" {
				humanContext = append(humanContext, content)
			}
		}
	}
	if len(humanContext) > 0 {
		sb.WriteString("## Active working context\n")
		for _, c := range humanContext {
			fmt.Fprintf(&sb, "- %s\n", c)
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

package model

import (
	"context"
	"time"
)

// EntityType distinguishes agents from humans in social memory.
type EntityType string

const (
	EntityTypeAgent EntityType = "agent"
	EntityTypeHuman EntityType = "human"
)

// Interaction records a single exchange with a contact (agent or human).
type Interaction struct {
	ContactID   string            `json:"contact_id"`
	ContactName string            `json:"contact_name,omitempty"`
	EntityType  EntityType        `json:"entity_type"`
	Type        string            `json:"type"`
	Outcome     string            `json:"outcome"`
	Valence     float64           `json:"valence"` // agents only; must be 0 or omitted for humans
	Notes       string            `json:"notes,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ContactProfile is a summary view of a known contact (agent or human).
// Trust is only meaningful for agents; for humans it is always 0 and should not be used.
type ContactProfile struct {
	ID               string     `json:"id"`
	EntityType       EntityType `json:"entity_type"`
	Name             string     `json:"name"`
	Trust            float64    `json:"trust"`             // agents only; legacy single-dimension score
	CompetenceTrust  float64    `json:"competence_trust"`  // agents only; quality of outputs
	ReliabilityTrust float64    `json:"reliability_trust"` // agents only; behavioral predictability
	InteractionCount int        `json:"interaction_count"`
	MemoryCount      int        `json:"memory_count"`
	LastInteraction  time.Time  `json:"last_interaction"`
	AvgValence       float64    `json:"avg_valence"`
	Notes            string     `json:"notes"`
	SocialSchema     string     `json:"social_schema,omitempty"`
	BehavioralPriors string     `json:"behavioral_priors,omitempty"`
	ConfidenceTier   string     `json:"confidence_tier,omitempty"` // low/medium/high
}

// ProfileInference holds the LLM-generated social schema and behavioral priors text.
type ProfileInference struct {
	SocialSchema     string `json:"social_schema"`
	BehavioralPriors string `json:"behavioral_priors"`
}

// InferProfileFunc is a callback the caller provides so the profile analyzer
// can ask an LLM to write or update a contact's social schema and behavioral
// priors.
// It receives a structured summary (profile + interactions + memories) and
// the current inference, and returns an updated inference to store.
// Nil return means no update should be applied.
type InferProfileFunc func(ctx context.Context, userSummary string, current *ProfileInference) (*ProfileInference, error)

// ProfileAnalysis captures what happened during a single user profile analysis.
type ProfileAnalysis struct {
	UserID                  string `json:"user_id"`
	SocialSchemaUpdated     bool   `json:"social_schema_updated"`
	BehavioralPriorsUpdated bool   `json:"behavioral_priors_updated"`
	NewMemories             int    `json:"new_memories"`
	Skipped                 bool   `json:"skipped"`
	SkipReason              string `json:"skip_reason,omitempty"`
}

// EncodingResult is the structured output from processing a conversation turn.
type EncodingResult struct {
	Episode string   `json:"episode"`
	Facts   []string `json:"facts"`
	Tags    []string `json:"tags"`
	Outcome string   `json:"outcome"`
	Valence float64  `json:"valence"`
}

// EncoderFunc is a callback that extracts memories from a conversation turn.
// It receives the user prompt and agent response, and returns structured
// memories to store.
type EncoderFunc func(ctx context.Context, userPrompt, agentResponse string) (*EncodingResult, error)

// EncodingTurn is one conversation turn plus metadata, used as input to batch encoding.
type EncodingTurn struct {
	UserPrompt    string
	AgentResponse string
	UserID        string
	UserName      string
	Project       string
	Agent         string
}

// BatchEncoderFunc extracts memories from multiple conversation turns in one LLM call.
// It receives all turns and returns one or more EncodingResult. The LLM may merge
// similar turns (fewer results) or split (more results); the pipeline consumes all
// pending turns and stores the returned memories.
type BatchEncoderFunc func(ctx context.Context, turns []EncodingTurn) ([]EncodingResult, error)

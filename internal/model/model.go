// Package model defines the core types shared across all internal packages.
package model

import (
	"encoding/json"
	"errors"
	"time"
)

// Sentinel errors.
var (
	ErrNotFound = errors.New("brain: not found")
	ErrExpired  = errors.New("brain: memory expired")
)

// MemoryType discriminates the kind of memory stored.
type MemoryType string

const (
	MemoryTypeWorking    MemoryType = "working"
	MemoryTypeEpisodic   MemoryType = "episodic"
	MemoryTypeSemantic   MemoryType = "semantic"
	MemoryTypeProcedural MemoryType = "procedural"
	MemoryTypeGoal       MemoryType = "goal"
)

// Memory is the universal record stored in the brain.
type Memory struct {
	ID           int64             `json:"id"`
	Type         MemoryType        `json:"type"`
	Content      string            `json:"content"`
	Salience     float64           `json:"salience"`
	Retrievals   int               `json:"retrievals"`
	Tags         []string          `json:"tags,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	UserID       string            `json:"user_id,omitempty"`
	Agent        string            `json:"agent,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	LastAccessed time.Time         `json:"last_accessed"`
	ExpiresAt    *time.Time        `json:"expires_at,omitempty"`
}

// Episode represents a recorded event or experience.
type Episode struct {
	Content  string            `json:"content"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	UserID   string            `json:"user_id,omitempty"`
	Agent    string            `json:"agent,omitempty"`
}

// Fact is a piece of semantic knowledge.
type Fact struct {
	Content  string            `json:"content"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	UserID   string            `json:"user_id,omitempty"`
	Agent    string            `json:"agent,omitempty"`
}

// Procedure represents a learned sequence of actions.
type Procedure struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	SuccessRate float64  `json:"success_rate"`
	UseCount    int      `json:"use_count"`
}

// EpisodicArchive is a compressed record of an episodic memory stored before
// semanticization. Used by brain timeline to reconstruct narrative history.
type EpisodicArchive struct {
	ID         int64
	Content    string
	Tags       []string
	Metadata   map[string]string
	UserID     string
	Agent      string
	CreatedAt  time.Time
	ArchivedAt time.Time
	SemanticID int64 // non-zero if the archive was compressed into a semantic memory
}

// EpisodicSearchOpts configures an episodic memory search.
type EpisodicSearchOpts struct {
	Keyword     string // optional keyword for semantic search
	After       time.Time
	Before      time.Time
	Tags        []string
	MinSalience float64
	Limit       int
}

// SemanticSearchOpts configures a semantic memory search.
type SemanticSearchOpts struct {
	Keyword     string
	Tags        []string
	MinSalience float64
	Limit       int
}

// EncodeTags serializes a string slice to JSON for storage.
func EncodeTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// DecodeTags deserializes a JSON string to a string slice.
func DecodeTags(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil {
		return nil
	}
	return tags
}

// EncodeMetadata serializes metadata to JSON.
func EncodeMetadata(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// DecodeMetadata deserializes JSON to metadata map.
func DecodeMetadata(s string) map[string]string {
	if s == "" || s == "{}" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

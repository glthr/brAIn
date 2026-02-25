package model

import (
	"context"
	"time"
)

// ConsolidationStats captures what happened during a sleep cycle.
type ConsolidationStats struct {
	Timestamp    time.Time     `json:"timestamp"`
	Expired      int           `json:"expired"`
	Decayed      int           `json:"decayed"`
	Transferred  int           `json:"transferred"`
	Forgotten    int           `json:"forgotten"`
	Semanticized int           `json:"semanticized"`
	Embedded     int           `json:"embedded"`
	Duration     time.Duration `json:"duration"`
}

// SemanticizationFunc is a callback the caller provides so the consolidation engine
// can ask an LLM to compress old episodic memories into semantic knowledge.
// Semanticization (also called semantic consolidation) is the neuroscience term
// for the cognitive process by which episodic detail is compressed into
// generalizable semantic knowledge during systems consolidation.
type SemanticizationFunc func(ctx context.Context, memories []Memory) (*Memory, error)

// Standard metadata keys stored in the brain_meta table.
const (
	MetaKeySchemaVersion = "schema_version"
	MetaKeyCreatedAt     = "created_at"
	MetaKeyDescription   = "description"
	MetaKeyLastBackupAt  = "last_backup_at"
)

// EncodingStats captures what happened during a ProcessEncodings pass.
type EncodingStats struct {
	Processed    int `json:"processed"`
	Episodes     int `json:"episodes"`
	Facts        int `json:"facts"`
	Interactions int `json:"interactions"`
	Errors       int `json:"errors"`
}

// ConsolidationLogEntry is a single row from the consolidation_log table.
type ConsolidationLogEntry struct {
	ID        int64     `json:"id"`
	StatsJSON string    `json:"stats"`
	CreatedAt time.Time `json:"created_at"`
}

// BrainMeta is a snapshot of the brain's identity and configuration metadata.
type BrainMeta struct {
	SchemaVersion string `json:"schema_version"`
	CreatedAt     string `json:"created_at"`
	Description   string `json:"description,omitempty"`
	LastBackupAt  string `json:"last_backup_at,omitempty"`
}

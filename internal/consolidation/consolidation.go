// Package consolidation implements the "sleep cycle" memory maintenance engine.
package consolidation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/glthr/brAIn/internal/llm"
	"github.com/glthr/brAIn/internal/memory"
	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// Consolidator runs a single "sleep cycle" over the brain's memories.
type Consolidator struct {
	Store               *store.Store
	DB                  *sql.DB
	Social              *memory.SocialMemory
	DecayRate           float64
	SalienceFloor       float64
	SemanticizationFunc model.SemanticizationFunc
	SemanticizationAge  time.Duration
	EmbedFunc           llm.EmbedFunc // optional; backfills embeddings for un-embedded memories
	Logger              *slog.Logger
}

// Run executes all consolidation phases.
func (c *Consolidator) Run(ctx context.Context) (*model.ConsolidationStats, error) {
	start := time.Now()
	stats := &model.ConsolidationStats{Timestamp: start}
	c.Logger.InfoContext(ctx, "consolidation started", "action", "run")

	n, err := c.expireWorkingMemory(ctx)
	if err != nil {
		c.Logger.ErrorContext(ctx, "expire working memory failed", "action", "expire", "error", err)
		return stats, c.logAndReturn(ctx, stats, start, err)
	}
	stats.Expired = n
	if n > 0 {
		c.Logger.InfoContext(ctx, "expired working memories", "action", "expire", "count", n)
	}

	n, err = c.decaySalience(ctx)
	if err != nil {
		c.Logger.ErrorContext(ctx, "decay salience failed", "action", "decay", "error", err)
		return stats, c.logAndReturn(ctx, stats, start, err)
	}
	stats.Decayed = n
	if n > 0 {
		c.Logger.InfoContext(ctx, "decayed memory salience", "action", "decay", "count", n, "rate", c.DecayRate)
	}

	n, err = c.systemsConsolidation(ctx)
	if err != nil {
		c.Logger.ErrorContext(ctx, "systems consolidation failed", "action", "systems_consolidation", "error", err)
		return stats, c.logAndReturn(ctx, stats, start, err)
	}
	stats.Transferred = n
	if n > 0 {
		c.Logger.InfoContext(ctx, "systems consolidation: promoted working to episodic", "action", "systems_consolidation", "count", n)
	}

	n, err = c.forgetMemories(ctx)
	if err != nil {
		c.Logger.ErrorContext(ctx, "forget memories failed", "action", "forget", "error", err)
		return stats, c.logAndReturn(ctx, stats, start, err)
	}
	stats.Forgotten = n
	if n > 0 {
		c.Logger.InfoContext(ctx, "forgot low-salience memories", "action", "forget", "count", n, "floor", c.SalienceFloor)
	}

	if c.SemanticizationFunc != nil {
		n, err = c.semanticizeEpisodes(ctx, c.SemanticizationFunc)
		if err != nil {
			c.Logger.WarnContext(ctx, "semanticization error (non-fatal)", "action", "semanticize", "error", err)
		}
		stats.Semanticized = n
		if n > 0 {
			c.Logger.InfoContext(ctx, "semanticized old episodes into semantic memory", "action", "semanticize", "count", n)
		}
	}

	if c.EmbedFunc != nil {
		n, err = c.backfillEmbeddings(ctx, c.EmbedFunc)
		if err != nil {
			c.Logger.WarnContext(ctx, "embedding backfill error (non-fatal)", "action", "embed_backfill", "error", err)
		}
		stats.Embedded = n
		if n > 0 {
			c.Logger.InfoContext(ctx, "backfilled embeddings for un-embedded memories", "action", "embed_backfill", "count", n)
		}
	}

	_ = c.pruneConsolidationLog(ctx)

	stats.Duration = time.Since(start)
	_ = c.logStats(ctx, stats)
	c.Logger.InfoContext(ctx, "consolidation completed", "action", "run",
		"expired", stats.Expired, "decayed", stats.Decayed,
		"transferred", stats.Transferred, "forgotten", stats.Forgotten,
		"semanticized", stats.Semanticized,
		"duration", stats.Duration)
	return stats, nil
}

func (c *Consolidator) expireWorkingMemory(ctx context.Context) (int, error) {
	res, err := c.DB.ExecContext(ctx, `
		DELETE FROM memories
		WHERE memory_type = 'working' AND expires_at IS NOT NULL AND expires_at <= strftime('%Y-%m-%d %H:%M:%f', 'now')`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// decaySalience implements the spacing effect: memories with more retrievals
// decay slower, zero-retrieval memories decay faster.
// Pinned memories (metadata.pinned == "true") are exempt from decay.
func (c *Consolidator) decaySalience(ctx context.Context) (int, error) {
	type rule struct {
		minR, maxR int
		factor     float64
	}
	rules := []rule{
		{0, 0, 2.0},       // 0 retrievals: twice as fast (rapid forgetting)
		{1, 3, 1.0},       // 1–3 retrievals: baseline decay
		{4, 1 << 30, 0.4}, // 4+ retrievals: 60% slower (well-established)
	}

	total := 0
	for _, r := range rules {
		decay := 1.0 - c.DecayRate*r.factor
		if decay < 0.05 {
			// Clamp: even maximum decay can't zero out salience
			// in a single pass. A minimum multiplier of 0.05 ensures memories
			// decay gradually to the floor rather than disappearing instantly.
			decay = 0.05
		}
		res, err := c.DB.ExecContext(ctx, `
			UPDATE memories SET salience = salience * ?
			WHERE memory_type IN ('episodic', 'semantic', 'procedural')
			  AND last_accessed < datetime('now', '-1 day')
			  AND salience > ?
			  AND retrievals >= ?
			  AND retrievals <= ?
			  AND COALESCE(json_extract(metadata, '$.pinned'), '') != 'true'`,
			decay, c.SalienceFloor, r.minR, r.maxR)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

// systemsConsolidation promotes frequently-retrieved working memories to
// episodic memory — the systems-consolidation transfer from hippocampus to cortex.
// This implements hippocampal-cortical transfer during systems consolidation.
func (c *Consolidator) systemsConsolidation(ctx context.Context) (int, error) {
	res, err := c.DB.ExecContext(ctx, `
		UPDATE memories SET
			memory_type = 'episodic',
			expires_at = NULL,
			salience = MIN(salience + 0.2, 1.0)
		WHERE memory_type = 'working'
		  AND retrievals >= 3
		  AND (expires_at IS NULL OR expires_at > strftime('%Y-%m-%d %H:%M:%f', 'now'))`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// forgetMemories deletes memories below the salience floor — traces that have
// decayed below the retention threshold are lost (Ribot's law).
// Pinned memories are exempt from forgetting.
func (c *Consolidator) forgetMemories(ctx context.Context) (int, error) {
	res, err := c.DB.ExecContext(ctx, `
		DELETE FROM memories
		WHERE memory_type IN ('episodic', 'semantic')
		  AND salience <= ?
		  AND COALESCE(json_extract(metadata, '$.pinned'), '') != 'true'`,
		c.SalienceFloor)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// semanticizeEpisodes implements semanticization: converting episodic memories into semantic knowledge.
// This is part of systems consolidation where episodic detail is compressed into generalizable facts.
// Only calls the LLM if there are new episodic memories since the last consolidation that did semanticization.
func (c *Consolidator) semanticizeEpisodes(ctx context.Context, semanticizationFunc model.SemanticizationFunc) (int, error) {
	// Get the timestamp of the last consolidation that did semanticization
	lastSemanticizationTime, err := c.lastSemanticizationTime(ctx)
	if err != nil {
		c.Logger.WarnContext(ctx, "failed to get last semanticization time, proceeding anyway", "action", "semanticize", "error", err)
		lastSemanticizationTime = nil
	}

	// Check if there are new episodic memories since last semanticization
	if lastSemanticizationTime != nil {
		var newMemoryCount int
		errCount := c.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM memories
			WHERE memory_type = 'episodic'
			  AND created_at > ?`,
			lastSemanticizationTime.UTC().Format(store.SQLiteTimeFmtFrac)).Scan(&newMemoryCount)
		if errCount != nil {
			c.Logger.WarnContext(ctx, "failed to check for new memories, proceeding anyway", "action", "semanticize", "error", errCount)
		} else if newMemoryCount == 0 {
			c.Logger.InfoContext(ctx, "no new episodic memories since last semanticization, skipping LLM call", "action", "semanticize", "last_semanticization", *lastSemanticizationTime)
			return 0, nil
		}
		c.Logger.InfoContext(ctx, "found new episodic memories since last semanticization", "action", "semanticize", "new_count", newMemoryCount, "last_semanticization", *lastSemanticizationTime)
	}

	cutoff := time.Now().Add(-c.SemanticizationAge).UTC().Format(store.SQLiteTimeFmtFrac)
	memories, err := c.Store.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'episodic'
		  AND created_at < ?
		ORDER BY created_at ASC`, cutoff)
	if err != nil {
		return 0, err
	}
	if len(memories) < 1 {
		return 0, nil
	}

	groups := make(map[string][]model.Memory)
	for _, m := range memories {
		day := m.CreatedAt.Format("2006-01-02")
		groups[day] = append(groups[day], m)
	}

	abstracted := 0
	for _, group := range groups {
		if len(group) < 1 {
			continue
		}
		semantic, err := semanticizationFunc(ctx, group)
		if err != nil {
			c.Logger.WarnContext(ctx, "semanticization batch error", "action", "semanticize", "error", err)
			continue
		}
		if semantic == nil {
			continue
		}

		semantic.Type = model.MemoryTypeSemantic
		now := time.Now().UTC()
		semantic.CreatedAt = now
		semantic.LastAccessed = now
		if semantic.Salience == 0 {
			semantic.Salience = 0.7
		}

		// Propagate user_id if all source memories share the same one.
		if uid := commonUserID(group); uid != "" {
			semantic.UserID = uid
		}

		n, err := c.semanticizeGroup(ctx, semantic, group)
		if err != nil {
			c.Logger.WarnContext(ctx, "semanticize group commit failed", "action", "semanticize", "error", err)
			continue
		}
		abstracted += n
	}

	return abstracted, nil
}

// semanticizeGroup atomically archives the source episodes, inserts the
// semanticized memory (with source IDs in metadata), and deletes the originals.
func (c *Consolidator) semanticizeGroup(ctx context.Context, semantic *model.Memory, group []model.Memory) (int, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var expiresAt *string
	if semantic.ExpiresAt != nil {
		t := semantic.ExpiresAt.UTC().Format(store.SQLiteTimeFmt)
		expiresAt = &t
	}

	var userID *string
	if semantic.UserID != "" {
		userID = &semantic.UserID
	}

	// Embed source episode IDs in the semantic memory's metadata so brain explain
	// can trace provenance.
	if semantic.Metadata == nil {
		semantic.Metadata = make(map[string]string)
	}
	sourceIDs := make([]string, len(group))
	for i, m := range group {
		sourceIDs[i] = fmt.Sprintf("%d", m.ID)
	}
	semantic.Metadata["source_ids"] = strings.Join(sourceIDs, ",")

	res, err := tx.ExecContext(ctx, `
		INSERT INTO memories (memory_type, content, salience, retrievals, tags, metadata, user_id, created_at, last_accessed, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(semantic.Type),
		semantic.Content,
		semantic.Salience,
		semantic.Retrievals,
		model.EncodeTags(semantic.Tags),
		model.EncodeMetadata(semantic.Metadata),
		userID,
		semantic.CreatedAt.UTC().Format(store.SQLiteTimeFmt),
		semantic.LastAccessed.UTC().Format(store.SQLiteTimeFmt),
		expiresAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert semanticized memory: %w", err)
	}
	semanticID, _ := res.LastInsertId()

	// Archive the source episodes before deletion so timeline reconstruction
	// remains possible. This mirrors hippocampal trace preservation.
	if err := c.Store.ArchiveEpisodes(ctx, tx, group, semanticID); err != nil {
		c.Logger.WarnContext(ctx, "archive episodes failed (non-fatal, continuing)", "action", "semanticize", "error", err)
	}

	for _, m := range group {
		if _, err := tx.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, m.ID); err != nil {
			return 0, fmt.Errorf("delete original id=%d: %w", m.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit semanticization tx: %w", err)
	}
	return len(group), nil
}

// backfillEmbeddings embeds any long-term memories that don't yet have an embedding vector.
// Processes up to 50 memories per consolidation cycle to avoid long pauses.
func (c *Consolidator) backfillEmbeddings(ctx context.Context, embedFunc llm.EmbedFunc) (int, error) {
	pending, err := c.Store.MemoriesWithoutEmbedding(ctx, 50)
	if err != nil {
		return 0, fmt.Errorf("query un-embedded memories: %w", err)
	}
	if len(pending) == 0 {
		return 0, nil
	}

	embedded := 0
	for _, m := range pending {
		vec, embedErr := embedFunc(ctx, m.Content)
		if embedErr != nil {
			c.Logger.WarnContext(ctx, "embed memory failed (skipping)", "action", "embed_backfill", "id", m.ID, "error", embedErr)
			continue
		}
		if upsertErr := c.Store.UpsertEmbedding(ctx, m.ID, vec); upsertErr != nil {
			c.Logger.WarnContext(ctx, "upsert embedding failed", "action", "embed_backfill", "id", m.ID, "error", upsertErr)
			continue
		}
		embedded++
	}
	return embedded, nil
}

const maxConsolidationLogEntries = 500

func (c *Consolidator) pruneConsolidationLog(ctx context.Context) error {
	_, err := c.DB.ExecContext(ctx, `
		DELETE FROM consolidation_log
		WHERE id NOT IN (
			SELECT id FROM consolidation_log ORDER BY id DESC LIMIT ?
		)`, maxConsolidationLogEntries)
	return err
}

func (c *Consolidator) logStats(ctx context.Context, stats *model.ConsolidationStats) error {
	b, _ := json.Marshal(stats)
	_, err := c.DB.ExecContext(ctx, `
		INSERT INTO consolidation_log (stats, created_at) VALUES (?, datetime('now'))`, string(b))
	return err
}

func (c *Consolidator) logAndReturn(ctx context.Context, stats *model.ConsolidationStats, start time.Time, origErr error) error {
	stats.Duration = time.Since(start)
	_ = c.logStats(ctx, stats)
	return origErr
}

// lastSemanticizationTime returns the timestamp of the most recent consolidation
// that performed semanticization (i.e., had Semanticized > 0).
// Returns nil if no semanticization has occurred yet.
func (c *Consolidator) lastSemanticizationTime(ctx context.Context) (*time.Time, error) {
	rows, err := c.DB.QueryContext(ctx, `
		SELECT stats, created_at FROM consolidation_log
		ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var statsJSON, createdAtStr string
		if err := rows.Scan(&statsJSON, &createdAtStr); err != nil {
			continue
		}

		var stats model.ConsolidationStats
		if err := json.Unmarshal([]byte(statsJSON), &stats); err != nil {
			continue
		}

		// Found a consolidation that did semanticization
		if stats.Semanticized > 0 {
			createdAt := store.ParseTime(createdAtStr)
			return &createdAt, nil
		}
	}

	// No semanticization found in recent consolidation logs
	return nil, nil
}

// commonUserID returns the shared user_id if every memory in the group
// belongs to the same user. Returns "" if there is no user or mixed users.
func commonUserID(group []model.Memory) string {
	uid := ""
	for _, m := range group {
		if m.UserID == "" {
			return ""
		}
		if uid == "" {
			uid = m.UserID
		} else if uid != m.UserID {
			return ""
		}
	}
	return uid
}

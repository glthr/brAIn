package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/glthr/brAIn/internal/llm"
	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// EpisodicMemory stores timestamped events and experiences.
type EpisodicMemory struct {
	s         *store.Store
	logger    *slog.Logger
	EmbedFunc llm.EmbedFunc // required for keyword search; enables semantic similarity search
}

// NewEpisodicMemory creates an EpisodicMemory instance.
func NewEpisodicMemory(s *store.Store, logger *slog.Logger) *EpisodicMemory {
	return &EpisodicMemory{s: s, logger: logger}
}

// Record saves a new episode with default salience (0.5).
func (e *EpisodicMemory) Record(ctx context.Context, ep model.Episode) (int64, error) {
	return e.RecordWithImportance(ctx, ep, 0.5)
}

// RecordWithImportance saves an episode with a specific salience level (0.0 - 1.0).
// When EmbedFunc is configured the content is embedded and stored alongside the episode.
func (e *EpisodicMemory) RecordWithImportance(ctx context.Context, ep model.Episode, salience float64) (int64, error) {
	now := time.Now().UTC()
	m := &model.Memory{
		Type:         model.MemoryTypeEpisodic,
		Content:      ep.Content,
		Salience:     salience,
		Tags:         ep.Tags,
		Metadata:     ep.Metadata,
		UserID:       ep.UserID,
		Agent:        ep.Agent,
		CreatedAt:    now,
		LastAccessed: now,
	}
	id, err := e.s.InsertMemory(ctx, m)
	if err != nil {
		e.logger.ErrorContext(ctx, "record failed", "action", "record", "error", err)
		return 0, err
	}
	e.logger.InfoContext(ctx, "recorded episode", "action", "record", "id", id, "salience", salience, "tags", ep.Tags)

	// Embed the content asynchronously-safe (best-effort, non-blocking caller).
	if e.EmbedFunc != nil {
		if vec, embedErr := e.EmbedFunc(ctx, ep.Content); embedErr == nil {
			if upsertErr := e.s.UpsertEmbedding(ctx, id, vec); upsertErr != nil {
				e.logger.WarnContext(ctx, "store embedding failed", "action", "record", "id", id, "error", upsertErr)
			}
		} else {
			e.logger.WarnContext(ctx, "embed content failed", "action", "record", "id", id, "error", embedErr)
		}
	}

	return id, nil
}

// Recall retrieves an episode by ID and potentiates it.
func (e *EpisodicMemory) Recall(ctx context.Context, id int64) (*model.Memory, error) {
	e.logger.InfoContext(ctx, "recalling episode", "action", "recall", "id", id)
	m, err := e.s.GetMemory(ctx, id)
	if err != nil {
		e.logger.ErrorContext(ctx, "recall failed", "action", "recall", "id", id, "error", err)
		return nil, err
	}
	if m.Type != model.MemoryTypeEpisodic {
		e.logger.WarnContext(ctx, "memory is not episodic type", "action", "recall", "id", id, "actual_type", m.Type)
		return nil, model.ErrNotFound
	}
	_ = e.s.Potentiate(ctx, id)
	return m, nil
}

// Recent returns the latest episodes.
func (e *EpisodicMemory) Recent(ctx context.Context, limit int) ([]model.Memory, error) {
	e.logger.InfoContext(ctx, "retrieving recent episodes", "action", "recent", "limit", limit)
	results, err := e.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'episodic'
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		e.logger.ErrorContext(ctx, "recent failed", "action", "recent", "error", err)
		return nil, err
	}
	e.logger.InfoContext(ctx, "retrieved episodes", "action", "recent", "count", len(results))
	return results, nil
}

// Count returns the number of episodic memories.
func (e *EpisodicMemory) Count(ctx context.Context) (int, error) {
	var count int
	err := e.s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM memories WHERE memory_type = 'episodic'`).Scan(&count)
	return count, err
}

// Lookup finds episodic memories matching the keyword.
// When EmbedFunc is configured, uses cosine similarity for semantic search.
// When EmbedFunc is not configured, falls back to keyword search using SQLite LIKE.
func (e *EpisodicMemory) Lookup(ctx context.Context, keyword string, limit int) ([]model.Memory, error) {
	e.logger.InfoContext(ctx, "looking up episodes", "action", "lookup", "keyword", keyword, "limit", limit)

	if e.EmbedFunc != nil {
		return e.lookupEmbeddings(ctx, keyword, limit)
	}
	return e.keywordFallback(ctx, keyword, limit)
}

// keywordFallback performs a lightweight LIKE search when no embedding model
// is configured. Searches both content and tags, ordered by salience.
func (e *EpisodicMemory) keywordFallback(ctx context.Context, keyword string, limit int) ([]model.Memory, error) {
	pattern := "%" + strings.ReplaceAll(keyword, "%", "\\%") + "%"
	results, err := e.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'episodic'
		  AND (content LIKE ? OR tags LIKE ?)
		ORDER BY salience DESC, created_at DESC
		LIMIT ?`, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	e.logger.InfoContext(ctx, "keyword fallback completed", "action", "lookup", "count", len(results))
	return results, nil
}

// lookupEmbeddings performs semantic similarity search using embeddings.
func (e *EpisodicMemory) lookupEmbeddings(ctx context.Context, keyword string, limit int) ([]model.Memory, error) {
	// 1. Embed the query.
	queryVec, err := e.EmbedFunc(ctx, keyword)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// 2. Load all episodic embeddings and compute cosine similarity.
	embeds, err := e.s.SemanticEmbeddings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load embeddings: %w", err)
	}
	type cosScore struct {
		id  int64
		sim float64
	}
	cosScores := make([]cosScore, 0, len(embeds))
	for _, emb := range embeds {
		// Filter to only episodic memories
		mem, memErr := e.s.GetMemory(ctx, emb.ID)
		if memErr != nil || mem.Type != model.MemoryTypeEpisodic {
			continue
		}
		sim := store.CosineSimilarity(queryVec, emb.Vec)
		cosScores = append(cosScores, cosScore{emb.ID, sim})
	}
	// Sort by cosine descending, keep top candidates.
	sort.Slice(cosScores, func(i, j int) bool { return cosScores[i].sim > cosScores[j].sim })
	const maxKNN = 200
	if len(cosScores) > maxKNN {
		cosScores = cosScores[:maxKNN]
	}

	// 3. Fetch candidate memories from DB (only episodic type).
	if len(cosScores) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(cosScores))
	for _, cs := range cosScores {
		ids = append(ids, cs.id)
	}
	memories, err := e.fetchMemoriesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("fetch candidates: %w", err)
	}
	if len(memories) == 0 {
		return nil, nil
	}

	// 4. Build cosine similarity map for scoring.
	cosMap := make(map[int64]float64, len(cosScores))
	for _, cs := range cosScores {
		cosMap[cs.id] = cs.sim
	}

	// 5. Score by cosine similarity × (0.5 + salience) × recencyFactor.
	type scored struct {
		mem   model.Memory
		score float64
	}
	candidates := make([]scored, 0, len(memories))
	for _, m := range memories {
		sim, ok := cosMap[m.ID]
		if !ok {
			continue
		}
		if sim < 0 {
			sim = 0
		}
		// recencyFactor gives a mild boost to recently accessed memories.
		score := sim * (0.5 + m.Salience) * recencyFactor(m.LastAccessed)
		candidates = append(candidates, scored{m, score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	results := make([]model.Memory, len(candidates))
	for i, c := range candidates {
		results[i] = c.mem
	}
	e.logger.InfoContext(ctx, "lookup completed", "action", "lookup", "count", len(results))
	return results, nil
}

// fetchMemoriesByIDs fetches episodic memories filtered to the provided IDs.
func (e *EpisodicMemory) fetchMemoriesByIDs(ctx context.Context, ids []int64) ([]model.Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Fetch all episodic memories and filter in Go (safe for typical brain sizes).
	all, err := e.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = ?`, string(model.MemoryTypeEpisodic))
	if err != nil {
		return nil, err
	}
	idSet := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	filtered := make([]model.Memory, 0, len(ids))
	for _, m := range all {
		if _, ok := idSet[m.ID]; ok {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// Search finds episodes matching a time range, keyword, and/or tags.
// When keyword is provided, uses semantic similarity search via embeddings.
func (e *EpisodicMemory) Search(ctx context.Context, opts model.EpisodicSearchOpts) ([]model.Memory, error) {
	e.logger.InfoContext(ctx, "searching episodes", "action", "search", "keyword", opts.Keyword, "tags", opts.Tags, "limit", opts.Limit)

	// If keyword is provided, use semantic similarity search.
	if opts.Keyword != "" && e.EmbedFunc != nil {
		results, err := e.Lookup(ctx, opts.Keyword, opts.Limit)
		if err != nil {
			return nil, err
		}
		// Apply additional filters (tags, time range, salience) to the results.
		filtered := e.filterEpisodes(results, opts)
		return filtered, nil
	}

	// Otherwise, use traditional SQL-based search (time range, tags, salience only).
	query := `
		SELECT ` + store.MemoryCols + `
		FROM memories
		WHERE memory_type = 'episodic'`
	args := []interface{}{}

	if !opts.After.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, opts.After.UTC().Format(store.SQLiteTimeFmt))
	}
	if !opts.Before.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, opts.Before.UTC().Format(store.SQLiteTimeFmt))
	}
	if opts.MinSalience > 0 {
		query += ` AND salience >= ?`
		args = append(args, opts.MinSalience)
	}
	if len(opts.Tags) > 0 {
		for _, tag := range opts.Tags {
			query += ` AND EXISTS (SELECT 1 FROM json_each(tags) WHERE value = ?)`
			args = append(args, tag)
		}
	}

	query += ` ORDER BY created_at DESC`
	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}

	results, err := e.s.QueryMemories(ctx, query, args...)
	if err != nil {
		e.logger.ErrorContext(ctx, "search failed", "action", "search", "error", err)
		return nil, err
	}
	e.logger.InfoContext(ctx, "search completed", "action", "search", "count", len(results))
	return results, nil
}

// filterEpisodes applies time range, tags, and salience filters to a list of episodes.
func (e *EpisodicMemory) filterEpisodes(episodes []model.Memory, opts model.EpisodicSearchOpts) []model.Memory {
	filtered := make([]model.Memory, 0, len(episodes))
	for _, ep := range episodes {
		// Time range filter
		if !opts.After.IsZero() && ep.CreatedAt.Before(opts.After) {
			continue
		}
		if !opts.Before.IsZero() && ep.CreatedAt.After(opts.Before) {
			continue
		}
		// Salience filter
		if opts.MinSalience > 0 && ep.Salience < opts.MinSalience {
			continue
		}
		// Tags filter
		if len(opts.Tags) > 0 {
			tagMatch := false
			for _, searchTag := range opts.Tags {
				for _, epTag := range ep.Tags {
					if epTag == searchTag {
						tagMatch = true
						break
					}
				}
				if tagMatch {
					break
				}
			}
			if !tagMatch {
				continue
			}
		}
		filtered = append(filtered, ep)
	}
	// Apply limit
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}
	return filtered
}

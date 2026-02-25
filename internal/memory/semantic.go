package memory

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"log/slog"

	"github.com/glthr/brAIn/internal/llm"
	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// SemanticMemory stores facts and knowledge, searchable by keyword and tags.
type SemanticMemory struct {
	s         *store.Store
	logger    *slog.Logger
	EmbedFunc llm.EmbedFunc // optional; enables semantic similarity search
}

// NewSemanticMemory creates a SemanticMemory instance.
func NewSemanticMemory(s *store.Store, logger *slog.Logger) *SemanticMemory {
	return &SemanticMemory{s: s, logger: logger}
}

// Store saves a new fact/concept into semantic memory.
// If a fact with identical content already exists, the existing ID is returned
// and potentiation is recorded instead of creating a duplicate.
func (sm *SemanticMemory) Store(ctx context.Context, fact model.Fact) (int64, error) {
	return sm.StoreWithImportance(ctx, fact, 0.6)
}

// StoreWithImportance saves a fact with a specific salience level.
// If a fact with identical content already exists, the existing ID is returned
// and potentiation is recorded instead of creating a duplicate.
// When EmbedFunc is configured the content is embedded and stored alongside the fact.
func (sm *SemanticMemory) StoreWithImportance(ctx context.Context, fact model.Fact, salience float64) (int64, error) {
	if existing, err := sm.s.ContentExists(ctx, model.MemoryTypeSemantic, fact.Content); err == nil && existing != 0 {
		_ = sm.s.Potentiate(ctx, existing)
		sm.logger.InfoContext(ctx, "fact already exists, potentiated", "action", "store", "id", existing)
		return existing, nil
	}

	now := time.Now().UTC()
	m := &model.Memory{
		Type:         model.MemoryTypeSemantic,
		Content:      fact.Content,
		Salience:     salience,
		Tags:         fact.Tags,
		Metadata:     fact.Metadata,
		UserID:       fact.UserID,
		Agent:        fact.Agent,
		CreatedAt:    now,
		LastAccessed: now,
	}
	id, err := sm.s.InsertMemory(ctx, m)
	if err != nil {
		sm.logger.ErrorContext(ctx, "store failed", "action", "store", "error", err)
		return 0, err
	}
	sm.logger.InfoContext(ctx, "stored fact", "action", "store", "id", id, "salience", salience, "tags", fact.Tags)

	// Embed the content asynchronously-safe (best-effort, non-blocking caller).
	if sm.EmbedFunc != nil {
		if vec, embedErr := sm.EmbedFunc(ctx, fact.Content); embedErr == nil {
			if upsertErr := sm.s.UpsertEmbedding(ctx, id, vec); upsertErr != nil {
				sm.logger.WarnContext(ctx, "store embedding failed", "action", "store", "id", id, "error", upsertErr)
			}
		} else {
			sm.logger.WarnContext(ctx, "embed content failed", "action", "store", "id", id, "error", embedErr)
		}
	}

	return id, nil
}

// Lookup finds semantic memories matching the keyword.
// When EmbedFunc is configured, uses cosine similarity for semantic search
// (e.g. "concurrency" matching a "goroutine" fact).
// When EmbedFunc is not configured, falls back to keyword search using
// SQLite LIKE — enabling offline/air-gapped use without an embedding model.
func (sm *SemanticMemory) Lookup(ctx context.Context, keyword string, limit int) ([]model.Memory, error) {
	sm.logger.InfoContext(ctx, "looking up facts", "action", "lookup", "keyword", keyword, "limit", limit)

	if sm.EmbedFunc != nil {
		return sm.lookupEmbeddings(ctx, keyword, limit)
	}
	return sm.keywordFallback(ctx, keyword, limit)
}

// keywordFallback performs a lightweight LIKE search when no embedding model
// is configured. Searches both content and tags, ordered by salience.
func (sm *SemanticMemory) keywordFallback(ctx context.Context, keyword string, limit int) ([]model.Memory, error) {
	pattern := "%" + strings.ReplaceAll(keyword, "%", "\\%") + "%"
	results, err := sm.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'semantic'
		  AND (content LIKE ? OR tags LIKE ?)
		ORDER BY salience DESC
		LIMIT ?`, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	sm.logger.InfoContext(ctx, "keyword fallback completed", "action", "lookup", "count", len(results))
	return results, nil
}

// lookupEmbeddings performs semantic similarity search using embeddings with
// a three-factor scoring formula: score = cosine_sim * (0.5 + salience) * recencyFactor.
func (sm *SemanticMemory) lookupEmbeddings(ctx context.Context, keyword string, limit int) ([]model.Memory, error) {
	// 1. Embed the query.
	queryVec, err := sm.EmbedFunc(ctx, keyword)
	if err != nil {
		return nil, err
	}

	// 2. Load all semantic embeddings and compute cosine similarity.
	embeds, err := sm.s.SemanticEmbeddings(ctx)
	if err != nil {
		return nil, err
	}
	type cosScore struct {
		id  int64
		sim float64
	}
	cosScores := make([]cosScore, 0, len(embeds))
	for _, e := range embeds {
		// Filter to only semantic memories
		mem, memErr := sm.s.GetMemory(ctx, e.ID)
		if memErr != nil || mem.Type != model.MemoryTypeSemantic {
			continue
		}
		sim := store.CosineSimilarity(queryVec, e.Vec)
		cosScores = append(cosScores, cosScore{e.ID, sim})
	}
	// Sort by cosine descending, keep top candidates.
	sort.Slice(cosScores, func(i, j int) bool { return cosScores[i].sim > cosScores[j].sim })
	const maxKNN = 200
	if len(cosScores) > maxKNN {
		cosScores = cosScores[:maxKNN]
	}

	// 3. Fetch candidate memories from DB.
	if len(cosScores) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(cosScores))
	for _, cs := range cosScores {
		ids = append(ids, cs.id)
	}
	memories, err := sm.fetchMemoriesByIDs(ctx, ids, model.MemoryTypeSemantic)
	if err != nil {
		return nil, err
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
	// The recency factor decays from 1.0 to 0.5 over a ~30-day half-life,
	// making recently accessed memories rank higher when scores are otherwise close.
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
	sm.logger.InfoContext(ctx, "lookup completed", "action", "lookup", "count", len(results))
	return results, nil
}

// recencyFactor returns a multiplier in [0.5, 1.0] that decays with the age
// of the last access. Uses a 30-day half-life: a memory accessed today gets
// 1.0; one not accessed in 30 days gets ~0.75; after many months it approaches 0.5.
func recencyFactor(lastAccessed time.Time) float64 {
	age := time.Since(lastAccessed)
	// half-life = 30 days expressed in hours
	const halfLifeHours = 30.0 * 24.0
	hours := age.Hours()
	if hours < 0 {
		hours = 0
	}
	return 0.5 + 0.5*math.Exp(-hours*math.Log(2)/halfLifeHours)
}

// fetchMemoriesByIDs fetches memories of the given type filtered to the provided IDs.
func (sm *SemanticMemory) fetchMemoriesByIDs(ctx context.Context, ids []int64, memType model.MemoryType) ([]model.Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	all, err := sm.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = ?`, string(memType))
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

// Search finds semantic memories matching keyword, tags, and/or salience filters.
// When keyword is provided, uses semantic similarity search via embeddings (or
// falls back to keyword search when no EmbedFunc is configured).
func (sm *SemanticMemory) Search(ctx context.Context, opts model.SemanticSearchOpts) ([]model.Memory, error) {
	sm.logger.InfoContext(ctx, "searching facts", "action", "search", "keyword", opts.Keyword, "tags", opts.Tags, "limit", opts.Limit)

	// If keyword is provided, use semantic similarity search (or fallback).
	if opts.Keyword != "" {
		results, err := sm.Lookup(ctx, opts.Keyword, opts.Limit)
		if err != nil {
			return nil, err
		}
		// Apply additional filters (tags, salience) to the results.
		filtered := sm.filterFacts(results, opts)
		return filtered, nil
	}

	// Otherwise, use traditional SQL-based search (tags and salience only).
	query := `
		SELECT ` + store.MemoryCols + `
		FROM memories
		WHERE memory_type = 'semantic'`
	args := []interface{}{}

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

	query += ` ORDER BY salience DESC`
	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}

	results, err := sm.s.QueryMemories(ctx, query, args...)
	if err != nil {
		sm.logger.ErrorContext(ctx, "search failed", "action", "search", "error", err)
		return nil, err
	}
	sm.logger.InfoContext(ctx, "search completed", "action", "search", "count", len(results))
	return results, nil
}

// ContextBudget returns a ranked, deduplicated set of memories that fit within
// the given token budget. Token count is estimated as len(content)/4.
// The caller can insert the result directly into an LLM context window.
func (sm *SemanticMemory) ContextBudget(ctx context.Context, topic string, tokenBudget int) ([]model.Memory, error) {
	// Fetch a generous set of candidates.
	candidates, err := sm.Lookup(ctx, topic, 50)
	if err != nil {
		return nil, err
	}

	// Deduplicate: collapse near-duplicates (same first 100 chars, lowercased) to
	// the first-seen version (already ranked highest by the lookup).
	seen := make(map[string]bool)
	deduped := make([]model.Memory, 0, len(candidates))
	for _, m := range candidates {
		key := strings.ToLower(strings.Join(strings.Fields(m.Content), " "))
		if len(key) > 100 {
			key = key[:100]
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, m)
	}

	// Fit within budget.
	fitted := make([]model.Memory, 0, len(deduped))
	remaining := tokenBudget
	for _, m := range deduped {
		tokens := estimateTokens(m.Content)
		if tokens > remaining {
			break
		}
		fitted = append(fitted, m)
		remaining -= tokens
	}
	return fitted, nil
}

// estimateTokens approximates the token count for a string (GPT-style, ~4 chars/token).
func estimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 {
		n = 1
	}
	return n
}

// filterFacts applies tags and salience filters to a list of facts.
func (sm *SemanticMemory) filterFacts(facts []model.Memory, opts model.SemanticSearchOpts) []model.Memory {
	filtered := make([]model.Memory, 0, len(facts))
	for _, fact := range facts {
		// Salience filter
		if opts.MinSalience > 0 && fact.Salience < opts.MinSalience {
			continue
		}
		// Tags filter
		if len(opts.Tags) > 0 {
			tagMatch := false
			for _, searchTag := range opts.Tags {
				for _, factTag := range fact.Tags {
					if factTag == searchTag {
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
		filtered = append(filtered, fact)
	}
	// Apply limit
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}
	return filtered
}

// All returns all semantic memories, ordered by salience.
func (sm *SemanticMemory) All(ctx context.Context) ([]model.Memory, error) {
	sm.logger.InfoContext(ctx, "retrieving all facts", "action", "all")
	return sm.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'semantic'
		ORDER BY salience DESC`)
}

// Count returns the number of semantic memories.
func (sm *SemanticMemory) Count(ctx context.Context) (int, error) {
	var count int
	err := sm.s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM memories WHERE memory_type = 'semantic'`).Scan(&count)
	return count, err
}

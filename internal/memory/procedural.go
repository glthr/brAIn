package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/glthr/brAIn/internal/llm"
	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// ProceduralMemory stores learned action sequences and strategies.
type ProceduralMemory struct {
	s         *store.Store
	logger    *slog.Logger
	EmbedFunc llm.EmbedFunc // required for keyword search; enables semantic similarity search
}

// NewProceduralMemory creates a ProceduralMemory instance.
func NewProceduralMemory(s *store.Store, logger *slog.Logger) *ProceduralMemory {
	return &ProceduralMemory{s: s, logger: logger}
}

// Store saves a new procedure.
func (p *ProceduralMemory) Store(ctx context.Context, proc model.Procedure) (int64, error) {
	now := time.Now().UTC()

	stepsJSON, _ := json.Marshal(proc.Steps)
	meta := map[string]string{
		"name":         proc.Name,
		"description":  proc.Description,
		"steps":        string(stepsJSON),
		"success_rate": formatFloat(proc.SuccessRate),
		"use_count":    formatInt(proc.UseCount),
	}

	m := &model.Memory{
		Type:         model.MemoryTypeProcedural,
		Content:      proc.Name + ": " + proc.Description,
		Salience:     0.5 + proc.SuccessRate*0.5,
		Tags:         []string{"procedure"},
		Metadata:     meta,
		CreatedAt:    now,
		LastAccessed: now,
	}
	id, err := p.s.InsertMemory(ctx, m)
	if err != nil {
		p.logger.ErrorContext(ctx, "store failed", "action", "store", "name", proc.Name, "error", err)
		return 0, err
	}
	p.logger.InfoContext(ctx, "stored procedure", "action", "store", "id", id, "name", proc.Name, "steps", len(proc.Steps))

	// Embed the content asynchronously-safe (best-effort, non-blocking caller).
	if p.EmbedFunc != nil {
		if vec, embedErr := p.EmbedFunc(ctx, m.Content); embedErr == nil {
			if upsertErr := p.s.UpsertEmbedding(ctx, id, vec); upsertErr != nil {
				p.logger.WarnContext(ctx, "store embedding failed", "action", "store", "id", id, "error", upsertErr)
			}
		} else {
			p.logger.WarnContext(ctx, "embed content failed", "action", "store", "id", id, "error", embedErr)
		}
	}

	return id, nil
}

// StoreEmerging saves a procedure that was extracted from conversation or consolidation (emerging skill).
// It is stored with tags ["procedure", "emerging"] so it can be listed separately via Emerging().
func (p *ProceduralMemory) StoreEmerging(ctx context.Context, proc model.Procedure) (int64, error) {
	now := time.Now().UTC()

	stepsJSON, _ := json.Marshal(proc.Steps)
	meta := map[string]string{
		"name":         proc.Name,
		"description":  proc.Description,
		"steps":        string(stepsJSON),
		"success_rate": formatFloat(proc.SuccessRate),
		"use_count":    formatInt(proc.UseCount),
	}

	m := &model.Memory{
		Type:         model.MemoryTypeProcedural,
		Content:      proc.Name + ": " + proc.Description,
		Salience:     0.5 + proc.SuccessRate*0.5,
		Tags:         []string{"procedure", "emerging"},
		Metadata:     meta,
		CreatedAt:    now,
		LastAccessed: now,
	}
	id, err := p.s.InsertMemory(ctx, m)
	if err != nil {
		p.logger.ErrorContext(ctx, "store emerging failed", "action", "store_emerging", "name", proc.Name, "error", err)
		return 0, err
	}
	p.logger.InfoContext(ctx, "stored emerging skill", "action", "store_emerging", "id", id, "name", proc.Name, "steps", len(proc.Steps))

	// Embed the content asynchronously-safe (best-effort, non-blocking caller).
	if p.EmbedFunc != nil {
		if vec, embedErr := p.EmbedFunc(ctx, m.Content); embedErr == nil {
			if upsertErr := p.s.UpsertEmbedding(ctx, id, vec); upsertErr != nil {
				p.logger.WarnContext(ctx, "store embedding failed", "action", "store_emerging", "id", id, "error", upsertErr)
			}
		} else {
			p.logger.WarnContext(ctx, "embed content failed", "action", "store_emerging", "id", id, "error", embedErr)
		}
	}

	return id, nil
}

// Lookup finds a procedure by name.
func (p *ProceduralMemory) Lookup(ctx context.Context, name string) (*model.Procedure, error) {
	p.logger.InfoContext(ctx, "looking up procedure", "action", "lookup", "name", name)
	memories, err := p.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'procedural' AND json_extract(metadata, '$.name') = ?
		LIMIT 1`, name)
	if err != nil {
		p.logger.ErrorContext(ctx, "lookup failed", "action", "lookup", "name", name, "error", err)
		return nil, err
	}
	if len(memories) == 0 {
		p.logger.InfoContext(ctx, "procedure not found", "action", "lookup", "name", name)
		return nil, model.ErrNotFound
	}

	_ = p.s.Potentiate(ctx, memories[0].ID)
	return memoryToProcedure(&memories[0]), nil
}

// Search finds procedures matching the query using semantic similarity search.
// Requires EmbedFunc to be configured. Uses cosine similarity to find semantically
// similar procedures (e.g. "convert to PDF" matching a "LibreOffice export" procedure).
func (p *ProceduralMemory) Search(ctx context.Context, query string, limit int) ([]model.Procedure, error) {
	p.logger.InfoContext(ctx, "searching procedures", "action", "search", "query", query, "limit", limit)

	if p.EmbedFunc == nil {
		return nil, fmt.Errorf("search requires EmbedFunc to be configured")
	}

	if limit <= 0 {
		limit = 5
	}

	// 1. Embed the query.
	queryVec, err := p.EmbedFunc(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// 2. Load all procedural embeddings and compute cosine similarity.
	embeds, err := p.s.SemanticEmbeddings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load embeddings: %w", err)
	}
	type cosScore struct {
		id  int64
		sim float64
	}
	cosScores := make([]cosScore, 0, len(embeds))
	for _, emb := range embeds {
		// Filter to only procedural memories
		mem, memErr := p.s.GetMemory(ctx, emb.ID)
		if memErr != nil || mem.Type != model.MemoryTypeProcedural {
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

	// 3. Fetch candidate memories from DB (only procedural type).
	if len(cosScores) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(cosScores))
	for _, cs := range cosScores {
		ids = append(ids, cs.id)
	}
	memories, err := p.fetchMemoriesByIDs(ctx, ids)
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

	// 5. Score and sort by cosine similarity weighted by salience.
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
		// Cosine similarity is typically in [0,1] for similar texts.
		// Clip to [0,1] to handle edge cases.
		if sim < 0 {
			sim = 0
		}
		// Score combines cosine similarity with salience.
		score := sim * (0.5 + m.Salience)
		candidates = append(candidates, scored{m, score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	procs := make([]model.Procedure, len(candidates))
	for i, c := range candidates {
		procs[i] = *memoryToProcedure(&c.mem)
	}
	p.logger.InfoContext(ctx, "search completed", "action", "search", "count", len(procs))
	return procs, nil
}

// fetchMemoriesByIDs fetches procedural memories filtered to the provided IDs.
func (p *ProceduralMemory) fetchMemoriesByIDs(ctx context.Context, ids []int64) ([]model.Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Fetch all procedural memories and filter in Go (safe for typical brain sizes).
	all, err := p.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = ?`, string(model.MemoryTypeProcedural))
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

// All returns all stored procedures ordered by salience.
func (p *ProceduralMemory) All(ctx context.Context) ([]model.Procedure, error) {
	p.logger.InfoContext(ctx, "retrieving all procedures", "action", "all")
	memories, err := p.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'procedural'
		ORDER BY salience DESC`)
	if err != nil {
		p.logger.ErrorContext(ctx, "all failed", "action", "all", "error", err)
		return nil, err
	}

	procs := make([]model.Procedure, len(memories))
	for i, m := range memories {
		procs[i] = *memoryToProcedure(&m)
	}
	p.logger.InfoContext(ctx, "retrieved procedures", "action", "all", "count", len(procs))
	return procs, nil
}

// Emerging returns procedures that were learned from conversation or consolidation (emerging skills).
func (p *ProceduralMemory) Emerging(ctx context.Context) ([]model.Procedure, error) {
	p.logger.InfoContext(ctx, "retrieving emerging skills", "action", "emerging")
	memories, err := p.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'procedural'
		  AND EXISTS (SELECT 1 FROM json_each(memories.tags) WHERE value = 'emerging')
		ORDER BY salience DESC`)
	if err != nil {
		p.logger.ErrorContext(ctx, "emerging failed", "action", "emerging", "error", err)
		return nil, err
	}

	procs := make([]model.Procedure, len(memories))
	for i, m := range memories {
		procs[i] = *memoryToProcedure(&m)
	}
	p.logger.InfoContext(ctx, "retrieved emerging skills", "action", "emerging", "count", len(procs))
	return procs, nil
}

// Count returns the number of stored procedures.
func (p *ProceduralMemory) Count(ctx context.Context) (int, error) {
	var count int
	err := p.s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM memories WHERE memory_type = 'procedural'`).Scan(&count)
	return count, err
}

// UpdateSuccessRate records the outcome of using a procedure.
func (p *ProceduralMemory) UpdateSuccessRate(ctx context.Context, name string, succeeded bool) error {
	p.logger.InfoContext(ctx, "updating procedure success rate", "action", "update_success_rate", "name", name, "succeeded", succeeded)
	memories, err := p.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'procedural' AND json_extract(metadata, '$.name') = ?
		LIMIT 1`, name)
	if err != nil {
		p.logger.ErrorContext(ctx, "update failed", "action", "update_success_rate", "name", name, "error", err)
		return err
	}
	if len(memories) == 0 {
		return model.ErrNotFound
	}

	m := &memories[0]
	proc := memoryToProcedure(m)
	proc.UseCount++
	outcome := 0.0
	if succeeded {
		outcome = 1.0
	}
	alpha := 0.2
	proc.SuccessRate = proc.SuccessRate*(1-alpha) + outcome*alpha

	newSalience := 0.5 + proc.SuccessRate*0.5

	stepsJSON, _ := json.Marshal(proc.Steps)
	_, err = p.s.DB.ExecContext(ctx, `
		UPDATE memories SET
			salience = ?,
			retrievals = retrievals + 1,
			last_accessed = datetime('now'),
			metadata = json_set(metadata,
				'$.success_rate', ?,
				'$.use_count', ?,
				'$.steps', ?
			)
		WHERE id = ?`,
		newSalience,
		formatFloat(proc.SuccessRate),
		formatInt(proc.UseCount),
		string(stepsJSON),
		m.ID)
	if err != nil {
		p.logger.ErrorContext(ctx, "update failed", "action", "update_success_rate", "name", name, "error", err)
		return err
	}
	p.logger.InfoContext(ctx, "updated procedure", "action", "update_success_rate", "name", name, "new_rate", proc.SuccessRate, "use_count", proc.UseCount)
	return nil
}

func memoryToProcedure(m *model.Memory) *model.Procedure {
	proc := &model.Procedure{
		Name:        m.Metadata["name"],
		Description: m.Metadata["description"],
	}
	if s, ok := m.Metadata["steps"]; ok {
		_ = json.Unmarshal([]byte(s), &proc.Steps)
	}
	if s, ok := m.Metadata["success_rate"]; ok {
		proc.SuccessRate, _ = strconv.ParseFloat(s, 64)
	}
	if s, ok := m.Metadata["use_count"]; ok {
		proc.UseCount, _ = strconv.Atoi(s)
	}
	return proc
}

func formatFloat(f float64) string { return fmt.Sprintf("%g", f) }
func formatInt(i int) string       { return fmt.Sprintf("%d", i) }

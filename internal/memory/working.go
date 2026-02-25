// Package memory implements the five memory subsystems.
package memory

import (
	"context"
	"log/slog"
	"time"

	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// WorkingMemory manages short-lived, active context.
type WorkingMemory struct {
	s      *store.Store
	ttl    time.Duration
	logger *slog.Logger
}

// NewWorkingMemory creates a WorkingMemory instance.
func NewWorkingMemory(s *store.Store, ttl time.Duration, logger *slog.Logger) *WorkingMemory {
	return &WorkingMemory{s: s, ttl: ttl, logger: logger}
}

// Store saves a piece of working context that auto-expires after the TTL.
func (w *WorkingMemory) Store(ctx context.Context, content string, metadata map[string]string) (int64, error) {
	return w.store(ctx, content, metadata, "", "", w.ttl)
}

// StoreForUser saves working context tagged with a user ID.
func (w *WorkingMemory) StoreForUser(ctx context.Context, content string, metadata map[string]string, userID string) (int64, error) {
	return w.store(ctx, content, metadata, userID, "", w.ttl)
}

// StoreForUserWithAgent saves working context tagged with a user ID and agent name.
func (w *WorkingMemory) StoreForUserWithAgent(ctx context.Context, content string, metadata map[string]string, userID, agent string) (int64, error) {
	return w.store(ctx, content, metadata, userID, agent, w.ttl)
}

// StoreWithAgent saves working context tagged with an agent name.
func (w *WorkingMemory) StoreWithAgent(ctx context.Context, content string, metadata map[string]string, agent string) (int64, error) {
	return w.store(ctx, content, metadata, "", agent, w.ttl)
}

// StoreWithTTL saves working context with a custom TTL override.
func (w *WorkingMemory) StoreWithTTL(ctx context.Context, content string, metadata map[string]string, ttl time.Duration) (int64, error) {
	return w.store(ctx, content, metadata, "", "", ttl)
}

// StoreWithTTLAndAgent saves working context with a custom TTL and agent name.
func (w *WorkingMemory) StoreWithTTLAndAgent(ctx context.Context, content string, metadata map[string]string, ttl time.Duration, agent string) (int64, error) {
	return w.store(ctx, content, metadata, "", agent, ttl)
}

func (w *WorkingMemory) store(ctx context.Context, content string, metadata map[string]string, userID, agent string, ttl time.Duration) (int64, error) {
	now := time.Now().UTC()
	exp := now.Add(ttl)
	m := &model.Memory{
		Type:         model.MemoryTypeWorking,
		Content:      content,
		Salience:     0.5,
		Metadata:     metadata,
		UserID:       userID,
		Agent:        agent,
		CreatedAt:    now,
		LastAccessed: now,
		ExpiresAt:    &exp,
	}
	id, err := w.s.InsertMemory(ctx, m)
	if err != nil {
		w.logger.ErrorContext(ctx, "store failed", "action", "store", "error", err)
		return 0, err
	}
	w.logger.InfoContext(ctx, "stored working memory", "action", "store", "id", id, "user_id", userID, "ttl", ttl)
	return id, nil
}

// Recent returns the most recent non-expired working memories.
func (w *WorkingMemory) Recent(ctx context.Context, limit int) ([]model.Memory, error) {
	w.logger.InfoContext(ctx, "retrieving recent working memories", "action", "recent", "limit", limit)
	results, err := w.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'working'
		  AND (expires_at IS NULL OR expires_at > strftime('%Y-%m-%d %H:%M:%f', 'now'))
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		w.logger.ErrorContext(ctx, "recent failed", "action", "recent", "error", err)
		return nil, err
	}
	w.logger.InfoContext(ctx, "retrieved working memories", "action", "recent", "count", len(results))
	return results, nil
}

// Get retrieves a specific working memory and potentiates it.
func (w *WorkingMemory) Get(ctx context.Context, id int64) (*model.Memory, error) {
	w.logger.InfoContext(ctx, "retrieving working memory", "action", "get", "id", id)
	m, err := w.s.GetMemory(ctx, id)
	if err != nil {
		w.logger.ErrorContext(ctx, "get failed", "action", "get", "id", id, "error", err)
		return nil, err
	}
	if m.Type != model.MemoryTypeWorking {
		w.logger.WarnContext(ctx, "memory is not working type", "action", "get", "id", id, "actual_type", m.Type)
		return nil, model.ErrNotFound
	}
	if m.ExpiresAt != nil && m.ExpiresAt.Before(time.Now().UTC()) {
		w.logger.InfoContext(ctx, "working memory expired", "action", "get", "id", id)
		return nil, model.ErrExpired
	}
	if err := w.s.Potentiate(ctx, id); err != nil {
		return nil, err
	}
	m.Retrievals++
	m.LastAccessed = time.Now().UTC()
	return m, nil
}

// Clear removes all working memories.
func (w *WorkingMemory) Clear(ctx context.Context) error {
	w.logger.InfoContext(ctx, "clearing all working memories", "action", "clear")
	_, err := w.s.DB.ExecContext(ctx, `DELETE FROM memories WHERE memory_type = 'working'`)
	if err != nil {
		w.logger.ErrorContext(ctx, "clear failed", "action", "clear", "error", err)
	}
	return err
}

// Count returns the number of active (non-expired) working memories.
func (w *WorkingMemory) Count(ctx context.Context) (int, error) {
	var count int
	err := w.s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM memories
		WHERE memory_type = 'working'
		  AND (expires_at IS NULL OR expires_at > strftime('%Y-%m-%d %H:%M:%f', 'now'))`).Scan(&count)
	return count, err
}

// StoreGoal creates a persistent goal memory that does not expire and appears
// prominently in brain attention. Goals survive until explicitly resolved.
func (w *WorkingMemory) StoreGoal(ctx context.Context, content, userID, agent string) (int64, error) {
	now := time.Now().UTC()
	m := &model.Memory{
		Type:         model.MemoryTypeGoal,
		Content:      content,
		Salience:     0.8,
		UserID:       userID,
		Agent:        agent,
		CreatedAt:    now,
		LastAccessed: now,
		// No ExpiresAt — goals persist until resolved.
	}
	id, err := w.s.InsertMemory(ctx, m)
	if err != nil {
		w.logger.ErrorContext(ctx, "store goal failed", "action", "store_goal", "error", err)
		return 0, err
	}
	w.logger.InfoContext(ctx, "stored goal", "action", "store_goal", "id", id, "user_id", userID)
	return id, nil
}

// ListGoals returns all active goals in creation order (oldest first).
func (w *WorkingMemory) ListGoals(ctx context.Context) ([]model.Memory, error) {
	return w.s.QueryMemories(ctx, `
		SELECT `+store.MemoryCols+`
		FROM memories
		WHERE memory_type = 'goal'
		ORDER BY created_at ASC`)
}

// ResolveGoal deletes a goal by ID. Returns ErrNotFound if the ID does not
// correspond to a goal memory.
func (w *WorkingMemory) ResolveGoal(ctx context.Context, id int64) error {
	m, err := w.s.GetMemory(ctx, id)
	if err != nil {
		return err
	}
	if m.Type != model.MemoryTypeGoal {
		return model.ErrNotFound
	}
	return w.s.DeleteMemory(ctx, id)
}

// Package store provides low-level SQLite CRUD operations for memories.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/glthr/brAIn/internal/model"
)

const SQLiteTimeFmt = "2006-01-02 15:04:05"

// SQLiteTimeFmtFrac includes fractional seconds (microsecond resolution).
const SQLiteTimeFmtFrac = "2006-01-02 15:04:05.000000"

// MemoryCols is the canonical column list for memories; scan into model.Memory in this order.
const MemoryCols = `id, memory_type, content, salience, retrievals, tags, metadata, user_id, agent, created_at, last_accessed, expires_at`

// Store wraps the SQLite database with low-level CRUD helpers.
type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{DB: db}
}

func (s *Store) InsertMemory(ctx context.Context, m *model.Memory) (int64, error) {
	var expiresAt *string
	if m.ExpiresAt != nil {
		t := m.ExpiresAt.UTC().Format(SQLiteTimeFmtFrac)
		expiresAt = &t
	}

	var userID *string
	if m.UserID != "" {
		userID = &m.UserID
	}

	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO memories (memory_type, content, salience, retrievals, tags, metadata, user_id, agent, created_at, last_accessed, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(m.Type),
		m.Content,
		m.Salience,
		m.Retrievals,
		model.EncodeTags(m.Tags),
		model.EncodeMetadata(m.Metadata),
		userID,
		m.Agent,
		m.CreatedAt.UTC().Format(SQLiteTimeFmtFrac),
		m.LastAccessed.UTC().Format(SQLiteTimeFmtFrac),
		expiresAt,
	)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	m.ID = id

	return id, nil
}

// Potentiate increments retrievals and updates last_accessed, simulating
// long-term potentiation: each retrieval strengthens the memory trace.
func (s *Store) Potentiate(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE memories SET retrievals = retrievals + 1, last_accessed = datetime('now') WHERE id = ?`, id)
	return err
}

func (s *Store) GetMemory(ctx context.Context, id int64) (*model.Memory, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+MemoryCols+`
		FROM memories WHERE id = ?`, id)
	return scanMemory(row)
}

func (s *Store) QueryMemories(ctx context.Context, query string, args ...interface{}) ([]model.Memory, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []model.Memory
	for rows.Next() {
		m, err := scanMemoryRows(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, *m)
	}
	return memories, rows.Err()
}

func (s *Store) MemoriesByUser(ctx context.Context, userID string, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.QueryMemories(ctx, `
		SELECT `+MemoryCols+`
		FROM memories
		WHERE user_id = ? AND memory_type IN ('episodic', 'semantic')
		ORDER BY created_at DESC
		LIMIT ?`, userID, limit)
}

// RetroactivelyAssignUserID assigns userID to memories with NULL/empty user_id in the time window (e.g. after user registers).
func (s *Store) RetroactivelyAssignUserID(ctx context.Context, userID string, timeWindow time.Duration) (int64, error) {
	if timeWindow <= 0 {
		timeWindow = 1 * time.Hour // Default: last hour
	}
	cutoff := time.Now().UTC().Add(-timeWindow)
	cutoffStr := cutoff.Format(SQLiteTimeFmtFrac)

	res, err := s.DB.ExecContext(ctx, `
		UPDATE memories
		SET user_id = ?
		WHERE (user_id IS NULL OR user_id = '' OR user_id = 'user-unknown')
		  AND created_at >= ?`, userID, cutoffStr)
	if err != nil {
		return 0, err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return count, nil
}

// MergeContact reassigns memories and interactions from fromID to toID, then deletes
// the fromID contact record if it is now empty. Returns the total rows reassigned.
func (s *Store) MergeContact(ctx context.Context, fromID, toID string) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	total := 0

	res, err := tx.ExecContext(ctx, `UPDATE memories SET user_id = ? WHERE user_id = ?`, toID, fromID)
	if err != nil {
		return 0, fmt.Errorf("reassign memories: %w", err)
	}
	n, _ := res.RowsAffected()
	total += int(n)

	res, err = tx.ExecContext(ctx, `UPDATE interactions SET contact_id = ? WHERE contact_id = ?`, toID, fromID)
	if err != nil {
		return 0, fmt.Errorf("reassign interactions: %w", err)
	}
	n, _ = res.RowsAffected()
	total += int(n)

	_, err = tx.ExecContext(ctx, `
		DELETE FROM contacts
		WHERE id = ?
		  AND NOT EXISTS (SELECT 1 FROM interactions WHERE contact_id = ?)
		  AND NOT EXISTS (SELECT 1 FROM memories WHERE user_id = ?)`,
		fromID, fromID, fromID)
	if err != nil {
		return 0, fmt.Errorf("delete source contact: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return total, nil
}

// UnprocessedIngests returns working memories with metadata type="conversation" not yet processed.
func (s *Store) UnprocessedIngests(ctx context.Context, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.QueryMemories(ctx, `
		SELECT `+MemoryCols+`
		FROM memories
		WHERE memory_type = 'working'
		  AND json_extract(metadata, '$.type') = 'conversation'
		  AND (expires_at IS NULL OR expires_at > datetime('now'))
		ORDER BY created_at ASC
		LIMIT ?`, limit)
}

// ContentExists returns the existing memory's ID if a duplicate exists, or 0.
func (s *Store) ContentExists(ctx context.Context, memType model.MemoryType, content string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT id FROM memories WHERE memory_type = ? AND content = ? LIMIT 1`,
		string(memType), content).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *Store) RecentConsolidationLogs(ctx context.Context, limit int) ([]model.ConsolidationLogEntry, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, stats, created_at FROM consolidation_log
		ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.ConsolidationLogEntry
	for rows.Next() {
		var e model.ConsolidationLogEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.StatsJSON, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = ParseTime(createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) DeleteMemory(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	return err
}

// PinMemory sets salience to 1.0 and marks the memory pinned (exempt from decay).
func (s *Store) PinMemory(ctx context.Context, id int64) error {
	m, err := s.GetMemory(ctx, id)
	if err != nil {
		return err
	}
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}
	m.Metadata["pinned"] = "true"
	_, err = s.DB.ExecContext(ctx, `
		UPDATE memories SET salience = 1.0, metadata = ? WHERE id = ?`,
		model.EncodeMetadata(m.Metadata), id)
	return err
}

func (s *Store) UnpinMemory(ctx context.Context, id int64) error {
	m, err := s.GetMemory(ctx, id)
	if err != nil {
		return err
	}
	delete(m.Metadata, "pinned")
	_, err = s.DB.ExecContext(ctx, `
		UPDATE memories SET metadata = ? WHERE id = ?`,
		model.EncodeMetadata(m.Metadata), id)
	return err
}

// CorrectMemory updates the content of a memory in-place while preserving
// its salience, tags, retrieval history, and all other metadata.
func (s *Store) CorrectMemory(ctx context.Context, id int64, newContent string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE memories SET content = ? WHERE id = ?`, newContent, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// ArchiveEpisodes saves episodes to the archive before deletion during semanticization; semanticID is the new semantic memory ID (0 if unknown).
func (s *Store) ArchiveEpisodes(ctx context.Context, tx interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}, episodes []model.Memory, semanticID int64) error {
	for _, ep := range episodes {
		var uid *string
		if ep.UserID != "" {
			uid = &ep.UserID
		}
		var semID *int64
		if semanticID != 0 {
			semID = &semanticID
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO episodic_archive (content, tags, metadata, user_id, agent, created_at, semantic_id)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ep.Content,
			model.EncodeTags(ep.Tags),
			model.EncodeMetadata(ep.Metadata),
			uid,
			ep.Agent,
			ep.CreatedAt.UTC().Format(SQLiteTimeFmt),
			semID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Timeline(ctx context.Context, userID string, limit int) ([]model.EpisodicArchive, error) {
	if limit <= 0 {
		limit = 20
	}
	var (
		rows *sql.Rows
		err  error
	)
	if userID != "" {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT id, content, tags, metadata, user_id, agent, created_at, archived_at, COALESCE(semantic_id, 0)
			FROM episodic_archive
			WHERE user_id = ?
			ORDER BY created_at DESC LIMIT ?`, userID, limit)
	} else {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT id, content, tags, metadata, user_id, agent, created_at, archived_at, COALESCE(semantic_id, 0)
			FROM episodic_archive
			ORDER BY created_at DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var archives []model.EpisodicArchive
	for rows.Next() {
		var a model.EpisodicArchive
		var uid sql.NullString
		var tags, metadata, createdAt, archivedAt string
		if err := rows.Scan(&a.ID, &a.Content, &tags, &metadata, &uid, &a.Agent, &createdAt, &archivedAt, &a.SemanticID); err != nil {
			return nil, err
		}
		a.Tags = model.DecodeTags(tags)
		a.Metadata = model.DecodeMetadata(metadata)
		if uid.Valid {
			a.UserID = uid.String
		}
		a.CreatedAt = ParseTime(createdAt)
		a.ArchivedAt = ParseTime(archivedAt)
		archives = append(archives, a)
	}
	return archives, rows.Err()
}

type MemoryExplanation struct {
	Memory     model.Memory
	SourceIDs  []int64 // episode IDs compressed into this semantic memory
	SemanticID int64   // non-zero if this archived episode was compressed into a semantic memory
	Pinned     bool
}

func (s *Store) ExplainMemory(ctx context.Context, id int64) (*MemoryExplanation, error) {
	m, err := s.GetMemory(ctx, id)
	if err != nil {
		return nil, err
	}
	exp := &MemoryExplanation{Memory: *m}

	if m.Metadata["pinned"] == "true" {
		exp.Pinned = true
	}

	if m.Type == model.MemoryTypeSemantic {
		if sources, ok := m.Metadata["source_ids"]; ok && sources != "" {
			for _, part := range strings.Split(sources, ",") {
				part = strings.TrimSpace(part)
				if sid, parseErr := strconv.ParseInt(part, 10, 64); parseErr == nil {
					exp.SourceIDs = append(exp.SourceIDs, sid)
				}
			}
		}
	}

	var semID sql.NullInt64
	archErr := s.DB.QueryRowContext(ctx, `
		SELECT semantic_id FROM episodic_archive WHERE id = ?`, id).Scan(&semID)
	if archErr == nil && semID.Valid {
		exp.SemanticID = semID.Int64
	}

	return exp, nil
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO brain_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM brain_meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) AllMeta(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM brain_meta ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

type scanner interface {
	Scan(dest ...interface{}) error
}

// timeFormats lists layouts tried by ParseTime (order: more specific first).
var timeFormats = []string{
	SQLiteTimeFmtFrac,           // 2006-01-02 15:04:05.000000
	SQLiteTimeFmt,               // 2006-01-02 15:04:05
	time.RFC3339,                // 2006-01-02T15:04:05Z07:00
	"2006-01-02 15:04:05-07:00", // 2006-01-02 15:04:05+00:00
}

func ParseTime(s string) time.Time {
	for _, layout := range timeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func scanMemoryFromScanner(s scanner) (*model.Memory, error) {
	var m model.Memory
	var memType, tags, metadata string
	var userID sql.NullString
	var createdAt, lastAccessed string
	var expiresAt sql.NullString

	err := s.Scan(&m.ID, &memType, &m.Content, &m.Salience, &m.Retrievals,
		&tags, &metadata, &userID, &m.Agent, &createdAt, &lastAccessed, &expiresAt)
	if err != nil {
		return nil, err
	}

	m.Type = model.MemoryType(memType)
	m.Tags = model.DecodeTags(tags)
	m.Metadata = model.DecodeMetadata(metadata)
	if userID.Valid {
		m.UserID = userID.String
	}
	m.CreatedAt = ParseTime(createdAt)
	m.LastAccessed = ParseTime(lastAccessed)
	if expiresAt.Valid && expiresAt.String != "" {
		t := ParseTime(expiresAt.String)
		if !t.IsZero() {
			m.ExpiresAt = &t
		}
	}

	return &m, nil
}

func scanMemory(row *sql.Row) (*model.Memory, error) {
	return scanMemoryFromScanner(row)
}

func scanMemoryRows(rows *sql.Rows) (*model.Memory, error) {
	return scanMemoryFromScanner(rows)
}

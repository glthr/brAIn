package memory

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// confidenceTier computes a low/medium/high confidence rating for a contact profile
// based on the amount of observed data. No LLM calls needed.
func confidenceTier(p *model.ContactProfile) string {
	score := 0
	if p.InteractionCount >= 5 {
		score++
	}
	if p.InteractionCount >= 15 {
		score++
	}
	if p.MemoryCount >= 5 {
		score++
	}
	if p.MemoryCount >= 15 {
		score++
	}
	// Observations spread over time signal a richer profile
	if !p.LastInteraction.IsZero() && time.Since(p.LastInteraction) > 7*24*time.Hour && p.InteractionCount >= 5 {
		score++
	}
	switch {
	case score >= 3:
		return "high"
	case score >= 1:
		return "medium"
	default:
		return "low"
	}
}

// SocialMemory manages relationships and interaction history with both
// other agents (A2A) and humans (A2H).
type SocialMemory struct {
	s      *store.Store
	logger *slog.Logger
}

// NewSocialMemory creates a SocialMemory instance.
func NewSocialMemory(s *store.Store, logger *slog.Logger) *SocialMemory {
	return &SocialMemory{s: s, logger: logger}
}

// entityTypeOrDefault returns the entity type, defaulting to "agent" if empty.
func entityTypeOrDefault(et model.EntityType) model.EntityType {
	if et == "" {
		return model.EntityTypeAgent
	}
	return et
}

// RecordInteraction logs an interaction with a contact (agent or human).
// If Interaction.EntityType is empty it defaults to "agent" for backward
// compatibility.
// When a new human user is registered, this retroactively assigns their user_id
// to recent memories that have NULL or empty user_id (within the last hour).
func (s *SocialMemory) RecordInteraction(ctx context.Context, i model.Interaction) (int64, error) {
	et := entityTypeOrDefault(i.EntityType)
	// Humans are not scored; valence is only used for agent trust.
	if et == model.EntityTypeHuman {
		i.Valence = 0
	}

	tx, err := s.s.DB.BeginTx(ctx, nil)
	if err != nil {
		s.logger.ErrorContext(ctx, "begin tx failed", "action", "record_interaction", "error", err)
		return 0, err
	}
	defer tx.Rollback()

	// Check if this is a new contact (before inserting)
	var existingCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM contacts WHERE id = ?`, i.ContactID).Scan(&existingCount)
	if err != nil {
		s.logger.ErrorContext(ctx, "check existing contact failed", "action", "record_interaction", "contact_id", i.ContactID, "error", err)
		return 0, err
	}
	isNewContact := existingCount == 0

	_, err = tx.ExecContext(ctx, `
		INSERT INTO contacts (id, entity_type, name, trust, updated_at)
		VALUES (?, ?, ?, 0.5, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			entity_type = CASE WHEN excluded.entity_type != '' THEN excluded.entity_type ELSE contacts.entity_type END,
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE contacts.name END,
			updated_at = datetime('now')`,
		i.ContactID, string(et), i.ContactName)
	if err != nil {
		s.logger.ErrorContext(ctx, "upsert contact failed", "action", "record_interaction", "contact_id", i.ContactID, "error", err)
		return 0, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO interactions (contact_id, type, outcome, valence, notes, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		i.ContactID, i.Type, i.Outcome, i.Valence, i.Notes,
		model.EncodeMetadata(i.Metadata), time.Now().UTC().Format(store.SQLiteTimeFmtFrac))
	if err != nil {
		s.logger.ErrorContext(ctx, "insert interaction failed", "action", "record_interaction", "contact_id", i.ContactID, "error", err)
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		s.logger.WarnContext(ctx, "last insert id failed", "action", "record_interaction", "error", err)
	}

	// Trust scores are only computed for agents; humans have no trust notion.
	if et == model.EntityTypeAgent {
		// Legacy single-dimension trust (valence-weighted recency average).
		_, err = tx.ExecContext(ctx, `
			UPDATE contacts SET trust = (
				SELECT COALESCE(
					SUM(valence * (1.0 / (1.0 + (julianday('now') - julianday(created_at))))) /
					NULLIF(SUM(1.0 / (1.0 + (julianday('now') - julianday(created_at)))), 0),
					0.5
				)
				FROM interactions WHERE contact_id = ?
			) WHERE id = ?`, i.ContactID, i.ContactID)
		if err != nil {
			s.logger.ErrorContext(ctx, "trust update failed", "action", "record_interaction", "contact_id", i.ContactID, "error", err)
			return 0, err
		}

		// Two-dimensional trust:
		// Competence trust: recency-weighted average of outcome quality
		//   (1.0 = success, 0.5 = partial/other, 0.0 = failure).
		_, err = tx.ExecContext(ctx, `
			UPDATE contacts SET competence_trust = (
				SELECT COALESCE(
					SUM(
						CASE
							WHEN outcome = 'success' THEN 1.0
							WHEN outcome = 'failure' THEN 0.0
							ELSE 0.5
						END * (1.0 / (1.0 + (julianday('now') - julianday(created_at))))
					) /
					NULLIF(SUM(1.0 / (1.0 + (julianday('now') - julianday(created_at)))), 0),
					0.5
				)
				FROM interactions WHERE contact_id = ?
			) WHERE id = ?`, i.ContactID, i.ContactID)
		if err != nil {
			s.logger.WarnContext(ctx, "competence trust update failed (non-fatal)", "action", "record_interaction", "contact_id", i.ContactID, "error", err)
		}

		// Reliability trust: recency-weighted average of the reliability signal
		// stored in interaction metadata as "reliability" (float 0.0–1.0, default 0.5).
		_, err = tx.ExecContext(ctx, `
			UPDATE contacts SET reliability_trust = (
				SELECT COALESCE(
					SUM(
						COALESCE(CAST(json_extract(metadata, '$.reliability') AS REAL), 0.5)
						* (1.0 / (1.0 + (julianday('now') - julianday(created_at))))
					) /
					NULLIF(SUM(1.0 / (1.0 + (julianday('now') - julianday(created_at)))), 0),
					0.5
				)
				FROM interactions WHERE contact_id = ?
			) WHERE id = ?`, i.ContactID, i.ContactID)
		if err != nil {
			s.logger.WarnContext(ctx, "reliability trust update failed (non-fatal)", "action", "record_interaction", "contact_id", i.ContactID, "error", err)
		}
	}

	// If this is a new human user, retroactively assign their user_id to recent memories
	if isNewContact && et == model.EntityTypeHuman {
		cutoff := time.Now().UTC().Add(-1 * time.Hour) // Last hour
		cutoffStr := cutoff.Format(store.SQLiteTimeFmtFrac)
		updateRes, err := tx.ExecContext(ctx, `
			UPDATE memories
			SET user_id = ?
			WHERE (user_id IS NULL OR user_id = '')
			  AND created_at >= ?`, i.ContactID, cutoffStr)
		if err != nil {
			s.logger.WarnContext(ctx, "retroactive user_id assignment failed (non-fatal)",
				"action", "retroactive_assign", "contact_id", i.ContactID, "error", err)
		} else {
			count, _ := updateRes.RowsAffected()
			if count > 0 {
				s.logger.InfoContext(ctx, "retroactively assigned user_id to memories",
					"action", "retroactive_assign", "contact_id", i.ContactID, "count", count)
			}
		}

		// Also migrate all records from the sentinel "user-unknown" contact to the
		// real user. "user-unknown" is a placeholder — its memories and interactions
		// belong to whoever first identifies themselves.
		unknownID := "user-unknown"
		if i.ContactID != unknownID {
			// Reassign memories stored under user-unknown.
			res, err := tx.ExecContext(ctx, `
				UPDATE memories SET user_id = ? WHERE user_id = ?`,
				i.ContactID, unknownID)
			if err != nil {
				s.logger.WarnContext(ctx, "user-unknown memory migration failed (non-fatal)",
					"action", "retroactive_assign", "contact_id", i.ContactID, "error", err)
			} else if n, _ := res.RowsAffected(); n > 0 {
				s.logger.InfoContext(ctx, "migrated user-unknown memories to real user",
					"action", "retroactive_assign", "contact_id", i.ContactID, "count", n)
			}

			// Reassign interactions stored under user-unknown.
			res, err = tx.ExecContext(ctx, `
				UPDATE interactions SET contact_id = ? WHERE contact_id = ?`,
				i.ContactID, unknownID)
			if err != nil {
				s.logger.WarnContext(ctx, "user-unknown interaction migration failed (non-fatal)",
					"action", "retroactive_assign", "contact_id", i.ContactID, "error", err)
			} else if n, _ := res.RowsAffected(); n > 0 {
				s.logger.InfoContext(ctx, "migrated user-unknown interactions to real user",
					"action", "retroactive_assign", "contact_id", i.ContactID, "count", n)
			}

			// Delete the user-unknown contact record if it's now empty.
			_, err = tx.ExecContext(ctx, `
				DELETE FROM contacts
				WHERE id = ?
				  AND NOT EXISTS (SELECT 1 FROM interactions WHERE contact_id = ?)
				  AND NOT EXISTS (SELECT 1 FROM memories WHERE user_id = ?)`,
				unknownID, unknownID, unknownID)
			if err != nil {
				s.logger.WarnContext(ctx, "user-unknown contact cleanup failed (non-fatal)",
					"action", "retroactive_assign", "error", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		s.logger.ErrorContext(ctx, "commit failed", "action", "record_interaction", "contact_id", i.ContactID, "error", err)
		return 0, err
	}
	s.logger.InfoContext(ctx, "recorded interaction", "action", "record_interaction", "id", id, "contact_id", i.ContactID, "entity_type", et, "outcome", i.Outcome)
	return id, nil
}

// GetContactProfile returns a summary of a known contact (agent or human),
// including their current person schema, dual trust scores, memory count,
// and computed confidence tier.
func (s *SocialMemory) GetContactProfile(ctx context.Context, contactID string) (*model.ContactProfile, error) {
	s.logger.InfoContext(ctx, "getting contact profile", "action", "get_profile", "contact_id", contactID)
	var p model.ContactProfile
	var lastInteraction sql.NullString

	err := s.s.DB.QueryRowContext(ctx, `
		SELECT
			a.id, a.entity_type, a.name, a.trust,
			COALESCE(a.competence_trust, 0.5), COALESCE(a.reliability_trust, 0.5),
			a.notes, a.social_schema, a.behavioral_priors,
			(SELECT COUNT(*) FROM interactions WHERE contact_id = a.id) AS interaction_count,
			(SELECT COUNT(*) FROM memories WHERE user_id = a.id AND memory_type IN ('episodic','semantic')) AS memory_count,
			(SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) AS last_interaction,
			(SELECT COALESCE(AVG(valence), 0) FROM interactions WHERE contact_id = a.id) AS avg_valence
		FROM contacts a
		WHERE a.id = ?`, contactID).Scan(
		&p.ID, &p.EntityType, &p.Name, &p.Trust,
		&p.CompetenceTrust, &p.ReliabilityTrust,
		&p.Notes, &p.SocialSchema, &p.BehavioralPriors,
		&p.InteractionCount, &p.MemoryCount, &lastInteraction, &p.AvgValence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastInteraction.Valid {
		p.LastInteraction = store.ParseTime(lastInteraction.String)
	}
	if p.EntityType == model.EntityTypeHuman {
		p.Trust = 0
		p.CompetenceTrust = 0
		p.ReliabilityTrust = 0
		p.AvgValence = 0 // no score for humans
	}
	p.ConfidenceTier = confidenceTier(&p)
	return &p, nil
}

// listByType returns profiles filtered by entity type (agents by trust, humans by last interaction).
func (s *SocialMemory) listByType(ctx context.Context, et model.EntityType) ([]model.ContactProfile, error) {
	orderBy := "a.trust DESC"
	if et == model.EntityTypeHuman {
		orderBy = "(SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) DESC"
	}
	rows, err := s.s.DB.QueryContext(ctx, `
		SELECT
			a.id, a.entity_type, a.name, a.trust, a.notes,
			(SELECT COUNT(*) FROM interactions WHERE contact_id = a.id) AS interaction_count,
			(SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) AS last_interaction,
			(SELECT COALESCE(AVG(valence), 0) FROM interactions WHERE contact_id = a.id) AS avg_valence
		FROM contacts a
		WHERE a.entity_type = ?
		ORDER BY `+orderBy, string(et))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProfiles(rows, et)
}

// ListAgents returns all known agent contacts ordered by trust.
func (s *SocialMemory) ListAgents(ctx context.Context) ([]model.ContactProfile, error) {
	s.logger.InfoContext(ctx, "listing agent contacts", "action", "list_agents")
	return s.listByType(ctx, model.EntityTypeAgent)
}

// ListHumans returns all known human contacts ordered by last interaction.
func (s *SocialMemory) ListHumans(ctx context.Context) ([]model.ContactProfile, error) {
	s.logger.InfoContext(ctx, "listing human contacts", "action", "list_humans")
	return s.listByType(ctx, model.EntityTypeHuman)
}

// ListAll returns all known contacts (agents first by trust, then humans by last interaction).
func (s *SocialMemory) ListAll(ctx context.Context) ([]model.ContactProfile, error) {
	s.logger.InfoContext(ctx, "listing all contacts", "action", "list_all")
	rows, err := s.s.DB.QueryContext(ctx, `
		SELECT
			a.id, a.entity_type, a.name, a.trust, a.notes,
			(SELECT COUNT(*) FROM interactions WHERE contact_id = a.id) AS interaction_count,
			(SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) AS last_interaction,
			(SELECT COALESCE(AVG(valence), 0) FROM interactions WHERE contact_id = a.id) AS avg_valence
		FROM contacts a
		ORDER BY a.entity_type, a.trust DESC, (SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProfiles(rows, "")
}

// UpdateNotes sets free-form notes about a contact.
func (s *SocialMemory) UpdateNotes(ctx context.Context, contactID, notes string) error {
	s.logger.InfoContext(ctx, "updating contact notes", "action", "update_notes", "contact_id", contactID)
	res, err := s.s.DB.ExecContext(ctx, `
		UPDATE contacts SET notes = ?, updated_at = datetime('now') WHERE id = ?`, notes, contactID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// InteractionHistory returns recent interactions with a specific contact.
func (s *SocialMemory) InteractionHistory(ctx context.Context, contactID string, limit int) ([]model.Interaction, error) {
	s.logger.InfoContext(ctx, "retrieving interaction history", "action", "history", "contact_id", contactID, "limit", limit)
	rows, err := s.s.DB.QueryContext(ctx, `
		SELECT i.contact_id, c.name, c.entity_type, i.type, i.outcome, i.valence, i.notes, i.metadata
		FROM interactions i
		JOIN contacts c ON c.id = i.contact_id
		WHERE i.contact_id = ?
		ORDER BY i.created_at DESC
		LIMIT ?`, contactID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var interactions []model.Interaction
	for rows.Next() {
		var i model.Interaction
		var metaStr string
		err := rows.Scan(&i.ContactID, &i.ContactName, &i.EntityType, &i.Type, &i.Outcome, &i.Valence, &i.Notes, &metaStr)
		if err != nil {
			return nil, err
		}
		i.Metadata = model.DecodeMetadata(metaStr)
		interactions = append(interactions, i)
	}
	return interactions, rows.Err()
}

// MostRecentHuman returns the human contact with the most recent interaction.
// Returns model.ErrNotFound if no human contacts exist.
func (s *SocialMemory) MostRecentHuman(ctx context.Context) (*model.ContactProfile, error) {
	s.logger.InfoContext(ctx, "looking up most recent human", "action", "whoami")
	var p model.ContactProfile
	var lastInteraction sql.NullString

	err := s.s.DB.QueryRowContext(ctx, `
		SELECT
			a.id, a.entity_type, a.name, a.trust, a.notes,
			(SELECT COUNT(*) FROM interactions WHERE contact_id = a.id) AS interaction_count,
			(SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) AS last_interaction,
			(SELECT COALESCE(AVG(valence), 0) FROM interactions WHERE contact_id = a.id) AS avg_valence
		FROM contacts a
		WHERE a.entity_type = 'human'
		ORDER BY (SELECT MAX(id) FROM interactions WHERE contact_id = a.id) DESC
		LIMIT 1`).Scan(
		&p.ID, &p.EntityType, &p.Name, &p.Trust, &p.Notes,
		&p.InteractionCount, &lastInteraction, &p.AvgValence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastInteraction.Valid {
		p.LastInteraction = store.ParseTime(lastInteraction.String)
	}
	p.Trust = 0      // humans have no trust notion
	p.AvgValence = 0 // no score for humans
	return &p, nil
}

// scanProfiles is a shared helper that reads ContactProfile rows. For human entityType (or mixed when et empty), human profiles have Trust zeroed.
func scanProfiles(rows *sql.Rows, entityType model.EntityType) ([]model.ContactProfile, error) {
	var profiles []model.ContactProfile
	for rows.Next() {
		var p model.ContactProfile
		var lastInteraction sql.NullString
		err := rows.Scan(&p.ID, &p.EntityType, &p.Name, &p.Trust, &p.Notes,
			&p.InteractionCount, &lastInteraction, &p.AvgValence)
		if err != nil {
			return nil, err
		}
		if lastInteraction.Valid {
			p.LastInteraction = store.ParseTime(lastInteraction.String)
		}
		if p.EntityType == model.EntityTypeHuman {
			p.Trust = 0
			p.AvgValence = 0 // no score for humans
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

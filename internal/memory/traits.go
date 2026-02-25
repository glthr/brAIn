package memory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/glthr/brAIn/internal/model"
)

// SetSocialSchema stores a social schema (cognitive model of a specific individual)
// for a contact. The contact must already exist (created via RecordInteraction).
func (s *SocialMemory) SetSocialSchema(ctx context.Context, contactID, text string) error {
	s.logger.InfoContext(ctx, "setting social schema", "action", "set_social_schema", "contact_id", contactID)
	res, err := s.s.DB.ExecContext(ctx, `
		UPDATE contacts SET social_schema = ?, updated_at = datetime('now')
		WHERE id = ?`, text, contactID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// GetSocialSchema returns the current social schema for a contact.
// Returns an empty string (not ErrNotFound) if the contact exists but has no
// social schema yet.
func (s *SocialMemory) GetSocialSchema(ctx context.Context, contactID string) (string, error) {
	s.logger.InfoContext(ctx, "getting social schema", "action", "get_social_schema", "contact_id", contactID)
	var text string
	err := s.s.DB.QueryRowContext(ctx, `SELECT social_schema FROM contacts WHERE id = ?`, contactID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", model.ErrNotFound
	}
	return text, err
}

// SetBehavioralPriors stores the learned behavioral priors (predictive expectations)
// for a contact. The contact must already exist (created via RecordInteraction).
func (s *SocialMemory) SetBehavioralPriors(ctx context.Context, contactID, text string) error {
	s.logger.InfoContext(ctx, "setting behavioral priors", "action", "set_behavioral_priors", "contact_id", contactID)
	res, err := s.s.DB.ExecContext(ctx, `
		UPDATE contacts SET behavioral_priors = ?, updated_at = datetime('now')
		WHERE id = ?`, text, contactID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// GetBehavioralPriors returns the behavioral priors for a contact.
// Returns an empty string (not ErrNotFound) if the contact exists but has no
// behavioral priors yet.
func (s *SocialMemory) GetBehavioralPriors(ctx context.Context, contactID string) (string, error) {
	s.logger.InfoContext(ctx, "getting behavioral priors", "action", "get_behavioral_priors", "contact_id", contactID)
	var text string
	err := s.s.DB.QueryRowContext(ctx, `SELECT behavioral_priors FROM contacts WHERE id = ?`, contactID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", model.ErrNotFound
	}
	return text, err
}

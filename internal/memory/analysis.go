package memory

import (
	"context"
	"time"

	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/store"
)

// NewUserReanalyzeThreshold is the interaction count below which a user is
// considered "new" and will be re-queued for profile analysis periodically
// even when there is no new data, so the profile is not skewed by the first
// few interactions. See HumansNeedingAnalysis.
const NewUserReanalyzeThreshold = 5

// NewUserReanalyzeInterval is the SQLite datetime modifier for the minimum
// time since last analysis before a new user (below NewUserReanalyzeThreshold
// interactions) is re-queued (e.g. "-10 minutes").
const NewUserReanalyzeInterval = "-10 minutes"

// HumansNeedingAnalysis returns human contacts that have new interactions or
// memories since their last analysis. Returns all humans if never analyzed.
// Users with fewer than NewUserReanalyzeThreshold interactions are also
// re-queued periodically (every NewUserReanalyzeInterval since last analysis)
// so their profile is refined as more evidence accumulates and not skewed by
// the first interactions.
func (s *SocialMemory) HumansNeedingAnalysis(ctx context.Context) ([]model.ContactProfile, error) {
	s.logger.InfoContext(ctx, "finding humans needing analysis", "action", "needs_analysis")
	rows, err := s.s.DB.QueryContext(ctx, `
		SELECT
			a.id, a.entity_type, a.name, a.trust, a.notes,
			(SELECT COUNT(*) FROM interactions WHERE contact_id = a.id) AS interaction_count,
			(SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) AS last_interaction,
			(SELECT COALESCE(AVG(valence), 0) FROM interactions WHERE contact_id = a.id) AS avg_valence
		FROM contacts a
		WHERE a.entity_type = 'human'
		  AND (
			a.last_analyzed_at IS NULL
			OR EXISTS (
				SELECT 1 FROM interactions
				WHERE contact_id = a.id AND created_at > a.last_analyzed_at
			)
			OR EXISTS (
				SELECT 1 FROM memories
				WHERE user_id = a.id AND created_at > a.last_analyzed_at
			)
			OR (
				(SELECT COUNT(*) FROM interactions WHERE contact_id = a.id) < ?
				AND (a.last_analyzed_at IS NULL OR a.last_analyzed_at < datetime('now', ?))
			)
		)
		ORDER BY (SELECT MAX(created_at) FROM interactions WHERE contact_id = a.id) DESC`,
		NewUserReanalyzeThreshold, NewUserReanalyzeInterval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProfiles(rows, model.EntityTypeHuman)
}

// MarkAnalyzed stamps the current time as last_analyzed_at for a contact.
// Uses Go's time.Now() with microsecond precision (same clock as memory/interaction
// timestamps) to ensure correct ordering.
func (s *SocialMemory) MarkAnalyzed(ctx context.Context, contactID string) error {
	now := time.Now().UTC().Format(store.SQLiteTimeFmtFrac)
	_, err := s.s.DB.ExecContext(ctx, `
		UPDATE contacts SET last_analyzed_at = ? WHERE id = ?`, now, contactID)
	return err
}

package helpers

import (
	"path/filepath"

	"github.com/glthr/brAIn/internal/model"
)

// RecallCandidateLimit returns the number of candidates to fetch from semantic lookup
// for recall. When scoping by project, fetches more (limit*5, cap 100) to allow
// for filtering; otherwise returns limit as-is.
func RecallCandidateLimit(limit int, all bool, project string) int {
	if all || project == "" || limit <= 0 {
		return limit
	}
	candidateLimit := limit * 5
	if candidateLimit > 100 {
		candidateLimit = 100
	}
	return candidateLimit
}

// FilterMemoriesByProject filters memories to those belonging to the given project
// or to the "common" project. Pass project as the resolved project path (e.g. from
// ResolveProject). Returns a new slice; does not modify the input.
func FilterMemoriesByProject(mems []model.Memory, project string) []model.Memory {
	if project == "" {
		return mems
	}
	cleanProj := filepath.Clean(project)
	filtered := make([]model.Memory, 0, len(mems))
	for _, m := range mems {
		p := m.Metadata["project"]
		if p == "" {
			continue
		}
		if p == "common" {
			filtered = append(filtered, m)
			continue
		}
		if filepath.Clean(p) == cleanProj {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

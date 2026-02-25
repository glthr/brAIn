package helpers

import "strings"

// ParseTags splits a comma-separated string into trimmed, non-empty tags.
// Used by the CLI for fact, episode, and other tag arguments.
func ParseTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

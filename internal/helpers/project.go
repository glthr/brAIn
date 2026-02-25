// Package helpers provides shared logic used by the brain CLI.
package helpers

import (
	"os"
	"strings"
)

// ResolveProject returns a project path from the given project string and all flag.
// If all is false and project is empty (after trimming), it defaults to the current
// working directory. Used by recall (project-scoped vs --all) and encode (default CWD).
func ResolveProject(project string, all bool) string {
	project = strings.TrimSpace(project)
	if !all && project == "" {
		if cwd, err := os.Getwd(); err == nil {
			project = cwd
		}
	}
	return project
}

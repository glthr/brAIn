// Package daemon: build and replace the running daemon binary from source.
package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const goBuildTags = ""

// BuildAndReplace compiles the daemon from sourcePath (brAIn repo root) and replaces
// the binary at destPath. It writes to destPath+".new" then renames over destPath,
// so the currently running process is unaffected; the next time the service starts
// it will run the new binary.
// Returns nil on success; the caller may then exit so launchd restarts the job.
func BuildAndReplace(sourcePath, destPath string) error {
	if sourcePath == "" || destPath == "" {
		return fmt.Errorf("source_path and dest_path are required")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("source path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source path is not a directory: %s", sourcePath)
	}

	destDir := filepath.Dir(destPath)
	newPath := filepath.Join(destDir, filepath.Base(destPath)+".new")

	cmd := exec.CommandContext(context.Background(), "go", "build", "-tags", goBuildTags, "-o", newPath, "./cmd/daemon/")
	cmd.Dir = sourcePath
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("go build: %w\n%s", err, out)
	}

	if err := os.Chmod(newPath, 0o755); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("chmod new binary: %w", err)
	}

	// Replace the installed binary. The running process keeps the old inode.
	if err := os.Rename(newPath, destPath); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}
